package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	gh "github.com/google/go-github/v88/github"
	"gopkg.in/yaml.v3"

	"github.com/ColdCrabby/cloud-presets/internal/submit"
)

// defaultBaseBranch is the branch pull requests target when a request does not
// name one. The presets repository's default branch is main.
const defaultBaseBranch = "main"

// Submitter adapts a Client to the submit.Submitter interface the API's claim
// handler depends on. It resolves the caller's writable namespace from the
// vendor manifests at the current head and opens single-commit pull requests
// via the Git Data API.
//
// It is a thin wrapper rather than methods on Client so the GitHub client stays
// a general-purpose primitive and the submission workflow (branch naming,
// idempotency, manifest resolution) lives in one place.
type Submitter struct {
	client *Client
	owner  string
	repo   string
}

// NewSubmitter builds a Submitter from an authenticated Client. It fails if the
// client's configured repository is not "owner/name", because every call needs
// both halves.
func NewSubmitter(c *Client) (*Submitter, error) {
	owner, repo, err := splitRepository(c.Repository())
	if err != nil {
		return nil, err
	}
	return &Submitter{client: c, owner: owner, repo: repo}, nil
}

// Ensure the adapter satisfies the interface at compile time.
var _ submit.Submitter = (*Submitter)(nil)

// vendorManifest is the subset of vendor.yaml this workflow reads: the binding
// from a Stytch organization to a directory slug. The rest of the manifest is
// irrelevant to resolving authority.
type vendorManifest struct {
	Slug                 string `yaml:"slug"`
	StytchOrganizationID string `yaml:"stytch_organization_id"`
}

// ResolveVendorSlug walks the vendor directories at the current head and returns
// the slug whose vendor.yaml declares the given organization ID. Authorization
// resolves against the head, not the served catalog, and fails closed: any error
// reaching GitHub is returned rather than swallowed into "not owned".
func (s *Submitter) ResolveVendorSlug(ctx context.Context, organizationID string) (string, error) {
	organizationID = strings.TrimSpace(organizationID)
	if organizationID == "" {
		return "", submit.ErrVendorNotFound
	}

	opts := &gh.RepositoryContentGetOptions{Ref: defaultBaseBranch}
	_, dirs, resp, err := s.client.rest.Repositories.GetContents(ctx, s.owner, s.repo, "vendors", opts)
	if err != nil {
		return "", Classify("list vendor directories", resp, err)
	}

	for _, entry := range dirs {
		if entry.GetType() != "dir" {
			continue
		}
		slug := entry.GetName()
		manifestPath := "vendors/" + slug + "/vendor.yaml"
		file, _, resp, err := s.client.rest.Repositories.GetContents(ctx, s.owner, s.repo, manifestPath, opts)
		if err != nil {
			// A vendor directory without a manifest is a repository problem, not
			// this caller's; skip it rather than failing every claim.
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				continue
			}
			return "", Classify("read "+manifestPath, resp, err)
		}
		content, err := file.GetContent()
		if err != nil {
			return "", fmt.Errorf("github: decoding %s: %w", manifestPath, err)
		}
		var m vendorManifest
		if err := yaml.Unmarshal([]byte(content), &m); err != nil {
			return "", fmt.Errorf("github: parsing %s: %w", manifestPath, err)
		}
		if strings.EqualFold(strings.TrimSpace(m.StytchOrganizationID), organizationID) {
			if m.Slug != "" {
				return m.Slug, nil
			}
			return slug, nil
		}
	}

	return "", submit.ErrVendorNotFound
}

// Submit creates the branch, commits the files as one commit, and opens a pull
// request — reusing whatever already exists so a retry is idempotent. Recovery
// reads state from GitHub, not from memory, so it survives a restart.
func (s *Submitter) Submit(ctx context.Context, req submit.Request) (submit.Result, error) {
	if len(req.Files) == 0 {
		return submit.Result{}, errors.New("github: submit called with no files")
	}
	base := req.BaseBranch
	if base == "" {
		base = defaultBaseBranch
	}

	// An already-open pull request for this deterministic branch is the whole
	// point of idempotency: return it without touching Git again.
	if existing, err := s.existingPR(ctx, req.Branch); err != nil {
		return submit.Result{}, err
	} else if existing != "" {
		return submit.Result{URL: existing, Branch: req.Branch, AlreadyExisted: true}, nil
	}

	if err := s.ensureBranch(ctx, base, req); err != nil {
		return submit.Result{}, err
	}

	pr, resp, err := s.client.rest.PullRequests.Create(ctx, s.owner, s.repo, &gh.NewPullRequest{
		Title: gh.Ptr(req.Title),
		Head:  gh.Ptr(s.owner + ":" + req.Branch),
		Base:  gh.Ptr(base),
		Body:  gh.Ptr(req.Body),
	})
	if err != nil {
		// A 422 here most commonly means a pull request already exists for the
		// head (a racing retry); recover by reading it back.
		if existing, lookupErr := s.existingPR(ctx, req.Branch); lookupErr == nil && existing != "" {
			return submit.Result{URL: existing, Branch: req.Branch, AlreadyExisted: true}, nil
		}
		return submit.Result{}, Classify("open pull request", resp, err)
	}

	return submit.Result{URL: pr.GetHTMLURL(), Branch: req.Branch}, nil
}

// existingPR returns the HTML URL of an open pull request for branch, or "" if
// none exists.
func (s *Submitter) existingPR(ctx context.Context, branch string) (string, error) {
	prs, resp, err := s.client.rest.PullRequests.List(ctx, s.owner, s.repo, &gh.PullRequestListOptions{
		State: "open",
		Head:  s.owner + ":" + branch,
	})
	if err != nil {
		return "", Classify("list pull requests", resp, err)
	}
	if len(prs) > 0 {
		return prs[0].GetHTMLURL(), nil
	}
	return "", nil
}

// ensureBranch creates req.Branch pointing at a new commit that applies the
// files onto base. If the branch already exists (an interrupted earlier attempt
// left it behind) it is left as-is: recovery reuses the branch rather than
// rewriting it, so a retry cannot clobber a commit a pull request already
// references.
func (s *Submitter) ensureBranch(ctx context.Context, base string, req submit.Request) error {
	if _, resp, err := s.client.rest.Git.GetRef(ctx, s.owner, s.repo, "heads/"+req.Branch); err == nil {
		return nil // branch already exists; reuse it
	} else if resp == nil || resp.StatusCode != http.StatusNotFound {
		return Classify("read branch "+req.Branch, resp, err)
	}

	baseRef, resp, err := s.client.rest.Git.GetRef(ctx, s.owner, s.repo, "heads/"+base)
	if err != nil {
		return Classify("resolve base branch "+base, resp, err)
	}
	baseSHA := baseRef.GetObject().GetSHA()

	entries := make([]*gh.TreeEntry, 0, len(req.Files))
	for _, f := range req.Files {
		entries = append(entries, &gh.TreeEntry{
			Path:    gh.Ptr(f.Path),
			Mode:    gh.Ptr("100644"),
			Type:    gh.Ptr("blob"),
			Content: gh.Ptr(string(f.Content)),
		})
	}
	tree, resp, err := s.client.rest.Git.CreateTree(ctx, s.owner, s.repo, baseSHA, entries)
	if err != nil {
		return Classify("create tree", resp, err)
	}

	commit, resp, err := s.client.rest.Git.CreateCommit(ctx, s.owner, s.repo, gh.Commit{
		Message: gh.Ptr(req.CommitMessage),
		Tree:    &gh.Tree{SHA: gh.Ptr(tree.GetSHA())},
		Parents: []*gh.Commit{{SHA: gh.Ptr(baseSHA)}},
	}, nil)
	if err != nil {
		return Classify("create commit", resp, err)
	}

	_, resp, err = s.client.rest.Git.CreateRef(ctx, s.owner, s.repo, gh.CreateRef{
		Ref: "refs/heads/" + req.Branch,
		SHA: commit.GetSHA(),
	})
	if err != nil {
		return Classify("create branch "+req.Branch, resp, err)
	}
	return nil
}

// splitRepository parses an "owner/name" string into its two parts.
func splitRepository(full string) (owner, repo string, err error) {
	full = strings.TrimSpace(full)
	owner, repo, ok := strings.Cut(full, "/")
	if !ok || owner == "" || repo == "" {
		return "", "", fmt.Errorf("%w: repository %q is not in owner/name form", ErrInvalidConfig, full)
	}
	return owner, repo, nil
}

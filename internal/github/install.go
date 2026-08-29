package github

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	gh "github.com/google/go-github/v88/github"
)

// Permissions the bot needs, and the level each must have. This is the whole
// job: create a branch, commit to it, open a pull request. Anything beyond it
// would let the bot do things the review model says only humans may do.
var requiredPermissions = map[string]string{
	"contents":      "write", // create branches, blobs, trees and commits
	"pull_requests": "write", // open and update pull requests
	"metadata":      "read",  // mandatory for every App
}

// VerifyInstallation checks, against the live API, that the installation is
// usable and no wider than intended. It mints a token (proving the private key
// and IDs agree), inspects the permissions GitHub actually granted, and
// confirms the expected repository is in scope.
//
// It is a preflight, not a gate: the caller decides whether a failure is fatal.
// Run it at startup so a misconfigured App is discovered then, rather than when
// the first vendor tries to submit a change — and before the client is shared
// with concurrent callers, because reading the installation's permissions
// touches transport state the transport does not itself guard.
func (c *Client) VerifyInstallation(ctx context.Context) error {
	if _, err := c.Token(ctx); err != nil {
		return err
	}

	granted, err := c.transport.Permissions()
	if err != nil {
		return fmt.Errorf("github: reading installation permissions: %w", err)
	}

	if err := checkPermissions(granted); err != nil {
		return err
	}

	return c.checkRepositoryScope(ctx)
}

// checkPermissions compares the granted permission set against what the design
// requires. Both directions matter: too little and the bot cannot open a pull
// request, too much and it can undermine review.
//
// Excess is judged by deny-by-default rather than against a list of dangerous
// permissions, because such a list is only ever as current as the last time
// someone read GitHub's permission catalogue. Several innocuous-sounding grants
// defeat the review model — `workflows` rewrites the CI that gates merging,
// `checks` and `statuses` can satisfy required status checks, `administration`
// edits branch protection itself — and the next one added by GitHub would not
// be on any list written today. Anything not needed is therefore reported.
func checkPermissions(granted gh.InstallationPermissions) error {
	actual, err := permissionMap(granted)
	if err != nil {
		return err
	}

	var missing, excess []string

	for name, want := range requiredPermissions {
		got := actual[name]
		if got == "" {
			missing = append(missing, fmt.Sprintf("%s:%s (not granted)", name, want))
			continue
		}
		if !satisfies(got, want) {
			missing = append(missing, fmt.Sprintf("%s:%s (granted %s)", name, want, got))
		}
	}

	for name, level := range actual {
		if _, required := requiredPermissions[name]; !required {
			excess = append(excess, fmt.Sprintf("%s:%s", name, level))
		}
	}

	sort.Strings(missing)
	sort.Strings(excess)

	switch {
	case len(missing) > 0 && len(excess) > 0:
		return &PermissionError{Op: "act as the presets bot", Message: fmt.Sprintf(
			"installation is missing %s and holds permissions it does not need (%s)",
			strings.Join(missing, ", "), strings.Join(excess, ", "))}
	case len(missing) > 0:
		return &PermissionError{Op: "act as the presets bot", Message: fmt.Sprintf(
			"installation is missing %s", strings.Join(missing, ", "))}
	case len(excess) > 0:
		return &PermissionError{Op: "act as the presets bot", Message: fmt.Sprintf(
			"installation holds permissions it does not need (%s); the bot proposes, humans merge",
			strings.Join(excess, ", "))}
	}

	return nil
}

// satisfies reports whether a granted level covers a required one. GitHub's
// levels are ordered, so write covers read. No repository permission this
// package requires offers an admin level today; the ordering simply avoids
// rejecting a grant that is stricter than asked for.
func satisfies(granted, required string) bool {
	rank := map[string]int{"read": 1, "write": 2, "admin": 3}
	return rank[granted] >= rank[required]
}

// checkRepositoryScope confirms the configured repository is one the
// installation can reach. An App installed on the wrong account, or on "selected
// repositories" that do not include the presets repo, authenticates perfectly
// and then 404s on the first real call — this turns that into a clear startup
// error.
func (c *Client) checkRepositoryScope(ctx context.Context) error {
	want := strings.TrimSpace(c.cfg.Repository)
	if want == "" {
		return nil
	}

	opts := &gh.ListOptions{PerPage: 100}
	var seen []string

	for {
		repos, resp, err := c.rest.Apps.ListRepos(ctx, opts)
		if err != nil {
			return Classify("list repositories for the installation", resp, err)
		}
		for _, repo := range repos.Repositories {
			full := repo.GetFullName()
			seen = append(seen, full)
			if strings.EqualFold(full, want) {
				return nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return &NotFoundError{Resource: fmt.Sprintf(
		"%s in this installation (it can reach: %s)", want, describeRepos(seen))}
}

func describeRepos(seen []string) string {
	if len(seen) == 0 {
		return "no repositories"
	}
	sort.Strings(seen)
	if len(seen) > 10 {
		return strings.Join(seen[:10], ", ") + fmt.Sprintf(", and %d more", len(seen)-10)
	}
	return strings.Join(seen, ", ")
}

// permissionMap flattens the granted permissions into a name/level map. It goes
// through the JSON encoding rather than naming fields, so every permission
// go-github models is checked without a hand-maintained list going stale.
//
// The honest limit: ghinstallation decodes the token response into go-github's
// typed struct, which has no catch-all, so a permission GitHub introduces after
// this go-github version is dropped before it gets here and cannot be reported
// as excess. Keeping the module current is what keeps the check complete.
func permissionMap(p gh.InstallationPermissions) (map[string]string, error) {
	encoded, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("github: encoding installation permissions: %w", err)
	}
	var levels map[string]string
	if err := json.Unmarshal(encoded, &levels); err != nil {
		return nil, fmt.Errorf("github: decoding installation permissions: %w", err)
	}
	return levels, nil
}

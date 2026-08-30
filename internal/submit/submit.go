// Package submit is the seam between the API's claim handler and whatever opens
// the pull request. The API depends only on the Submitter interface here, so it
// carries no direct dependency on GitHub and can be exercised in tests with a
// fake. The GitHub-backed implementation lives in internal/github.
//
// Splitting the interface into its own package keeps the dependency arrows
// pointing one way: both internal/api and internal/github import internal/submit,
// and neither imports the other.
package submit

import (
	"context"
	"errors"
)

// ErrVendorNotFound is returned by ResolveVendorSlug when no vendor.yaml at the
// current head maps its stytch_organization_id to the caller's organization.
// The API renders this as a 403: the caller authenticated, but authority (a
// reviewed manifest in Git) does not grant them a namespace to write to. See
// docs/vendor-workflow.md ("Authorization").
var ErrVendorNotFound = errors.New("submit: no vendor namespace is owned by this organization")

// File is one file to write in the proposed commit, at its full path within the
// presets repository (for example "vendors/prusa/printers/prusa-mk4-0.4.yaml").
type File struct {
	Path    string
	Content []byte
}

// Request is a change set to propose as a single-commit pull request. The
// caller (the claim handler) owns naming and provenance: it derives a
// deterministic Branch from the content so a retry re-drives the same branch
// rather than opening a duplicate, and it bakes vendor/organization/member
// provenance into Body and CommitMessage.
type Request struct {
	// Branch is the deterministic head branch name. Reusing it makes submission
	// idempotent across retries and restarts.
	Branch string

	// BaseBranch is the branch to open the pull request against. Empty means the
	// repository default ("main").
	BaseBranch string

	Title         string
	Body          string
	CommitMessage string

	Files []File
}

// Result is the outcome of a submission: the pull request URL, the branch it
// lives on, and whether the pull request already existed (an idempotent retry).
type Result struct {
	URL            string
	Branch         string
	AlreadyExisted bool
}

// Submitter resolves the caller's writable namespace and opens pull requests on
// their behalf. Implementations authorize against the manifest at the current
// head and must fail closed if the head cannot be resolved.
type Submitter interface {
	// ResolveVendorSlug returns the vendor slug whose vendor.yaml at the current
	// head declares the given Stytch organization ID, or ErrVendorNotFound.
	ResolveVendorSlug(ctx context.Context, organizationID string) (string, error)

	// Submit creates (or idempotently reuses) a branch, commits the files as one
	// commit, and opens a pull request. It returns the pull request URL.
	Submit(ctx context.Context, req Request) (Result, error)
}

// Package catalog holds the in-memory, immutable preset catalog and the
// atomic swap seam that ingest uses to publish a new revision.
//
// Every byte the API serves is derivable from a single Git commit: the
// catalog revision. Until the first successful ingest completes there is no
// catalog, the served revision is nil, and the server is not ready. See
// ARCHITECTURE.md ("Ingest & Catalog Rebuild") and docs/api-surface.md
// ("Operational Endpoints").
package catalog

import (
	"sync/atomic"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/search"
)

// Catalog is an immutable snapshot of the presets served for one Git commit.
//
// Later foundation-wave issues fill this in with the parsed presets and the
// fuzzy index. For the skeleton it carries only the provenance the health
// endpoint reports, so ingest has a concrete value to swap in.
type Catalog struct {
	// Revision is the Git commit SHA the snapshot was built from.
	Revision string

	// BuildID is the server build identity the snapshot was assembled by
	// (schema digest, validator version, binary build). Pagination cursors are
	// bound to Revision+BuildID so a cursor issued before either changed is
	// rejected rather than silently paginating across two catalogs.
	BuildID string

	// BuiltAt is when this snapshot was assembled.
	BuiltAt time.Time

	// Records is the in-memory search index over the snapshot: the short text
	// fields search ranks and a result row renders, built atomically with the
	// catalog from the same commit.
	Records []search.Record
}

// Holder is the concurrency-safe pointer to the current catalog.
//
// Readers take the current snapshot with a single atomic load and never
// block; ingest publishes a new snapshot with Swap. A nil snapshot means no
// catalog has been loaded yet, which is the not-ready state.
type Holder struct {
	current atomic.Pointer[Catalog]
}

// NewHolder returns an empty Holder. It reports not ready and a nil revision
// until the first Swap.
func NewHolder() *Holder {
	return &Holder{}
}

// Current returns the catalog snapshot being served, or nil if none has been
// loaded yet.
func (h *Holder) Current() *Catalog {
	return h.current.Load()
}

// Swap publishes c as the catalog now served and returns the previous
// snapshot, if any. Passing a non-nil catalog is what flips readiness to true.
func (h *Holder) Swap(c *Catalog) *Catalog {
	return h.current.Swap(c)
}

// Ready reports whether a catalog has been loaded. Readiness is false until
// the first successful ingest so an instance with an empty catalog never
// serves confident, empty results.
func (h *Holder) Ready() bool {
	return h.current.Load() != nil
}

// Revision returns the served catalog revision, or nil until the first
// ingest. A pointer is used so the health endpoint can render null rather
// than an empty string when nothing has been loaded.
func (h *Holder) Revision() *string {
	c := h.current.Load()
	if c == nil {
		return nil
	}
	rev := c.Revision
	return &rev
}

// LastIngestAt returns when the served catalog was built, or nil until the
// first ingest.
func (h *Holder) LastIngestAt() *time.Time {
	c := h.current.Load()
	if c == nil {
		return nil
	}
	t := c.BuiltAt
	return &t
}

// Package catalog holds the in-memory, immutable preset catalog and the
// bookkeeping the rest of the system needs to reason about freshness.
//
// The catalog is rebuilt from a single Git commit on ingest and swapped in
// atomically. Until the first successful ingest completes there is no catalog,
// so the store reports an unset revision and is not ready to serve.
package catalog

import (
	"sync"
	"time"
)

// State is an immutable snapshot of what the store is currently serving. A nil
// *State means no catalog has been loaded yet.
type State struct {
	// Revision is the Git commit SHA the served catalog was built from.
	Revision string
	// LastIngest is the time the catalog was swapped in.
	LastIngest time.Time
}

// Store guards the currently served catalog state behind an atomic swap. It is
// safe for concurrent use. Later waves attach the actual preset data and search
// index to the swap; this skeleton tracks only the revision and readiness that
// the health endpoint and caching layer depend on.
type Store struct {
	mu    sync.RWMutex
	state *State
}

// New returns an empty store. It is not ready until the first Swap.
func New() *Store {
	return &Store{}
}

// Swap atomically replaces the served state with a catalog built from revision
// at the given time. This is the only way the store becomes ready.
func (s *Store) Swap(revision string, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = &State{Revision: revision, LastIngest: at}
}

// State returns the current snapshot, or nil if no catalog has been loaded yet.
func (s *Store) State() *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Ready reports whether a catalog has been loaded and can be served. It is
// false until the first successful ingest, because an empty catalog would
// otherwise serve confident, empty results.
func (s *Store) Ready() bool {
	return s.State() != nil
}

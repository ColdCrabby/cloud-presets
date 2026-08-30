// Package upload holds the short-lived, in-memory store of uploaded preset
// drafts that a vendor claims from the admin app.
//
// This is deliberately not a database. A draft is a transient artifact of the
// "upload then claim" hand-off: a vendor (or a tool) POSTs preset files, the
// server parks the parsed result under an unguessable id, and the admin UI
// loads it back by that id to review and submit. Drafts expire on a TTL and are
// dropped on restart — nothing here is a system of record. The pull request is
// the record (see docs/vendor-workflow.md).
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/preset"
)

// DefaultTTL is how long a draft is claimable before it is garbage-collected.
// It is generous enough for a vendor to finish signing in and reviewing, but
// short enough that abandoned uploads do not accumulate in memory.
const DefaultTTL = 30 * time.Minute

// ErrNotFound is returned when a draft id is unknown or has expired. The two are
// deliberately indistinguishable to a client so an expired id cannot be told
// apart from one that never existed.
var ErrNotFound = errors.New("upload: draft not found or expired")

// File is one uploaded preset within a draft: its parsed identity plus the
// original YAML bytes, so the exact authored file can be committed later.
type File struct {
	// Kind is the preset category, inferred from the upload (printer, filament,
	// or process).
	Kind preset.Kind

	// ID is the preset id declared in the file body.
	ID string

	// Name is the human-readable preset name declared in the file body.
	Name string

	// Vendor is the vendor string declared in the body, for printer and filament
	// presets. Empty for a process preset.
	Vendor string

	// FileName is the leaf file name the preset must be stored as ("<id>.yaml").
	FileName string

	// Content is the original, unmodified YAML the vendor uploaded.
	Content []byte
}

// Draft is a set of uploaded preset files parked for claiming, with a lifetime.
type Draft struct {
	ID        string
	CreatedAt time.Time
	ExpiresAt time.Time
	Files     []File
}

// Store is a concurrency-safe, TTL-bounded map of drafts. Expired drafts are
// swept lazily on every access, so no background goroutine is needed and the
// store adds nothing to shut down cleanly.
type Store struct {
	mu     sync.Mutex
	drafts map[string]*Draft
	ttl    time.Duration
	now    func() time.Time
}

// NewStore returns a store whose drafts live for ttl. A non-positive ttl uses
// DefaultTTL.
func NewStore(ttl time.Duration) *Store {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &Store{
		drafts: make(map[string]*Draft),
		ttl:    ttl,
		now:    time.Now,
	}
}

// Create parks files under a freshly generated, unguessable id and returns the
// stored draft. The id is the only capability needed to load the draft back, so
// it is 128 bits of randomness rather than a guessable counter.
func (s *Store) Create(files []File) (*Draft, error) {
	id, err := newID()
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()

	now := s.now()
	d := &Draft{
		ID:        id,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
		Files:     files,
	}
	s.drafts[id] = d
	return d, nil
}

// Get returns the draft for id, or ErrNotFound if it is unknown or expired.
func (s *Store) Get(id string) (*Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()

	d, ok := s.drafts[id]
	if !ok {
		return nil, ErrNotFound
	}
	return d, nil
}

// Delete removes a draft, if present. Claiming a draft consumes it, so the same
// upload cannot be submitted twice by replaying its id.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.drafts, id)
}

// Len reports the number of live (non-expired) drafts. It exists for tests and
// for an eventual metric; it sweeps first so the count excludes expired drafts.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcLocked()
	return len(s.drafts)
}

// gcLocked drops expired drafts. The caller must hold s.mu.
func (s *Store) gcLocked() {
	now := s.now()
	for id, d := range s.drafts {
		if !now.Before(d.ExpiresAt) {
			delete(s.drafts, id)
		}
	}
}

// sortedIDs returns the live draft ids in order, used only by tests.
func (s *Store) sortedIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.drafts))
	for id := range s.drafts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// newID returns 16 cryptographically random bytes as a hex string.
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

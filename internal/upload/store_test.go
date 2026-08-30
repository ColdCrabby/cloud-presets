package upload

import (
	"testing"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/preset"
)

func sampleFiles() []File {
	return []File{{
		Kind:     preset.KindPrinter,
		ID:       "prusa-mk4-0.4",
		Name:     "Prusa MK4",
		Vendor:   "Prusa",
		FileName: "prusa-mk4-0.4.yaml",
		Content:  []byte("schema_version: 1\n"),
	}}
}

func TestCreateAndGet(t *testing.T) {
	s := NewStore(time.Minute)
	d, err := s.Create(sampleFiles())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if d.ID == "" {
		t.Fatal("Create returned an empty id")
	}
	got, err := s.Get(d.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Files) != 1 || got.Files[0].ID != "prusa-mk4-0.4" {
		t.Fatalf("Get returned unexpected files: %+v", got.Files)
	}
}

func TestGetUnknownIsNotFound(t *testing.T) {
	s := NewStore(time.Minute)
	if _, err := s.Get("nope"); err != ErrNotFound {
		t.Fatalf("Get unknown = %v, want ErrNotFound", err)
	}
}

func TestDeleteConsumesDraft(t *testing.T) {
	s := NewStore(time.Minute)
	d, _ := s.Create(sampleFiles())
	s.Delete(d.ID)
	if _, err := s.Get(d.ID); err != ErrNotFound {
		t.Fatalf("Get after Delete = %v, want ErrNotFound", err)
	}
}

func TestExpiryDropsDraft(t *testing.T) {
	s := NewStore(time.Minute)
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }

	d, _ := s.Create(sampleFiles())
	if s.Len() != 1 {
		t.Fatalf("Len = %d, want 1", s.Len())
	}

	now = now.Add(2 * time.Minute) // past the TTL
	if _, err := s.Get(d.ID); err != ErrNotFound {
		t.Fatalf("Get expired = %v, want ErrNotFound", err)
	}
	if s.Len() != 0 {
		t.Fatalf("Len after expiry = %d, want 0", s.Len())
	}
}

func TestIDsAreUniqueAndUnguessable(t *testing.T) {
	s := NewStore(time.Minute)
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		d, err := s.Create(sampleFiles())
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if len(d.ID) != 32 { // 16 random bytes as hex
			t.Fatalf("id %q has length %d, want 32", d.ID, len(d.ID))
		}
		if seen[d.ID] {
			t.Fatalf("duplicate id %q", d.ID)
		}
		seen[d.ID] = true
	}
	_ = s.sortedIDs()
}

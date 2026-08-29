package catalog

import (
	"testing"
	"time"
)

func TestHolderEmptyBeforeSwap(t *testing.T) {
	h := NewHolder()

	if h.Ready() {
		t.Error("Ready() = true on empty holder, want false")
	}
	if h.Current() != nil {
		t.Error("Current() != nil on empty holder")
	}
	if h.Revision() != nil {
		t.Errorf("Revision() = %v on empty holder, want nil", *h.Revision())
	}
	if h.LastIngestAt() != nil {
		t.Error("LastIngestAt() != nil on empty holder")
	}
}

func TestHolderSwapPublishesRevision(t *testing.T) {
	h := NewHolder()
	builtAt := time.Now().UTC()

	prev := h.Swap(&Catalog{Revision: "deadbeef", BuiltAt: builtAt})
	if prev != nil {
		t.Error("first Swap returned non-nil previous catalog")
	}

	if !h.Ready() {
		t.Error("Ready() = false after Swap, want true")
	}
	if rev := h.Revision(); rev == nil || *rev != "deadbeef" {
		t.Errorf("Revision() = %v, want deadbeef", rev)
	}
	if at := h.LastIngestAt(); at == nil || !at.Equal(builtAt) {
		t.Errorf("LastIngestAt() = %v, want %v", at, builtAt)
	}

	prev = h.Swap(&Catalog{Revision: "cafef00d", BuiltAt: builtAt})
	if prev == nil || prev.Revision != "deadbeef" {
		t.Errorf("second Swap returned previous = %v, want deadbeef", prev)
	}
	if rev := h.Revision(); rev == nil || *rev != "cafef00d" {
		t.Errorf("Revision() = %v after second Swap, want cafef00d", rev)
	}
}

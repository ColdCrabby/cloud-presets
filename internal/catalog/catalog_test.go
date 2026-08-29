package catalog

import (
	"testing"
	"time"
)

func TestStoreNotReadyUntilSwap(t *testing.T) {
	s := New()
	if s.Ready() {
		t.Fatal("new store must not be ready")
	}
	if s.State() != nil {
		t.Fatal("new store must have nil state")
	}

	now := time.Now()
	s.Swap("abc123", now)

	if !s.Ready() {
		t.Fatal("store must be ready after swap")
	}
	state := s.State()
	if state == nil {
		t.Fatal("state must be set after swap")
	}
	if state.Revision != "abc123" {
		t.Fatalf("revision = %q, want abc123", state.Revision)
	}
	if !state.LastIngest.Equal(now) {
		t.Fatalf("lastIngest = %v, want %v", state.LastIngest, now)
	}
}

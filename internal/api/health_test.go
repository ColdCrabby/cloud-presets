package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func decode(t *testing.T, data []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestHealthEmptyCatalog(t *testing.T) {
	store := catalog.New()
	_, humaAPI := humatest.New(t)
	registerHealth(humaAPI, store)

	resp := humaAPI.Get("/v1/health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}

	var body HealthBody
	decode(t, resp.Body.Bytes(), &body)

	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.Ready {
		t.Fatal("ready must be false before first ingest")
	}
	if body.Revision != nil {
		t.Fatalf("revision = %v, want nil before first ingest", *body.Revision)
	}
	if body.LastIngest != nil {
		t.Fatalf("lastIngest = %v, want nil before first ingest", *body.LastIngest)
	}
}

func TestHealthAfterIngest(t *testing.T) {
	store := catalog.New()
	now := time.Now().UTC().Truncate(time.Second)
	store.Swap("deadbeef", now)

	_, humaAPI := humatest.New(t)
	registerHealth(humaAPI, store)

	resp := humaAPI.Get("/v1/health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.Code)
	}

	var body HealthBody
	decode(t, resp.Body.Bytes(), &body)

	if !body.Ready {
		t.Fatal("ready must be true after ingest")
	}
	if body.Revision == nil || *body.Revision != "deadbeef" {
		t.Fatalf("revision = %v, want deadbeef", body.Revision)
	}
	if body.LastIngest == nil || !body.LastIngest.Equal(now) {
		t.Fatalf("lastIngest = %v, want %v", body.LastIngest, now)
	}
}

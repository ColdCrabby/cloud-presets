package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func TestCacheableEndpointsCarryRevisionAndETag(t *testing.T) {
	holder := catalog.NewHolder()
	holder.Swap(&catalog.Catalog{Revision: "rev-1", BuildID: "build-1", BuiltAt: time.Now()})
	_, handler := New(holder, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	for _, path := range []string{"/v1/presets", "/v1/vendors"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, resp.StatusCode)
		}
		if got := resp.Header.Get("X-Catalog-Revision"); got != "rev-1" {
			t.Errorf("GET %s: X-Catalog-Revision = %q, want rev-1", path, got)
		}
		if resp.Header.Get("ETag") == "" {
			t.Errorf("GET %s: missing ETag header", path)
		}
	}
}

func TestMatchingIfNoneMatchReturns304WithoutBody(t *testing.T) {
	holder := catalog.NewHolder()
	holder.Swap(&catalog.Catalog{Revision: "rev-1", BuildID: "build-1", BuiltAt: time.Now()})
	_, handler := New(holder, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	first, err := http.Get(srv.URL + "/v1/presets")
	if err != nil {
		t.Fatal(err)
	}
	etag := first.Header.Get("ETag")
	first.Body.Close()
	if etag == "" {
		t.Fatal("first response carried no ETag")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/presets", nil)
	req.Header.Set("If-None-Match", etag)
	second, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()

	if second.StatusCode != http.StatusNotModified {
		t.Fatalf("status = %d, want 304", second.StatusCode)
	}
	if got := second.Header.Get("X-Catalog-Revision"); got != "rev-1" {
		t.Errorf("304 response X-Catalog-Revision = %q, want rev-1", got)
	}
}

func TestStaleETagAfterRevisionChangeIsRejected(t *testing.T) {
	holder := catalog.NewHolder()
	holder.Swap(&catalog.Catalog{Revision: "rev-1", BuildID: "build-1", BuiltAt: time.Now()})
	_, handler := New(holder, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	first, err := http.Get(srv.URL + "/v1/presets")
	if err != nil {
		t.Fatal(err)
	}
	etag := first.Header.Get("ETag")
	first.Body.Close()

	// A new ingest publishes a new revision; the old ETag no longer applies.
	holder.Swap(&catalog.Catalog{Revision: "rev-2", BuildID: "build-1", BuiltAt: time.Now()})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/presets", nil)
	req.Header.Set("If-None-Match", etag)
	second, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Body.Close()

	if second.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (stale ETag must not short-circuit)", second.StatusCode)
	}
	if got := second.Header.Get("X-Catalog-Revision"); got != "rev-2" {
		t.Errorf("X-Catalog-Revision = %q, want rev-2", got)
	}
}

func TestErrorResponsesAreNotTaggedWithETag(t *testing.T) {
	// No catalog loaded: /v1/presets serves 503, which must never be cached as
	// if it were the (not yet existing) successful representation.
	holder := catalog.NewHolder()
	_, handler := New(holder, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/presets")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if got := resp.Header.Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none on an error response", got)
	}
	if got := resp.Header.Get("X-Catalog-Revision"); got != "" {
		t.Errorf("X-Catalog-Revision = %q, want none before the first ingest", got)
	}
}

func TestHealthIsNotRevisionCached(t *testing.T) {
	holder := catalog.NewHolder()
	holder.Swap(&catalog.Catalog{Revision: "rev-1", BuildID: "build-1", BuiltAt: time.Now()})
	_, handler := New(holder, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("ETag"); got != "" {
		t.Errorf("ETag = %q, want none on /v1/health", got)
	}
}

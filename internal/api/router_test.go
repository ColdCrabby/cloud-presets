package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// decodeProblem asserts an RFC 9457 problem+json response and returns its body.
func decodeProblem(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode problem body: %v", err)
	}
	return body
}

func TestUnknownPathRendersProblemJSON(t *testing.T) {
	_, handler := New(catalog.NewHolder(), nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body := decodeProblem(t, resp)
	if body["status"] != float64(http.StatusNotFound) {
		t.Errorf("status field = %v, want 404", body["status"])
	}
	if body["title"] != http.StatusText(http.StatusNotFound) {
		t.Errorf("title field = %v, want %q", body["title"], http.StatusText(http.StatusNotFound))
	}
}

func TestWrongMethodRendersProblemJSONWithAllow(t *testing.T) {
	_, handler := New(catalog.NewHolder(), nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/health", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if allow := resp.Header.Get("Allow"); !strings.Contains(allow, http.MethodGet) {
		t.Errorf("Allow header = %q, want it to include GET", allow)
	}
	body := decodeProblem(t, resp)
	if body["status"] != float64(http.StatusMethodNotAllowed) {
		t.Errorf("status field = %v, want 405", body["status"])
	}
}

func TestHealthRoutesThroughWrappedHandler(t *testing.T) {
	_, handler := New(catalog.NewHolder(), nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

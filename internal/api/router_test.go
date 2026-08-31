package api

import (
	"encoding/json"
	"io"
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

// TestOpenAPISpecIsServed verifies the machine-readable spec is reachable at the
// mounted path, so the client generator (and anyone else) can fetch it.
func TestOpenAPISpecIsServed(t *testing.T) {
	_, handler := New(catalog.NewHolder(), nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	for _, path := range []string{
		"/v1/openapi.json",     // OpenAPI 3.1 (the Angular generator's input)
		"/v1/openapi-3.0.json", // downgrade for 3.0-only generators
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s: status = %d, want 200", path, resp.StatusCode)
			}
			var doc map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
				t.Fatalf("GET %s: decode spec: %v", path, err)
			}
			if doc["openapi"] == nil {
				t.Errorf("GET %s: spec missing the \"openapi\" version field", path)
			}
		}()
	}
}

// TestDocsRendersScalarReference verifies /v1/docs serves the Scalar reference
// page and points it at the mounted spec, so the viewer resolves same-origin.
func TestDocsRendersScalarReference(t *testing.T) {
	_, handler := New(catalog.NewHolder(), nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(body)
	if !strings.Contains(page, "@scalar/api-reference") {
		t.Errorf("docs page is not the Scalar renderer:\n%s", page)
	}
	if !strings.Contains(page, "/v1/openapi.json") {
		t.Errorf("docs page does not reference the mounted spec /v1/openapi.json")
	}
}

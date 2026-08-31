package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

const allowedOrigin = "https://slicer.maxsopp.de"

func corsHandler() http.Handler {
	return CORS(DefaultAllowedOrigins, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
}

func TestCORSAllowsSlicerOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/presets", nil)
	req.Header.Set("Origin", allowedOrigin)
	rec := httptest.NewRecorder()

	corsHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("body = %q, want passthrough to next", rec.Body.String())
	}
}

func TestCORSPreflightIsAnsweredWith204(t *testing.T) {
	req := httptest.NewRequest(http.MethodOptions, "/v1/presets", nil)
	req.Header.Set("Origin", allowedOrigin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	rec := httptest.NewRecorder()

	corsHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != allowedOrigin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != corsAllowMethods {
		t.Errorf("Access-Control-Allow-Methods = %q, want %q", got, corsAllowMethods)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "authorization" {
		t.Errorf("Access-Control-Allow-Headers = %q, want the requested headers echoed", got)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("preflight body = %q, want empty (next not called)", rec.Body.String())
	}
}

func TestCORSIgnoresDisallowedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/presets", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	corsHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a disallowed origin", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (request still served)", rec.Code)
	}
}

func TestCORSPassesThroughWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/presets", nil)
	rec := httptest.NewRecorder()

	corsHandler().ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q, want empty for a non-CORS request", got)
	}
	if got := rec.Header().Get("Vary"); got != "" {
		t.Errorf("Vary = %q, want empty for a non-CORS request", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

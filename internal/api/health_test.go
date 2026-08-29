package api

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func TestHealthNotReadyBeforeIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	resp := api.Get("/v1/health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	for _, want := range []string{`"status":"ok"`, `"ready":false`, `"revision":null`, `"lastIngestAt":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got %s", want, body)
		}
	}
}

func TestHealthReadyAfterIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	builtAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	holder.Swap(&catalog.Catalog{Revision: "abc123", BuiltAt: builtAt})

	resp := api.Get("/v1/health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	for _, want := range []string{`"ready":true`, `"revision":"abc123"`, `"lastIngestAt":"2026-01-02T03:04:05Z"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got %s", want, body)
		}
	}
}

// TestErrorsRenderAsProblemJSON confirms the API renders errors as RFC 9457
// application/problem+json, using a throwaway failing operation registered on
// the same Config the server uses.
func TestErrorsRenderAsProblemJSON(t *testing.T) {
	_, api := humatest.New(t, Config())
	huma.Register(api, huma.Operation{
		OperationID: "boom",
		Method:      http.MethodGet,
		Path:        "/boom",
	}, func(_ context.Context, _ *struct{}) (*struct{}, error) {
		return nil, huma.Error503ServiceUnavailable("no catalog loaded yet")
	})

	resp := api.Get("/boom")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	if body := resp.Body.String(); !strings.Contains(body, `"status":503`) {
		t.Errorf("problem body missing status; got %s", body)
	}
}

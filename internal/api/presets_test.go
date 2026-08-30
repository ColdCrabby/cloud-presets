package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func TestSearchPresetsUnavailableBeforeIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	resp := api.Get("/v1/presets")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	for _, want := range []string{`"status":503`, `"detail":`, "not loaded yet"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got %s", want, body)
		}
	}
}

func TestSearchPresetsReturnsPageAfterIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	holder.Swap(&catalog.Catalog{Revision: "abc123", BuiltAt: time.Now()})

	resp := api.Get("/v1/presets")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	for _, want := range []string{`"results":[]`, `"revision":"abc123"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got %s", want, body)
		}
	}
}

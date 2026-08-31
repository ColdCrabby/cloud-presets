package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/ColdCrabby/cloud-presets/internal/search"
)

// detailCatalog builds a catalog carrying both search summaries and the full
// profiles the detail endpoint serves.
func detailCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Revision: "rev-1",
		BuildID:  "build-1",
		BuiltAt:  time.Now(),
		Records: []search.Record{
			{ID: "prusa-mk4", Type: "printer", Name: "Prusa MK4", Vendor: "prusa", Model: "MK4", Spec: "250mm"},
		},
		Presets: map[string]catalog.FullPreset{
			"prusa-mk4": {
				ID:     "prusa-mk4",
				Type:   "printer",
				Name:   "Prusa MK4",
				Vendor: "prusa",
				Params: map[string]any{"nozzle_diameter": 0.4, "bed_shape": "250x210"},
			},
		},
	}
}

func decodePreset(t *testing.T, body []byte) PresetResponse {
	t.Helper()
	var resp PresetResponse
	if err := json.Unmarshal(body, &resp.Body); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	return resp
}

func TestGetPresetUnavailableBeforeIngest(t *testing.T) {
	api := newTestAPI(t, nil)

	resp := api.Get("/v1/presets/prusa-mk4")
	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body=%s", resp.Code, resp.Body.String())
	}
}

func TestGetPresetReturnsFullProfile(t *testing.T) {
	api := newTestAPI(t, detailCatalog())

	resp := api.Get("/v1/presets/prusa-mk4")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	got := decodePreset(t, resp.Body.Bytes())
	if got.Body.ID != "prusa-mk4" {
		t.Errorf("id = %q, want prusa-mk4", got.Body.ID)
	}
	if got.Body.Source != "catalog" {
		t.Errorf("source = %q, want catalog", got.Body.Source)
	}
	if !strings.HasSuffix(got.Body.ImportURL, "/v1/presets/prusa-mk4") {
		t.Errorf("import_url = %q, want to end with /v1/presets/prusa-mk4", got.Body.ImportURL)
	}
	if got.Body.Params["nozzle_diameter"] == nil {
		t.Errorf("params missing nozzle_diameter; got %v", got.Body.Params)
	}
}

func TestGetPresetUnknownIDReturns404(t *testing.T) {
	api := newTestAPI(t, detailCatalog())

	resp := api.Get("/v1/presets/does-not-exist")
	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"status":404`) {
		t.Errorf("body missing status 404; got %s", resp.Body.String())
	}
}

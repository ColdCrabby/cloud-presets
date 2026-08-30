package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/ColdCrabby/cloud-presets/internal/search"
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

// testCatalog builds a deterministic catalog for the handler tests.
func testCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Revision: "rev-1",
		BuildID:  "build-1",
		BuiltAt:  time.Now(),
		Records: []search.Record{
			{ID: "prusa-mk4", Type: "printer", Name: "Prusa MK4", Vendor: "prusa", Model: "MK4", Spec: "250mm"},
			{ID: "prusa-mini", Type: "printer", Name: "Prusa Mini+", Vendor: "prusa", Model: "Mini+", Spec: "180mm"},
			{ID: "bambu-x1c", Type: "printer", Name: "Bambu Lab X1 Carbon", Vendor: "bambulab", Model: "X1 Carbon", Spec: "256mm"},
			{ID: "prusament-pla", Type: "filament", Name: "Prusament PLA", Vendor: "prusa", Material: "PLA", Spec: "1.24 g/cm3"},
			{ID: "bambu-abs", Type: "filament", Name: "Bambu ABS", Vendor: "bambulab", Material: "ABS", Spec: "1.05 g/cm3"},
		},
	}
}

func newTestAPI(t *testing.T, c *catalog.Catalog) humatest.TestAPI {
	t.Helper()
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)
	if c != nil {
		holder.Swap(c)
	}
	return api
}

func decodeSearch(t *testing.T, body []byte) SearchResponse {
	t.Helper()
	var resp SearchResponse
	if err := json.Unmarshal(body, &resp.Body); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	return resp
}

func TestSearchPresetsBrowseReturnsSummaries(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	resp := api.Get("/v1/presets")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	got := decodeSearch(t, resp.Body.Bytes())
	if len(got.Body.Results) != 5 {
		t.Fatalf("results = %d, want 5", len(got.Body.Results))
	}
	if got.Body.Revision != "rev-1" {
		t.Errorf("revision = %q, want rev-1", got.Body.Revision)
	}
	if got.Body.NextCursor != nil {
		t.Errorf("next_cursor = %v, want nil on a single page", *got.Body.NextCursor)
	}
	// Absent model/material render as null (omitempty pointer -> absent/null).
	for _, r := range got.Body.Results {
		if r.Type == "filament" && r.Model != nil {
			t.Errorf("filament %s has a model; want null", r.ID)
		}
		if r.Type == "printer" && r.Material != nil {
			t.Errorf("printer %s has a material; want null", r.ID)
		}
	}
}

func TestSearchPresetsFilterByType(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	resp := api.Get("/v1/presets?type=filament")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	got := decodeSearch(t, resp.Body.Bytes())
	if len(got.Body.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(got.Body.Results))
	}
	for _, r := range got.Body.Results {
		if r.Type != "filament" {
			t.Errorf("result %s type = %s, want filament", r.ID, r.Type)
		}
	}
}

func TestSearchPresetsQueryReturnsMatchInfo(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	resp := api.Get("/v1/presets?q=bambu")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	got := decodeSearch(t, resp.Body.Bytes())
	if len(got.Body.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(got.Body.Results))
	}
	for _, r := range got.Body.Results {
		if r.Match == nil {
			t.Fatalf("result %s missing match info", r.ID)
		}
		if len(r.Match.Ranges) == 0 {
			t.Errorf("result %s has empty ranges", r.ID)
		}
	}
}

func TestSearchPresetsPaginationWithCursor(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	resp := api.Get("/v1/presets?limit=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	page1 := decodeSearch(t, resp.Body.Bytes())
	if len(page1.Body.Results) != 2 {
		t.Fatalf("page1 results = %d, want 2", len(page1.Body.Results))
	}
	if page1.Body.NextCursor == nil {
		t.Fatal("page1 next_cursor is nil; want a continuation token")
	}

	seen := map[string]bool{}
	for _, r := range page1.Body.Results {
		seen[r.ID] = true
	}

	cursor := *page1.Body.NextCursor
	pages := 1
	for cursor != "" {
		resp := api.Get("/v1/presets?limit=2&cursor=" + cursor)
		if resp.Code != http.StatusOK {
			t.Fatalf("page status = %d, want 200; body=%s", resp.Code, resp.Body.String())
		}
		page := decodeSearch(t, resp.Body.Bytes())
		for _, r := range page.Body.Results {
			if seen[r.ID] {
				t.Errorf("id %s appeared on two pages", r.ID)
			}
			seen[r.ID] = true
		}
		pages++
		if page.Body.NextCursor == nil {
			break
		}
		cursor = *page.Body.NextCursor
	}
	if len(seen) != 5 {
		t.Fatalf("paginated through %d distinct ids, want 5", len(seen))
	}
	if pages != 3 {
		t.Fatalf("paginated in %d pages, want 3 (2+2+1)", pages)
	}
}

func TestSearchPresetsStaleCursorReturns409(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	// A cursor issued against a different revision must be rejected.
	stale := encodeCursor(pageCursor{Revision: "old-rev", BuildID: "build-1", Offset: 2})
	resp := api.Get("/v1/presets?cursor=" + stale)
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"status":409`) {
		t.Errorf("body missing status 409; got %s", resp.Body.String())
	}
}

func TestSearchPresetsMalformedCursorReturns409(t *testing.T) {
	api := newTestAPI(t, testCatalog())

	resp := api.Get("/v1/presets?cursor=not-a-valid-cursor")
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
}

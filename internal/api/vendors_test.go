package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func decodeVendors(t *testing.T, body []byte) ListVendorsResponse {
	t.Helper()
	var resp ListVendorsResponse
	if err := json.Unmarshal(body, &resp.Body); err != nil {
		t.Fatalf("decode body: %v; raw=%s", err, body)
	}
	return resp
}

func TestListVendorsUnavailableBeforeIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	resp := api.Get("/v1/vendors")
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

func TestListVendorsReturnsDirectoryAfterIngest(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	website := "https://www.prusa3d.com"
	holder.Swap(&catalog.Catalog{
		Revision: "abc123",
		BuiltAt:  time.Now(),
		Vendors: []catalog.Vendor{
			{
				Slug:        "prusa",
				DisplayName: "Prusa",
				Website:     &website,
			},
		},
	})

	resp := api.Get("/v1/vendors")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	got := decodeVendors(t, resp.Body.Bytes())
	if len(got.Body.Vendors) != 1 {
		t.Fatalf("vendors = %d, want 1", len(got.Body.Vendors))
	}
	v := got.Body.Vendors[0]
	if v.Slug != "prusa" || v.DisplayName != "Prusa" {
		t.Errorf("vendor = %+v, want prusa/Prusa", v)
	}
	if v.Website == nil || *v.Website != website {
		t.Errorf("website = %v, want %s", v.Website, website)
	}
	if got.Body.Revision != "abc123" {
		t.Errorf("revision = %q, want abc123", got.Body.Revision)
	}
	if got.Body.NextCursor != nil {
		t.Errorf("next_cursor = %v, want nil on a single page", *got.Body.NextCursor)
	}
}

func TestListVendorsReturnsEmptyPageWhenNoVendors(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	holder.Swap(&catalog.Catalog{Revision: "abc123", BuiltAt: time.Now()})

	resp := api.Get("/v1/vendors")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	got := decodeVendors(t, resp.Body.Bytes())
	if len(got.Body.Vendors) != 0 {
		t.Errorf("vendors = %d, want 0", len(got.Body.Vendors))
	}
	if got.Body.NextCursor != nil {
		t.Errorf("next_cursor = %v, want nil", *got.Body.NextCursor)
	}
}

func TestListVendorsOmitsWebsiteWhenAbsent(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	holder.Swap(&catalog.Catalog{
		Revision: "abc123",
		BuiltAt:  time.Now(),
		Vendors: []catalog.Vendor{
			{Slug: "acme", DisplayName: "Acme"},
		},
	})

	resp := api.Get("/v1/vendors")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	if body := resp.Body.String(); strings.Contains(body, "website") {
		t.Errorf("body should omit website; got %s", body)
	}
}

func vendorCatalog() *catalog.Catalog {
	return &catalog.Catalog{
		Revision: "rev-1",
		BuildID:  "build-1",
		BuiltAt:  time.Now(),
		Vendors: []catalog.Vendor{
			{Slug: "delta", DisplayName: "Delta"},
			{Slug: "alpha", DisplayName: "Alpha"},
			{Slug: "charlie", DisplayName: "Charlie"},
			{Slug: "bravo", DisplayName: "Bravo"},
			{Slug: "echo", DisplayName: "Echo"},
		},
	}
}

func TestListVendorsPaginatesInSlugOrder(t *testing.T) {
	api := newTestAPI(t, vendorCatalog())

	resp := api.Get("/v1/vendors?limit=2")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	page1 := decodeVendors(t, resp.Body.Bytes())
	if len(page1.Body.Vendors) != 2 {
		t.Fatalf("page1 vendors = %d, want 2", len(page1.Body.Vendors))
	}
	if page1.Body.Vendors[0].Slug != "alpha" || page1.Body.Vendors[1].Slug != "bravo" {
		t.Errorf("page1 order = %s,%s want alpha,bravo", page1.Body.Vendors[0].Slug, page1.Body.Vendors[1].Slug)
	}
	if page1.Body.NextCursor == nil {
		t.Fatal("page1 next_cursor is nil; want a continuation token")
	}

	var slugs []string
	slugs = append(slugs, page1.Body.Vendors[0].Slug, page1.Body.Vendors[1].Slug)
	cursor := *page1.Body.NextCursor
	pages := 1
	for cursor != "" {
		resp := api.Get("/v1/vendors?limit=2&cursor=" + cursor)
		if resp.Code != http.StatusOK {
			t.Fatalf("page status = %d, want 200; body=%s", resp.Code, resp.Body.String())
		}
		page := decodeVendors(t, resp.Body.Bytes())
		for _, v := range page.Body.Vendors {
			slugs = append(slugs, v.Slug)
		}
		pages++
		if page.Body.NextCursor == nil {
			break
		}
		cursor = *page.Body.NextCursor
	}
	want := []string{"alpha", "bravo", "charlie", "delta", "echo"}
	if strings.Join(slugs, ",") != strings.Join(want, ",") {
		t.Fatalf("paginated slugs = %v, want %v", slugs, want)
	}
	if pages != 3 {
		t.Fatalf("paginated in %d pages, want 3 (2+2+1)", pages)
	}
}

func TestListVendorsStaleCursorReturns409(t *testing.T) {
	api := newTestAPI(t, vendorCatalog())

	stale := encodeCursor(pageCursor{Revision: "old-rev", BuildID: "build-1", Offset: 2})
	resp := api.Get("/v1/vendors?cursor=" + stale)
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
}

func TestListVendorsMalformedCursorReturns409(t *testing.T) {
	api := newTestAPI(t, vendorCatalog())

	resp := api.Get("/v1/vendors?cursor=not-a-valid-cursor")
	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", resp.Code, resp.Body.String())
	}
}

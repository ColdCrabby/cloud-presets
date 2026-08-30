package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

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

	body := resp.Body.String()
	for _, want := range []string{
		`"slug":"prusa"`,
		`"display_name":"Prusa"`,
		`"website":"https://www.prusa3d.com"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q; got %s", want, body)
		}
	}
}

func TestListVendorsReturnsEmptyArrayWhenNoVendors(t *testing.T) {
	_, api := humatest.New(t, Config())
	holder := catalog.NewHolder()
	Register(api, holder)

	holder.Swap(&catalog.Catalog{Revision: "abc123", BuiltAt: time.Now()})

	resp := api.Get("/v1/vendors")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}

	if body := strings.TrimSpace(resp.Body.String()); body != "[]" {
		t.Errorf("body = %s, want []", body)
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

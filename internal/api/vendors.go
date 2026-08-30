package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// Vendor is the public representation of a vendor directory entry. It mirrors
// the Vendor schema in the hand-written OpenAPI document
// (openapi/openapi.json): stytch_organization_id is intentionally omitted
// because it is an authorization detail with no meaning to API consumers. See
// docs/api-surface.md ("Vendors").
type Vendor struct {
	Slug        string   `json:"slug" doc:"Stable vendor slug, matching the vendor.yaml directory name."`
	DisplayName string   `json:"display_name" doc:"Human-readable vendor name."`
	Brands      []string `json:"brands" doc:"Brand names this vendor may use in a preset's vendor field."`
	Website     *string  `json:"website,omitempty" format:"uri" doc:"Vendor website, when the manifest declares one."`
}

// ListVendorsResponse is the vendor directory as a top-level JSON array, the
// shape the hand-written spec's listVendors operation returns.
type ListVendorsResponse struct {
	Body []Vendor
}

// registerVendors wires GET /v1/vendors.
//
// Like search, the vendor directory is derived from the ingested catalog, so
// before the first ingest there is nothing to serve: the handler returns a 503
// problem document making the not-ready state explicit rather than a confident
// empty array that reads like "no vendors exist". Once a catalog is loaded it
// returns the well-formed (possibly empty) directory.
func registerVendors(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "listVendors",
		Method:      http.MethodGet,
		Path:        BasePath + "/vendors",
		Summary:     "List the vendor directory",
		Description: "Returns the vendor directory derived from the vendor.yaml " +
			"manifests in the served catalog. The stytch_organization_id is not " +
			"part of the public representation.",
		Tags: []string{"Vendors"},
	}, func(_ context.Context, _ *struct{}) (*ListVendorsResponse, error) {
		current := holder.Current()
		if current == nil {
			return nil, huma.Error503ServiceUnavailable(
				"The preset catalog is not loaded yet, so the vendor directory is temporarily unavailable. Please try again shortly.")
		}
		resp := &ListVendorsResponse{Body: []Vendor{}}
		for _, v := range current.Vendors {
			brands := v.Brands
			if brands == nil {
				brands = []string{}
			}
			resp.Body = append(resp.Body, Vendor{
				Slug:        v.Slug,
				DisplayName: v.DisplayName,
				Brands:      brands,
				Website:     v.Website,
			})
		}
		return resp, nil
	})
}

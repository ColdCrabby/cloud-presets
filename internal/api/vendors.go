package api

import (
	"context"
	"net/http"
	"sort"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// Vendor is the public representation of a vendor directory entry. It mirrors
// the Vendor schema in the hand-written OpenAPI document
// (openapi/openapi.json): stytch_organization_id is intentionally omitted
// because it is an authorization detail with no meaning to API consumers. See
// docs/api-surface.md ("Vendors").
type Vendor struct {
	Slug        string  `json:"slug" doc:"Stable vendor slug, matching the vendor.yaml directory name."`
	DisplayName string  `json:"display_name" doc:"Human-readable vendor name."`
	Website     *string `json:"website,omitempty" format:"uri" doc:"Vendor website, when the manifest declares one."`
}

// ListVendorsInput carries the pagination controls for GET /v1/vendors. Like
// search, the directory is never dumped in one response: a client walks it a
// page at a time. Limit is bounded server-side; Cursor continues a walk.
type ListVendorsInput struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"100" doc:"Maximum vendors to return (bounded server-side)."`
	Cursor string `query:"cursor" doc:"Opaque continuation token from a previous page."`
}

// ListVendorsResponse is a page of vendor directory entries plus the catalog
// revision it was served from, matching the cursor-paginated shape of search so
// no endpoint returns an unbounded array.
type ListVendorsResponse struct {
	Body struct {
		Vendors    []Vendor `json:"vendors" doc:"The page of vendor directory entries."`
		NextCursor *string  `json:"next_cursor,omitempty" doc:"Continuation token; absent on the last page."`
		Revision   string   `json:"revision" doc:"Catalog revision the page was served from."`
	}
}

// registerVendors wires GET /v1/vendors.
//
// Like search, the vendor directory is derived from the ingested catalog, so
// before the first ingest there is nothing to serve: the handler returns a 503
// problem document making the not-ready state explicit rather than a confident
// empty array that reads like "no vendors exist". Once a catalog is loaded it
// returns a cursor-paginated page of the (possibly empty) directory ordered by
// slug, so the response is bounded regardless of directory size and pagination
// is deterministic. A stale or malformed cursor fails with 409 exactly as
// search does.
func registerVendors(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "listVendors",
		Method:      http.MethodGet,
		Path:        BasePath + "/vendors",
		Summary:     "List the vendor directory",
		Description: "Returns a page of the vendor directory derived from the " +
			"vendor.yaml manifests in the served catalog. The directory is " +
			"cursor-paginated so it is never dumped in one response; the " +
			"stytch_organization_id is not part of the public representation.",
		Tags: []string{"Vendors"},
		// 409: a stale or malformed pagination cursor. 503: catalog not loaded.
		Errors: []int{http.StatusConflict, http.StatusServiceUnavailable},
	}, func(_ context.Context, in *ListVendorsInput) (*ListVendorsResponse, error) {
		current := holder.Current()
		if current == nil {
			return nil, huma.Error503ServiceUnavailable(
				"The preset catalog is not loaded yet, so the vendor directory is temporarily unavailable. Please try again shortly.")
		}

		offset := 0
		if in.Cursor != "" {
			c, ok := decodeCursor(in.Cursor)
			if !ok {
				return nil, huma.Error409Conflict(
					"The pagination cursor is not valid. Restart from the first page without a cursor.")
			}
			if c.Revision != current.Revision || c.BuildID != current.BuildID {
				return nil, huma.Error409Conflict(
					"The pagination cursor was issued against a different catalog revision and can no longer be honored. Restart from the first page without a cursor.")
			}
			offset = c.Offset
		}

		limit := in.Limit
		if limit <= 0 {
			limit = defaultLimit
		}
		if limit > maxLimit {
			limit = maxLimit
		}

		// Order by slug so pagination is deterministic across identical
		// requests, a requirement for stable cursor offsets.
		vendors := make([]catalog.Vendor, len(current.Vendors))
		copy(vendors, current.Vendors)
		sort.Slice(vendors, func(i, j int) bool { return vendors[i].Slug < vendors[j].Slug })

		total := len(vendors)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := vendors[offset:end]

		resp := &ListVendorsResponse{}
		resp.Body.Vendors = make([]Vendor, 0, len(page))
		for _, v := range page {
			resp.Body.Vendors = append(resp.Body.Vendors, Vendor{
				Slug:        v.Slug,
				DisplayName: v.DisplayName,
				Website:     v.Website,
			})
		}
		resp.Body.Revision = current.Revision
		if end < total {
			next := encodeCursor(pageCursor{
				Revision: current.Revision,
				BuildID:  current.BuildID,
				Offset:   end,
			})
			resp.Body.NextCursor = &next
		}
		return resp, nil
	})
}

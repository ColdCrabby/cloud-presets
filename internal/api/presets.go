package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// MatchRange is a [start, end) pair of UTF-16 code-unit offsets into a field
// value, so the client can highlight exactly what the query matched.
type MatchRange [2]int

// MatchInfo says which field matched the query and where.
type MatchInfo struct {
	Field  string       `json:"field" doc:"Name of the field the query matched."`
	Ranges []MatchRange `json:"ranges" doc:"Offsets within the field value that matched."`
}

// PresetSummary is enough to render a result row without shipping every
// slicing parameter. It mirrors the SearchResponse contract the frontends
// consume via the generated client.
type PresetSummary struct {
	ID       string     `json:"id" doc:"Stable identifier of the preset."`
	Type     string     `json:"type" enum:"printer,filament,process" doc:"Kind of profile the preset describes."`
	Name     string     `json:"name" doc:"Human-readable preset name."`
	Vendor   string     `json:"vendor" doc:"Vendor slug the preset belongs to."`
	Model    *string    `json:"model,omitempty" doc:"Printer model, when applicable."`
	Material *string    `json:"material,omitempty" doc:"Filament material, when applicable."`
	Spec     string     `json:"spec" doc:"Short human-readable spec string."`
	Match    *MatchInfo `json:"match,omitempty" doc:"Where the query matched, for highlighting."`
}

// SearchInput carries the query-string filters for GET /v1/presets.
type SearchInput struct {
	Q        string `query:"q" doc:"Fuzzy query across name, vendor, model, material. Omit to browse."`
	Type     string `query:"type" enum:"printer,filament,process" doc:"Restrict to one preset type."`
	Vendor   string `query:"vendor" doc:"Restrict to one vendor slug."`
	Material string `query:"material" doc:"Restrict to one filament material."`
	Limit    int    `query:"limit" minimum:"1" maximum:"100" default:"20" doc:"Maximum results to return."`
	Cursor   string `query:"cursor" doc:"Opaque continuation token from a previous page."`
}

// SearchResponse is a page of preset summaries plus the catalog revision it was
// served from.
type SearchResponse struct {
	Body struct {
		Results    []PresetSummary `json:"results" doc:"The page of matching preset summaries."`
		NextCursor *string         `json:"next_cursor,omitempty" doc:"Continuation token; absent on the last page."`
		Revision   string          `json:"revision" doc:"Catalog revision the page was served from."`
	}
}

// registerPresets wires GET /v1/presets.
//
// Until an ingest has published a catalog, there is nothing to search: the
// handler returns a 503 problem document making the not-ready state explicit,
// rather than a bare 404 that reads like a routing bug. Once a catalog is
// loaded it returns a well-formed (possibly empty) page so the client can
// render a real "no results" state instead of an error.
func registerPresets(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "searchPresets",
		Method:      http.MethodGet,
		Path:        BasePath + "/presets",
		Summary:     "Search presets",
		Description: "Searches the in-memory catalog of presets ingested from " +
			"ColdCrabby/presets. Returns a page of summaries for rendering result rows.",
		Tags: []string{"Presets"},
	}, func(_ context.Context, _ *SearchInput) (*SearchResponse, error) {
		current := holder.Current()
		if current == nil {
			return nil, huma.Error503ServiceUnavailable(
				"The preset catalog is not loaded yet, so search is temporarily unavailable. Please try again shortly.")
		}
		resp := &SearchResponse{}
		resp.Body.Results = []PresetSummary{}
		resp.Body.Revision = current.Revision
		return resp, nil
	})
}

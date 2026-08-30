package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/ColdCrabby/cloud-presets/internal/search"
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
//
// Limit has no default in the schema; an omitted or zero limit is bounded
// server-side (see defaultLimit), matching the hand-written contract where page
// size is "bounded server-side" rather than a fixed advertised default.
type SearchInput struct {
	Q        string `query:"q" doc:"Fuzzy query across name, vendor, model, material. Omit to browse."`
	Type     string `query:"type" enum:"printer,filament,process" doc:"Restrict to one preset type."`
	Vendor   string `query:"vendor" doc:"Restrict to one vendor slug."`
	Material string `query:"material" doc:"Restrict to one filament material."`
	Limit    int    `query:"limit" minimum:"1" maximum:"100" doc:"Maximum results to return (bounded server-side)."`
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

// defaultLimit is the page size used when the client omits limit. maxLimit is
// the server-side ceiling enforced regardless of what the client asks for.
const (
	defaultLimit = 20
	maxLimit     = 100
)

// pageCursor is the opaque continuation token, base64url(JSON). It is bound to
// the revision and build identity it was issued against so it cannot be honored
// after the catalog changes: only the current catalog is retained, so an old
// cursor would paginate across two different catalogs. Presenting a stale (or
// malformed) cursor fails loudly with 409 rather than silently.
type pageCursor struct {
	Revision string `json:"r"`
	BuildID  string `json:"b"`
	Offset   int    `json:"o"`
}

func encodeCursor(c pageCursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeCursor(s string) (pageCursor, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, false
	}
	var c pageCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return pageCursor{}, false
	}
	if c.Offset < 0 {
		return pageCursor{}, false
	}
	return c, true
}

// registerPresets wires GET /v1/presets.
//
// Until an ingest has published a catalog, there is nothing to search: the
// handler returns a 503 problem document making the not-ready state explicit,
// rather than a bare 404 that reads like a routing bug. Once a catalog is
// loaded it filters and fuzzy-ranks the in-memory index, returning a
// cursor-paginated page of summaries (with match positions for highlighting)
// so the client can render real results or a genuine "no results" state.
func registerPresets(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "searchPresets",
		Method:      http.MethodGet,
		Path:        BasePath + "/presets",
		Summary:     "Search or browse the preset catalog",
		Description: "Searches the in-memory catalog of presets ingested from " +
			"ColdCrabby/presets. Returns a page of summaries for rendering result rows.",
		Tags: []string{"public"},
		// 409: a stale or malformed pagination cursor. 503: catalog not loaded
		// yet. Huma expands these into explicit problem+json responses.
		Errors: []int{http.StatusConflict, http.StatusServiceUnavailable},
	}, func(_ context.Context, in *SearchInput) (*SearchResponse, error) {
		current := holder.Current()
		if current == nil {
			return nil, huma.Error503ServiceUnavailable(
				"The preset catalog is not loaded yet, so search is temporarily unavailable. Please try again shortly.")
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

		results := search.Search(current.Records, search.Query{
			Text:     in.Q,
			Type:     in.Type,
			Vendor:   in.Vendor,
			Material: in.Material,
		})

		total := len(results)
		if offset > total {
			offset = total
		}
		end := offset + limit
		if end > total {
			end = total
		}
		page := results[offset:end]

		resp := &SearchResponse{}
		resp.Body.Results = make([]PresetSummary, 0, len(page))
		for _, r := range page {
			resp.Body.Results = append(resp.Body.Results, toSummary(r))
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

// toSummary maps a search result to the wire summary, rendering absent model
// and material as JSON null and carrying match positions when present.
func toSummary(r search.Result) PresetSummary {
	s := PresetSummary{
		ID:     r.Record.ID,
		Type:   r.Record.Type,
		Name:   r.Record.Name,
		Vendor: r.Record.Vendor,
		Spec:   r.Record.Spec,
	}
	if r.Record.Model != "" {
		model := r.Record.Model
		s.Model = &model
	}
	if r.Record.Material != "" {
		material := r.Record.Material
		s.Material = &material
	}
	if r.Match != nil {
		ranges := make([]MatchRange, len(r.Match.Ranges))
		for i, rg := range r.Match.Ranges {
			ranges[i] = MatchRange{rg[0], rg[1]}
		}
		s.Match = &MatchInfo{Field: r.Match.Field, Ranges: ranges}
	}
	return s
}

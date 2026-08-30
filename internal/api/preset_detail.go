package api

import (
	"context"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// PresetDetailInput carries the path id for GET /v1/presets/{id}.
//
// scheme and host are not request parameters; they are captured in Resolve from
// the matched request so the handler can build an absolute import_url that
// points back at this same server without a hard-coded base URL.
type PresetDetailInput struct {
	ID string `path:"id" doc:"Stable identifier of the preset to fetch."`

	scheme string
	host   string
}

// Resolve records the request scheme and host so the handler can render an
// absolute canonical URL for the preset. It never rejects the request.
func (in *PresetDetailInput) Resolve(ctx huma.Context) []error {
	in.host = ctx.Host()
	in.scheme = "http"
	if ctx.TLS() != nil {
		in.scheme = "https"
	}
	// Honour a reverse proxy's forwarded scheme so import_url stays https when
	// TLS terminates upstream.
	if proto := ctx.Header("X-Forwarded-Proto"); proto != "" {
		in.scheme = proto
	}
	return nil
}

// PresetResponse is the complete preset in the slicer's profile shape. It is the
// exact JSON the slicer consumes directly: identity fields, source set to
// "catalog", the canonical import_url, and the sparse params bag.
type PresetResponse struct {
	Body struct {
		ID        string         `json:"id" doc:"Stable identifier of the preset."`
		Type      string         `json:"type" enum:"printer,filament,process" doc:"Kind of profile the preset describes."`
		Name      string         `json:"name" doc:"Human-readable preset name."`
		Vendor    string         `json:"vendor" doc:"Vendor slug the preset belongs to."`
		Source    string         `json:"source" enum:"catalog" doc:"Where the preset came from; always \"catalog\" when served here."`
		ImportURL string         `json:"import_url" format:"uri" doc:"Canonical URL of this preset, for re-import into the slicer."`
		Params    map[string]any `json:"params" doc:"The preset's slicing parameters in the slicer's own field names and units."`
	}
}

// registerPresetDetail wires GET /v1/presets/{id}.
//
// Search returns summaries only, so this is how a client pulls the full slicing
// parameters for one preset to import into the slicer — instead of downloading
// the whole catalog and filtering locally. Before the first ingest there is no
// catalog (503); a well-formed id that names no preset is a 404 so a client can
// tell "unknown" from "not ready". See docs/api-surface.md ("Detail").
func registerPresetDetail(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "getPreset",
		Method:      http.MethodGet,
		Path:        BasePath + "/presets/{id}",
		Summary:     "Fetch a complete preset in the slicer's profile shape",
		Description: "Returns the complete preset identified by id, with source " +
			"\"catalog\" and a canonical import_url. This is the exact JSON the " +
			"slicer can consume directly, so a client imports one preset without " +
			"downloading the whole catalog.",
		Tags:   []string{"public"},
		Errors: []int{http.StatusNotFound, http.StatusServiceUnavailable},
	}, func(_ context.Context, in *PresetDetailInput) (*PresetResponse, error) {
		current := holder.Current()
		if current == nil {
			return nil, huma.Error503ServiceUnavailable(
				"The preset catalog is not loaded yet, so this preset is temporarily unavailable. Please try again shortly.")
		}

		p, ok := current.Preset(in.ID)
		if !ok {
			return nil, huma.Error404NotFound(
				"No preset matches the id " + in.ID + " in the current catalog.")
		}

		resp := &PresetResponse{}
		resp.Body.ID = p.ID
		resp.Body.Type = p.Type
		resp.Body.Name = p.Name
		resp.Body.Vendor = p.Vendor
		resp.Body.Source = "catalog"
		resp.Body.ImportURL = importURL(in.scheme, in.host, p.ID)
		resp.Body.Params = p.Params
		if resp.Body.Params == nil {
			resp.Body.Params = map[string]any{}
		}
		return resp, nil
	})
}

// importURL builds the canonical URL for a preset. With a known host it is an
// absolute URL back to this server's detail endpoint; without one (e.g. a test
// harness that sets no host) it falls back to a path-absolute reference so the
// field is always populated and resolvable against the API origin.
func importURL(scheme, host, id string) string {
	path := BasePath + "/presets/" + url.PathEscape(id)
	if host == "" {
		return path
	}
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + host + path
}

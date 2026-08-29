// Package api wires the Huma v2 operations and their request/response DTOs.
//
// Huma generates the OpenAPI 3.1 document from these Go types, so the spec
// cannot drift from the handlers, and it renders errors as RFC 9457
// application/problem+json out of the box. See docs/api-surface.md.
package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// BasePath is the version prefix every endpoint lives under. It is the API
// version, independent of the preset schema version. See
// docs/api-surface.md ("Versioning and Base Path").
const BasePath = "/v1"

// New builds the API on a stdlib http.ServeMux via the humago adapter and
// returns both the Huma API (for OpenAPI export) and the HTTP handler to
// serve. The handler wraps the mux so unmatched routes and disallowed methods
// also render as RFC 9457 application/problem+json.
func New(holder *catalog.Holder) (huma.API, http.Handler) {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, Config())
	Register(humaAPI, holder)
	return humaAPI, &problemRouter{mux: mux}
}

// Register attaches every operation to api, reading served state from holder.
//
// It is the single place operations are wired, so both the running server and
// the OpenAPI export command produce an identical spec.
func Register(api huma.API, holder *catalog.Holder) {
	registerHealth(api, holder)
}

// Config returns the Huma config used for both the server and the OpenAPI
// export, so the generated document matches what the server serves.
func Config() huma.Config {
	cfg := huma.DefaultConfig("Cold Crabby Preset Cloud", "1.0.0")
	cfg.OpenAPI.Info.Description = "In-memory, searchable catalog of 3D printer presets " +
		"served from ColdCrabby/presets. See docs/api-surface.md."
	return cfg
}

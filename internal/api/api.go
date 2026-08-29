// Package api wires the Huma v2 application onto the stdlib http.ServeMux via
// the humago adapter and registers the HTTP operations every other API issue
// builds on. This skeleton provides the operational health endpoint and the
// shared configuration that the OpenAPI export consumes.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
)

// Title and Version identify the API in the generated OpenAPI document.
const (
	Title   = "Cold Crabby Preset Cloud"
	Version = "0.1.0"
)

// Config describes the shared OpenAPI configuration. Errors already render as
// RFC 9457 application/problem+json because that is Huma's default error model.
func Config() huma.Config {
	config := huma.DefaultConfig(Title, Version)
	config.Info.Description = "In-memory, searchable catalog of Cold Crabby presets served from ColdCrabby/presets. No database; every served structure is derived from one Git commit."
	return config
}

// New builds a fresh http.ServeMux, mounts the Huma API on it, and registers
// every operation against the given catalog store. It returns both the mux (to
// serve) and the huma.API (to export the OpenAPI document).
func New(store *catalog.Store) (*http.ServeMux, huma.API) {
	mux := http.NewServeMux()
	humaAPI := humago.New(mux, Config())
	register(humaAPI, store)
	// Render routing misses as RFC 9457 problem+json too, so unknown paths look
	// like API errors rather than the stdlib mux's plain-text default.
	mux.HandleFunc("/", notFound)
	return mux, humaAPI
}

func notFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(&huma.ErrorModel{
		Title:  http.StatusText(http.StatusNotFound),
		Status: http.StatusNotFound,
		Detail: "No operation is registered for " + r.Method + " " + r.URL.Path + ".",
	})
}

func register(humaAPI huma.API, store *catalog.Store) {
	registerHealth(humaAPI, store)
}

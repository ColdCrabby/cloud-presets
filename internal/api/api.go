// Package api wires the Huma v2 operations and their request/response DTOs.
//
// Huma generates the OpenAPI 3.1 document from these Go types, so the spec
// cannot drift from the handlers, and it renders errors as RFC 9457
// application/problem+json out of the box. See docs/api-surface.md.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/ColdCrabby/cloud-presets/internal/auth"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/ColdCrabby/cloud-presets/internal/preset"
	"github.com/ColdCrabby/cloud-presets/internal/submit"
	"github.com/ColdCrabby/cloud-presets/internal/upload"
)

// BasePath is the version prefix every endpoint lives under. It is the API
// version, independent of the preset schema version. See
// docs/api-surface.md ("Versioning and Base Path").
const BasePath = "/v1"

// Option configures optional API dependencies passed to New. They are wired
// through options so the OpenAPI export and tests can build the API without a
// preset validator, upload store, or GitHub submitter.
type Option func(*uploadDeps)

// WithUploads enables the manual upload and claim endpoints, backed by the given
// validator and draft store. submitter may be nil, in which case claiming
// resolves and validates the change set but does not open a pull request.
func WithUploads(v *preset.Validator, store *upload.Store, submitter submit.Submitter) Option {
	return func(d *uploadDeps) {
		d.validator = v
		d.store = store
		d.submitter = submitter
	}
}

// New builds the API on a stdlib http.ServeMux via the humago adapter and
// returns both the Huma API (for OpenAPI export) and the HTTP handler to
// serve. The handler wraps the mux so unmatched routes and disallowed methods
// also render as RFC 9457 application/problem+json.
//
// When mw is non-nil, the caller-identity endpoint GET /v1/me is registered
// behind it; when nil (e.g. the OpenAPI export), auth-gated routes are omitted.
// The upload/claim endpoints are registered only when WithUploads supplies a
// draft store.
func New(holder *catalog.Holder, mw *auth.Middleware, opts ...Option) (huma.API, http.Handler) {
	var deps uploadDeps
	for _, o := range opts {
		o(&deps)
	}

	mux := http.NewServeMux()
	humaAPI := humago.New(mux, Config())
	Register(humaAPI, holder)
	if mw != nil {
		mux.Handle("GET "+BasePath+"/me", mw.RequireAuth(http.HandlerFunc(handleMe)))
	}
	registerUploads(mux, mw, deps)
	return humaAPI, &problemRouter{mux: mux}
}

// handleMe returns the validated caller's identity. It runs only behind the
// auth middleware, so claims are always present.
func handleMe(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"memberId":         claims.Subject,
		"organizationId":   claims.OrganizationID,
		"organizationSlug": claims.OrganizationSlug,
		"roles":            claims.Roles,
	})
}

// Register attaches every operation to api, reading served state from holder.
//
// It is the single place operations are wired, so both the running server and
// the OpenAPI export command produce an identical spec.
func Register(api huma.API, holder *catalog.Holder) {
	registerHealth(api, holder)
	registerPresets(api, holder)
	registerVendors(api, holder)
}

// Config returns the Huma config used for both the server and the OpenAPI
// export, so the generated document matches what the server serves.
func Config() huma.Config {
	cfg := huma.DefaultConfig("Cold Crabby Preset Cloud", "1.0.0")
	cfg.OpenAPI.Info.Description = "In-memory, searchable catalog of 3D printer presets " +
		"served from ColdCrabby/presets. See docs/api-surface.md."
	// The whole API is mounted under BasePath ("/v1") by the server, so Huma's
	// auto-served meta paths must live there too. At their defaults (site root)
	// the schema URLs embedded in every response ($schema and the describedBy
	// Link header) point outside the mount and resolve to the SPA, so they are
	// unreachable.
	cfg.SchemasPath = BasePath + cfg.SchemasPath
	cfg.OpenAPIPath = BasePath + cfg.OpenAPIPath
	cfg.DocsPath = BasePath + cfg.DocsPath
	return cfg
}

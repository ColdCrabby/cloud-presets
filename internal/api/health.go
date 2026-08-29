package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

// HealthResponse is the body returned by GET /v1/health.
//
// Liveness answers "is the process running"; readiness answers "has a catalog
// been loaded". Revision and LastIngestAt expose staleness so a stalled
// catalog is visible to monitoring rather than silent. Revision is null until
// the first successful ingest. See docs/api-surface.md ("Operational
// Endpoints").
type HealthResponse struct {
	Body struct {
		Status       string     `json:"status" example:"ok" doc:"Liveness marker; always \"ok\" when the process can respond."`
		Ready        bool       `json:"ready" doc:"True once a catalog has been loaded. False until the first successful ingest."`
		Revision     *string    `json:"revision" doc:"Git commit SHA of the served catalog, or null until the first ingest."`
		LastIngestAt *time.Time `json:"lastIngestAt" doc:"Time of the last successful ingest, or null until the first ingest."`
	}
}

// registerHealth wires GET /v1/health.
func registerHealth(api huma.API, holder *catalog.Holder) {
	huma.Register(api, huma.Operation{
		OperationID: "getHealth",
		Method:      http.MethodGet,
		Path:        BasePath + "/health",
		Summary:     "Liveness and readiness",
		Description: "Reports process liveness, catalog readiness, the served catalog " +
			"revision, and the time of the last successful ingest.",
		Tags: []string{"Operations"},
	}, func(_ context.Context, _ *struct{}) (*HealthResponse, error) {
		resp := &HealthResponse{}
		resp.Body.Status = "ok"
		resp.Body.Ready = holder.Ready()
		resp.Body.Revision = holder.Revision()
		resp.Body.LastIngestAt = holder.LastIngestAt()
		return resp, nil
	})
}

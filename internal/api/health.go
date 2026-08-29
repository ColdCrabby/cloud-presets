package api

import (
	"context"
	"net/http"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	"github.com/danielgtaylor/huma/v2"
)

// HealthOutput is the body of GET /v1/health. It reports liveness (the process
// is up and answering) and readiness (a catalog has been loaded), along with
// the served catalog revision and the time of the last successful ingest, so a
// stalled catalog is visible to monitoring rather than silent.
type HealthOutput struct {
	Body HealthBody
}

// HealthBody is the JSON payload of the health response.
type HealthBody struct {
	// Status is "ok" whenever the process can answer requests.
	Status string `json:"status" example:"ok" doc:"Liveness marker; always \"ok\" when the process is answering."`
	// Ready is false until the first successful ingest loads a catalog.
	Ready bool `json:"ready" example:"false" doc:"Whether a catalog has been loaded and can be served."`
	// Revision is the Git commit SHA of the served catalog, or null until the
	// first ingest completes.
	Revision *string `json:"revision" doc:"Git commit SHA of the served catalog, or null until the first ingest."`
	// LastIngest is the time of the last successful ingest, or null until the
	// first ingest completes.
	LastIngest *time.Time `json:"lastIngest" doc:"Time of the last successful ingest, or null until the first ingest."`
}

func registerHealth(humaAPI huma.API, store *catalog.Store) {
	huma.Register(humaAPI, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/v1/health",
		Summary:     "Liveness and readiness",
		Description: "Reports liveness plus readiness and the served catalog revision. Readiness is false until the first successful ingest loads a catalog.",
		Tags:        []string{"Operations"},
	}, func(_ context.Context, _ *struct{}) (*HealthOutput, error) {
		out := &HealthOutput{Body: HealthBody{
			Status: "ok",
			Ready:  store.Ready(),
		}}
		if state := store.State(); state != nil {
			revision := state.Revision
			lastIngest := state.LastIngest
			out.Body.Revision = &revision
			out.Body.LastIngest = &lastIngest
		}
		return out, nil
	})
}

// Command server runs the Cold Crabby preset cloud HTTP API.
//
// It serves the Huma v2 API on the standard library http.ServeMux via the
// humago adapter — no router dependency. The catalog starts empty, so the
// server reports not ready until ingest (a later foundation-wave issue)
// publishes the first revision.
package main

import (
	"log"
	"net/http"
	"os"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	holder := catalog.NewHolder()
	_, handler := api.New(holder)

	log.Printf("preset cloud API listening on %s (catalog not ready until first ingest)", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

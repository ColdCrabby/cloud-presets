// Command api runs the Cold Crabby preset cloud HTTP server. It serves the Huma
// v2 API on the stdlib http.ServeMux via the humago adapter.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	store := catalog.New()
	mux, _ := api.New(store)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("starting preset cloud api", "addr", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

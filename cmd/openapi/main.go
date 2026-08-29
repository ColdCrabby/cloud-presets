// Command openapi writes the OpenAPI 3.1 document for the preset cloud API to a
// file. The Angular client generator consumes this document, so it is exported
// at build time rather than fetched from a running server.
package main

import (
	"log/slog"
	"os"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func main() {
	out := "openapi.yaml"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	_, humaAPI := api.New(catalog.New())

	doc, err := humaAPI.OpenAPI().YAML()
	if err != nil {
		slog.Error("failed to render openapi document", "error", err)
		os.Exit(1)
	}

	if err := os.WriteFile(out, doc, 0o644); err != nil {
		slog.Error("failed to write openapi document", "path", out, "error", err)
		os.Exit(1)
	}

	slog.Info("wrote openapi document", "path", out)
}

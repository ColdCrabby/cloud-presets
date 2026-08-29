// Command openapi writes the OpenAPI 3.1 document that describes the API to a
// file, without starting a server. The Angular client generator in the
// frontend track consumes this document, so exporting it is a build step.
//
// Usage:
//
//	go run ./cmd/openapi [output-path]
//
// The default output path is "openapi.yaml". Huma emits OpenAPI 3.1; no 3.0
// downgrade is performed here. See docs/api-surface.md ("Client Generation").
package main

import (
	"log"
	"os"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
)

func main() {
	out := "openapi.yaml"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// Build the same API the server serves so the exported spec matches it.
	humaAPI, _ := api.New(catalog.NewHolder(), nil)

	doc, err := humaAPI.OpenAPI().YAML()
	if err != nil {
		log.Fatalf("marshal OpenAPI document: %v", err)
	}

	if err := os.WriteFile(out, doc, 0o644); err != nil {
		log.Fatalf("write %s: %v", out, err)
	}

	log.Printf("wrote OpenAPI 3.1 document to %s", out)
}

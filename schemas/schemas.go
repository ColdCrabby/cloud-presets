// Package schemas holds the JSON Schemas vendored from the slicer engine.
//
// These files are a pinned, reviewable copy of the schemas the slicer generates
// from its Rust profile types (see README.md for the exact source commit). They
// are the single contract a preset is validated against; there is no
// second, cloud-side description of a preset to keep in sync.
//
// The copy is vendored deliberately rather than pulled as a submodule so that a
// schema bump is a visible, explicit pull request.
package schemas

import "embed"

// FS embeds the vendored slicer profile schemas.
//
//go:embed *.json
var FS embed.FS

// Profile schema file names, keyed by preset kind.
const (
	PrinterSchemaFile  = "slicer-engine-printer-profile-v1.json"
	FilamentSchemaFile = "slicer-engine-filament-profile-v1.json"
	ProcessSchemaFile  = "slicer-engine-process-profile-v1.json"
)

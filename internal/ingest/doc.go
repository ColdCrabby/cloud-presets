// Package ingest fetches ColdCrabby/presets, parses and validates every preset
// file against the vendored schemas, and publishes a new catalog revision via
// an atomic swap. Triggers (startup, webhook, poll, manual) are hints only;
// each resolves the current main and builds that. Implemented in a later
// foundation-wave issue. See ARCHITECTURE.md ("Ingest & Catalog Rebuild").
package ingest

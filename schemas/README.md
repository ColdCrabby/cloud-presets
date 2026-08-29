# Vendored slicer schemas

These are a **pinned, vendored copy** of the JSON Schemas the slicer generates
from its Rust profile types. They are the contract every preset is validated
against. See [`docs/preset-schema.md`](../docs/preset-schema.md) for how they
are used and the strictness layered on top of them.

| File | Slicer type | `schema_id` |
| --- | --- | --- |
| `slicer-engine-printer-profile-v1.json`  | `PrinterProfile`  | `slicer-engine/printer-profile-v1`  |
| `slicer-engine-filament-profile-v1.json` | `FilamentProfile` | `slicer-engine/filament-profile-v1` |
| `slicer-engine-process-profile-v1.json`  | `ProcessProfile`  | `slicer-engine/process-profile-v1`  |

## Provenance

- **Source repository:** [`ColdCrabby/slicer`](https://github.com/ColdCrabby/slicer)
- **Source commit:** `fd4ea9efbe54d9224aa5d4742c8c1782862efb64`
- **Generated with:** `cargo run --release -- gen-schemas --output-dir <dir>`

The schemas are JSON Schema **draft 2020-12** documents and each file is
self-contained (all `$ref`s resolve within its own `$defs`).

## Updating

Bumping this copy is an **explicit pull request**, never automatic. Regenerate
with the slicer's `gen-schemas` command at the new commit, replace the three
files, update the commit hash above, and re-run this repository's validator
tests. A schema bump is a deliberate, visible act because it can change what
counts as a valid preset.

> **Do not hand-edit these files.** They are generated output. Any cloud-side
> strictness the raw schemas lack (unknown-field rejection, semantic ranges,
> per-type parameter allowlists) lives in the Go validator, not here.

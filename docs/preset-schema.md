# Canonical Preset Schema

The preset format served by the cloud and authored in
[`ColdCrabby/presets`](https://github.com/ColdCrabby/presets).

Presets are **flat YAML**. Every file is self-contained: there is no `inherits:`
key, no base presets, and no resolution step. What you read in the file is what
the slicer receives.

## Table of Contents

1. [Why Flat](#why-flat)
2. [Where the Schema Comes From](#where-the-schema-comes-from)
3. [Preset Types](#preset-types)
4. [Authored vs. Cloud-Managed Fields](#authored-vs-cloud-managed-fields)
5. [The `params` Bag](#the-params-bag)
6. [Printer Presets](#printer-presets)
7. [Filament Presets](#filament-presets)
8. [Process Presets](#process-presets)
9. [Validation Rules](#validation-rules)
10. [Schema Versioning](#schema-versioning)

---

## Why Flat

Inheritance is the obvious way to remove duplication in a preset catalog — one
printer definition, three nozzle variants deriving from it. It was considered and
rejected.

The review model for this project is **reading a diff on GitHub**. With
inheritance, a one-line change to a base preset silently alters every descendant,
and the diff shows none of the affected output. A reviewer would have to resolve
the graph mentally to know what a PR actually changes. Flat files cost
duplication and buy reviewability, and reviewability is what makes vendor
ownership and community contribution work.

It also keeps the cloud honest: with no resolution step, validating a file is a
pure function of that file.

The duplication is real but bounded — a vendor with three nozzle variants
maintains three files. Tooling can generate them; the repository stores the
result.

---

## Where the Schema Comes From

Preset structure is **not defined by this project**. It is defined by the slicer,
which generates JSON Schemas from its Rust profile types:

```
ui/src/schemas/slicer-engine-printer-profile-v1.json
ui/src/schemas/slicer-engine-filament-profile-v1.json
ui/src/schemas/slicer-engine-process-profile-v1.json
```

These are JSON Schema **draft 2020-12** documents. They are exhaustive — the
shared `SlicingParams` definition carries 92 typed properties with types, units,
ranges, and defaults.

The cloud **vendors these files** into `schemas/` and validates against them
directly. There is no second, cloud-side description of a preset to keep in sync.
A preset that would fail to load in the slicer fails validation here first.

---

## Preset Types

Three types, matching the slicer's three profile categories:

| Type | Describes | Required fields |
| --- | --- | --- |
| **printer** | A machine: bed, kinematics, hardware limits | `id`, `name`, `vendor`, `model` |
| **filament** | A material: temperatures, cooling, flow | `id`, `name`, `vendor`, `material`, `color`, `density_g_cm3`, `cost_per_kg` |
| **process** | A quality profile: layer height, speeds, walls | `id`, `name` |

These three compose at slice time in the slicer — printer, then filament, then
process, then the user's own overrides, with later stages winning on shared keys.
**The cloud does not perform this composition.** It serves the three categories
independently; composition is the slicer's job.

---

## Authored vs. Cloud-Managed Fields

The slicer's profile schema covers both catalog presets and a user's own local
profiles, so some fields in it are meaningless in a catalog file.

**Authored in YAML** — the domain fields for the type, plus `params`, plus
`schema_version`.

**Never authored; injected by the API on output:**

| Field | Value | Reason |
| --- | --- | --- |
| `source` | `catalog` | Marks the entry read-only in the slicer UI. Authoring it invites drift. |
| `import_url` | canonical API URL for this preset | Provenance. Only the server knows its own public URL. |

**Authored, but stripped before serving:**

| Field | Reason |
| --- | --- |
| `schema_version` | Describes the *file*, not the profile. See [Schema Versioning](#schema-versioning). |

**Never authored; never served** — these are local-only concepts:

| Field | Reason |
| --- | --- |
| `label_ids` | User's private tagging vocabulary. |
| `connection` | A printer's network address is personal, not a catalog property. |
| `based_on` | Records that a *user* forked a catalog entry. A catalog entry is not forked from anything. |

Including any of these in a preset file is a **validation error**, not a warning.
Silently ignoring them would let an author believe they had set something that
has no effect.

---

## The `params` Bag

Every type carries a `params` object holding that preset's **sparse** slicing
overrides — only the keys it actually sets. Omitted keys fall back to the
slicer's defaults, and each type is expected to contribute a different slice of
the parameter space:

- **printer** — hardware: `nozzle_diameter_mm`, `filament_diameter_mm`,
  `gcode_flavor`, `start_gcode`, `end_gcode`, `retract_mm`, `z_hop_mm`,
  `travel_speed_mm_min`, `extruder_count`
- **filament** — material: `nozzle_temp`, `bed_temp`, `fan_speed`, `flow_ratio`,
  `pressure_advance`, `max_volumetric_speed`
- **process** — quality: `layer_height`, `wall_count`, `infill_density`,
  `infill_pattern`, `seam_position`, `print_speed`, `adhesion_type`

**These groupings are enforced, not advisory.** The schema shares one
`SlicingParams` definition across all three types, so nothing in it stops a
process preset from setting `gcode_flavor` or a printer preset from setting
`infill_density`. Since composition lets later stages win, a stray key in the
wrong category would silently override the category that legitimately owns it.
The cloud therefore validates each type against an allowlist of the parameters it
may contribute, and rejects the rest.

### Flat does not mean frozen

"Self-contained" means no preset depends on *another preset*. It does not mean a
preset fully determines print behaviour: omitted keys resolve to the slicer's
built-in defaults, so a default that changes between slicer versions changes the
effective result with no diff in the presets repository.

This is the accepted cost of sparse presets — the alternative, materialising all
92 parameters into every file, would make diffs unreadable and couple every
preset to every engine default change. Vendors who need a parameter pinned should
set it explicitly rather than relying on the default staying put.

Field names and units are the slicer's own — **no translation layer**. Two units
routinely surprise authors:

- **Fan speeds are fractions `0.0`–`1.0`**, not percentages. `fan_speed: 100`
  means one hundred times full speed.
- **Some speeds are mm/min, not mm/s** — note the `_mm_min` suffix on
  `travel_speed_mm_min` (default `9000`) and `retract_speed_mm_min` (default
  `2400`), while `print_speed` and `perimeter_speed` are mm/s.

Note that most numeric parameters declare a type and a default but **no
`minimum`/`maximum`**. Raw schema validation would therefore accept
`fan_speed: 100` as a valid number. The cloud adds semantic range checks on top
for the bounded quantities — fractions, temperatures, densities — because these
mistakes are silent: the file is well-formed, and the failure shows up as ruined
prints rather than an error.

The schema is the authority for every parameter's type, range, and default. Each
parameter also carries an `x-group` annotation, which the vendor admin uses to
group fields in its editor.

---

## Printer Presets

```yaml
schema_version: 1

id: prusa-mk4-0.4
name: Prusa MK4 — 0.4 mm nozzle
vendor: Prusa
model: MK4

bed_shape: rectangular
bed_width: 250
bed_depth: 210
bed_height: 220
origin_at_center: false

params:
  nozzle_diameter_mm: 0.4
  filament_diameter_mm: 1.75
  extruder_count: 1
  gcode_flavor: marlin
  retract_mm: 0.8
  retract_speed_mm_min: 2400
  z_hop_mm: 0.2
  travel_speed_mm_min: 15000
  start_gcode: |
    G21 ; millimetres
    G90 ; absolute positioning
    G28 ; home all axes
  end_gcode: |
    M104 S0 ; turn off hotend
    M140 S0 ; turn off bed
    G28 X0  ; home X
    M84     ; disable motors
```

**Field notes**

- `bed_shape` is `rectangular` or `circular`. For `circular`, `bed_width` is the
  **diameter** and `bed_depth` is ignored.
- `origin_at_center` is `true` for delta and other centre-origin machines.
- `gcode_flavor` accepts `marlin` and `klipper`. Other flavors exist in the
  schema but are not first-class in the engine.

---

## Filament Presets

```yaml
schema_version: 1

id: prusament-pla-galaxy-black
name: Prusament PLA Galaxy Black
vendor: Prusament
material: PLA
color: "#1b1b1f"
density_g_cm3: 1.24
cost_per_kg: 29.99

params:
  nozzle_temp: 215
  nozzle_temp_first_layer: 220
  bed_temp: 60
  bed_temp_first_layer: 60
  fan_speed: 1.0
  first_layer_fan_speed: 0.0
  disable_fan_first_layers: 1
  flow_ratio: 1.0
  pressure_advance: 0.04
  max_volumetric_speed: 15
  filament_diameter_mm: 1.75
```

**Field notes**

- `material` is one of `PLA`, `PETG`, `ABS`, `ASA`, `TPU`, `PC`, `Nylon`, `PVA`.
  It is a **material family**, not a product name — the product name belongs in
  `name`.
- `color` is a hex string and must be quoted, or YAML reads `#` as a comment.
- `density_g_cm3` and `cost_per_kg` drive weight and cost estimation, so a wrong
  density produces confidently wrong numbers in the slicer.
- Fan speeds are fractions: `first_layer_fan_speed: 0.0` means off.

---

## Process Presets

```yaml
schema_version: 1

id: coldcrabby-standard-0.20
name: Standard — 0.20 mm
quality: standard

params:
  layer_height: 0.2
  first_layer_height: 0.24
  line_width: 0.44
  wall_generator: arachne
  wall_count: 3
  top_layers: 4
  bottom_layers: 3
  seam_position: aligned
  infill_density: 0.2
  infill_pattern: Gyroid
  infill_base_angle: 45
  print_speed: 120
  perimeter_speed: 80
  infill_speed: 150
  top_surface_speed: 60
  first_layer_speed: 30
  support_threshold_angle: 55
  adhesion_type: skirt
  skirt_loops: 1
```

**Field notes**

- `quality` is `draft`, `standard`, or `fine` — a coarse tag for sorting and
  badges, not a functional setting.
- Process presets have **no `vendor` field** in the slicer schema. Ownership is
  therefore determined by the file's location in the repository. See
  [presets-repo-layout.md](./presets-repo-layout.md).
- `infill_density` is a fraction (`0.2` = 20%), consistent with fan speeds.
- `infill_pattern` is capitalised (`Gyroid`, `Grid`, `Honeycomb`, `Rectilinear`,
  `TpmsD`) while most other enums are lowercase. This is inherited from the
  engine's Rust enum names.

---

## Validation Rules

Validation runs in three places — the vendor admin's dry-run endpoint, CI on
every pull request to `ColdCrabby/presets`, and ingest — and all three run the
**same code with the same pinned schemas**. CI is the fast feedback; ingest is
the guarantee.

The presets repository does not keep its own schema copy; its CI invokes the
validator published by this project. See
[presets-repo-layout.md](./presets-repo-layout.md#one-validator-not-two-copies)
for why two copies would be worse than none.

**Structural**

1. The file validates against the draft 2020-12 schema for its type.
2. **Unknown fields are rejected.** The upstream schemas leave
   `additionalProperties` unset, which JSON Schema treats as permissive; the
   cloud layers strictness on top. A misspelled `layer_hieght` that validated
   silently would ship a preset that ignores the setting its author intended.
3. Values must satisfy the declared type and enum constraints, plus the cloud's
   semantic range checks for bounded quantities (fractions, temperatures,
   densities) that the upstream schemas leave unbounded.
4. Cloud-managed and local-only fields (`source`, `import_url`, `label_ids`,
   `connection`, `based_on`) must be absent.

**Cross-file**

5. `id` is unique across the entire catalog, not just within a type, and does not
   collide with a retired ID held by a tombstone.
6. `id` is lowercase, matching `^[a-z0-9]+(?:[-.][a-z0-9]+)*$` — it appears in
   URLs and must be stable.
7. Presets under `vendors/<slug>/` must resolve to a valid `vendor.yaml`, and for
   printer and filament presets the `vendor` field must match that vendor's name
   or one of its declared brands.
8. Presets under `processes/` are project-owned and must **not** resolve to a
   vendor manifest. Files outside both trees are rejected.

Rules 7 and 8 are separate invariants, not one rule with an exception: process
presets have no `vendor` field in the slicer schema, so requiring vendor
ownership of every preset would reject the entire `processes/` tree. See
[presets-repo-layout.md](./presets-repo-layout.md#ownership-is-path-based).

**Ingest behaviour**

A validation failure **fails the whole ingest**. The catalog is not partially
updated; the previous revision keeps serving. A partially-applied catalog would
be a state that no commit describes, and therefore not reproducible.

---

## Schema Versioning

Every preset declares the schema major it targets:

```yaml
schema_version: 1
```

This is a **cloud-level field**: it describes the file, not the profile, and the
API strips it before serving — exactly like the injected `source` and
`import_url` fields, and for the same reason. Self-describing files mean a
validator never has to guess which schema to apply.

Because older slicer builds keep running indefinitely, introducing `v2` cannot be
a flag day.

**Policy**

- **Additive changes stay in `v1`.** A new optional parameter with a default is
  not a breaking change: older slicers ignore fields they do not know.
- **A new major version is only for breaking changes** — removing a field,
  narrowing a type, or changing units or semantics of an existing field.
- **During a transition, both representations live in Git.** A preset being
  migrated exists as both a `v1` and a `v2` file, and the catalog indexes both.
  The API serves whichever major the client asks for.
- **Where a preset exists only in `v2`, the `v1` API omits it** rather than
  serving a downgraded approximation. An older slicer sees a smaller catalog,
  which is honest; a silently degraded preset would not be.
- **The vendored schema copy is bumped by an explicit PR**, never automatically,
  and the validator digest pinned by the presets repository is bumped in the same
  change.

The reason both representations live in Git rather than being generated on demand
is the no-database constraint: the service rebuilds from exactly one commit and
retains no history, so it cannot reconstruct "the last revision that was still
`v1`-compatible". If a representation must be servable, it has to exist in the
tree. Anything else would be a promise the architecture cannot keep.

Note that projecting between schema majors is a different thing from the "no
translation layer" property described under [The `params` Bag](#the-params-bag).
That property is about field naming — the cloud never renames or re-cases the
slicer's fields. Serving two schema majors side by side does not reintroduce a
mapping layer, because each major is served as authored.

This policy needs to be settled before `v2` is required. Deciding it under the
pressure of a needed breaking change is how compatibility gets broken.

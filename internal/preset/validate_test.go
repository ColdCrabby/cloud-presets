package preset

import (
	"strings"
	"testing"
)

// validPrinter, validFilament and validProcess are the canonical examples from
// docs/preset-schema.md. They must validate cleanly; the trap cases below are
// minimal edits of these.
const validPrinter = `schema_version: 1
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
  start_gcode: "G28"
  end_gcode: "M84"
`

const validFilament = `schema_version: 1
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
`

const validProcess = `schema_version: 1
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
`

func newValidator(t *testing.T) *Validator {
	t.Helper()
	v, err := New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	return v
}

func TestValidate(t *testing.T) {
	v := newValidator(t)

	cases := []struct {
		name      string
		kind      Kind
		filename  string
		yaml      string
		wantValid bool
		// wantErr, if set, are substrings each of which must appear in at least
		// one error's rendered "path: message" string.
		wantErr []string
	}{
		// --- Canonical valid examples ---
		{name: "valid printer", kind: KindPrinter, filename: "prusa-mk4-0.4.yaml", yaml: validPrinter, wantValid: true},
		{name: "valid filament", kind: KindFilament, filename: "prusament-pla-galaxy-black.yaml", yaml: validFilament, wantValid: true},
		{name: "valid process", kind: KindProcess, filename: "coldcrabby-standard-0.20.yaml", yaml: validProcess, wantValid: true},

		// --- The known traps (docs call these out explicitly) ---
		{
			name:      "fan_speed 100 is out of the fraction range",
			kind:      KindFilament,
			yaml:      replace(validFilament, "fan_speed: 1.0", "fan_speed: 100"),
			wantValid: false,
			wantErr:   []string{"params.fan_speed", "fraction (0.0–1.0)"},
		},
		{
			name:      "misspelled layer_hieght is rejected",
			kind:      KindProcess,
			yaml:      replace(validProcess, "layer_height: 0.2", "layer_hieght: 0.2"),
			wantValid: false,
			wantErr:   []string{"params.layer_hieght", "unknown parameter"},
		},
		{
			name:      "process may not set a printer param (gcode_flavor)",
			kind:      KindProcess,
			yaml:      appendParam(validProcess, "gcode_flavor: marlin"),
			wantValid: false,
			wantErr:   []string{"params.gcode_flavor", "not a process parameter"},
		},
		{
			name:      "printer may not set a process param (infill_density)",
			kind:      KindPrinter,
			yaml:      appendParam(validPrinter, "infill_density: 0.2"),
			wantValid: false,
			wantErr:   []string{"params.infill_density", "not a printer parameter"},
		},
		{
			name:      "filament may not set a process param (layer_height)",
			kind:      KindFilament,
			yaml:      appendParam(validFilament, "layer_height: 0.2"),
			wantValid: false,
			wantErr:   []string{"params.layer_height", "not a filament parameter"},
		},
		{
			name:      "filament may not set a printer param (gcode_flavor)",
			kind:      KindFilament,
			yaml:      appendParam(validFilament, "gcode_flavor: marlin"),
			wantValid: false,
			wantErr:   []string{"params.gcode_flavor", "not a filament parameter"},
		},

		// --- Cloud-managed / local-only fields rejected when authored ---
		{name: "forbidden source", kind: KindProcess, yaml: validProcess + "source: catalog\n", wantValid: false, wantErr: []string{"source", "not allowed"}},
		{name: "forbidden import_url", kind: KindProcess, yaml: validProcess + "import_url: https://x\n", wantValid: false, wantErr: []string{"import_url", "not allowed"}},
		{name: "forbidden label_ids", kind: KindProcess, yaml: validProcess + "label_ids: [a]\n", wantValid: false, wantErr: []string{"label_ids", "not allowed"}},
		{name: "forbidden based_on", kind: KindProcess, yaml: validProcess + "based_on: other\n", wantValid: false, wantErr: []string{"based_on", "not allowed"}},
		{name: "forbidden connection", kind: KindPrinter, yaml: validPrinter + "connection: {kind: octoprint}\n", wantValid: false, wantErr: []string{"connection", "not allowed"}},

		// --- schema_version handling ---
		{name: "missing schema_version", kind: KindProcess, yaml: replace(validProcess, "schema_version: 1\n", ""), wantValid: false, wantErr: []string{"schema_version", "required"}},
		{name: "non-integer schema_version", kind: KindProcess, yaml: replace(validProcess, "schema_version: 1", "schema_version: 1.5"), wantValid: false, wantErr: []string{"schema_version", "integer"}},
		{name: "unsupported schema_version", kind: KindProcess, yaml: replace(validProcess, "schema_version: 1", "schema_version: 2"), wantValid: false, wantErr: []string{"schema_version", "unsupported"}},

		// --- ID pattern + file name ---
		{name: "uppercase id rejected", kind: KindProcess, yaml: replace(validProcess, "id: coldcrabby-standard-0.20", "id: ColdCrabby-Standard"), wantValid: false, wantErr: []string{"id", "lowercase"}},
		{name: "id with illegal char", kind: KindProcess, yaml: replace(validProcess, "id: coldcrabby-standard-0.20", "id: bad_id"), wantValid: false, wantErr: []string{"id"}},
		{
			name:      "file name must equal id",
			kind:      KindProcess,
			filename:  "wrong-name.yaml",
			yaml:      validProcess,
			wantValid: false,
			wantErr:   []string{"must equal the preset id"},
		},
		{
			name:      "correct file name passes",
			kind:      KindProcess,
			filename:  "coldcrabby-standard-0.20.yaml",
			yaml:      validProcess,
			wantValid: true,
		},

		// --- Unknown top-level field ---
		{name: "unknown top-level field", kind: KindProcess, yaml: validProcess + "flavour: vanilla\n", wantValid: false, wantErr: []string{"flavour", "unknown field"}},

		// --- Enum + type + required (schema layer) ---
		{name: "bad material enum", kind: KindFilament, yaml: replace(validFilament, "material: PLA", "material: WOOD"), wantValid: false, wantErr: []string{"material", "value must be one of"}},
		{name: "wrong-case infill_pattern", kind: KindProcess, yaml: replace(validProcess, "infill_pattern: Gyroid", "infill_pattern: gyroid"), wantValid: false, wantErr: []string{"infill_pattern"}},
		{name: "missing required vendor on printer", kind: KindPrinter, yaml: replace(validPrinter, "vendor: Prusa\n", ""), wantValid: false, wantErr: []string{"vendor"}},

		// --- Semantic ranges ---
		{name: "nozzle_temp too high", kind: KindFilament, yaml: replace(validFilament, "nozzle_temp: 215", "nozzle_temp: 9000"), wantValid: false, wantErr: []string{"params.nozzle_temp", "temperature"}},
		{name: "density out of range", kind: KindFilament, yaml: replace(validFilament, "density_g_cm3: 1.24", "density_g_cm3: 900"), wantValid: false, wantErr: []string{"density_g_cm3", "density"}},
		{name: "negative cost rejected", kind: KindFilament, yaml: replace(validFilament, "cost_per_kg: 29.99", "cost_per_kg: -5"), wantValid: false, wantErr: []string{"cost_per_kg", "cost per kg"}},
		{name: "fraction boundary 1.0 is allowed", kind: KindFilament, yaml: replace(validFilament, "fan_speed: 1.0", "fan_speed: 1.0"), wantValid: true},

		// --- Structural / parse ---
		{name: "invalid yaml", kind: KindProcess, yaml: "id: [unclosed\n", wantValid: false, wantErr: []string{"invalid YAML"}},
		{name: "duplicate key", kind: KindProcess, yaml: replace(validProcess, "id: coldcrabby-standard-0.20", "id: coldcrabby-standard-0.20\nid: other"), wantValid: false, wantErr: []string{"invalid YAML", "already defined"}},
		{name: "empty file", kind: KindProcess, yaml: "\n", wantValid: false, wantErr: []string{"empty"}},
		{name: "unknown kind", kind: Kind("blender"), yaml: validProcess, wantValid: false, wantErr: []string{"unknown preset kind"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := v.ValidateFile(tc.kind, tc.filename, []byte(tc.yaml))
			if res.Valid() != tc.wantValid {
				t.Fatalf("Valid()=%v, want %v; errors:\n%s", res.Valid(), tc.wantValid, renderErrors(res))
			}
			for _, want := range tc.wantErr {
				if !hasError(res, want) {
					t.Errorf("missing expected error substring %q; got:\n%s", want, renderErrors(res))
				}
			}
		})
	}
}

func TestSanitizeStripsSchemaVersion(t *testing.T) {
	v := newValidator(t)
	res := v.Validate(KindProcess, []byte(validProcess))
	if !res.Valid() {
		t.Fatalf("expected valid, got:\n%s", renderErrors(res))
	}
	if _, ok := res.Sanitized["schema_version"]; ok {
		t.Errorf("sanitized body still contains schema_version: %v", res.Sanitized["schema_version"])
	}
	if res.Sanitized["id"] != "coldcrabby-standard-0.20" {
		t.Errorf("sanitized body lost id: %v", res.Sanitized["id"])
	}
}

func TestDecodeTypedStructs(t *testing.T) {
	v := newValidator(t)

	var printer Printer
	if err := v.Validate(KindPrinter, []byte(validPrinter)).As(&printer); err != nil {
		t.Fatalf("decode printer: %v", err)
	}
	if printer.ID != "prusa-mk4-0.4" || printer.Vendor != "Prusa" || printer.Model != "MK4" {
		t.Errorf("printer decoded wrong: %+v", printer)
	}
	if printer.SchemaVersion != 0 {
		t.Errorf("schema_version should have been stripped, got %d", printer.SchemaVersion)
	}
	if got := printer.Params["gcode_flavor"]; got != "marlin" {
		t.Errorf("printer params.gcode_flavor = %v, want marlin", got)
	}

	var filament Filament
	if err := v.Validate(KindFilament, []byte(validFilament)).As(&filament); err != nil {
		t.Fatalf("decode filament: %v", err)
	}
	if filament.Material != "PLA" || filament.DensityGCm3 != 1.24 {
		t.Errorf("filament decoded wrong: %+v", filament)
	}

	var process Process
	if err := v.Validate(KindProcess, []byte(validProcess)).As(&process); err != nil {
		t.Fatalf("decode process: %v", err)
	}
	if process.Quality != "standard" {
		t.Errorf("process quality = %q, want standard", process.Quality)
	}
}

// TestParamAllowlistsAreSchemaSubsets guards against drift: every param a kind
// is allowed to set must actually exist in the vendored SlicingParams schema,
// and printer/filament allowlists must not overlap (a param owned by one is not
// owned by the other), which keeps the process "everything else" set coherent.
func TestParamAllowlistsAreSchemaSubsets(t *testing.T) {
	v := newValidator(t)
	for key := range printerParams {
		if !v.allParamKeys.has(key) {
			t.Errorf("printer allowlist references unknown param %q", key)
		}
	}
	for key := range filamentParams {
		if !v.allParamKeys.has(key) {
			t.Errorf("filament allowlist references unknown param %q", key)
		}
	}
	// Process must be disjoint from the two hardware/material buckets.
	process := v.paramAllow[KindProcess]
	for key := range process {
		if printerParams.has(key) || filamentParams.has(key) {
			t.Errorf("process allowlist wrongly includes hardware/material param %q", key)
		}
	}
	// The three allowlists together must cover every schema param exactly once
	// for process, and printer+filament shared keys are permitted only within
	// their own buckets.
	if len(process)+countOwned(v.allParamKeys, printerParams, filamentParams) != len(v.allParamKeys) {
		t.Errorf("param partition mismatch: process=%d owned=%d total=%d",
			len(process), countOwned(v.allParamKeys, printerParams, filamentParams), len(v.allParamKeys))
	}
}

// countOwned counts schema params claimed by printer or filament.
func countOwned(all, printer, filament set) int {
	n := 0
	for key := range all {
		if printer.has(key) || filament.has(key) {
			n++
		}
	}
	return n
}

// --- helpers ---

func replace(s, old, new string) string {
	out := strings.Replace(s, old, new, 1)
	if out == s && old != new {
		panic("replace: pattern not found: " + old)
	}
	return out
}

// appendParam adds a line under the params block of one of the canonical
// examples (each ends its params list before any trailing top-level content).
func appendParam(s, line string) string {
	return strings.TrimRight(s, "\n") + "\n  " + line + "\n"
}

func hasError(res *Result, substr string) bool {
	for _, e := range res.Errors {
		if strings.Contains(e.String(), substr) {
			return true
		}
	}
	return false
}

func renderErrors(res *Result) string {
	var b strings.Builder
	for _, e := range res.Errors {
		b.WriteString("  - ")
		b.WriteString(e.String())
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "  (no errors)\n"
	}
	return b.String()
}

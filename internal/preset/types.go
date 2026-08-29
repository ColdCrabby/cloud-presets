// Package preset defines the canonical preset types served by the cloud and
// validates authored preset files against the vendored slicer schemas plus the
// additional strictness the raw schemas do not provide.
//
// The raw slicer schemas are permissive by design: they leave
// additionalProperties unset (so unknown and misspelled fields validate
// silently) and declare most numeric parameters with a type and default but no
// minimum/maximum (so out-of-range values such as fan_speed: 100 validate too).
// This package layers the load-bearing strictness on top:
//
//   - unknown top-level and params fields are rejected;
//   - cloud-managed and local-only fields must be absent when authored;
//   - each preset kind may only set the params it owns (per-type allowlists);
//   - bounded quantities (fractions, temperatures, densities) are range-checked;
//   - the id pattern and the file-name-equals-id rule are enforced;
//   - schema_version is required, validated, and stripped before serving.
package preset

// Kind is one of the three preset categories, matching the slicer's three
// profile schemas.
type Kind string

const (
	KindPrinter  Kind = "printer"
	KindFilament Kind = "filament"
	KindProcess  Kind = "process"
)

// Valid reports whether k is a recognised preset kind.
func (k Kind) Valid() bool {
	switch k {
	case KindPrinter, KindFilament, KindProcess:
		return true
	default:
		return false
	}
}

// String returns the kind as a string.
func (k Kind) String() string { return string(k) }

// SupportedSchemaVersion is the only schema major this validator accepts.
// Introducing v2 is a deliberate, breaking change (see
// docs/preset-schema.md#schema-versioning), never an automatic bump.
const SupportedSchemaVersion = 1

// Params is a preset's sparse bag of slicing overrides: only the keys the
// preset actually sets. Every value uses the slicer's own field names and units
// with no translation layer. Omitted keys fall back to the slicer's defaults.
type Params map[string]any

// Printer is a machine preset: bed geometry, kinematics and hardware limits,
// plus the sparse params it contributes.
type Printer struct {
	SchemaVersion int    `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	ID            string `json:"id" yaml:"id"`
	Name          string `json:"name" yaml:"name"`

	Vendor string `json:"vendor" yaml:"vendor"`
	Model  string `json:"model" yaml:"model"`

	BedShape       string  `json:"bed_shape,omitempty" yaml:"bed_shape,omitempty"`
	BedWidth       float64 `json:"bed_width,omitempty" yaml:"bed_width,omitempty"`
	BedDepth       float64 `json:"bed_depth,omitempty" yaml:"bed_depth,omitempty"`
	BedHeight      float64 `json:"bed_height,omitempty" yaml:"bed_height,omitempty"`
	OriginAtCenter bool    `json:"origin_at_center,omitempty" yaml:"origin_at_center,omitempty"`

	Params Params `json:"params,omitempty" yaml:"params,omitempty"`
}

// Filament is a material preset: temperatures, cooling and flow, plus display
// and estimation fields, plus the sparse params it contributes.
type Filament struct {
	SchemaVersion int    `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	ID            string `json:"id" yaml:"id"`
	Name          string `json:"name" yaml:"name"`

	Vendor      string  `json:"vendor" yaml:"vendor"`
	Material    string  `json:"material" yaml:"material"`
	Color       string  `json:"color" yaml:"color"`
	DensityGCm3 float64 `json:"density_g_cm3" yaml:"density_g_cm3"`
	CostPerKg   float64 `json:"cost_per_kg" yaml:"cost_per_kg"`

	Params Params `json:"params,omitempty" yaml:"params,omitempty"`
}

// Process is a quality preset: layer height, speeds and walls. It has no vendor
// field; ownership is determined by its location in the presets repository.
type Process struct {
	SchemaVersion int    `json:"schema_version,omitempty" yaml:"schema_version,omitempty"`
	ID            string `json:"id" yaml:"id"`
	Name          string `json:"name" yaml:"name"`

	Quality string `json:"quality,omitempty" yaml:"quality,omitempty"`

	Params Params `json:"params,omitempty" yaml:"params,omitempty"`
}

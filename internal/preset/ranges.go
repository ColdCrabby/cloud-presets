package preset

// rangeBound is an inclusive [min, max] bound applied to a bounded quantity the
// raw schema leaves unbounded. The schema declares these as plain numbers with a
// default but no minimum/maximum, so a value like fan_speed: 100 (a fraction
// meant to be 0.0–1.0) validates structurally and fails only as a ruined print.
type rangeBound struct {
	min  float64
	max  float64
	kind string // human-readable category, for the error message
}

// topLevelRanges bounds the numeric domain fields that live at the top level of
// a preset (currently filament's density and cost).
var topLevelRanges = map[string]rangeBound{
	"density_g_cm3": {min: 0.1, max: 25, kind: "density (g/cm³)"},
	"cost_per_kg":   {min: 0, max: 100000, kind: "cost per kg"},
}

// paramRanges bounds the params (SlicingParams) quantities that are physically
// bounded but which the schema declares without a minimum/maximum. These are
// the silent traps: fractions expressed as percentages, and temperatures or
// densities outside any plausible range.
var paramRanges = map[string]rangeBound{
	// Fan speeds and other fractions are 0.0–1.0, not percentages.
	"fan_speed":             {min: 0, max: 1, kind: "fraction (0.0–1.0)"},
	"first_layer_fan_speed": {min: 0, max: 1, kind: "fraction (0.0–1.0)"},
	"bridge_fan_speed":      {min: 0, max: 1, kind: "fraction (0.0–1.0)"},
	"infill_density":        {min: 0, max: 1, kind: "fraction (0.0–1.0)"},
	"support_density":       {min: 0, max: 1, kind: "fraction (0.0–1.0)"},

	// Temperatures in °C.
	"nozzle_temp":             {min: 0, max: 500, kind: "temperature (°C)"},
	"nozzle_temp_first_layer": {min: 0, max: 500, kind: "temperature (°C)"},
	"bed_temp":                {min: 0, max: 200, kind: "temperature (°C)"},
	"bed_temp_first_layer":    {min: 0, max: 200, kind: "temperature (°C)"},
	"chamber_temp":            {min: 0, max: 150, kind: "temperature (°C)"},

	// Densities in g/cm³.
	"filament_density_g_cm3": {min: 0.1, max: 25, kind: "density (g/cm³)"},

	// Flow multipliers are ratios centred on 1.0; a value like 100 is a slip.
	"flow_ratio":        {min: 0.1, max: 2, kind: "flow ratio"},
	"bridge_flow_ratio": {min: 0.1, max: 2, kind: "flow ratio"},
}

// asFloat coerces a JSON/YAML-decoded numeric value to float64. It reports
// ok=false for non-numeric values (which the schema layer already rejects with
// a type error, so range checking simply skips them).
func asFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

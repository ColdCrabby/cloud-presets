package preset

// forbiddenFields are the cloud-managed and local-only fields that must never
// appear in an authored preset file. They are valid properties in the slicer's
// profile schema (it describes a user's local profiles too), so the raw schema
// accepts them; rejecting them here is what stops an author from believing they
// set something the cloud will silently drop or inject itself.
//
//   - source / import_url  — injected by the API on output.
//   - based_on             — records a user forking a catalog entry; a catalog
//     entry is not forked from anything.
//   - label_ids            — a user's private tagging vocabulary.
//   - connection           — a printer's personal network address.
var forbiddenFields = map[string]string{
	"source":     "injected by the API on output",
	"import_url": "injected by the API on output",
	"based_on":   "records a user forking a catalog entry; not authored in the catalog",
	"label_ids":  "a user's private tagging vocabulary; not a catalog property",
	"connection": "a printer's network address is personal, not a catalog property",
}

// schemaVersionField is authored but describes the file rather than the profile,
// so it is accepted, validated, then stripped before serving. It is not a
// property of the slicer schema, so it must be handled (and removed) before the
// body is validated against that schema.
const schemaVersionField = "schema_version"

// printerParams is the set of slicing params a printer preset may contribute:
// hardware, kinematics and firmware. A printer describes the machine, so it
// owns nozzle/filament geometry, retraction, travel kinematics, extruder count,
// firmware flavor and the machine's start/end G-code.
var printerParams = newSet(
	"nozzle_diameter_mm",
	"filament_diameter_mm",
	"extruder_count",
	"gcode_flavor",
	"start_gcode",
	"end_gcode",
	"retract_mm",
	"retract_speed_mm_min",
	"z_hop_mm",
	"travel_speed_mm_min",
)

// filamentParams is the set of slicing params a filament preset may contribute:
// the material's thermal, cooling and flow behaviour, plus its physical
// diameter/density.
var filamentParams = newSet(
	"nozzle_temp",
	"nozzle_temp_first_layer",
	"bed_temp",
	"bed_temp_first_layer",
	"chamber_temp",
	"fan_speed",
	"first_layer_fan_speed",
	"bridge_fan_speed",
	"disable_fan_first_layers",
	"flow_ratio",
	"pressure_advance",
	"max_volumetric_speed",
	"filament_diameter_mm",
	"filament_density_g_cm3",
)

// paramAllowlist returns the set of params the given kind may contribute.
//
// Printer and filament are the two constrained buckets (hardware and material);
// process is the general quality bucket and owns every remaining param. Because
// the schema shares one SlicingParams definition across all three kinds,
// nothing in it stops a process preset from setting gcode_flavor or a printer
// preset from setting infill_density — and since the slicer lets later
// composition stages win, a stray key in the wrong category would silently
// override the category that legitimately owns it. These allowlists reject that.
//
// allParamKeys is the full set of valid param keys (the SlicingParams schema
// properties); process is defined as everything not owned by printer or
// filament so that a newly added quality param is owned by process by default.
func paramAllowlist(kind Kind, allParamKeys set) set {
	switch kind {
	case KindPrinter:
		return printerParams
	case KindFilament:
		return filamentParams
	case KindProcess:
		process := make(set, len(allParamKeys))
		for key := range allParamKeys {
			if !printerParams.has(key) && !filamentParams.has(key) {
				process[key] = struct{}{}
			}
		}
		return process
	default:
		return nil
	}
}

// set is a string set.
type set map[string]struct{}

func newSet(keys ...string) set {
	s := make(set, len(keys))
	for _, k := range keys {
		s[k] = struct{}{}
	}
	return s
}

func (s set) has(k string) bool {
	_, ok := s[k]
	return ok
}

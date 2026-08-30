package catalog

import (
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/search"
)

// Sample returns a small, self-contained catalog snapshot for local runs and
// demos before the ingest pipeline (a later foundation-wave issue) exists to
// build a real revision from ColdCrabby/presets. It lets GET /v1/presets serve
// real, rankable data today; ingest will replace it with a Swap of the genuine
// commit-derived catalog and this file can be deleted.
func Sample() *Catalog {
	return &Catalog{
		Revision: "sample-0001",
		BuildID:  "sample",
		BuiltAt:  time.Now().UTC(),
		Records: []search.Record{
			{
				ID:     "prusa-mk4",
				Type:   "printer",
				Name:   "Prusa MK4",
				Vendor: "prusa",
				Model:  "MK4",
				Spec:   "250 × 210 × 220 mm, 0.4 mm nozzle",
			},
			{
				ID:     "prusa-mk3s",
				Type:   "printer",
				Name:   "Prusa MK3S+",
				Vendor: "prusa",
				Model:  "MK3S+",
				Spec:   "250 × 210 × 210 mm, 0.4 mm nozzle",
			},
			{
				ID:     "prusa-mini",
				Type:   "printer",
				Name:   "Prusa Mini+",
				Vendor: "prusa",
				Model:  "Mini+",
				Spec:   "180 × 180 × 180 mm, 0.4 mm nozzle",
			},
			{
				ID:     "bambu-x1c",
				Type:   "printer",
				Name:   "Bambu Lab X1 Carbon",
				Vendor: "bambulab",
				Model:  "X1 Carbon",
				Spec:   "256 × 256 × 256 mm, 0.4 mm nozzle",
			},
			{
				ID:       "prusament-pla-galaxy-black",
				Type:     "filament",
				Name:     "Prusament PLA Galaxy Black",
				Vendor:   "prusa",
				Material: "PLA",
				Spec:     "1.24 g/cm³, 215 °C nozzle, 60 °C bed",
			},
			{
				ID:       "prusament-petg-jet-black",
				Type:     "filament",
				Name:     "Prusament PETG Jet Black",
				Vendor:   "prusa",
				Material: "PETG",
				Spec:     "1.27 g/cm³, 240 °C nozzle, 85 °C bed",
			},
			{
				ID:       "bambu-abs-black",
				Type:     "filament",
				Name:     "Bambu ABS Black",
				Vendor:   "bambulab",
				Material: "ABS",
				Spec:     "1.05 g/cm³, 260 °C nozzle, 90 °C bed",
			},
			{
				ID:     "process-0-20mm-quality",
				Type:   "process",
				Name:   "0.20 mm Quality",
				Vendor: "prusa",
				Spec:   "0.20 mm layers, 2 walls, 15% infill",
			},
			{
				ID:     "process-0-28mm-draft",
				Type:   "process",
				Name:   "0.28 mm Draft",
				Vendor: "prusa",
				Spec:   "0.28 mm layers, 2 walls, 10% infill",
			},
		},
		Vendors: []Vendor{
			{
				Slug:        "prusa",
				DisplayName: "Prusa",
				Website:     strptr("https://www.prusa3d.com"),
			},
			{
				Slug:        "bambulab",
				DisplayName: "Bambu Lab",
				Website:     strptr("https://bambulab.com"),
			},
		},
		Presets: map[string]FullPreset{
			"prusa-mk4": {
				ID: "prusa-mk4", Type: "printer", Name: "Prusa MK4", Vendor: "prusa",
				Params: map[string]any{
					"bed_shape":        "250x210",
					"nozzle_diameter":  0.4,
					"max_print_height": 220,
				},
			},
			"prusa-mk3s": {
				ID: "prusa-mk3s", Type: "printer", Name: "Prusa MK3S+", Vendor: "prusa",
				Params: map[string]any{
					"bed_shape":        "250x210",
					"nozzle_diameter":  0.4,
					"max_print_height": 210,
				},
			},
			"prusa-mini": {
				ID: "prusa-mini", Type: "printer", Name: "Prusa Mini+", Vendor: "prusa",
				Params: map[string]any{
					"bed_shape":        "180x180",
					"nozzle_diameter":  0.4,
					"max_print_height": 180,
				},
			},
			"bambu-x1c": {
				ID: "bambu-x1c", Type: "printer", Name: "Bambu Lab X1 Carbon", Vendor: "bambulab",
				Params: map[string]any{
					"bed_shape":        "256x256",
					"nozzle_diameter":  0.4,
					"max_print_height": 256,
				},
			},
			"prusament-pla-galaxy-black": {
				ID: "prusament-pla-galaxy-black", Type: "filament", Name: "Prusament PLA Galaxy Black", Vendor: "prusa",
				Params: map[string]any{
					"filament_type":    "PLA",
					"temperature":      215,
					"bed_temperature":  60,
					"filament_density": 1.24,
				},
			},
			"prusament-petg-jet-black": {
				ID: "prusament-petg-jet-black", Type: "filament", Name: "Prusament PETG Jet Black", Vendor: "prusa",
				Params: map[string]any{
					"filament_type":    "PETG",
					"temperature":      240,
					"bed_temperature":  85,
					"filament_density": 1.27,
				},
			},
			"bambu-abs-black": {
				ID: "bambu-abs-black", Type: "filament", Name: "Bambu ABS Black", Vendor: "bambulab",
				Params: map[string]any{
					"filament_type":    "ABS",
					"temperature":      260,
					"bed_temperature":  90,
					"filament_density": 1.05,
				},
			},
			"process-0-20mm-quality": {
				ID: "process-0-20mm-quality", Type: "process", Name: "0.20 mm Quality", Vendor: "prusa",
				Params: map[string]any{
					"layer_height": 0.20,
					"perimeters":   2,
					"fill_density": "15%",
				},
			},
			"process-0-28mm-draft": {
				ID: "process-0-28mm-draft", Type: "process", Name: "0.28 mm Draft", Vendor: "prusa",
				Params: map[string]any{
					"layer_height": 0.28,
					"perimeters":   2,
					"fill_density": "10%",
				},
			},
		},
	}
}

// strptr returns a pointer to s, for optional string fields in sample data.
func strptr(s string) *string { return &s }

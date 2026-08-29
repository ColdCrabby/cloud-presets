package preset

import "encoding/json"

// As decodes the sanitized preset body into target, which should be a pointer to
// the typed struct for this preset's kind (*Printer, *Filament or *Process).
//
// It is only meaningful for a valid Result: the sanitized body has already had
// schema_version stripped and, on a valid preset, carries no forbidden fields.
// Decoding goes through JSON so the slicer's field names and units are preserved
// exactly, with no translation layer.
func (r *Result) As(target any) error {
	data, err := json.Marshal(r.Sanitized)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

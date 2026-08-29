package preset

import (
	"fmt"
	"sort"
	"strings"
)

// Error is a single validation failure. Path locates the offending field using
// dotted notation (for example "params.fan_speed" or "id"); it is empty for
// file-level failures such as a syntax error or a file-name mismatch.
type Error struct {
	Path    string
	Message string
}

func (e Error) String() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func (e Error) Error() string { return e.String() }

// Result is the outcome of validating one preset file. A preset is valid only
// when Errors is empty; the validator collects every failure it can rather than
// stopping at the first, so an author sees all of a file's problems at once.
type Result struct {
	Kind Kind

	// Sanitized is the preset body ready to serve: the authored map with
	// schema_version stripped. It is populated on a best-effort basis even when
	// Errors is non-empty (it is only meaningful when the preset is valid).
	Sanitized map[string]any

	Errors []Error
}

// Valid reports whether the preset passed every check.
func (r *Result) Valid() bool { return len(r.Errors) == 0 }

// Err returns a single error summarising every failure, or nil when valid.
func (r *Result) Err() error {
	if r.Valid() {
		return nil
	}
	msgs := make([]string, len(r.Errors))
	for i, e := range r.Errors {
		msgs[i] = e.String()
	}
	return fmt.Errorf("%d validation error(s):\n  - %s", len(r.Errors), strings.Join(msgs, "\n  - "))
}

// add appends an error to the result.
func (r *Result) add(path, format string, args ...any) {
	r.Errors = append(r.Errors, Error{Path: path, Message: fmt.Sprintf(format, args...)})
}

// sortErrors orders errors by path then message for stable, readable output.
func (r *Result) sortErrors() {
	sort.SliceStable(r.Errors, func(i, j int) bool {
		if r.Errors[i].Path != r.Errors[j].Path {
			return r.Errors[i].Path < r.Errors[j].Path
		}
		return r.Errors[i].Message < r.Errors[j].Message
	})
}

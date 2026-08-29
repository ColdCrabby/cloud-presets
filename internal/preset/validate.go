package preset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/ColdCrabby/cloud-presets/schemas"
)

// idPattern is the required shape of a preset id: lowercase alphanumerics in
// dot- or dash-separated segments. Ids appear in URLs and in the based_on field
// of profiles users have imported, so they must be stable and URL-safe.
var idPattern = regexp.MustCompile(`^[a-z0-9]+(?:[-.][a-z0-9]+)*$`)

// schemaFiles maps each kind to its vendored schema file name.
var schemaFiles = map[Kind]string{
	KindPrinter:  schemas.PrinterSchemaFile,
	KindFilament: schemas.FilamentSchemaFile,
	KindProcess:  schemas.ProcessSchemaFile,
}

// Validator validates preset files against the vendored slicer schemas plus the
// cloud's additional strictness. It is safe for concurrent use once built, so a
// single instance can be shared across CI, ingest and the dry-run endpoint —
// the three places validation runs, all with the same pinned schemas.
type Validator struct {
	compiled     map[Kind]*jsonschema.Schema
	allowedTop   map[Kind]set
	paramAllow   map[Kind]set
	allParamKeys set
}

// New builds a Validator from the embedded, vendored schemas.
func New() (*Validator, error) {
	return newFromFS(schemas.FS)
}

func newFromFS(fsys fs.FS) (*Validator, error) {
	v := &Validator{
		compiled:   make(map[Kind]*jsonschema.Schema),
		allowedTop: make(map[Kind]set),
		paramAllow: make(map[Kind]set),
	}
	for kind, file := range schemaFiles {
		raw, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read schema %q: %w", file, err)
		}

		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("parse schema %q: %w", file, err)
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(file, doc); err != nil {
			return nil, fmt.Errorf("add schema %q: %w", file, err)
		}
		sch, err := c.Compile(file)
		if err != nil {
			return nil, fmt.Errorf("compile schema %q: %w", file, err)
		}
		v.compiled[kind] = sch

		top, params, err := schemaKeys(raw)
		if err != nil {
			return nil, fmt.Errorf("schema %q: %w", file, err)
		}
		// Allowed top-level fields are the schema's own properties minus the
		// forbidden ones, plus the authored-but-stripped schema_version.
		allowed := make(set, len(top)+1)
		for key := range top {
			if _, bad := forbiddenFields[key]; !bad {
				allowed[key] = struct{}{}
			}
		}
		allowed[schemaVersionField] = struct{}{}
		v.allowedTop[kind] = allowed

		if v.allParamKeys == nil {
			v.allParamKeys = params
		}
	}
	for kind := range schemaFiles {
		v.paramAllow[kind] = paramAllowlist(kind, v.allParamKeys)
	}
	return v, nil
}

// schemaKeys returns the top-level property names and the SlicingParams
// property names declared by a profile schema.
func schemaKeys(raw []byte) (top set, params set, err error) {
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       struct {
			SlicingParams struct {
				Properties map[string]json.RawMessage `json:"properties"`
			} `json:"SlicingParams"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, err
	}
	top = make(set, len(doc.Properties))
	for k := range doc.Properties {
		top[k] = struct{}{}
	}
	params = make(set, len(doc.Defs.SlicingParams.Properties))
	for k := range doc.Defs.SlicingParams.Properties {
		params[k] = struct{}{}
	}
	if len(params) == 0 {
		return nil, nil, fmt.Errorf("no SlicingParams properties found in schema")
	}
	return top, params, nil
}

// Validate checks a preset body of the given kind. Use ValidateFile when the
// file name is known so the file-name-equals-id rule can be enforced.
func (v *Validator) Validate(kind Kind, data []byte) *Result {
	return v.ValidateFile(kind, "", data)
}

// ValidateFile checks a preset body of the given kind, and — when filename is
// non-empty — also enforces that the file is named <id>.yaml.
func (v *Validator) ValidateFile(kind Kind, filename string, data []byte) *Result {
	res := &Result{Kind: kind}

	if !kind.Valid() {
		res.add("", "unknown preset kind %q", kind)
		return res
	}

	body, err := decodeYAML(data)
	if err != nil {
		res.add("", "%s", err.Error())
		return res
	}

	// schema_version: required, must be the supported major, then stripped.
	v.checkSchemaVersion(body, res)

	// Strictness that operates on the authored map.
	v.checkForbiddenAndUnknownTop(kind, body, res)
	v.checkParams(kind, body, res)
	v.checkTopLevelRanges(body, res)
	v.checkID(body, filename, res)

	// Sanitized body: the authored map with schema_version removed, ready to
	// serve. Structural schema validation runs against this.
	sanitized := make(map[string]any, len(body))
	for k, val := range body {
		if k == schemaVersionField {
			continue
		}
		sanitized[k] = val
	}
	res.Sanitized = sanitized

	v.checkSchema(kind, sanitized, res)

	res.sortErrors()
	return res
}

// decodeYAML parses a preset file into a generic map, rejecting anything that is
// not a YAML mapping (and, via yaml.v3, any duplicate keys).
func decodeYAML(data []byte) (map[string]any, error) {
	var body map[string]any
	if err := yaml.Unmarshal(data, &body); err != nil {
		return nil, fmt.Errorf("invalid YAML: %v", err)
	}
	if body == nil {
		return nil, fmt.Errorf("preset is empty")
	}
	return body, nil
}

func (v *Validator) checkSchemaVersion(body map[string]any, res *Result) {
	raw, ok := body[schemaVersionField]
	if !ok {
		res.add(schemaVersionField, "required: every preset must declare the schema major it targets")
		return
	}
	n, ok := asFloat(raw)
	if !ok || n != float64(int(n)) {
		res.add(schemaVersionField, "must be an integer, got %v", raw)
		return
	}
	if int(n) != SupportedSchemaVersion {
		res.add(schemaVersionField, "unsupported schema version %d (this validator serves v%d)", int(n), SupportedSchemaVersion)
	}
}

func (v *Validator) checkForbiddenAndUnknownTop(kind Kind, body map[string]any, res *Result) {
	allowed := v.allowedTop[kind]
	for key := range body {
		if reason, bad := forbiddenFields[key]; bad {
			res.add(key, "field is not allowed in an authored preset (%s)", reason)
			continue
		}
		if !allowed.has(key) {
			res.add(key, "unknown field")
		}
	}
}

func (v *Validator) checkParams(kind Kind, body map[string]any, res *Result) {
	rawParams, ok := body["params"]
	if !ok {
		return
	}
	params, ok := rawParams.(map[string]any)
	if !ok {
		// A non-object params is caught with a clearer message by the schema
		// layer; nothing more to check here.
		return
	}
	allow := v.paramAllow[kind]
	for key, val := range params {
		p := "params." + key
		if !v.allParamKeys.has(key) {
			res.add(p, "unknown parameter")
			continue
		}
		if !allow.has(key) {
			res.add(p, "not a %s parameter: this key belongs to a different preset type", kind)
			continue
		}
		if bound, ok := paramRanges[key]; ok {
			checkBound(p, val, bound, res)
		}
	}
}

func (v *Validator) checkTopLevelRanges(body map[string]any, res *Result) {
	for key, bound := range topLevelRanges {
		if val, ok := body[key]; ok {
			checkBound(key, val, bound, res)
		}
	}
}

func checkBound(path string, val any, bound rangeBound, res *Result) {
	n, ok := asFloat(val)
	if !ok {
		return // non-numeric is reported by the schema layer
	}
	if n < bound.min || n > bound.max {
		res.add(path, "value %v is outside the plausible %s range [%g, %g]", val, bound.kind, bound.min, bound.max)
	}
}

func (v *Validator) checkID(body map[string]any, filename string, res *Result) {
	rawID, ok := body["id"]
	if !ok {
		return // required-field error comes from the schema layer
	}
	id, ok := rawID.(string)
	if !ok {
		return // type error comes from the schema layer
	}
	if !idPattern.MatchString(id) {
		res.add("id", "%q must be lowercase and match %s", id, idPattern.String())
	}
	if filename == "" {
		return
	}
	base := path.Base(filename)
	name := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")
	if name != id {
		res.add("", "file name %q must equal the preset id %q plus .yaml", base, id)
	}
}

func (v *Validator) checkSchema(kind Kind, sanitized map[string]any, res *Result) {
	sch := v.compiled[kind]
	if sch == nil {
		return
	}
	// Re-encode through JSON so numbers become json.Number and types match the
	// JSON data model the schema validator expects.
	encoded, err := json.Marshal(sanitized)
	if err != nil {
		res.add("", "could not encode preset for schema validation: %v", err)
		return
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		res.add("", "could not decode preset for schema validation: %v", err)
		return
	}
	if err := sch.Validate(instance); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			for _, leaf := range flattenSchemaErrors(verr) {
				res.add(leaf.path, "%s", leaf.msg)
			}
			return
		}
		res.add("", "%s", err.Error())
	}
}

// schemaLeaf is a single leaf failure extracted from the nested schema error
// tree: the instance location that failed and a human-readable reason.
type schemaLeaf struct {
	path string
	msg  string
}

var schemaPrinter = message.NewPrinter(language.English)

// flattenSchemaErrors walks the nested ValidationError tree and returns its leaf
// failures. A leaf (a node with no further causes) is the actual keyword that
// rejected a concrete value, which is what an author needs to see. oneOf/anyOf
// composites — how the slicer schema encodes enums — are collapsed into a single
// "value must be one of …" message rather than one failure per branch.
func flattenSchemaErrors(root *jsonschema.ValidationError) []schemaLeaf {
	var out []schemaLeaf
	var walk func(*jsonschema.ValidationError)
	walk = func(n *jsonschema.ValidationError) {
		switch n.ErrorKind.(type) {
		case *kind.OneOf, *kind.AnyOf:
			if allowed := collectEnumValues(n); len(allowed) > 0 {
				out = append(out, schemaLeaf{
					path: strings.Join(n.InstanceLocation, "."),
					msg:  "value must be one of " + strings.Join(allowed, ", "),
				})
				return
			}
		}
		if len(n.Causes) == 0 {
			out = append(out, schemaLeaf{
				path: strings.Join(n.InstanceLocation, "."),
				msg:  n.ErrorKind.LocalizedString(schemaPrinter),
			})
			return
		}
		for _, c := range n.Causes {
			walk(c)
		}
	}
	walk(root)
	if len(out) == 0 {
		out = append(out, schemaLeaf{path: "", msg: root.Error()})
	}
	return out
}

// collectEnumValues gathers the permitted const/enum values from the causes of a
// oneOf/anyOf node, so an enum failure reads as one message listing the choices.
func collectEnumValues(n *jsonschema.ValidationError) []string {
	var vals []string
	for _, c := range n.Causes {
		switch k := c.ErrorKind.(type) {
		case *kind.Const:
			vals = append(vals, fmt.Sprintf("%v", k.Want))
		case *kind.Enum:
			for _, w := range k.Want {
				vals = append(vals, fmt.Sprintf("%v", w))
			}
		default:
			return nil // not a pure enum-style composite; fall back to per-leaf
		}
	}
	return vals
}

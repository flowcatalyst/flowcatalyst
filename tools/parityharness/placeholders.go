package main

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// matchShape compares a response body against a placeholder-typed
// shape. The shape comes from YAML; nodes are either:
//
//   - A placeholder string like `tsid`, `uuid`, `iso8601-microsecond`,
//     `any-string`, `any-int`, `any-number`, `any-bool`, `any-array`,
//     `any-object`, `any` — these match any value of the named kind.
//   - A literal value — string/number/bool — that must equal the body.
//   - A map — recursive object comparison.
//   - A slice with one element — every body element must match that
//     element's shape (think "array of T").
//   - A slice with N elements — body must be a same-length array,
//     element-wise compared.
//
// `target` is the label ("rust" or "go") used in error messages.
func matchShape(body []byte, shape any, target string) error {
	got, err := parseLooseJSON(body)
	if err != nil {
		return fmt.Errorf("%s body is not JSON: %w", target, err)
	}
	return matchShapeRec(got, shape, "")
}

func matchShapeRec(got, shape any, path string) error {
	switch sv := shape.(type) {
	case string:
		return matchPlaceholderOrLiteral(got, sv, path)
	case map[string]any:
		gv, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object, got %T", or(path, "<root>"), got)
		}
		for k, child := range sv {
			gc, ok := gv[k]
			if !ok {
				return fmt.Errorf("%s: missing key %q", or(path, "<root>"), k)
			}
			if err := matchShapeRec(gc, child, joinPath(path, k)); err != nil {
				return err
			}
		}
		return nil
	case []any:
		gv, ok := got.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array, got %T", or(path, "<root>"), got)
		}
		if len(sv) == 1 {
			// "Array of T" — every element must match sv[0].
			for i, e := range gv {
				if err := matchShapeRec(e, sv[0], fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
			return nil
		}
		// Pinned shape — element-wise.
		if len(gv) != len(sv) {
			return fmt.Errorf("%s: length got=%d want=%d", or(path, "<root>"), len(gv), len(sv))
		}
		for i := range sv {
			if err := matchShapeRec(gv[i], sv[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case float64, int, bool, nil:
		// Numeric/bool/null literal — JSON decoder gives float64 for numbers.
		if !literalEqual(got, sv) {
			return fmt.Errorf("%s: literal mismatch got=%v want=%v", or(path, "<root>"), got, sv)
		}
		return nil
	default:
		// yaml.v3 may produce json.Number for some inputs; do a best-effort
		// re-marshal and recurse.
		bb, err := json.Marshal(sv)
		if err != nil {
			return fmt.Errorf("%s: unsupported shape type %T", or(path, "<root>"), sv)
		}
		var v any
		if err := json.Unmarshal(bb, &v); err != nil {
			return fmt.Errorf("%s: unsupported shape: %w", or(path, "<root>"), err)
		}
		return matchShapeRec(got, v, path)
	}
}

// matchPlaceholderOrLiteral resolves a string shape leaf. Known
// placeholder names match by regex / type; anything else is treated as
// a literal value to compare exactly.
func matchPlaceholderOrLiteral(got any, shape, path string) error {
	switch shape {
	case "tsid":
		s, ok := got.(string)
		if !ok || !tsidPattern.MatchString(s) {
			return fmt.Errorf("%s: not a TSID: %v", or(path, "<root>"), got)
		}
	case "uuid":
		s, ok := got.(string)
		if !ok || !uuidPattern.MatchString(s) {
			return fmt.Errorf("%s: not a UUID: %v", or(path, "<root>"), got)
		}
	case "iso8601-microsecond":
		s, ok := got.(string)
		if !ok || !iso8601MicroPattern.MatchString(s) {
			return fmt.Errorf("%s: not an ISO-8601 microsecond timestamp: %v", or(path, "<root>"), got)
		}
	case "any-string":
		if _, ok := got.(string); !ok {
			return fmt.Errorf("%s: not a string: %v", or(path, "<root>"), got)
		}
	case "any-int":
		f, ok := got.(float64)
		if !ok || f != float64(int64(f)) {
			return fmt.Errorf("%s: not an integer: %v", or(path, "<root>"), got)
		}
	case "any-number":
		if _, ok := got.(float64); !ok {
			return fmt.Errorf("%s: not a number: %v", or(path, "<root>"), got)
		}
	case "any-bool":
		if _, ok := got.(bool); !ok {
			return fmt.Errorf("%s: not a bool: %v", or(path, "<root>"), got)
		}
	case "any-array":
		if _, ok := got.([]any); !ok {
			return fmt.Errorf("%s: not an array: %v", or(path, "<root>"), got)
		}
	case "any-object":
		if _, ok := got.(map[string]any); !ok {
			return fmt.Errorf("%s: not an object: %v", or(path, "<root>"), got)
		}
	case "any":
		// Wildcard — accept anything, including null.
	default:
		// Treat as literal string.
		s, ok := got.(string)
		if !ok || s != shape {
			return fmt.Errorf("%s: rust=%v expected literal %q", or(path, "<root>"), got, shape)
		}
	}
	return nil
}

// literalEqual is a permissive equality for JSON literals — handles
// the float64/int interconversion the YAML library introduces.
func literalEqual(got, want any) bool {
	if got == nil && want == nil {
		return true
	}
	if gv, ok := got.(float64); ok {
		switch wv := want.(type) {
		case float64:
			return gv == wv
		case int:
			return gv == float64(wv)
		case int64:
			return gv == float64(wv)
		}
	}
	return got == want
}

// ── Placeholder patterns ─────────────────────────────────────────────────

// tsidPattern matches both untyped (`6F68K4CRY247K`) and typed
// (`clt_6F68K4CRY247K`) TSIDs. Prefix is 3-4 lowercase letters per
// internal/tsid/tsid.go's catalog.
var tsidPattern = regexp.MustCompile(`^([a-z]{3,4}_)?[0-9A-HJ-NP-TV-Z]{13}$`)

// uuidPattern matches RFC 4122 UUIDs in any version, lowercased.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// iso8601MicroPattern matches RFC3339 with exactly six subsec digits +
// `Z` suffix (the format Rust's chrono::DateTime<Utc> emits). The
// parity contract requires this exact precision so SDK timestamp
// parsers don't see drift; see docs/api-parity.md §Timestamps.
var iso8601MicroPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z$`)

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// compareResponses returns the empty string when Rust and Go responses
// are compatible per the parity contract (docs/api-parity.md), or a
// diff blob otherwise.
//
// Three checks:
//  1. Status code: Rust and Go must match each other, AND if
//     expect.status was set, both must equal it.
//  2. Headers in `comparedHeaderNames` must match (case-insensitive).
//     Headers outside that set are ignored — Date/Server/Content-Length
//     legitimately differ between runtimes.
//  3. Bodies:
//     - When expect.body_shape is present, both responses are matched
//     against the shape (placeholder types resolve via matchPlaceholder).
//     - Otherwise the Rust body and Go body are diffed against each
//     other directly (every key, every value, recursive).
func compareResponses(expect expectSpec, rustResp *http.Response, rustBody []byte, goResp *http.Response, goBody []byte) string {
	var diffs []string

	// Status code.
	if rustResp.StatusCode != goResp.StatusCode {
		diffs = append(diffs, fmt.Sprintf("status: rust=%d go=%d", rustResp.StatusCode, goResp.StatusCode))
	} else if expect.Status != 0 && rustResp.StatusCode != expect.Status {
		diffs = append(diffs,
			fmt.Sprintf("status: both returned %d, expected %d", rustResp.StatusCode, expect.Status))
	}

	// Headers.
	for _, name := range comparedHeaderNames {
		rv := rustResp.Header.Get(name)
		gv := goResp.Header.Get(name)
		if rv != gv {
			diffs = append(diffs, fmt.Sprintf("header %s: rust=%q go=%q", name, rv, gv))
		}
	}

	// Bodies.
	if expect.BodyShape != nil {
		if err := matchShape(rustBody, expect.BodyShape, "rust"); err != nil {
			diffs = append(diffs, "rust body does not match expect.body_shape: "+err.Error())
		}
		if err := matchShape(goBody, expect.BodyShape, "go"); err != nil {
			diffs = append(diffs, "go body does not match expect.body_shape: "+err.Error())
		}
	} else if len(rustBody) > 0 || len(goBody) > 0 {
		if d := diffJSONBytes(rustBody, goBody); d != "" {
			diffs = append(diffs, "body diff:\n"+indent(d, "  "))
		}
	}

	if len(diffs) == 0 {
		return ""
	}
	return strings.Join(diffs, "\n")
}

// comparedHeaderNames is the closed set of response headers that the
// parity contract treats as load-bearing. Other headers are ignored.
// (Per docs/api-parity.md §"What 'compatible' means".)
var comparedHeaderNames = []string{
	"Content-Type",
	"Cache-Control",
	"WWW-Authenticate",
	"Location",
	// X-FC-* headers are exposed to consumers; explicit list keeps the
	// header check cheap. Add as the surface grows.
	"X-Fc-Correlation-Id",
}

// diffJSONBytes parses both blobs as JSON and walks them. Non-JSON
// bodies fall back to byte comparison.
func diffJSONBytes(rust, go_ []byte) string {
	rustVal, errR := parseLooseJSON(rust)
	goVal, errG := parseLooseJSON(go_)
	if errR != nil || errG != nil {
		// One or both are non-JSON: compare bytes.
		if string(rust) == string(go_) {
			return ""
		}
		return fmt.Sprintf("rust: %s\ngo:   %s", trunc(string(rust), 200), trunc(string(go_), 200))
	}
	return diffJSON(rustVal, goVal, "")
}

func parseLooseJSON(b []byte) (any, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return v, nil
}

// diffJSON recursively diffs two decoded JSON values. Returns the empty
// string when they're structurally equal. JSON objects compare by key
// set; arrays compare element-by-element in order.
func diffJSON(a, b any, path string) string {
	if a == nil && b == nil {
		return ""
	}
	if (a == nil) != (b == nil) {
		return fmt.Sprintf("%s: rust=%v go=%v", or(path, "<root>"), a, b)
	}
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: rust is object, go is %T", or(path, "<root>"), b)
		}
		return diffObjects(av, bv, path)
	case []any:
		bv, ok := b.([]any)
		if !ok {
			return fmt.Sprintf("%s: rust is array, go is %T", or(path, "<root>"), b)
		}
		return diffArrays(av, bv, path)
	case string:
		bv, ok := b.(string)
		if !ok || av != bv {
			return fmt.Sprintf("%s: rust=%q go=%v", or(path, "<root>"), av, b)
		}
	case float64:
		bv, ok := b.(float64)
		if !ok || av != bv {
			return fmt.Sprintf("%s: rust=%v go=%v", or(path, "<root>"), av, b)
		}
	case bool:
		bv, ok := b.(bool)
		if !ok || av != bv {
			return fmt.Sprintf("%s: rust=%v go=%v", or(path, "<root>"), av, b)
		}
	}
	return ""
}

func diffObjects(a, b map[string]any, path string) string {
	var diffs []string
	// Stable diff order — alphabetical keys.
	allKeys := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		allKeys[k] = struct{}{}
	}
	for k := range b {
		allKeys[k] = struct{}{}
	}
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		av, aok := a[k]
		bv, bok := b[k]
		child := joinPath(path, k)
		switch {
		case !aok:
			diffs = append(diffs, fmt.Sprintf("%s: missing in rust (go=%v)", child, summarise(bv)))
		case !bok:
			diffs = append(diffs, fmt.Sprintf("%s: missing in go (rust=%v)", child, summarise(av)))
		default:
			if d := diffJSON(av, bv, child); d != "" {
				diffs = append(diffs, d)
			}
		}
	}
	return strings.Join(diffs, "\n")
}

func diffArrays(a, b []any, path string) string {
	if len(a) != len(b) {
		return fmt.Sprintf("%s: length rust=%d go=%d", or(path, "<root>"), len(a), len(b))
	}
	var diffs []string
	for i := range a {
		if d := diffJSON(a[i], b[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
			diffs = append(diffs, d)
		}
	}
	return strings.Join(diffs, "\n")
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func or(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func summarise(v any) string {
	const max = 40
	s := fmt.Sprintf("%v", v)
	return trunc(s, max)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

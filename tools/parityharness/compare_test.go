package main

import (
	"net/http"
	"strings"
	"testing"
)

func TestDiffJSON_Equal(t *testing.T) {
	a := []byte(`{"id":"x","count":2,"tags":["a","b"]}`)
	b := []byte(`{"count":2,"tags":["a","b"],"id":"x"}`) // different key order
	if d := diffJSONBytes(a, b); d != "" {
		t.Fatalf("expected no diff for key-order difference, got:\n%s", d)
	}
}

func TestDiffJSON_MissingKey(t *testing.T) {
	a := []byte(`{"id":"x","count":2}`)
	b := []byte(`{"id":"x"}`)
	d := diffJSONBytes(a, b)
	if !strings.Contains(d, "count") || !strings.Contains(d, "missing in go") {
		t.Fatalf("expected 'count: missing in go' diff, got:\n%s", d)
	}
}

func TestDiffJSON_TypeMismatch(t *testing.T) {
	a := []byte(`{"x":1}`)
	b := []byte(`{"x":"1"}`)
	d := diffJSONBytes(a, b)
	if d == "" {
		t.Fatal("expected diff for type mismatch (number vs string), got none")
	}
}

func TestDiffJSON_NestedArray(t *testing.T) {
	a := []byte(`{"items":[{"k":1},{"k":2}]}`)
	b := []byte(`{"items":[{"k":1},{"k":3}]}`)
	d := diffJSONBytes(a, b)
	if !strings.Contains(d, "items[1].k") {
		t.Fatalf("expected path 'items[1].k' in diff, got:\n%s", d)
	}
}

func TestDiffJSON_NonJSONFallback(t *testing.T) {
	if d := diffJSONBytes([]byte("plain"), []byte("plain")); d != "" {
		t.Fatalf("identical non-JSON should diff empty, got %q", d)
	}
	d := diffJSONBytes([]byte("a"), []byte("b"))
	if d == "" || !strings.Contains(d, "rust:") {
		t.Fatalf("non-JSON divergence should report bytes, got: %q", d)
	}
}

func TestCompareResponses_StatusMismatch(t *testing.T) {
	rust := mkResp(200, "application/json", `{"x":1}`)
	go_ := mkResp(201, "application/json", `{"x":1}`)
	d := compareResponses(expectSpec{}, rust, []byte(`{"x":1}`), go_, []byte(`{"x":1}`))
	if !strings.Contains(d, "status: rust=200 go=201") {
		t.Fatalf("expected status diff, got:\n%s", d)
	}
}

func TestCompareResponses_ExpectStatusOverride(t *testing.T) {
	rust := mkResp(200, "", "")
	go_ := mkResp(200, "", "")
	d := compareResponses(expectSpec{Status: 201}, rust, nil, go_, nil)
	if !strings.Contains(d, "expected 201") {
		t.Fatalf("expected 'expected 201' diff, got:\n%s", d)
	}
}

func TestCompareResponses_HeaderMismatch(t *testing.T) {
	rust := mkResp(200, "application/json", "")
	go_ := mkResp(200, "text/plain", "")
	d := compareResponses(expectSpec{}, rust, nil, go_, nil)
	if !strings.Contains(d, "Content-Type") {
		t.Fatalf("expected Content-Type diff, got:\n%s", d)
	}
}

func TestCompareResponses_BodyShape(t *testing.T) {
	rust := mkResp(200, "application/json", "")
	go_ := mkResp(200, "application/json", "")
	body := []byte(`{"id":"6F68K4CRY247K","created":"2026-05-26T10:30:00.123456Z","count":3}`)
	shape := map[string]any{
		"id":      "tsid",
		"created": "iso8601-microsecond",
		"count":   "any-int",
	}
	d := compareResponses(expectSpec{BodyShape: shape}, rust, body, go_, body)
	if d != "" {
		t.Fatalf("expected match, got diff:\n%s", d)
	}
}

func TestCompareResponses_BodyShape_BadPlaceholder(t *testing.T) {
	rust := mkResp(200, "application/json", "")
	go_ := mkResp(200, "application/json", "")
	body := []byte(`{"id":"not-a-tsid"}`)
	d := compareResponses(expectSpec{BodyShape: map[string]any{"id": "tsid"}}, rust, body, go_, body)
	if !strings.Contains(d, "not a TSID") {
		t.Fatalf("expected 'not a TSID' in diff, got:\n%s", d)
	}
}

// mkResp builds a minimal *http.Response with the given status,
// Content-Type, and body. The body is set as a string but the
// comparator reads it from the byte argument we pass separately, so
// the body string here is informational only.
func mkResp(status int, contentType, _ string) *http.Response {
	h := http.Header{}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	return &http.Response{StatusCode: status, Header: h}
}

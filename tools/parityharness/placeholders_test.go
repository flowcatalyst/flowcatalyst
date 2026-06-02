package main

import (
	"strings"
	"testing"
)

func TestMatchShape_TSID(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"id":"6F68K4CRY247K"}`, true},      // untyped
		{`{"id":"clt_6F68K4CRY247K"}`, true},  // typed (3-char prefix)
		{`{"id":"djbr_6F68K4CRY247K"}`, true}, // typed (4-char prefix)
		{`{"id":"too-short"}`, false},
		{`{"id":"6F68K4CRY247KQ"}`, false}, // 14 chars
		{`{"id":42}`, false},               // wrong type
	}
	for _, tc := range cases {
		err := matchShape([]byte(tc.body), map[string]any{"id": "tsid"}, "go")
		got := err == nil
		if got != tc.want {
			t.Errorf("body=%q want match=%v, got match=%v (err=%v)", tc.body, tc.want, got, err)
		}
	}
}

func TestMatchShape_ISO8601Microsecond(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`{"t":"2026-05-26T10:30:00.123456Z"}`, true},
		{`{"t":"2026-05-26T10:30:00.123Z"}`, false},       // millisecond
		{`{"t":"2026-05-26T10:30:00.123456789Z"}`, false}, // nanosecond
		{`{"t":"2026-05-26T10:30:00Z"}`, false},           // no subsec
	}
	for _, tc := range cases {
		err := matchShape([]byte(tc.body), map[string]any{"t": "iso8601-microsecond"}, "go")
		got := err == nil
		if got != tc.want {
			t.Errorf("body=%q want match=%v, got match=%v (err=%v)", tc.body, tc.want, got, err)
		}
	}
}

func TestMatchShape_ArrayOfT(t *testing.T) {
	body := []byte(`{"items":[{"id":"6F68K4CRY247K"},{"id":"6F68K4CRY247L"}]}`)
	shape := map[string]any{
		"items": []any{
			map[string]any{"id": "tsid"},
		},
	}
	if err := matchShape(body, shape, "go"); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
}

func TestMatchShape_AnyTypes(t *testing.T) {
	body := []byte(`{"s":"x","n":1,"b":true,"a":[1,2],"o":{"k":1},"z":null}`)
	shape := map[string]any{
		"s": "any-string",
		"n": "any-int",
		"b": "any-bool",
		"a": "any-array",
		"o": "any-object",
		"z": "any",
	}
	if err := matchShape(body, shape, "go"); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
}

func TestMatchShape_MissingKey(t *testing.T) {
	body := []byte(`{"id":"6F68K4CRY247K"}`)
	shape := map[string]any{"id": "tsid", "name": "any-string"}
	err := matchShape(body, shape, "rust")
	if err == nil || !strings.Contains(err.Error(), "missing key") {
		t.Fatalf("expected 'missing key' error, got: %v", err)
	}
}

func TestSubstituteVars(t *testing.T) {
	t.Setenv("FOO", "bar")
	t.Setenv("X", "y")

	s, miss := substituteVars("hello ${FOO} ${X}")
	if s != "hello bar y" {
		t.Errorf("substitution: got %q", s)
	}
	if len(miss) != 0 {
		t.Errorf("missing: got %v", miss)
	}

	s, miss = substituteVars("auth ${MISSING_VAR}")
	if !strings.Contains(s, "${MISSING_VAR}") {
		t.Errorf("missing var should stay in output, got %q", s)
	}
	if len(miss) != 1 || miss[0] != "MISSING_VAR" {
		t.Errorf("missing report: got %v want [MISSING_VAR]", miss)
	}

	if s, _ := substituteVars("no vars"); s != "no vars" {
		t.Errorf("no-vars passthrough: got %q", s)
	}
}

package abi

import (
	"strings"
	"testing"
)

func TestParseDescribeTraceContexts(t *testing.T) {
	ok := `{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"orders","description":"line by line"},{"name":"mapping-2"}]}`
	d, err := ParseDescribe([]byte(ok))
	if err != nil {
		t.Fatal(err)
	}
	if !d.HasTraceContext("orders") || !d.HasTraceContext("mapping-2") || d.HasTraceContext("http") || d.HasTraceContext("other") {
		t.Fatalf("contexts = %+v", d.TraceContexts)
	}
	many := make([]string, MaxTraceContexts+1)
	for i := range many {
		many[i] = `{"name":"c` + strings.Repeat("a", i%5) + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + `"}`
	}
	cases := map[string]struct{ doc, want string }{
		"http is built in": {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"http"}]}`, "built-in"},
		"upper case":       {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"Orders"}]}`, "not a valid context name"},
		"underscore":       {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"a_b"}]}`, "not a valid context name"},
		"empty":            {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":""}]}`, "not a valid context name"},
		"too long":         {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"a` + strings.Repeat("b", 40) + `"}]}`, "not a valid context name"},
		"twice":            {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"a"},{"name":"a"}]}`, "declared twice"},
		"long description": {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"a","description":"` + strings.Repeat("x", MaxTraceContextDescLength+1) + `"}]}`, "description is over"},
		"too many":         {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[` + strings.Join(many, ",") + `]}`, "at most 20"},
		"unknown field":    {`{"abi":1,"endpoints":[{"path":"/x","auth":"none"}],"traceContexts":[{"name":"a","level":"info"}]}`, "unknown field"},
	}
	for name, c := range cases {
		_, err := ParseDescribe([]byte(c.doc))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want it to contain %q", name, err, c.want)
		}
	}
}

func TestTraceOpsAreNamed(t *testing.T) {
	if OpTrace.String() != "trace" || OpTracing.String() != "tracing" {
		t.Fatalf("names: %s %s", OpTrace, OpTracing)
	}
	if OpTrace != 12 || OpTracing != 13 || OpTraceReference != 14 {
		t.Fatal("op numbers are part of the ABI")
	}
	if OpTraceReference.String() != "trace.reference" {
		t.Fatalf("name: %s", OpTraceReference)
	}
}

//go:build !wasip1

package fn

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func traceErrHost(code string) *FakeHost {
	return &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: code, Message: "x"}, nil
	}}
}

func captureTraceStderr(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := traceStderr
	traceStderr = buf
	warnedTrace.Clear()
	t.Cleanup(func() { traceStderr = prev; warnedTrace.Clear() })
	return buf
}

func lastTraceMeta(t *testing.T, fake *FakeHost) traceMeta {
	t.Helper()
	if len(fake.Calls) == 0 {
		t.Fatal("no host call made")
	}
	c := fake.Calls[len(fake.Calls)-1]
	if c.Op != int32(opTrace) {
		t.Fatalf("op = %d, want trace", c.Op)
	}
	var m traceMeta
	if err := json.Unmarshal(c.Meta, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestTraceSendsOp12(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()

	Trace(context.Background(), "orders", "resolved", map[string]any{"lines": 3}, WithLevel(TraceWarn), WithReference("TRIP-1"))

	m := lastTraceMeta(t, fake)
	if m.Context != "orders" || m.Msg != "resolved" || m.Level != TraceWarn || m.Reference != "TRIP-1" || m.Data["lines"] != float64(3) {
		t.Fatalf("meta = %+v", m)
	}
}

func TestTraceOmitsUnsetOptionalFields(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()

	Trace(context.Background(), "orders", "hi", nil)

	if got := string(fake.Calls[0].Meta); got != `{"context":"orders","msg":"hi"}` {
		t.Fatalf("meta = %s", got)
	}
}

func TestSetReferenceStampsLaterCalls(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()
	ctx := context.WithValue(context.Background(), ctxKeyTrace, &traceState{})

	Trace(ctx, "orders", "before", nil)
	SetReference(ctx, "ORD-9")
	Trace(ctx, "orders", "after", nil)
	Trace(ctx, "orders", "override", nil, WithReference("ORD-10"))

	refs := make([]string, 0, 3)
	for _, c := range fake.Calls {
		if c.Op != int32(opTrace) {
			continue
		}
		var m traceMeta
		_ = json.Unmarshal(c.Meta, &m)
		refs = append(refs, m.Reference)
	}
	if refs[0] != "" || refs[1] != "ORD-9" || refs[2] != "ORD-10" {
		t.Fatalf("references = %q", refs)
	}
}

func TestSetReferenceOutsideAnInvocationIsHarmless(t *testing.T) {
	SetReference(context.Background(), "x") // must not panic
}

func TestTraceCutsOverLongMessageAndReference(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()

	Trace(context.Background(), "orders", strings.Repeat("é", 3000), nil, WithReference(strings.Repeat("r", 500)))

	m := lastTraceMeta(t, fake)
	if len(m.Msg) > traceMaxMessage || !utf8.ValidString(m.Msg) {
		t.Fatalf("message not cut on a character boundary: %d bytes, valid=%v", len(m.Msg), utf8.ValidString(m.Msg))
	}
	if utf8.RuneCountInString(m.Reference) != traceMaxReference {
		t.Fatalf("reference = %d chars", utf8.RuneCountInString(m.Reference))
	}
}

func TestTraceNeverFailsAndWarnsOncePerUndeclaredContext(t *testing.T) {
	buf := captureTraceStderr(t)

	for _, code := range []string{"UNKNOWN_OP", ErrCodeTooLarge, ErrCodeBadRequest, ErrCodeUnavailable} {
		restore := SetHost(traceErrHost(code))
		Trace(context.Background(), "orders", "m", map[string]any{"a": 1})
		restore()
	}
	if buf.Len() != 0 {
		t.Fatalf("unexpected warning: %q", buf.String())
	}

	defer SetHost(traceErrHost(ErrCodeNotDeclared))()
	Trace(context.Background(), "nope", "a", nil)
	Trace(context.Background(), "nope", "b", nil)
	Tracing(context.Background(), "nope")
	Trace(context.Background(), "other", "c", nil)
	if n := strings.Count(buf.String(), "is not declared"); n != 2 {
		t.Fatalf("warnings = %d, want 2: %q", n, buf.String())
	}
}

func TestTraceSwallowsTransportErrors(t *testing.T) {
	defer SetHost(&FakeHost{Handler: func(int32, []byte, []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, nil, context.Canceled
	}})()
	Trace(context.Background(), "orders", "m", nil)
	if Tracing(context.Background(), "orders") {
		t.Fatal("Tracing = true on a transport error")
	}
}

func TestTracingSendsOp13AndReturnsLive(t *testing.T) {
	live := false
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opTracing) {
			t.Fatalf("op = %d, want tracing", op)
		}
		var m tracingMeta
		_ = json.Unmarshal(meta, &m)
		if m.Context != "orders" {
			t.Fatalf("context = %q", m.Context)
		}
		rm, _ := json.Marshal(tracingResult{Live: live})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	if Tracing(context.Background(), "orders") {
		t.Fatal("Tracing = true, want false")
	}
	live = true // the next invocation was sampled in: nothing may be cached
	if !Tracing(context.Background(), "orders") {
		t.Fatal("Tracing = false, want true")
	}
	if len(fake.Calls) != 2 {
		t.Fatalf("host calls = %d, want 2 (no caching)", len(fake.Calls))
	}
}

func TestTracingIsFalseOnEmptyResultAndErrors(t *testing.T) {
	restore := SetHost(&FakeHost{})
	if Tracing(context.Background(), "orders") {
		t.Fatal("Tracing = true on an empty result")
	}
	restore()
	captureTraceStderr(t)
	for _, code := range []string{"UNKNOWN_OP", ErrCodeNotDeclared, ErrCodeUnavailable} {
		restore := SetHost(traceErrHost(code))
		if Tracing(context.Background(), "orders") {
			t.Fatalf("Tracing = true on %s", code)
		}
		restore()
	}
}

func TestTraceContextsDescribeKey(t *testing.T) {
	resetRegistry()
	if strings.Contains(string(describeJSON()), "traceContexts") {
		t.Fatal("traceContexts present with none declared")
	}
	TraceContexts(TraceContext{Name: "orders", Description: "What the mapping decided."}, TraceContext{Name: "retry-3"})
	assertJSONEqual(t, describeJSON(), []byte(`{"abi":1,"endpoints":[],"traceContexts":[{"name":"orders","description":"What the mapping decided."},{"name":"retry-3"}]}`))
}

func TestTraceContextsRejectBadDeclarations(t *testing.T) {
	cases := map[string]struct {
		c    TraceContext
		want string
	}{
		"http":        {TraceContext{Name: "http"}, "built-in capture context"},
		"uppercase":   {TraceContext{Name: "Orders"}, "not a valid context name"},
		"digit first": {TraceContext{Name: "1x"}, "not a valid context name"},
		"too long":    {TraceContext{Name: strings.Repeat("a", 41)}, "not a valid context name"},
		"description": {TraceContext{Name: "ok", Description: strings.Repeat("d", 201)}, "description is over 200 bytes"},
		"description multibyte": {TraceContext{Name: "ok", Description: strings.Repeat("é", 101)}, "description is over 200 bytes"},
	}
	for name, tc := range cases {
		resetRegistry()
		func() {
			defer func() {
				msg, _ := recover().(string)
				if !strings.Contains(msg, tc.want) {
					t.Errorf("%s: panic = %q, want %q", name, msg, tc.want)
				}
			}()
			TraceContexts(tc.c)
		}()
		if len(reg.traceCtxs) != 0 {
			t.Errorf("%s: a refused declaration was registered", name)
		}
	}

	resetRegistry()
	TraceContexts(TraceContext{Name: "dup"})
	func() {
		defer func() {
			if msg, _ := recover().(string); !strings.Contains(msg, "declared twice") {
				t.Errorf("duplicate: panic = %q", msg)
			}
		}()
		TraceContexts(TraceContext{Name: "dup"})
	}()

	resetRegistry()
	for i := range 20 {
		TraceContexts(TraceContext{Name: "c" + string(rune('a'+i))})
	}
	func() {
		defer func() {
			if msg, _ := recover().(string); !strings.Contains(msg, "at most 20") {
				t.Errorf("limit: panic = %q", msg)
			}
		}()
		TraceContexts(TraceContext{Name: "one-more"})
	}()
}

func TestSetReferenceMakesOp14Call(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()
	ctx := context.WithValue(context.Background(), ctxKeyTrace, &traceState{})

	SetReference(ctx, "TRIP-123")
	SetReference(ctx, strings.Repeat("é", 300))

	if len(fake.Calls) != 2 || fake.Calls[0].Op != int32(opTraceReference) {
		t.Fatalf("calls = %+v", fake.Calls)
	}
	if got := string(fake.Calls[0].Meta); got != `{"reference":"TRIP-123"}` {
		t.Fatalf("meta = %s", got)
	}
	var m traceReferenceMeta
	_ = json.Unmarshal(fake.Calls[1].Meta, &m)
	if utf8.RuneCountInString(m.Reference) != traceMaxReference {
		t.Fatalf("reference = %d chars", utf8.RuneCountInString(m.Reference))
	}
}

func TestSetReferenceSwallowsHostErrors(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxKeyTrace, &traceState{})
	for _, code := range []string{"UNKNOWN_OP", ErrCodeTooLarge, ErrCodeBadRequest} {
		restore := SetHost(traceErrHost(code))
		SetReference(ctx, "R") // must not panic
		restore()
	}
	restore := SetHost(&FakeHost{Handler: func(int32, []byte, []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, nil, context.Canceled
	}})
	SetReference(ctx, "R")
	restore()
	fake := &FakeHost{}
	defer SetHost(fake)()
	Trace(ctx, "orders", "m", nil)
	if lastTraceMeta(t, fake).Reference != "R" {
		t.Fatal("remembered value lost after a host error")
	}
}

func TestSetReferenceOutsideInvocationMakesNoHostCall(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()
	SetReference(context.Background(), "x")
	if len(fake.Calls) != 0 {
		t.Fatalf("host calls = %d, want 0", len(fake.Calls))
	}
}

func TestPerCallReferenceDoesNotStick(t *testing.T) {
	fake := &FakeHost{}
	defer SetHost(fake)()
	ctx := context.WithValue(context.Background(), ctxKeyTrace, &traceState{})

	Trace(ctx, "orders", "one", nil, WithReference("ONE"))
	Trace(ctx, "orders", "two", nil)
	SetReference(ctx, "SET")
	Trace(ctx, "orders", "three", nil, WithReference("THREE"))
	Trace(ctx, "orders", "four", nil)

	var refs []string
	for _, c := range fake.Calls {
		if c.Op != int32(opTrace) {
			continue
		}
		var m traceMeta
		_ = json.Unmarshal(c.Meta, &m)
		refs = append(refs, m.Reference)
	}
	if len(refs) != 4 || refs[0] != "ONE" || refs[1] != "" || refs[2] != "THREE" || refs[3] != "SET" {
		t.Fatalf("references = %q", refs)
	}
}

func TestTraceContextDescriptionCountsBytes(t *testing.T) {
	resetRegistry()
	TraceContexts(TraceContext{Name: "ok", Description: strings.Repeat("é", 100)}) // 200 bytes
	if len(reg.traceCtxs) != 1 {
		t.Fatal("200-byte description rejected")
	}
}

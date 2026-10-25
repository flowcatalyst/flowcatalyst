package fn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sync"
	"unicode/utf8"
)

// Trace logs (host ops 12 trace / 13 tracing; docs/function-trace-design.md
// sections 2 and 9). A context is a named trace point the function declares
// with TraceContexts; it records nothing unless an operator has switched it on
// for a limited time. Trace and Tracing NEVER fail: a runner without the ops
// (UNKNOWN_OP), NOT_DECLARED, TOO_LARGE or any other host error is swallowed,
// so tracing can never fail an invocation. NOT_DECLARED additionally writes
// one warning per context to stderr, since it is the author's mistake.

// TraceLevel is the severity of a trace record.
type TraceLevel string

const (
	TraceDebug TraceLevel = "debug"
	TraceInfo  TraceLevel = "info"
	TraceWarn  TraceLevel = "warn"
	TraceError TraceLevel = "error"
)

// Limits (internal/functions/abi: TraceMaxMessage, TraceMaxReference,
// MaxTraceContexts, MaxTraceContextDescLength).
const (
	traceMaxMessage   = 4096
	traceMaxReference = 200
	maxTraceContexts  = 20
	maxTraceDescBytes = 200
)

var traceContextPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// TraceContext declares one trace context. Name is a lowercase letter first,
// then lowercase letters, digits or '-', at most 40 characters, and not "http"
// (the built-in capture context). Description (at most 200 bytes of UTF-8) tells
// operators what the context records.
type TraceContext struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// TraceContexts declares the trace contexts this function writes to (describe
// key "traceContexts"). Call it from init(). A bad declaration panics at
// registration, so build, describe and deploy all fail with the message.
func TraceContexts(contexts ...TraceContext) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	for _, c := range contexts {
		at := fmt.Sprintf("traceContexts[%d]", len(reg.traceCtxs))
		switch {
		case c.Name == "http":
			panic(fmt.Sprintf("fn: %s: %q is the built-in capture context and cannot be declared", at, c.Name))
		case !traceContextPattern.MatchString(c.Name):
			panic(fmt.Sprintf("fn: %s: %q is not a valid context name (lowercase letter first, then lowercase letters, digits or -; at most 40 characters)", at, c.Name))
		case len(c.Description) > maxTraceDescBytes:
			panic(fmt.Sprintf("fn: %s: description is over %d bytes", at, maxTraceDescBytes))
		case len(reg.traceCtxs) >= maxTraceContexts:
			panic(fmt.Sprintf("fn: traceContexts: at most %d contexts", maxTraceContexts))
		}
		for _, x := range reg.traceCtxs {
			if x.Name == c.Name {
				panic(fmt.Sprintf("fn: %s: %q is declared twice", at, c.Name))
			}
		}
		reg.traceCtxs = append(reg.traceCtxs, c)
	}
}

// traceState is the per-invocation trace state carried in the request context.
type traceState struct {
	mu        sync.Mutex
	reference string
}

func (s *traceState) ref() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reference
}

// SetReference stamps every later record of this invocation with ref (the
// author's business key: trip number, order number): the author's Trace calls
// and, on a runner that knows host op 14, the built-in http and log records
// too. The value is also remembered here so Trace calls still carry it to an
// older runner. A call's own WithReference applies to that record only. It
// never fails: a host error (UNKNOWN_OP, TOO_LARGE, ...) is swallowed. Outside
// a dispatched request it does nothing.
func SetReference(ctx context.Context, ref string) {
	s, ok := ctx.Value(ctxKeyTrace).(*traceState)
	if !ok {
		return
	}
	s.mu.Lock()
	s.reference = ref
	s.mu.Unlock()
	if r := []rune(ref); len(r) > traceMaxReference {
		ref = string(r[:traceMaxReference])
	}
	mb, err := json.Marshal(traceReferenceMeta{Reference: ref})
	if err != nil {
		return
	}
	_, _, _, _ = currentHost.call(opTraceReference, mb, nil)
}

type traceReferenceMeta struct {
	Reference string `json:"reference"`
}

// TraceOption adjusts one Trace call.
type TraceOption func(*traceOpts)

type traceOpts struct {
	level     TraceLevel
	reference string
}

// WithLevel sets the record's level (default TraceInfo).
func WithLevel(l TraceLevel) TraceOption { return func(o *traceOpts) { o.level = l } }

// WithReference sets the business key for this record, overriding the
// invocation's SetReference value for this call only.
func WithReference(r string) TraceOption { return func(o *traceOpts) { o.reference = r } }

type traceMeta struct {
	Context   string         `json:"context"`
	Level     TraceLevel     `json:"level,omitempty"`
	Msg       string         `json:"msg"`
	Data      map[string]any `json:"data,omitempty"`
	Reference string         `json:"reference,omitempty"`
}

type tracingMeta struct {
	Context string `json:"context"`
}

type tracingResult struct {
	Live bool `json:"live"`
}

// traceStderr is where NOT_DECLARED warnings go (a variable so tests can capture it).
var traceStderr io.Writer = os.Stderr

// warnedTrace remembers which undeclared contexts were already reported.
var warnedTrace sync.Map

func warnNotDeclared(context string) {
	if _, dup := warnedTrace.LoadOrStore(context, struct{}{}); dup {
		return
	}
	fmt.Fprintf(traceStderr, "fn: trace context %q is not declared; add it with fn.TraceContexts (records for it are dropped)\n", context)
}

// clipUTF8 cuts s to at most max bytes without splitting a character.
func clipUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// Trace records msg (and optional data) under context. It is a silent no-op
// unless an operator has switched the context on and this invocation was
// sampled in; ask Tracing first to skip expensive work. It never fails.
func Trace(ctx context.Context, name, msg string, data map[string]any, opts ...TraceOption) {
	var o traceOpts
	for _, opt := range opts {
		opt(&o)
	}
	if o.reference == "" {
		if s, ok := ctx.Value(ctxKeyTrace).(*traceState); ok {
			o.reference = s.ref()
		}
	}
	meta := traceMeta{Context: name, Level: o.level, Msg: clipUTF8(msg, traceMaxMessage), Data: data}
	if r := []rune(o.reference); len(r) > traceMaxReference {
		meta.Reference = string(r[:traceMaxReference])
	} else {
		meta.Reference = o.reference
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		return // data that does not encode is dropped, never an error
	}
	_, _, hostErr, _ := currentHost.call(opTrace, mb, nil)
	if hostErr != nil && hostErr.Code == ErrCodeNotDeclared {
		warnNotDeclared(name)
	}
}

// Tracing reports whether context is switched on now and this invocation was
// sampled in. The answer is not cached: it depends on the invocation. It
// returns false on any failure, including a runner that predates the op.
func Tracing(_ context.Context, name string) bool {
	mb, err := json.Marshal(tracingMeta{Context: name})
	if err != nil {
		return false
	}
	rm, _, hostErr, err := currentHost.call(opTracing, mb, nil)
	if hostErr != nil {
		if hostErr.Code == ErrCodeNotDeclared {
			warnNotDeclared(name)
		}
		return false
	}
	if err != nil || len(rm) == 0 {
		return false
	}
	var out tracingResult
	if json.Unmarshal(rm, &out) != nil {
		return false
	}
	return out.Live
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// Limits the real dispatch route enforces
// (internal/platform/function/control/dispatch.go).
const (
	devMaxDispatchPayload = 256 << 10
	devMaxMetadata        = 50
	devMaxMetaKey         = 100
	devMaxMetaValue       = 1000
	devMaxCorrelation     = 100
)

// validateDispatch applies the real route's declaration and shape rules to a
// dispatch request, against the describe of the version that made it, and
// returns the matching declared entry. Errors are the ABI errors the real
// route's answers map to (403 -> NOT_ALLOWED, 400 -> BAD_REQUEST). It cannot
// check what only a platform knows: that the target function, pool and
// connection exist.
func validateDispatch(d *abi.Describe, r control.DispatchRequest) (abi.DispatchTarget, *abi.Error) {
	req := r.Dispatch
	if strings.TrimSpace(req.Target) == "" || strings.TrimSpace(req.Path) == "" {
		return abi.DispatchTarget{}, abi.Errorf(abi.CodeBadRequest, "dispatch.target and dispatch.path are required")
	}
	var declared abi.DispatchTarget
	ok := false
	if d != nil {
		declared, ok = d.DispatchFor(req.Target, req.Path)
	}
	if !ok {
		return abi.DispatchTarget{}, abi.Errorf(abi.CodeNotAllowed, fmt.Sprintf(
			"dispatch target %s%s is not declared in this version's dispatches (declared: %s)",
			req.Target, req.Path, declaredDispatches(d)))
	}
	bad := func(msg string) (abi.DispatchTarget, *abi.Error) {
		return abi.DispatchTarget{}, abi.Errorf(abi.CodeBadRequest, msg)
	}
	if len(r.Payload) > devMaxDispatchPayload {
		return bad("the dispatch payload is over 256 KiB")
	}
	if !utf8.Valid(r.Payload) {
		return bad("the dispatch payload must be UTF-8 text")
	}
	ct := strings.TrimSpace(req.ContentType)
	if ct == "" {
		ct = "application/json"
	}
	if isJSONContentType(ct) && !json.Valid(r.Payload) {
		return bad("the dispatch payload is not valid JSON for content type " + ct)
	}
	if code := strings.TrimSpace(req.Code); code != "" && (strings.ContainsAny(code, " \t\r\n") || len(code) > 255) {
		return bad("code must contain no whitespace and be at most 255 characters")
	}
	switch common.DispatchMode(req.Mode) {
	case "", common.DispatchImmediate, common.DispatchNextOnError, common.DispatchBlockOnError:
	default:
		return bad("mode must be IMMEDIATE, NEXT_ON_ERROR or BLOCK_ON_ERROR")
	}
	if req.MaxRetries != nil && *req.MaxRetries < 0 {
		return bad("maxRetries must not be negative")
	}
	if req.TimeoutSeconds != nil && *req.TimeoutSeconds <= 0 {
		return bad("timeoutSeconds must be positive")
	}
	if len(req.Metadata) > devMaxMetadata {
		return bad("metadata is over 50 entries")
	}
	seen := map[string]bool{}
	for _, kv := range req.Metadata {
		k := strings.TrimSpace(kv.Key)
		switch {
		case k == "":
			return bad("a metadata key is empty")
		case len(k) > devMaxMetaKey || len(kv.Value) > devMaxMetaValue:
			return bad("metadata key " + k + ": key over 100 or value over 1000 characters")
		case strings.HasPrefix(strings.ToLower(k), "fn."):
			return bad("metadata keys starting with fn. are the platform's")
		case seen[k]:
			return bad("metadata key " + k + " appears twice")
		}
		seen[k] = true
	}
	corr := strings.TrimSpace(req.CorrelationID)
	if corr == "" {
		corr = strings.TrimSpace(r.RequestCorrelationID)
	}
	if len(corr) > devMaxCorrelation {
		return bad("correlationId is over 100 characters")
	}
	return declared, nil
}

func declaredDispatches(d *abi.Describe) string {
	if d == nil || len(d.Dispatches) == 0 {
		return "none"
	}
	var parts []string
	for _, t := range d.Dispatches {
		parts = append(parts, t.Target+t.Path)
	}
	return strings.Join(parts, ", ")
}

func isJSONContentType(ct string) bool {
	mt, _, _ := strings.Cut(strings.ToLower(ct), ";")
	mt = strings.TrimSpace(mt)
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

// describeWouldCreate renders the "would create job" line for a declared,
// validated dispatch.
func describeWouldCreate(declared abi.DispatchTarget, r control.DispatchRequest) string {
	pool := declared.Pool
	if pool == "" {
		pool = "default"
	}
	group := r.Dispatch.MessageGroup
	if group == "" {
		group = "-"
	}
	return fmt.Sprintf("would create job: target=%s path=%s pool=%s connection=%s group=%s payload=%s",
		declared.Target, declared.Path, pool, declared.Connection, group, boundedText(r.Payload, 200))
}

// boundedText is b as one line of at most max characters.
func boundedText(b []byte, max int) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > max {
		s = s[:max] + fmt.Sprintf("… (%d bytes)", len(b))
	}
	return s
}

// devRecorder collects the events and dispatches a call produces. While a
// request is collecting they are returned to the caller to print grouped
// under it; otherwise they print as they happen.
type devRecorder struct {
	mu     sync.Mutex
	active bool
	lines  []string
	print  func(string)
}

func (r *devRecorder) note(line string) {
	r.mu.Lock()
	if r.active {
		r.lines = append(r.lines, line)
		r.mu.Unlock()
		return
	}
	r.mu.Unlock()
	r.print(line)
}

func (r *devRecorder) begin() {
	r.mu.Lock()
	r.active, r.lines = true, nil
	r.mu.Unlock()
}

func (r *devRecorder) end() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = false
	out := r.lines
	r.lines = nil
	return out
}

// devCP is the local control plane `fn dev` serves: localCP plus validated
// dispatches, noted events, and the runner's own report of when a reloaded
// version is live.
type devCP struct {
	*localCP
	rec *devRecorder
	seq atomic.Int64

	mu     sync.Mutex
	desc   *abi.Describe
	states map[int]versionState
}

type versionState struct{ state, reason string }

func newDevCP(l *localCP, rec *devRecorder) *devCP {
	return &devCP{localCP: l, rec: rec, states: map[int]versionState{}}
}

// apply makes the artifact at path the live version; on failure the previous
// version (and path) stay.
func (c *devCP) apply(ctx context.Context, path, runtime string) (*loadInfo, error) {
	oldPath, oldRT := c.path, c.runtime
	c.path, c.runtime = path, runtime
	li, err := c.reload(ctx)
	if err != nil {
		c.path, c.runtime = oldPath, oldRT
		return nil, err
	}
	c.mu.Lock()
	c.desc = li.Describe
	c.mu.Unlock()
	return li, nil
}

func (c *devCP) current() *abi.Describe {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.desc
}

// Heartbeat records which versions the runner has loaded or failed.
func (c *devCP) Heartbeat(_ context.Context, hb control.Heartbeat) error {
	c.mu.Lock()
	for _, v := range hb.Versions {
		if v.FunctionID == c.fn.ID {
			c.states[v.Number] = versionState{v.State, v.Reason}
		}
	}
	c.mu.Unlock()
	return nil
}

// waitLoaded blocks until the runner reports version n loaded, or fails.
func (c *devCP) waitLoaded(ctx context.Context, n int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		c.mu.Lock()
		st := c.states[n]
		c.mu.Unlock()
		switch st.state {
		case control.StateLoaded:
			return nil
		case control.StateFailed:
			return fmt.Errorf("the runner could not load v%d: %s", n, st.reason)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("v%d was not loaded within %s", n, timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (c *devCP) Emit(_ context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error) {
	line := "emit  type=" + r.Event.Type
	if r.Event.Subject != "" {
		line += " subject=" + r.Event.Subject
	}
	if r.Event.MessageGroup != "" {
		line += " group=" + r.Event.MessageGroup
	}
	line += " data=" + boundedText(r.Data, 200)
	c.rec.note(line)
	return &control.EmitResponse{EventID: "evt_local_" + strconv.FormatInt(c.seq.Add(1), 10)}, nil
}

func (c *devCP) Dispatch(_ context.Context, r control.DispatchRequest) (*control.DispatchResponse, *abi.Error) {
	declared, aerr := validateDispatch(c.current(), r)
	if aerr != nil {
		c.rec.note("dispatch refused: " + aerr.Error())
		return nil, aerr
	}
	c.rec.note(describeWouldCreate(declared, r))
	return &control.DispatchResponse{JobID: "djob_local_" + strconv.FormatInt(c.seq.Add(1), 10)}, nil
}

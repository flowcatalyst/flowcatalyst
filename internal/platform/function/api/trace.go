package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// Trace-log read limits (design section 7).
const (
	defaultTraceLogLimit = 100
	maxTraceLogLimit     = 500
)

// TraceContextResponse is one trace context's state.
type TraceContextResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	BuiltIn     bool   `json:"builtIn" doc:"True for the built-in http context"`
	// EnabledUntil is when the window closes; null when it was never switched on or was switched off.
	EnabledUntil *httpcompat.Time `json:"enabledUntil" nullable:"true"`
	SampleRate   int              `json:"sampleRate" doc:"One invocation in N is captured; 1 = all"`
	Live         bool             `json:"live" doc:"The window is open now (enabledUntil is in the future)"`
	MaxMinutes   int              `json:"maxMinutes" doc:"The longest window this context may be enabled for"`
}

func traceContextResponse(st operations.TraceContextState) TraceContextResponse {
	out := TraceContextResponse{
		Name: st.Name, Description: st.Description, BuiltIn: st.BuiltIn,
		SampleRate: st.SampleRate, Live: st.Live, MaxMinutes: st.MaxMinutes,
	}
	if st.EnabledUntil != nil {
		t := jsontime.New(st.EnabledUntil.UTC())
		out.EnabledUntil = &t
	}
	return out
}

// TraceContextsResponse is the body of GET /api/functions/{id}/trace.
type TraceContextsResponse struct {
	Contexts []TraceContextResponse `json:"contexts"`
}

// EnableTraceRequest is the wire body for PUT …/trace/{context}.
type EnableTraceRequest struct {
	Minutes    int `json:"minutes" doc:"How long the window stays open, from 1 up to the context's maxMinutes"`
	SampleRate int `json:"sampleRate,omitempty" doc:"Capture one invocation in N (1 to 1000). Default 1: all."`
}

// TraceRecordResponse is one trace-log record.
type TraceRecordResponse struct {
	ID            string          `json:"id"`
	At            httpcompat.Time `json:"at"`
	Version       int32           `json:"version"`
	InvocationID  string          `json:"invocationId"`
	CorrelationID *string         `json:"correlationId" nullable:"true"`
	Reference     *string         `json:"reference" nullable:"true"`
	Context       string          `json:"context"`
	Kind          string          `json:"kind" enum:"trace,http,log"`
	Level         string          `json:"level"`
	Message       string          `json:"message"`
	Data          json.RawMessage `json:"data" doc:"The record's data. For kind http: request, response, durationMs, outcome."`
	RunnerID      *string         `json:"runnerId" nullable:"true"`
	// BodiesRedacted is true on an http record whose request and response bodies
	// were withheld because the caller lacks platform:function:trace:view-bodies.
	BodiesRedacted bool `json:"bodiesRedacted,omitempty"`
}

// TraceLogResponse is the body of GET /api/functions/{id}/trace-log.
type TraceLogResponse struct {
	Records    []TraceRecordResponse `json:"records"`
	NextCursor *string               `json:"nextCursor" nullable:"true" doc:"Pass as cursor to fetch the next (older) page; null on the last page"`
}

func (s *State) traceLimits() function.TraceLimits { return s.Trace.Normalised() }

type getTraceInput struct {
	ID string `path:"id"`
}

func (s *State) getTrace(ctx context.Context, in *getTraceInput) (*apicommon.Out[TraceContextsResponse], error) {
	if err := auth.CanViewFunctionTrace(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, states, err := operations.ListTraceContexts(ctx, s.Repo, s.traceLimits(), in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[TraceContextsResponse]{Body: TraceContextsResponse{
		Contexts: apicommon.MapSlice(states, func(st *operations.TraceContextState) TraceContextResponse { return traceContextResponse(*st) }),
	}}, nil
}

type putTraceInput struct {
	ID      string `path:"id"`
	Context string `path:"context"`
	Body    EnableTraceRequest
}

// putTrace switches a context on for a bounded time. `http` captures request
// and response bodies, so enabling it also needs view-bodies.
func (s *State) putTrace(ctx context.Context, in *putTraceInput) (*apicommon.Out[TraceContextResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanManageFunctionTrace(ac); err != nil {
		return nil, err
	}
	if in.Context == function.TraceHTTPContext {
		if err := auth.RequireFunctionTraceBodies(ac); err != nil {
			return nil, err
		}
	}
	ec := auth.NewExecutionContext(ctx)
	limits := s.traceLimits()
	cmd := operations.EnableTraceCommand{
		FunctionID: in.ID, Context: in.Context, Minutes: in.Body.Minutes, SampleRate: in.Body.SampleRate,
	}
	st, err := usecaseop.RunTx(ctx, s.UoW, operations.EnableTraceContext(s.Repo, limits), cmd, ec)
	if err != nil {
		return nil, err
	}
	// The enable has committed. Pruning old records is housekeeping after it.
	operations.PruneTraceAfterEnable(ctx, s.Repo, limits, in.ID)
	return &apicommon.Out[TraceContextResponse]{Body: traceContextResponse(st)}, nil
}

type deleteTraceInput struct {
	ID      string `path:"id"`
	Context string `path:"context"`
}

func (s *State) deleteTrace(ctx context.Context, in *deleteTraceInput) (*apicommon.Empty, error) {
	if err := auth.CanManageFunctionTrace(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.DisableTraceCommand{FunctionID: in.ID, Context: in.Context}
	if _, err := usecaseop.RunTx(ctx, s.UoW, operations.DisableTraceContext(s.Repo), cmd, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type traceLogInput struct {
	ID            string `path:"id"`
	Context       string `query:"context" doc:"Only this context"`
	Kind          string `query:"kind" enum:"trace,http,log" doc:"Only this record kind"`
	Level         string `query:"level" doc:"Only this level (debug, info, warn, error)"`
	Reference     string `query:"reference" doc:"The author's business key, exact match"`
	CorrelationID string `query:"correlationId" doc:"The request's X-Correlation-Id, exact match"`
	InvocationID  string `query:"invocationId" doc:"One invocation's records"`
	Version       int32  `query:"version" doc:"Only this function version"`
	Since         string `query:"since" doc:"Oldest record time: RFC 3339 or unix milliseconds"`
	Until         string `query:"until" doc:"Newest record time: RFC 3339 or unix milliseconds"`
	Limit         int    `query:"limit" doc:"Page size, default 100, max 500"`
	Cursor        string `query:"cursor" doc:"nextCursor of the previous page"`
}

func (s *State) getTraceLog(ctx context.Context, in *traceLogInput) (*apicommon.Out[TraceLogResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanViewFunctionTrace(ac); err != nil {
		return nil, err
	}
	f, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, httperror.NotFound("Function", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, f.ClientID); err != nil {
		return nil, err
	}

	filter := function.TraceLogFilter{
		FunctionID: f.ID, Context: in.Context, Kind: in.Kind, Level: in.Level, Reference: in.Reference,
		CorrelationID: in.CorrelationID, InvocationID: in.InvocationID, Version: in.Version,
	}
	if filter.Since, err = parseTraceTime("since", in.Since); err != nil {
		return nil, err
	}
	if filter.Until, err = parseTraceTime("until", in.Until); err != nil {
		return nil, err
	}
	if filter.After, err = decodeTraceCursor(in.Cursor); err != nil {
		return nil, err
	}
	limit := in.Limit
	switch {
	case limit <= 0:
		limit = defaultTraceLogLimit
	case limit > maxTraceLogLimit:
		limit = maxTraceLogLimit
	}
	filter.Limit = limit + 1 // one extra row says whether another page exists

	recs, err := s.Repo.QueryTraceLog(ctx, filter)
	if err != nil {
		return nil, usecase.Internal("REPO", "query_trace_log failed", err)
	}
	var next *string
	if len(recs) > limit {
		recs = recs[:limit]
		last := recs[len(recs)-1]
		c := encodeTraceCursor(function.TraceCursor{At: last.At, ID: last.ID})
		next = &c
	}
	withBodies := auth.CanViewFunctionTraceBodies(ac)
	out := make([]TraceRecordResponse, 0, len(recs))
	for _, r := range recs {
		out = append(out, traceRecordResponse(r, withBodies))
	}
	return &apicommon.Out[TraceLogResponse]{Body: TraceLogResponse{Records: out, NextCursor: next}}, nil
}

func traceRecordResponse(r function.TraceLogRecord, withBodies bool) TraceRecordResponse {
	out := TraceRecordResponse{
		ID: r.ID, At: jsontime.New(r.At.UTC()), Version: r.Version, InvocationID: r.InvocationID,
		CorrelationID: r.CorrelationID, Reference: r.Reference, Context: r.Context, Kind: r.Kind,
		Level: r.Level, Message: r.Message, Data: json.RawMessage(r.Data), RunnerID: r.RunnerID,
	}
	if len(out.Data) == 0 {
		out.Data = json.RawMessage("null")
	}
	if r.Kind == "http" && !withBodies {
		out.Data = redactTraceBodies(out.Data)
		out.BodiesRedacted = true
	}
	return out
}

// redactTraceBodies nulls data.request.body and data.response.body of an http
// record; everything else in the record (headers, sizes, status, outcome)
// stays readable.
func redactTraceBodies(data json.RawMessage) json.RawMessage {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
		return data
	}
	for _, side := range []string{"request", "response"} {
		raw, ok := doc[side]
		if !ok {
			continue
		}
		var part map[string]json.RawMessage
		if err := json.Unmarshal(raw, &part); err != nil || part == nil {
			continue
		}
		part["body"] = json.RawMessage("null")
		if b, err := json.Marshal(part); err == nil {
			doc[side] = b
		}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}

// parseTraceTime accepts RFC 3339 or unix milliseconds; empty is no bound.
func parseTraceTime(name, v string) (*time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		t := time.UnixMilli(ms).UTC()
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil, usecase.Validation("TRACE_TIME_INVALID", name+" must be RFC 3339 or unix milliseconds")
	}
	t = t.UTC()
	return &t, nil
}

// The cursor is the keyset position as base64url("<unix micros>:<id>").
func encodeTraceCursor(c function.TraceCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixMicro(), 10) + ":" + c.ID))
}

func decodeTraceCursor(v string) (*function.TraceCursor, error) {
	if v == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(v)
	if err != nil {
		return nil, usecase.Validation("TRACE_CURSOR_INVALID", "cursor is not a cursor this API issued")
	}
	micros, id, ok := strings.Cut(string(raw), ":")
	n, perr := strconv.ParseInt(micros, 10, 64)
	if !ok || perr != nil || id == "" {
		return nil, usecase.Validation("TRACE_CURSOR_INVALID", "cursor is not a cursor this API issued")
	}
	return &function.TraceCursor{At: time.UnixMicro(n).UTC(), ID: id}, nil
}

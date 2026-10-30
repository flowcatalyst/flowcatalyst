package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Ingestion traces (docs/ingestion-trace-design.md section 7).

func registerTrace(g apiroute.Group, s *State) {
	apiroute.Get(g, "getIngestionTrace", "/api/ingestions/{id}/trace", "Read an ingestion's trace window: whether tracing is on and until when", s.getTrace)
	apiroute.Put(g, "enableIngestionTrace", "/api/ingestions/{id}/trace", "Switch tracing on: a new window every runtime that runs the ingestion captures under", 200, s.putTrace)
	apiroute.Delete(g, "disableIngestionTrace", "/api/ingestions/{id}/trace", "Switch tracing off now (idempotent)", 204, s.deleteTrace)
	apiroute.Get(g, "listIngestionTraceLog", "/api/ingestions/{id}/trace-log", "List the trace records runtimes captured for an ingestion, newest first", s.getTraceLog)
}

// IngestionTraceStateResponse is an ingestion's trace window. Every field but live and
// maxWindowSecs is null when tracing was never switched on.
type IngestionTraceStateResponse struct {
	Live            bool             `json:"live" doc:"The window is open now (hardStop is in the future)"`
	WindowID        *string          `json:"windowId" nullable:"true" doc:"A new id every time tracing is switched on"`
	WindowSecs      *int             `json:"windowSecs" nullable:"true" doc:"How long each runtime captures, counted from when it first sees the window"`
	SampleEverySecs *int             `json:"sampleEverySecs" nullable:"true" doc:"A runtime starts a capture of this ingestion at most this often"`
	EnabledAt       *httpcompat.Time `json:"enabledAt" nullable:"true"`
	HardStop        *httpcompat.Time `json:"hardStop" nullable:"true" doc:"No runtime captures past this; switching off sets it to the switch-off time"`
	EnabledBy       *string          `json:"enabledBy" nullable:"true"`
	MaxWindowSecs   int              `json:"maxWindowSecs" doc:"The longest window that may be switched on"`
}

func traceStateResponse(st operations.TraceState) IngestionTraceStateResponse {
	out := IngestionTraceStateResponse{Live: st.Live, MaxWindowSecs: ingestion.TraceMaxWindowSecs}
	if w := st.Window; w != nil {
		enabledAt, hardStop := jsontime.New(w.EnabledAt.UTC()), jsontime.New(w.HardStop.UTC())
		out.WindowID, out.WindowSecs, out.SampleEverySecs = &w.WindowID, &w.WindowSecs, &w.SampleEverySecs
		out.EnabledAt, out.HardStop, out.EnabledBy = &enabledAt, &hardStop, w.EnabledBy
	}
	return out
}

// IngestionEnableTraceRequest is the wire body for PUT /api/ingestions/{id}/trace.
type IngestionEnableTraceRequest struct {
	WindowSecs      int `json:"windowSecs,omitempty" doc:"How long each runtime captures: 60, 300 or 900 (default 60). 400 TRACE_WINDOW_INVALID otherwise."`
	SampleEverySecs int `json:"sampleEverySecs,omitempty" doc:"A runtime starts a capture at most this often, 1 to 60 seconds (default 5). 400 TRACE_SAMPLE_INVALID otherwise."`
}

// IngestionTraceRecordResponse is one trace-log record.
type IngestionTraceRecordResponse struct {
	ID        string          `json:"id"`
	At        httpcompat.Time `json:"at"`
	Version   int32           `json:"version"`
	WindowID  string          `json:"windowId"`
	CaptureID string          `json:"captureId" doc:"Shared by the records of one capture: a poll cycle or one exchange"`
	RuntimeID string          `json:"runtimeId"`
	Kind      string          `json:"kind" enum:"poll,exchange,http,error"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data" doc:"The record's data, by kind"`
	Redacted  bool            `json:"redacted,omitempty" doc:"Sensitive parts withheld (no platform:ingestion:trace:view-bodies): an http record keeps the client, method, URL path, status, sizes, duration, attempt and failure; an exchange record its counts and outcome; a poll record all but its error; an error record nothing in data. The message of an http, exchange or error record is then rebuilt from what stays."`
}

func traceRecordResponse(r ingestion.TraceLogRecord, withBodies bool) IngestionTraceRecordResponse {
	redacted := false
	if !withBodies {
		redacted = ingestion.WithholdTraceDetail(&r)
	}
	out := IngestionTraceRecordResponse{
		ID: r.ID, At: jsontime.New(r.At.UTC()), Version: r.Version, WindowID: r.WindowID, CaptureID: r.CaptureID,
		RuntimeID: r.RuntimeID, Kind: r.Kind, Level: r.Level, Message: r.Message, Data: json.RawMessage(r.Data), Redacted: redacted,
	}
	if len(out.Data) == 0 {
		out.Data = json.RawMessage("null")
	}
	return out
}

// IngestionTraceLogResponse is the body of GET /api/ingestions/{id}/trace-log.
type IngestionTraceLogResponse struct {
	Records    []IngestionTraceRecordResponse `json:"records"`
	NextCursor *string                        `json:"nextCursor" nullable:"true" doc:"Pass as cursor to fetch the next (older) page; null on the last page"`
}

type getTraceInput struct {
	ID string `path:"id"`
}

func (s *State) getTrace(ctx context.Context, in *getTraceInput) (*apicommon.Out[IngestionTraceStateResponse], error) {
	if err := auth.CanViewIngestionTrace(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, st, err := operations.ReadTrace(ctx, s.Repo, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionTraceStateResponse]{Body: traceStateResponse(st)}, nil
}

type putTraceInput struct {
	ID   string `path:"id"`
	Body IngestionEnableTraceRequest
}

// putTrace switches tracing on. The permissions (manage and view-bodies) are
// the use case's.
func (s *State) putTrace(ctx context.Context, in *putTraceInput) (*apicommon.Out[IngestionTraceStateResponse], error) {
	cmd := operations.EnableTraceCommand{IngestionID: in.ID, WindowSecs: in.Body.WindowSecs, SampleEverySecs: in.Body.SampleEverySecs}
	st, err := usecaseop.RunTx(ctx, s.UoW, operations.EnableTrace(s.Repo, ingestion.TracePickupGrace()), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[IngestionTraceStateResponse]{Body: traceStateResponse(st)}, nil
}

func (s *State) deleteTrace(ctx context.Context, in *getTraceInput) (*apicommon.Empty, error) {
	cmd := operations.DisableTraceCommand{IngestionID: in.ID}
	if _, err := usecaseop.RunTx(ctx, s.UoW, operations.DisableTrace(s.Repo), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type traceLogInput struct {
	ID        string `path:"id"`
	Kind      string `query:"kind" enum:"poll,exchange,http,error" doc:"Only this record kind"`
	RuntimeID string `query:"runtimeId" doc:"Only this runtime's records"`
	CaptureID string `query:"captureId" doc:"One capture's records"`
	Since     string `query:"since" doc:"Oldest record time: RFC 3339 or unix milliseconds"`
	Until     string `query:"until" doc:"Newest record time: RFC 3339 or unix milliseconds"`
	Limit     int    `query:"limit" doc:"Page size, default 100, max 500"`
	Cursor    string `query:"cursor" doc:"nextCursor of the previous page"`
}

func (s *State) getTraceLog(ctx context.Context, in *traceLogInput) (*apicommon.Out[IngestionTraceLogResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanViewIngestionTrace(ac); err != nil {
		return nil, err
	}
	i, err := s.find(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	filter := ingestion.TraceLogFilter{IngestionID: i.ID, Kind: in.Kind, RuntimeID: in.RuntimeID, CaptureID: in.CaptureID}
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
		limit = ingestion.DefaultTraceLogLimit
	case limit > ingestion.MaxTraceLogLimit:
		limit = ingestion.MaxTraceLogLimit
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
		c := encodeTraceCursor(ingestion.TraceCursor{At: last.At, ID: last.ID})
		next = &c
	}
	withBodies := auth.CanViewIngestionTraceBodies(ac)
	out := make([]IngestionTraceRecordResponse, 0, len(recs))
	for _, r := range recs {
		out = append(out, traceRecordResponse(r, withBodies))
	}
	return &apicommon.Out[IngestionTraceLogResponse]{Body: IngestionTraceLogResponse{Records: out, NextCursor: next}}, nil
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

// encodeTraceCursor is the keyset position as base64url("<unix micros>:<id>").
func encodeTraceCursor(c ingestion.TraceCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.At.UnixMicro(), 10) + ":" + c.ID))
}

// decodeTraceCursor reads a cursor encodeTraceCursor issued; empty is none.
func decodeTraceCursor(v string) (*ingestion.TraceCursor, error) {
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
	return &ingestion.TraceCursor{At: time.UnixMicro(n).UTC(), ID: id}, nil
}

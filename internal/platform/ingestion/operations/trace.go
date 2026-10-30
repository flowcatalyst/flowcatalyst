// trace.go is the ingestion trace-window write path
// (docs/ingestion-trace-design.md sections 1 and 7): EnableTrace opens a
// time-boxed window, DisableTrace closes it.
package operations

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Trace error codes (design section 7).
const (
	TraceWindowInvalid = "TRACE_WINDOW_INVALID"
	TraceSampleInvalid = "TRACE_SAMPLE_INVALID"
)

// traceWindowIDPrefix is the TSID prefix of a window id (itw_…).
const traceWindowIDPrefix = "itw"

// EnableTraceCommand is the input DTO for PUT /api/ingestions/{id}/trace.
type EnableTraceCommand struct {
	IngestionID string `json:"ingestionId"`
	// WindowSecs is how long each runtime captures, from when it first sees
	// the window: 60, 300 or 900. Zero is the default (60).
	WindowSecs int `json:"windowSecs"`
	// SampleEverySecs is how often a runtime starts a capture, 1 to 60. Zero
	// is the default (5).
	SampleEverySecs int `json:"sampleEverySecs"`
}

// TraceState is an ingestion's trace window as a read or a write reports it.
// Window is nil when tracing was never switched on.
type TraceState struct {
	Window *ingestion.TraceWindow
	Live   bool
}

// TraceStateOf renders a window row (nil: never on) as of now.
func TraceStateOf(w *ingestion.TraceWindow, now time.Time) TraceState {
	return TraceState{Window: w, Live: w != nil && w.Live(now)}
}

// EnableTrace switches tracing on for an ingestion with a new window (a new
// window id, so every runtime starts a fresh capture) whose hard stop is now
// plus the window plus grace (the pickup grace, FC_INGESTION_TRACE_PICKUP_GRACE),
// replacing the ingestion's earlier window. Capture records headers and
// bodies, so the caller needs view-bodies as well as manage.
func EnableTrace(repo *ingestion.Repository, grace time.Duration) usecaseop.TxOperation[EnableTraceCommand, TraceState] {
	return usecaseop.TxOperation[EnableTraceCommand, TraceState]{
		Name: "EnableIngestionTrace",
		Validate: func(_ context.Context, cmd EnableTraceCommand) error {
			if err := requireID(cmd.IngestionID); err != nil {
				return err
			}
			if cmd.WindowSecs != 0 && !ingestion.ValidTraceWindowSecs(cmd.WindowSecs) {
				return usecase.Validation(TraceWindowInvalid, "windowSecs must be 60, 300 or 900")
			}
			if cmd.SampleEverySecs != 0 && (cmd.SampleEverySecs < ingestion.MinTraceSampleEverySecs || cmd.SampleEverySecs > ingestion.MaxTraceSampleEverySecs) {
				return usecase.Validation(TraceSampleInvalid, "sampleEverySecs must be between 1 and 60")
			}
			return nil
		},
		// The permissions are the use case's, so the API, the page and any
		// other caller are held to the same; reach needs the row (Execute).
		Authorize: func(ctx context.Context, _ EnableTraceCommand) error {
			ac := auth.FromContext(ctx)
			if err := auth.CanManageIngestionTrace(ac); err != nil {
				return err
			}
			return auth.RequireIngestionTraceBodies(ac)
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd EnableTraceCommand, ec usecase.ExecutionContext) (TraceState, error) {
			var zero TraceState
			i, err := load(ctx, repo, cmd.IngestionID)
			if err != nil {
				return zero, err
			}
			windowSecs, sampleEvery := cmd.WindowSecs, cmd.SampleEverySecs
			if windowSecs == 0 {
				windowSecs = ingestion.DefaultTraceWindowSecs
			}
			if sampleEvery == 0 {
				sampleEvery = ingestion.DefaultTraceSampleEverySecs
			}
			now := time.Now().UTC()
			by := ec.PrincipalID
			w := ingestion.TraceWindow{
				IngestionID: i.ID, WindowID: tsid.GenerateWithPrefix(traceWindowIDPrefix),
				WindowSecs: windowSecs, SampleEverySecs: sampleEvery,
				HardStop:  now.Add(time.Duration(windowSecs)*time.Second + grace),
				EnabledBy: &by, EnabledAt: now, UpdatedAt: now,
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error { return repo.UpsertTraceWindowTx(ctx, &w, tx) }); err != nil {
				return zero, usecase.Internal("REPO", "trace window write failed", err)
			}
			event := TraceEnabled{
				ingestionEvent: newEvent(ec, TraceEnabledType, i.ID), Code: i.Code,
				WindowID: w.WindowID, WindowSecs: w.WindowSecs, SampleEverySecs: w.SampleEverySecs, HardStop: w.HardStop,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return TraceStateOf(&w, now), nil
		},
	}
}

// DisableTraceCommand is the input DTO for DELETE /api/ingestions/{id}/trace.
type DisableTraceCommand struct {
	IngestionID string `json:"ingestionId"`
}

// DisableTrace ends the ingestion's window now (hard stop = now, so records
// captured just before and still queued in a runtime are accepted within the
// ingest grace). Idempotent: with no live window it succeeds and emits
// nothing; the event is for a live window that ended.
func DisableTrace(repo *ingestion.Repository) usecaseop.TxOperation[DisableTraceCommand, TraceState] {
	return usecaseop.TxOperation[DisableTraceCommand, TraceState]{
		Name: "DisableIngestionTrace",
		Validate: func(_ context.Context, cmd DisableTraceCommand) error {
			return requireID(cmd.IngestionID)
		},
		Authorize: func(ctx context.Context, _ DisableTraceCommand) error {
			return auth.CanManageIngestionTrace(auth.FromContext(ctx))
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DisableTraceCommand, ec usecase.ExecutionContext) (TraceState, error) {
			var zero TraceState
			i, err := load(ctx, repo, cmd.IngestionID)
			if err != nil {
				return zero, err
			}
			now := time.Now().UTC()
			var (
				w       *ingestion.TraceWindow
				wasLive bool
			)
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				var e error
				w, wasLive, e = repo.EndTraceWindowTx(ctx, i.ID, now, tx)
				return e
			}); err != nil {
				return zero, usecase.Internal("REPO", "trace window write failed", err)
			}
			if wasLive {
				event := TraceDisabled{ingestionEvent: newEvent(ec, TraceDisabledType, i.ID), Code: i.Code, WindowID: w.WindowID}
				if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
			}
			return TraceStateOf(w, now), nil
		},
	}
}

// ReadTrace is a pure read of an ingestion's trace window, with the reach
// check of any by-id read. It returns the ingestion beside it.
func ReadTrace(ctx context.Context, repo *ingestion.Repository, id string) (*ingestion.Ingestion, TraceState, error) {
	i, err := load(ctx, repo, id)
	if err != nil {
		return nil, TraceState{}, err
	}
	w, err := repo.GetTraceWindow(ctx, i.ID)
	if err != nil {
		return nil, TraceState{}, usecase.Internal("REPO", "get trace window failed", err)
	}
	return i, TraceStateOf(w, time.Now().UTC()), nil
}

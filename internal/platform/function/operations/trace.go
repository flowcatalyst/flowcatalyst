// trace.go is the function trace-context write path
// (docs/function-trace-design.md sections 1 and 7): EnableTraceContext opens
// a time-boxed window, DisableTraceContext closes it. Every write bumps the
// function's pool revision in the same transaction, which wakes the runners'
// long-poll on fng_desired (mirrors settings.go's PutSetting).
package operations

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Trace error codes (design section 7).
const (
	TraceWindowTooLong   = "TRACE_WINDOW_TOO_LONG"
	TraceContextUnknown  = "TRACE_CONTEXT_UNKNOWN"
	traceInvalidSampling = "TRACE_SAMPLE_RATE_INVALID"
	// TraceIndefiniteNotAllowed refuses "until switched off" for the http
	// context: its minute cap always applies (it captures bodies).
	TraceIndefiniteNotAllowed = "TRACE_INDEFINITE_NOT_ALLOWED"
)

// EnableTraceCommand is the input DTO for PUT /api/functions/{id}/trace/{context}.
type EnableTraceCommand struct {
	FunctionID string `json:"functionId"`
	Context    string `json:"context"`
	Minutes    int    `json:"minutes"`
	// Indefinite switches a declared context on until it is switched off, at
	// most TraceLimits.IndefiniteMaxDays (then it closes by itself). Minutes
	// is ignored. Needs platform:function:trace:indefinite; never for http.
	Indefinite bool `json:"indefinite"`
	// SampleRate captures one invocation in N; 0 means 1 (all).
	SampleRate int `json:"sampleRate"`
}

// TraceContextState is a context's state after a write or in a listing.
type TraceContextState struct {
	Name         string
	Description  string
	BuiltIn      bool
	EnabledUntil *time.Time // nil when never on; the switch-off time once off; the ceiling when Indefinite
	Indefinite   bool
	SampleRate   int
	Live         bool
	MaxMinutes   int
	// IndefiniteMaxDays is the ceiling of an "until switched off" window for
	// this context; 0 for http, which never takes one.
	IndefiniteMaxDays int
}

// EnableTraceContext opens (or re-opens) a window on one context of a
// function: `http`, or a context declared by a READY version of it. The
// window is [now, now+minutes], capped by limits (the `http` cap is lower),
// or, indefinite, [now, now+IndefiniteMaxDays] for a declared context. Pruning
// the function's old records is housekeeping after the commit
// (PruneTraceAfterEnable, in the background).
func EnableTraceContext(repo *function.Repository, limits function.TraceLimits) usecaseop.TxOperation[EnableTraceCommand, TraceContextState] {
	limits = limits.Normalised()
	return usecaseop.TxOperation[EnableTraceCommand, TraceContextState]{
		Name: "EnableFunctionTraceContext",
		Validate: func(_ context.Context, cmd EnableTraceCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if cmd.Context != function.TraceHTTPContext && !function.TraceContextPattern.MatchString(cmd.Context) {
				return usecase.Validation(TraceContextUnknown, "context '"+cmd.Context+"' is not a valid trace context name")
			}
			if cmd.Indefinite && cmd.Context == function.TraceHTTPContext {
				return usecase.Validation(TraceIndefiniteNotAllowed, "the http context cannot stay on until switched off; at most "+
					strconv.Itoa(limits.MaxMinutesFor(cmd.Context))+" minutes")
			}
			if !cmd.Indefinite {
				if cmd.Minutes < 1 {
					return usecase.Validation("TRACE_MINUTES_INVALID", "minutes must be at least 1 (or set indefinite)")
				}
				if capMinutes := limits.MaxMinutesFor(cmd.Context); cmd.Minutes > capMinutes {
					return usecase.Validation(TraceWindowTooLong, "minutes is over the "+strconv.Itoa(capMinutes)+
						"-minute limit for the "+cmd.Context+" context")
				}
			}
			if cmd.SampleRate != 0 && (cmd.SampleRate < function.MinTraceSampleRate || cmd.SampleRate > function.MaxTraceSampleRate) {
				return usecase.Validation(traceInvalidSampling, "sampleRate must be between 1 and 1000")
			}
			return nil
		},
		Authorize: usecaseop.Public[EnableTraceCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd EnableTraceCommand, ec usecase.ExecutionContext) (TraceContextState, error) {
			var zero TraceContextState

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}
			if cmd.Indefinite {
				if err := auth.CanEnableFunctionTraceIndefinite(auth.FromContext(ctx)); err != nil {
					return zero, err
				}
			}

			declared, err := DeclaredTraceContexts(ctx, repo, f.ID)
			if err != nil {
				return zero, err
			}
			desc, known := traceContextKnown(declared, cmd.Context)
			if !known {
				return zero, usecase.Validation(TraceContextUnknown,
					"context '"+cmd.Context+"' is not declared by a READY version of this function")
			}

			now := time.Now().UTC()
			until := now.Add(time.Duration(cmd.Minutes) * time.Minute)
			if cmd.Indefinite {
				until = now.Add(limits.IndefiniteCeiling())
			}
			rate := int32(max(cmd.SampleRate, function.MinTraceSampleRate)) //nolint:gosec // validated 1..1000
			by := ec.PrincipalID
			row := function.TraceContext{
				FunctionID: f.ID, Context: cmd.Context, EnabledUntil: &until, Indefinite: cmd.Indefinite,
				SampleRate: rate, EnabledBy: &by, UpdatedAt: now,
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if upErr := repo.UpsertTraceContextTx(ctx, &row, tx); upErr != nil {
					return upErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "trace context write failed", err)
			}

			event := TraceEnabled{
				Metadata:   usecase.NewEventMetadata(ec, TraceEnabledType, Source, subjectFor(f.ID)),
				FunctionID: f.ID, Address: f.Address, Context: cmd.Context,
				EnabledUntil: until, Indefinite: cmd.Indefinite, SampleRate: int(rate),
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return TraceStateOf(cmd.Context, desc, &row, limits, now), nil
		},
	}
}

// DisableTraceCommand is the input DTO for DELETE /api/functions/{id}/trace/{context}.
type DisableTraceCommand struct {
	FunctionID string `json:"functionId"`
	Context    string `json:"context"`
}

// DisableTraceContext closes a context's window now (enabled_until = now, so
// records captured just before and still queued are accepted within the
// ingest grace). Idempotent: a context that is already off, or was never on,
// succeeds the same way.
func DisableTraceContext(repo *function.Repository) usecaseop.TxOperation[DisableTraceCommand, struct{}] {
	return usecaseop.TxOperation[DisableTraceCommand, struct{}]{
		Name: "DisableFunctionTraceContext",
		Validate: func(_ context.Context, cmd DisableTraceCommand) error {
			if strings.TrimSpace(cmd.FunctionID) == "" {
				return usecase.Validation("FUNCTION_ID_REQUIRED", "functionId is required")
			}
			if cmd.Context != function.TraceHTTPContext && !function.TraceContextPattern.MatchString(cmd.Context) {
				return usecase.Validation(TraceContextUnknown, "context '"+cmd.Context+"' is not a valid trace context name")
			}
			return nil
		},
		Authorize: usecaseop.Public[DisableTraceCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DisableTraceCommand, ec usecase.ExecutionContext) (struct{}, error) {
			var zero struct{}

			f, err := repo.FindByID(ctx, cmd.FunctionID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return zero, httperror.NotFound("Function", cmd.FunctionID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return zero, err
			}

			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				if dErr := repo.DisableTraceContextTx(ctx, f.ID, cmd.Context, time.Now().UTC(), tx); dErr != nil {
					return dErr
				}
				_, bumpErr := repo.BumpPoolRevision(ctx, usecasepgx.WrapTxForBootstrap(tx), f.RunnerPool())
				return bumpErr
			}); err != nil {
				return zero, usecase.Internal("REPO", "trace context write failed", err)
			}

			event := TraceDisabled{
				Metadata:   usecase.NewEventMetadata(ec, TraceDisabledType, Source, subjectFor(f.ID)),
				FunctionID: f.ID, Address: f.Address, Context: cmd.Context,
			}
			if r := usecasepgx.EmitEventScoped(ctx, s, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return zero, nil
		},
	}
}

// DeclaredTraceContexts is the union of the trace contexts declared by the
// function's READY versions, by name. A version whose describe document no
// longer parses is skipped (it cannot declare anything the runner would act on).
func DeclaredTraceContexts(ctx context.Context, repo *function.Repository, functionID string) ([]abi.TraceContext, error) {
	versions, err := repo.ListVersionsByFunction(ctx, functionID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_versions_by_function failed", err)
	}
	byName := map[string]abi.TraceContext{}
	for _, v := range versions {
		if v.Status != function.VersionReady {
			continue
		}
		contexts, ok := versionTraceContexts(functionID, &v)
		if !ok {
			continue
		}
		for _, c := range contexts {
			if prev, ok := byName[c.Name]; !ok || (prev.Description == "" && c.Description != "") {
				byName[c.Name] = c
			}
		}
	}
	out := make([]abi.TraceContext, 0, len(byName))
	for _, c := range byName {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b abi.TraceContext) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// traceContextCacheMax bounds the per-version cache of declared contexts; a
// full cache is emptied and refilled (versions are few per function, and
// the cache only saves parsing).
const traceContextCacheMax = 4096

// traceContextCache maps a version (its id, digest and describe size: a
// published version's describe never changes) to the trace contexts its
// describe declares, so the UI's refresh and every listing do not parse each
// READY version's describe document again.
var traceContextCache = struct {
	sync.Mutex
	m map[string][]abi.TraceContext
}{m: map[string][]abi.TraceContext{}}

// versionTraceContexts is the version's declared contexts; false when its
// describe document does not parse (it cannot declare anything the runner
// would act on).
func versionTraceContexts(functionID string, v *function.Version) ([]abi.TraceContext, bool) {
	key := v.ID + "\x00" + v.Digest + "\x00" + strconv.Itoa(len(v.Describe))
	traceContextCache.Lock()
	cached, ok := traceContextCache.m[key]
	traceContextCache.Unlock()
	if ok {
		return cached, true
	}
	d, err := abi.ParseDescribe(v.Describe)
	if err != nil {
		slog.Warn("trace contexts: skipping a version with an unparseable describe document",
			"functionId", functionID, "version", v.Number, "err", err)
		return nil, false
	}
	contexts := slices.Clone(d.TraceContexts)
	traceContextCache.Lock()
	if len(traceContextCache.m) >= traceContextCacheMax {
		clear(traceContextCache.m)
	}
	traceContextCache.m[key] = contexts
	traceContextCache.Unlock()
	return contexts, true
}

// traceContextKnown reports whether name is `http` or declared, and its description.
func traceContextKnown(declared []abi.TraceContext, name string) (string, bool) {
	if name == function.TraceHTTPContext {
		return httpTraceDescription, true
	}
	for _, c := range declared {
		if c.Name == name {
			return c.Description, true
		}
	}
	return "", false
}

const httpTraceDescription = "Built-in capture of each request and response, with the function's log output."

// TraceStateOf renders a context's state. row may be nil (never switched on).
func TraceStateOf(name, description string, row *function.TraceContext, limits function.TraceLimits, now time.Time) TraceContextState {
	st := TraceContextState{
		Name: name, Description: description, BuiltIn: name == function.TraceHTTPContext,
		SampleRate: 1, MaxMinutes: limits.Normalised().MaxMinutesFor(name),
		IndefiniteMaxDays: limits.IndefiniteMaxDaysFor(name),
	}
	if row != nil {
		st.EnabledUntil = row.EnabledUntil
		st.SampleRate = int(row.SampleRate)
		st.Live = row.Live(now)
		st.Indefinite = row.Indefinite && st.Live
	}
	return st
}

// ListTraceContexts is a pure read: the union of the contexts declared by the
// function's READY versions plus the built-in `http`, with each one's window.
// `http` comes first, then the declared contexts by name.
func ListTraceContexts(ctx context.Context, repo *function.Repository, limits function.TraceLimits, functionID string) (*function.Function, []TraceContextState, error) {
	f, err := repo.FindByID(ctx, functionID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if f == nil {
		return nil, nil, httperror.NotFound("Function", functionID)
	}
	if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
		return nil, nil, err
	}
	declared, err := DeclaredTraceContexts(ctx, repo, f.ID)
	if err != nil {
		return nil, nil, err
	}
	rows, err := repo.ListTraceContexts(ctx, f.ID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list_trace_contexts failed", err)
	}
	byName := make(map[string]*function.TraceContext, len(rows))
	for i := range rows {
		byName[rows[i].Context] = &rows[i]
	}
	now := time.Now().UTC()
	out := make([]TraceContextState, 0, len(declared)+1)
	out = append(out, TraceStateOf(function.TraceHTTPContext, httpTraceDescription, byName[function.TraceHTTPContext], limits, now))
	for _, c := range declared {
		out = append(out, TraceStateOf(c.Name, c.Description, byName[c.Name], limits, now))
	}
	return f, out, nil
}

// tracePruneTimeout bounds one background prune after an enable.
const tracePruneTimeout = 5 * time.Minute

// pruning holds the functions whose background prune is running, so enabling
// again (or several contexts at once) does not stack prunes of one function.
var pruning sync.Map

// PruneTraceAfterEnable deletes the function's trace records past the
// retention, chunked, after an enable committed. Housekeeping: it runs in the
// background, detached from the request (so enabling cannot hang on a large
// delete and the request ending does not cancel it), bounded by
// tracePruneTimeout, one at a time per function, and it never fails the
// enable. The returned channel closes when it is done (tests wait on it).
func PruneTraceAfterEnable(ctx context.Context, repo *function.Repository, limits function.TraceLimits, functionID string) <-chan struct{} {
	done := make(chan struct{})
	if _, busy := pruning.LoadOrStore(functionID, struct{}{}); busy {
		close(done)
		return done
	}
	limits = limits.Normalised()
	bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), tracePruneTimeout)
	go func() {
		defer close(done)
		defer pruning.Delete(functionID)
		defer cancel()
		n, err := repo.PruneTraceLog(bg, functionID, time.Now().Add(-limits.Retention()),
			function.TracePruneChunk, limits.PruneChunks())
		if err != nil {
			slog.Warn("function trace prune failed", "functionId", functionID, "err", err)
			return
		}
		if n > 0 {
			slog.Info("function trace records pruned", "functionId", functionID, "deleted", n)
		}
	}()
	return done
}

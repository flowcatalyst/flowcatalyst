// trace.go is the platform side of function trace logs
// (docs/function-trace-design.md): the context windows an operator switches
// on, the records the runners ship, and the limits around both.
package function

import (
	"context"
	"encoding/json"
	"log/slog"
	"regexp"
	"time"
)

// TraceHTTPContext is the built-in context that captures requests and
// responses; it needs no declaration (abi.TraceHTTP).
const TraceHTTPContext = "http"

// TraceContextPattern is the shape of a declared context name (design
// section 3).
var TraceContextPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)

// Trace limits and defaults.
const (
	// DefaultTraceMaxMinutes caps a declared context's window
	// (FC_FUNCTIONS_TRACE_MAX_MINUTES).
	DefaultTraceMaxMinutes = 60
	// DefaultTraceHTTPMaxMinutes caps the http context's window, lower because
	// bodies are sensitive (FC_FUNCTIONS_TRACE_HTTP_MAX_MINUTES).
	DefaultTraceHTTPMaxMinutes = 15
	// DefaultTraceRetentionDays is how long records are kept
	// (FC_FUNCTIONS_TRACE_RETENTION_DAYS).
	DefaultTraceRetentionDays = 30

	// MinTraceSampleRate and MaxTraceSampleRate bound sampleRate: capture one
	// invocation in N.
	MinTraceSampleRate = 1
	MaxTraceSampleRate = 1000

	// MaxTraceReference and MaxTraceCorrelation cut a record's business key
	// and correlation id.
	MaxTraceReference   = 200
	MaxTraceCorrelation = 200

	// DefaultTraceIndefiniteMaxDays is the hard ceiling of a window switched
	// on "until switched off" (FC_FUNCTIONS_TRACE_INDEFINITE_MAX_DAYS): after
	// it the window closes by itself, like any other.
	DefaultTraceIndefiniteMaxDays = 7

	// TracePruneChunk is how many rows one retention DELETE removes.
	TracePruneChunk = 5000
	// DefaultTracePruneMaxRows bounds one retention sweep
	// (FC_FUNCTIONS_TRACE_PRUNE_MAX_ROWS); a larger backlog is finished by the
	// next sweep. See RunTraceRetention for the arithmetic.
	DefaultTracePruneMaxRows = 5_000_000

	// TraceAtFutureSkew and TraceAtMaxAge bound a record's runner-supplied
	// `at` against the platform's clock when it arrives: a later `at` is
	// clamped to the arrival time (a runner clock running ahead), an older one
	// than TraceAtMaxAge is dropped (a runner clock far behind, or a record
	// held through a long outage). Retention never reads `at`.
	TraceAtFutureSkew = time.Minute
	TraceAtMaxAge     = time.Hour
)

// TraceLimits are the operator-set trace settings.
type TraceLimits struct {
	MaxMinutes     int // declared contexts
	HTTPMaxMinutes int // the http context
	RetentionDays  int
	// IndefiniteMaxDays caps a window switched on "until switched off"
	// (declared contexts only; http never takes one).
	IndefiniteMaxDays int
	// PruneMaxRows bounds one retention sweep (all functions, or one function).
	PruneMaxRows int
	// PruneInterval paces the sweep across all functions.
	PruneInterval time.Duration
}

// DefaultTraceLimits are the settings when nothing is configured.
func DefaultTraceLimits() TraceLimits {
	return TraceLimits{
		MaxMinutes:     DefaultTraceMaxMinutes,
		HTTPMaxMinutes: DefaultTraceHTTPMaxMinutes,
		RetentionDays:  DefaultTraceRetentionDays,

		IndefiniteMaxDays: DefaultTraceIndefiniteMaxDays,
		PruneMaxRows:      DefaultTracePruneMaxRows,
		PruneInterval:     DefaultTracePruneInterval,
	}
}

// Normalised returns l with every non-positive field replaced by its default.
func (l TraceLimits) Normalised() TraceLimits {
	d := DefaultTraceLimits()
	if l.MaxMinutes <= 0 {
		l.MaxMinutes = d.MaxMinutes
	}
	if l.HTTPMaxMinutes <= 0 {
		l.HTTPMaxMinutes = d.HTTPMaxMinutes
	}
	if l.RetentionDays <= 0 {
		l.RetentionDays = d.RetentionDays
	}
	if l.IndefiniteMaxDays <= 0 {
		l.IndefiniteMaxDays = d.IndefiniteMaxDays
	}
	if l.PruneMaxRows <= 0 {
		l.PruneMaxRows = d.PruneMaxRows
	}
	if l.PruneInterval <= 0 {
		l.PruneInterval = d.PruneInterval
	}
	return l
}

// IndefiniteMaxDaysFor is the ceiling of an "until switched off" window for
// context, 0 for http (which never takes one: its minute cap always applies).
func (l TraceLimits) IndefiniteMaxDaysFor(context string) int {
	if context == TraceHTTPContext {
		return 0
	}
	return l.Normalised().IndefiniteMaxDays
}

// IndefiniteCeiling is how long an "until switched off" window may stay open.
func (l TraceLimits) IndefiniteCeiling() time.Duration {
	return time.Duration(l.Normalised().IndefiniteMaxDays) * 24 * time.Hour
}

// PruneChunks is PruneMaxRows in DELETE statements of TracePruneChunk rows.
func (l TraceLimits) PruneChunks() int {
	return max(1, (l.Normalised().PruneMaxRows+TracePruneChunk-1)/TracePruneChunk)
}

// MaxMinutesFor is the window cap for context.
func (l TraceLimits) MaxMinutesFor(context string) int {
	if context == TraceHTTPContext {
		return l.HTTPMaxMinutes
	}
	return l.MaxMinutes
}

// Retention is the age beyond which records are pruned.
func (l TraceLimits) Retention() time.Duration {
	return time.Duration(l.RetentionDays) * 24 * time.Hour
}

// TraceContext is one (function, context) row of fng_trace_contexts.
type TraceContext struct {
	FunctionID string
	Context    string
	// EnabledUntil is nil when the context was never on. Switching off sets
	// it to the switch-off time, so records captured just before and still
	// queued in a runner are accepted (the ingest grace applies to it).
	EnabledUntil *time.Time
	// Indefinite marks a window switched on "until switched off"; EnabledUntil
	// is then its hard ceiling (TraceLimits.IndefiniteMaxDays).
	Indefinite bool
	SampleRate int32
	EnabledBy  *string
	UpdatedAt  time.Time
}

// Live reports whether the window is open at now. Compared when read: an
// expired window needs no cleanup.
func (c TraceContext) Live(now time.Time) bool {
	return c.EnabledUntil != nil && c.EnabledUntil.After(now)
}

// TraceLogRecord is one row of fng_trace_log.
type TraceLogRecord struct {
	ID            string
	FunctionID    string
	Version       int32
	InvocationID  string
	CorrelationID *string
	Reference     *string
	Context       string
	Kind          string
	Level         string
	Message       string
	Data          []byte // JSON; nil when none
	At            time.Time
	RunnerID      *string
	CreatedAt     time.Time
}

// TraceLogFilter drives QueryTraceLog. Zero fields do not filter.
type TraceLogFilter struct {
	FunctionID    string
	Context       string
	Kind          string
	Level         string
	Reference     string
	CorrelationID string
	InvocationID  string
	Version       int32 // 0 = any
	Since         *time.Time
	Until         *time.Time
	// After is the keyset cursor: only rows strictly older than (At, ID).
	After *TraceCursor
	Limit int
}

// TraceCursor is the keyset position (at, id), newest first.
type TraceCursor struct {
	At time.Time
	ID string
}

// DefaultTracePruneDelay, DefaultTracePruneInterval and TraceLivePruneEvery
// pace retention: the first sweep after start, the sweep across all functions
// (FC_FUNCTIONS_TRACE_PRUNE_INTERVAL), and the prune of each function with a
// live window.
const (
	DefaultTracePruneDelay    = 2 * time.Minute
	DefaultTracePruneInterval = time.Hour
	TraceLivePruneEvery       = 10 * time.Minute
)

// RunTraceRetention is trace-log retention: it prunes records whose
// created_at (the platform's own clock, never the runner's `at`) is older than
// the retention, across all functions once after delay and then every
// interval (0 = limits.PruneInterval), and every TraceLivePruneEvery for each
// function that has a live window, until ctx ends. Idempotent deletes, so it
// is safe on every replica without a leader gate (the same posture as the
// dispatch-job reaper).
//
// Convergence: ingest is capped at 200 records a second per function per
// platform process, i.e. at most 17.28 M rows a day for one function at full
// rate on one process. The sweep deletes up to PruneMaxRows (default 5 M) an
// interval (default hourly): 120 M rows a day, about 1,389 rows a second, so
// it keeps up with roughly seven functions traced flat out around the clock
// on one process, far beyond the intended use; each live function is also
// pruned every 10 minutes with the same per-sweep bound (720 M rows a day for
// that function). Raise FC_FUNCTIONS_TRACE_PRUNE_MAX_ROWS or shorten
// FC_FUNCTIONS_TRACE_PRUNE_INTERVAL for more.
func RunTraceRetention(ctx context.Context, repo *Repository, limits TraceLimits, delay, interval time.Duration) {
	limits = limits.Normalised()
	if delay <= 0 {
		delay = DefaultTracePruneDelay
	}
	if interval <= 0 {
		interval = limits.PruneInterval
	}
	sweep := time.NewTimer(delay)
	defer sweep.Stop()
	live := time.NewTicker(TraceLivePruneEvery)
	defer live.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			n, err := repo.PruneTraceLog(ctx, "", time.Now().Add(-limits.Retention()), TracePruneChunk, limits.PruneChunks())
			switch {
			case err != nil:
				slog.Warn("function trace retention sweep failed", "err", err)
			case n > 0:
				slog.Info("function trace retention sweep", "deleted", n)
			}
			sweep.Reset(interval)
		case <-live.C:
			PruneLiveTraceFunctions(ctx, repo, limits)
		}
	}
}

// PruneLiveTraceFunctions prunes, function by function, the records past
// retention of every function with a live window: the functions that are
// accruing records right now.
func PruneLiveTraceFunctions(ctx context.Context, repo *Repository, limits TraceLimits) {
	limits = limits.Normalised()
	ids, err := repo.ListLiveTraceFunctionIDs(ctx, time.Now())
	if err != nil {
		slog.Warn("function trace live prune: listing live functions failed", "err", err)
		return
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return
		}
		n, err := repo.PruneTraceLog(ctx, id, time.Now().Add(-limits.Retention()), TracePruneChunk, limits.PruneChunks())
		switch {
		case err != nil:
			slog.Warn("function trace prune failed", "functionId", id, "err", err)
		case n > 0:
			slog.Info("function trace records pruned", "functionId", id, "deleted", n)
		}
	}
}

// Trace record kinds (control.TraceKind*).
const (
	TraceKindTrace = "trace"
	TraceKindHTTP  = "http"
	TraceKindLog   = "log"
)

// WithholdTraceDetail strips what a reader without
// platform:function:trace:view-bodies may not see, and reports whether
// anything was withheld. Such a reader sees an http record's method, path
// (never the query string), status, duration, outcome and body sizes; no
// headers, query string, bodies or caller. A log record keeps its metadata
// but not the line the function printed (message emptied, data dropped); an
// author trace record keeps its message but not its data. The record's
// metadata (time, context, kind, level, invocation, correlation, reference,
// version, runner) is always shown.
func WithholdTraceDetail(r *TraceLogRecord) bool {
	switch r.Kind {
	case TraceKindHTTP:
		r.Data = httpTraceSummary(r.Data)
		return true
	case TraceKindLog:
		withheld := r.Message != "" || r.Data != nil
		r.Message, r.Data = "", nil
		return withheld
	default:
		withheld := r.Data != nil
		r.Data = nil
		return withheld
	}
}

// httpTraceSummary keeps the non-sensitive fields of an http record's data.
func httpTraceSummary(data []byte) []byte {
	var in struct {
		Request struct {
			Method        string `json:"method"`
			Path          string `json:"path"`
			BodyBytes     int64  `json:"bodyBytes"`
			BodyTruncated bool   `json:"bodyTruncated"`
		} `json:"request"`
		Response struct {
			Status        int   `json:"status"`
			BodyBytes     int64 `json:"bodyBytes"`
			BodyTruncated bool  `json:"bodyTruncated"`
		} `json:"response"`
		DurationMs int64  `json:"durationMs"`
		Outcome    string `json:"outcome"`
	}
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return nil
	}
	type part struct {
		Method        string  `json:"method,omitempty"`
		Path          string  `json:"path,omitempty"`
		Status        int     `json:"status,omitempty"`
		Body          *string `json:"body"`
		BodyBytes     int64   `json:"bodyBytes"`
		BodyTruncated bool    `json:"bodyTruncated"`
	}
	out, err := json.Marshal(struct {
		Request    part   `json:"request"`
		Response   part   `json:"response"`
		DurationMs int64  `json:"durationMs"`
		Outcome    string `json:"outcome"`
	}{
		Request:    part{Method: in.Request.Method, Path: in.Request.Path, BodyBytes: in.Request.BodyBytes, BodyTruncated: in.Request.BodyTruncated},
		Response:   part{Status: in.Response.Status, BodyBytes: in.Response.BodyBytes, BodyTruncated: in.Response.BodyTruncated},
		DurationMs: in.DurationMs, Outcome: in.Outcome,
	})
	if err != nil {
		return nil
	}
	return out
}

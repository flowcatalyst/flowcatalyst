// trace.go is the platform side of function trace logs
// (docs/function-trace-design.md): the context windows an operator switches
// on, the records the runners ship, and the limits around both.
package function

import (
	"context"
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

	// TracePruneChunk is how many rows one retention DELETE removes; a sweep
	// runs at most TracePruneMaxChunks of them.
	TracePruneChunk     = 5000
	TracePruneMaxChunks = 200
)

// TraceLimits are the operator-set trace settings.
type TraceLimits struct {
	MaxMinutes     int // declared contexts
	HTTPMaxMinutes int // the http context
	RetentionDays  int
}

// DefaultTraceLimits are the settings when nothing is configured.
func DefaultTraceLimits() TraceLimits {
	return TraceLimits{
		MaxMinutes:     DefaultTraceMaxMinutes,
		HTTPMaxMinutes: DefaultTraceHTTPMaxMinutes,
		RetentionDays:  DefaultTraceRetentionDays,
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
	return l
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
	// EnabledUntil is nil when the context was switched off (or never on).
	EnabledUntil *time.Time
	SampleRate   int32
	EnabledBy    *string
	UpdatedAt    time.Time
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

// DefaultTracePruneDelay and DefaultTracePruneInterval pace the daily
// backstop.
const (
	DefaultTracePruneDelay    = 2 * time.Minute
	DefaultTracePruneInterval = 24 * time.Hour
)

// RunTraceRetention is the daily backstop of trace-log retention: it prunes
// records older than the retention across all functions, once after delay and
// then every interval, until ctx ends. Idempotent deletes, so it is safe on
// every replica without a leader gate (the same posture as the dispatch-job
// reaper). The per-function prune after an enable handles the common case.
func RunTraceRetention(ctx context.Context, repo *Repository, limits TraceLimits, delay, interval time.Duration) {
	limits = limits.Normalised()
	if delay <= 0 {
		delay = DefaultTracePruneDelay
	}
	if interval <= 0 {
		interval = DefaultTracePruneInterval
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		n, err := repo.PruneTraceLog(ctx, "", time.Now().Add(-limits.Retention()), TracePruneChunk, TracePruneMaxChunks)
		switch {
		case err != nil:
			slog.Warn("function trace retention sweep failed", "err", err)
		case n > 0:
			slog.Info("function trace retention sweep", "deleted", n)
		}
		timer.Reset(interval)
	}
}

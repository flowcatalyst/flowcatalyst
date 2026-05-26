// Package logging configures structured logging via slog.
//
// Field names match the Rust tracing JSON output so logs from both
// codebases can be aggregated in the same pipeline during cutover:
//   - correlation_id, causation_id, principal_id, execution_id
//   - aggregate_type, aggregate_id, event_type
package logging

import (
	"context"
	"log/slog"
	"os"
)

type ctxKey int

const (
	correlationIDKey ctxKey = iota
	causationIDKey
	principalIDKey
	executionIDKey
)

// Init configures the default slog logger with JSON output to stderr.
// Level is read from FC_LOG_LEVEL (default info).
func Init() {
	lvl := slog.LevelInfo
	switch os.Getenv("FC_LOG_LEVEL") {
	case "debug", "DEBUG":
		lvl = slog.LevelDebug
	case "warn", "WARN", "warning", "WARNING":
		lvl = slog.LevelWarn
	case "error", "ERROR":
		lvl = slog.LevelError
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	slog.SetDefault(slog.New(h))
}

// WithCorrelationID stores a correlation ID on the context for log enrichment.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// WithCausationID stores a causation ID on the context.
func WithCausationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, causationIDKey, id)
}

// WithPrincipalID stores the acting principal on the context.
func WithPrincipalID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, principalIDKey, id)
}

// WithExecutionID stores the per-request execution ID on the context.
func WithExecutionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, executionIDKey, id)
}

// FromContext returns a logger enriched with the trace fields stored on the context.
func FromContext(ctx context.Context) *slog.Logger {
	l := slog.Default()
	if v, ok := ctx.Value(correlationIDKey).(string); ok && v != "" {
		l = l.With("correlation_id", v)
	}
	if v, ok := ctx.Value(causationIDKey).(string); ok && v != "" {
		l = l.With("causation_id", v)
	}
	if v, ok := ctx.Value(principalIDKey).(string); ok && v != "" {
		l = l.With("principal_id", v)
	}
	if v, ok := ctx.Value(executionIDKey).(string); ok && v != "" {
		l = l.With("execution_id", v)
	}
	return l
}

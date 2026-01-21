package common

import (
	"context"
	"net/http"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// Context keys for storing tracing information
type contextKey string

const (
	correlationIDKey contextKey = "correlationID"
	causationIDKey   contextKey = "causationID"
	executionCtxKey  contextKey = "executionContext"
)

// HTTP header names for distributed tracing
const (
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderRequestID     = "X-Request-ID"
	HeaderCausationID   = "X-Causation-ID"
)

// ExecutionContext contains metadata about the current use case execution.
// It provides tracing information for distributed systems and audit logging.
//
// This is the Go equivalent of Java's ExecutionContext record.
type ExecutionContext struct {
	// ExecutionID is a unique identifier for this specific execution.
	// Generated fresh for each use case invocation.
	ExecutionID string

	// CorrelationID is the distributed tracing identifier.
	// Propagated across service boundaries to track a request through the system.
	CorrelationID string

	// CausationID is the ID of the event that caused this execution.
	// Used for building event chains and debugging causality.
	CausationID string

	// PrincipalID identifies who is performing the action.
	// Can be a user ID or service account ID.
	PrincipalID string

	// InitiatedAt is when the execution started.
	InitiatedAt time.Time
}

// NewExecutionContext creates a new execution context for a fresh request.
// Both ExecutionID and CorrelationID are set to new TSIDs.
func NewExecutionContext(principalID string) *ExecutionContext {
	execID := "exec-" + tsid.Generate()
	return &ExecutionContext{
		ExecutionID:   execID,
		CorrelationID: execID, // correlation starts as execution ID
		CausationID:   "",     // no causation for fresh requests
		PrincipalID:   principalID,
		InitiatedAt:   time.Now(),
	}
}

// ExecutionContextFromRequest creates an execution context from an HTTP request.
// It extracts correlation and causation IDs from headers if present.
func ExecutionContextFromRequest(r *http.Request, principalID string) *ExecutionContext {
	execID := "exec-" + tsid.Generate()

	// Try to get correlation ID from headers
	correlationID := r.Header.Get(HeaderCorrelationID)
	if correlationID == "" {
		correlationID = r.Header.Get(HeaderRequestID)
	}
	if correlationID == "" {
		correlationID = execID // fallback to execution ID
	}

	// Get causation ID from headers (may be empty)
	causationID := r.Header.Get(HeaderCausationID)

	return &ExecutionContext{
		ExecutionID:   execID,
		CorrelationID: correlationID,
		CausationID:   causationID,
		PrincipalID:   principalID,
		InitiatedAt:   time.Now(),
	}
}

// ExecutionContextFromContext extracts execution context from a Go context.
// Returns nil if no execution context is present.
func ExecutionContextFromContext(ctx context.Context) *ExecutionContext {
	if ec, ok := ctx.Value(executionCtxKey).(*ExecutionContext); ok {
		return ec
	}
	return nil
}

// WithCorrelation creates a new execution context with a specific correlation ID.
// Useful for background jobs that need to continue an existing trace.
func WithCorrelation(principalID, correlationID string) *ExecutionContext {
	return &ExecutionContext{
		ExecutionID:   "exec-" + tsid.Generate(),
		CorrelationID: correlationID,
		CausationID:   "",
		PrincipalID:   principalID,
		InitiatedAt:   time.Now(),
	}
}

// FromParentEvent creates an execution context from a parent domain event.
// The parent event's ID becomes the causation ID, and its correlation ID is preserved.
func FromParentEvent(event DomainEvent, principalID string) *ExecutionContext {
	return &ExecutionContext{
		ExecutionID:   "exec-" + tsid.Generate(),
		CorrelationID: event.CorrelationID(),
		CausationID:   event.EventID(),
		PrincipalID:   principalID,
		InitiatedAt:   time.Now(),
	}
}

// WithCausation creates a child context with the specified event as the cause.
// Shares the execution ID and correlation ID but sets a new causation.
func (ec *ExecutionContext) WithCausation(causingEventID string) *ExecutionContext {
	return &ExecutionContext{
		ExecutionID:   ec.ExecutionID,
		CorrelationID: ec.CorrelationID,
		CausationID:   causingEventID,
		PrincipalID:   ec.PrincipalID,
		InitiatedAt:   ec.InitiatedAt,
	}
}

// ToContext stores the execution context in a Go context.
func (ec *ExecutionContext) ToContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, executionCtxKey, ec)
}

// CorrelationIDFromContext extracts just the correlation ID from a context.
// Returns empty string if not present.
func CorrelationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationIDKey).(string); ok {
		return id
	}
	if ec := ExecutionContextFromContext(ctx); ec != nil {
		return ec.CorrelationID
	}
	return ""
}

// CausationIDFromContext extracts just the causation ID from a context.
// Returns empty string if not present.
func CausationIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(causationIDKey).(string); ok {
		return id
	}
	if ec := ExecutionContextFromContext(ctx); ec != nil {
		return ec.CausationID
	}
	return ""
}

// WithCorrelationID adds a correlation ID to a context.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey, correlationID)
}

// WithCausationID adds a causation ID to a context.
func WithCausationID(ctx context.Context, causationID string) context.Context {
	return context.WithValue(ctx, causationIDKey, causationID)
}

// TracingContext provides tracing information for background jobs.
// Use this when spawning goroutines or async work that needs tracing context.
//
// Example usage:
//
//	// Before spawning background work, capture the tracing context
//	tc := common.CaptureTracingContext(r)
//
//	go func() {
//	    // Create execution context from captured tracing
//	    execCtx := tc.ToExecutionContext(principalID)
//	    // ... use execCtx for domain operations
//	}()
type TracingContext struct {
	CorrelationID string
	CausationID   string
}

// CaptureTracingContext captures the current tracing context from an HTTP request.
// Use this before spawning background goroutines to maintain trace continuity.
func CaptureTracingContext(r *http.Request) *TracingContext {
	correlationID := r.Header.Get(HeaderCorrelationID)
	if correlationID == "" {
		correlationID = r.Header.Get(HeaderRequestID)
	}
	if correlationID == "" {
		correlationID = "trace-" + tsid.Generate()
	}

	return &TracingContext{
		CorrelationID: correlationID,
		CausationID:   r.Header.Get(HeaderCausationID),
	}
}

// CaptureTracingContextFromContext captures tracing from a Go context.
// Use this when you have a context but not the original HTTP request.
func CaptureTracingContextFromContext(ctx context.Context) *TracingContext {
	tc := &TracingContext{}

	if correlationID, ok := ctx.Value(correlationIDKey).(string); ok {
		tc.CorrelationID = correlationID
	}
	if causationID, ok := ctx.Value(causationIDKey).(string); ok {
		tc.CausationID = causationID
	}

	// Try to get from ExecutionContext if not found directly
	if tc.CorrelationID == "" {
		if ec := ExecutionContextFromContext(ctx); ec != nil {
			tc.CorrelationID = ec.CorrelationID
			if tc.CausationID == "" {
				tc.CausationID = ec.CausationID
			}
		}
	}

	// Generate new correlation ID if still empty
	if tc.CorrelationID == "" {
		tc.CorrelationID = "trace-" + tsid.Generate()
	}

	return tc
}

// ToExecutionContext creates an ExecutionContext from the captured tracing info.
// Use this in background jobs to create context for domain operations.
func (tc *TracingContext) ToExecutionContext(principalID string) *ExecutionContext {
	return &ExecutionContext{
		ExecutionID:   "exec-" + tsid.Generate(),
		CorrelationID: tc.CorrelationID,
		CausationID:   tc.CausationID,
		PrincipalID:   principalID,
		InitiatedAt:   time.Now(),
	}
}

// ToContext adds the tracing information to a Go context.
// Use this when you need to pass tracing info through context.Context.
func (tc *TracingContext) ToContext(ctx context.Context) context.Context {
	ctx = context.WithValue(ctx, correlationIDKey, tc.CorrelationID)
	if tc.CausationID != "" {
		ctx = context.WithValue(ctx, causationIDKey, tc.CausationID)
	}
	return ctx
}

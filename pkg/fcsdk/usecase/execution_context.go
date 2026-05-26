package usecase

import (
	"time"

	"github.com/google/uuid"
)

// ExecutionContext threads trace identifiers and the acting principal
// through the use case pipeline. Mirrors the Rust ExecutionContext.
type ExecutionContext struct {
	// PrincipalID is the acting user or service account.
	PrincipalID string
	// CorrelationID is propagated from inbound requests; new requests
	// generate a fresh ID.
	CorrelationID string
	// CausationID points to the event that caused this one (for chains).
	// Empty for top-level / user-initiated actions.
	CausationID string
	// ExecutionID is unique per use case invocation.
	ExecutionID string
	// InitiatedAt is when this context was created.
	InitiatedAt time.Time
}

// NewExecutionContext creates a fresh context with the given principal
// and freshly-generated correlation + execution IDs.
//
// CorrelationID is seeded from ExecutionID for top-level requests.
// Use WithCorrelation when an inbound request already carries one.
func NewExecutionContext(principalID string) ExecutionContext {
	execID := uuid.NewString()
	return ExecutionContext{
		PrincipalID:   principalID,
		CorrelationID: execID,
		ExecutionID:   execID,
		InitiatedAt:   time.Now().UTC(),
	}
}

// WithCorrelation creates a context using an inbound correlation ID.
// Use this when handling a request that originated upstream and already
// carries trace identifiers.
func WithCorrelation(principalID, correlationID string) ExecutionContext {
	return ExecutionContext{
		PrincipalID:   principalID,
		CorrelationID: correlationID,
		ExecutionID:   uuid.NewString(),
		InitiatedAt:   time.Now().UTC(),
	}
}

// WithCausation returns a copy with the causation ID set, for events
// emitted as a consequence of another event.
func (ec ExecutionContext) WithCausation(causationID string) ExecutionContext {
	ec.CausationID = causationID
	return ec
}

// FromParentEvent derives a context from a parent event, preserving the
// correlation chain and recording the parent as the causation.
func FromParentEvent(parent DomainEvent, principalID string) ExecutionContext {
	return ExecutionContext{
		PrincipalID:   principalID,
		CorrelationID: parent.CorrelationID(),
		CausationID:   parent.EventID(),
		ExecutionID:   uuid.NewString(),
		InitiatedAt:   time.Now().UTC(),
	}
}

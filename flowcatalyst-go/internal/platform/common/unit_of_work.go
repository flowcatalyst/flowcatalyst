package common

import "context"

// UnitOfWork defines the interface for atomic operations that persist
// aggregates, emit domain events, and create audit logs transactionally.
//
// This is the ONLY way to return a successful Result from a use case.
// The Commit methods return Result using the unexported newSuccess constructor,
// which guarantees that:
//  1. Domain events are always emitted when state changes
//  2. Audit logs are always created for operations
//  3. Entity state and events are consistent (atomic commit)
//
// Use cases MUST call one of the Commit methods to return success.
// Direct calls to Result.Success() are not possible (it's unexported).
//
// Example usage in a use case:
//
//	func (uc *CreateEventTypeUseCase) Execute(
//	    ctx context.Context,
//	    cmd CreateEventTypeCommand,
//	    execCtx *common.ExecutionContext,
//	) common.Result[*EventTypeCreated] {
//	    // Validation - can return failure directly
//	    if !isValid(cmd) {
//	        return common.Failure[*EventTypeCreated](common.ValidationError(...))
//	    }
//
//	    // Create aggregate
//	    eventType := &eventtype.EventType{...}
//
//	    // Create domain event
//	    event := &EventTypeCreated{...}
//
//	    // Atomic commit - ONLY way to return success
//	    return uc.unitOfWork.Commit(ctx, eventType, event, cmd)
//	}
type UnitOfWork interface {
	// Commit persists an aggregate with its domain event atomically.
	//
	// Within a single database transaction:
	//  1. Persists or updates the aggregate entity (upsert by ID)
	//  2. Creates the domain event in the events collection
	//  3. Creates the audit log entry
	//
	// If any step fails, the entire transaction is rolled back.
	//
	// Parameters:
	//   - ctx: Context for the operation (includes timeout, cancellation)
	//   - aggregate: The entity to persist (must have an ID field)
	//   - event: The domain event representing what happened
	//   - command: The command that was executed (for audit logging)
	//
	// Returns:
	//   - Success with the event if commit succeeds
	//   - Failure with error if commit fails
	Commit(ctx context.Context, aggregate any, event DomainEvent, command any) Result[DomainEvent]

	// CommitDelete deletes an aggregate with its domain event atomically.
	//
	// Within a single database transaction:
	//  1. Deletes the aggregate entity
	//  2. Creates the domain event in the events collection
	//  3. Creates the audit log entry
	//
	// If any step fails, the entire transaction is rolled back.
	CommitDelete(ctx context.Context, aggregate any, event DomainEvent, command any) Result[DomainEvent]

	// CommitAll persists multiple aggregates with a domain event atomically.
	//
	// Use for operations that affect multiple aggregates, such as:
	//   - Provisioning (creates Application + ServiceAccount + OAuthClient)
	//   - Bulk operations
	//
	// Within a single database transaction:
	//  1. Persists or updates all aggregate entities
	//  2. Creates the domain event in the events collection
	//  3. Creates the audit log entry
	//
	// If any step fails, the entire transaction is rolled back.
	CommitAll(ctx context.Context, aggregates []any, event DomainEvent, command any) Result[DomainEvent]

	// CommitWithClientID is like Commit but also sets the clientId on the event.
	// Use for multi-tenant operations where events are scoped to a client.
	CommitWithClientID(ctx context.Context, aggregate any, event DomainEvent, command any, clientID string) Result[DomainEvent]
}

// AggregateRoot is an optional interface that aggregates can implement
// to provide collection name and ID extraction.
type AggregateRoot interface {
	// AggregateID returns the unique identifier for this aggregate.
	AggregateID() string

	// CollectionName returns the MongoDB collection name for this aggregate type.
	CollectionName() string
}

// Auditable is an optional interface that commands can implement
// to customize how they are serialized for audit logging.
type Auditable interface {
	// ToAuditJSON returns the JSON representation for audit logging.
	// Use this to redact sensitive fields like passwords.
	ToAuditJSON() string
}

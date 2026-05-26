package usecase

import "context"

// UseCase is implemented once per write operation in the system. Generic
// over the command type C (input DTO) and the event type E (emitted on
// success). The pipeline is Validate → Authorize → Execute, orchestrated
// by Run. Handlers always call Run, never the individual methods.
type UseCase[C any, E DomainEvent] interface {
	// Validate checks command shape: field presence, format, length,
	// patterns. Anything that doesn't require loading data from the DB.
	// Returns a *Error (validation kind) on failure.
	Validate(ctx context.Context, cmd C) error

	// Authorize checks resource-level access (ownership, client access,
	// state-based permissions). Runs after Validate so command fields
	// are well-formed. Handler-level RBAC checks happen in the HTTP
	// layer; Authorize handles the per-resource checks.
	Authorize(ctx context.Context, cmd C, ec ExecutionContext) error

	// Execute is the core business logic: load aggregates, check
	// business rules, build the domain event, call one of the Commit*
	// functions from usecasepgx / usecasesql. The only legal return
	// values are those Commit* calls (the happy path) or Failure(err)
	// (the error path) — enforced by the seal on Result[E].
	Execute(ctx context.Context, cmd C, ec ExecutionContext) Result[E]
}

// Run executes Validate → Authorize → Execute in order, short-circuiting
// on the first error. This is what HTTP handlers call.
//
// Run is a free function, not a method on the interface, so users cannot
// override the pipeline shape.
func Run[C any, E DomainEvent](
	ctx context.Context,
	uc UseCase[C, E],
	cmd C,
	ec ExecutionContext,
) Result[E] {
	if err := uc.Validate(ctx, cmd); err != nil {
		return Failure[E](err)
	}
	if err := uc.Authorize(ctx, cmd, ec); err != nil {
		return Failure[E](err)
	}
	return uc.Execute(ctx, cmd, ec)
}

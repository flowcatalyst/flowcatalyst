package usecaseop

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Operation is one business operation expressed as four named phases. Build
// it with a constructor that captures the operation's dependencies (repos,
// services) in the phase closures, and run it with [Run]. See the package
// doc for the contract and an example.
type Operation[C any, E usecase.DomainEvent] struct {
	// Name identifies the operation in diagnostics. Optional but recommended.
	Name string

	// Validate checks command shape: presence, format, length, patterns —
	// anything that does not require loading data. Return a validation-kind
	// [usecase.Error] on failure. Optional: leave nil when the command has
	// nothing to check, though most operations validate at least one field.
	Validate func(ctx context.Context, cmd C) error

	// Authorize is the resource-level access check (ownership, scope,
	// state-based permission). REQUIRED: set it to a real check, or to
	// [Public] to declare the operation intentionally open. Authorization
	// data (the authenticated principal) is carried on ctx — platform code
	// reads it with auth.FromContext(ctx). Run refuses to proceed if this is
	// nil (fail closed), and the uowseal analyzer flags any Operation literal
	// that omits it.
	Authorize func(ctx context.Context, cmd C) error

	// Execute runs invariant checks (loads aggregates, applies business
	// rules), builds the domain event, and returns the [Plan] describing the
	// change to commit. Returning a Plan is the ONLY way an operation can
	// reach the database; Run applies the Plan after Validate and Authorize
	// have passed. REQUIRED. Return a non-nil error (typically a
	// [usecase.Error]) to fail without committing.
	Execute func(ctx context.Context, cmd C, ec usecase.ExecutionContext) (Plan[E], error)
}

// Public is the explicit Authorize value for operations that are
// intentionally open — no resource-level authorization beyond whatever the
// transport layer already enforced. Prefer this over leaving Authorize nil:
// "deliberately open" should be a visible decision in the code, not an
// omission, and it keeps the operation passing the uowseal analyzer.
func Public[C any](_ context.Context, _ C) error { return nil }

// Run executes op's phases in order — Validate, Authorize, Execute — short
// circuiting on the first error, then applies the returned [Plan] in one
// transaction (aggregate change + domain event + audit log, atomically) and
// returns the committed event.
//
// Run is a free function because Go does not allow type parameters on
// methods. It is the only consumer of a Plan, so every successful event it
// returns went through validation, authorization, and an atomic commit.
func Run[C any, E usecase.DomainEvent](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	op Operation[C, E],
	cmd C,
	ec usecase.ExecutionContext,
) (E, error) {
	var zero E

	// Fail closed if a required phase is missing. The analyzer catches this
	// statically; this is the runtime backstop.
	if op.Authorize == nil {
		return zero, usecase.Internal("USECASE_MISCONFIGURED",
			"operation "+op.Name+" has no Authorize phase (set it, or use usecaseop.Public)", nil)
	}
	if op.Execute == nil {
		return zero, usecase.Internal("USECASE_MISCONFIGURED",
			"operation "+op.Name+" has no Execute phase", nil)
	}

	if op.Validate != nil {
		if err := op.Validate(ctx, cmd); err != nil {
			return zero, err
		}
	}
	if err := op.Authorize(ctx, cmd); err != nil {
		return zero, err
	}
	plan, err := op.Execute(ctx, cmd, ec)
	if err != nil {
		return zero, err
	}
	if plan == nil {
		return zero, usecase.Internal("USECASE_NIL_PLAN",
			"operation "+op.Name+" Execute returned a nil Plan without an error", nil)
	}
	return plan.apply(ctx, uow, cmd)
}

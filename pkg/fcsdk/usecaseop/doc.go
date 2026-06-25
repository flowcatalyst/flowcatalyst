// Package usecaseop is the enforced use-case envelope for FlowCatalyst
// business operations.
//
// Every state-changing business operation is modelled as an [Operation] — a
// value with four named phases that the [Run] driver executes in a fixed
// order:
//
//	Validate   command shape (presence, format, length) — pure, no DB
//	Authorize  resource-level access — REQUIRED, or [Public] to declare open
//	Execute    invariant checks (loads, business rules) → returns a Plan
//	────────── Run takes over ──────────
//	apply      persist aggregate + write domain event + write audit log,
//	           atomically in one transaction
//
// # What this guarantees, and how
//
// Unlike a bare function that "remembers" to validate, authorize, and commit,
// the envelope makes the four guarantees structural:
//
//   - Validation and authorization ALWAYS run, because Run runs them — an
//     operation cannot skip a phase, only leave it empty.
//   - Authorization is never silently absent: Authorize is required. An
//     operation that is intentionally open sets it to [Public], which is a
//     visible, greppable, reviewable decision; an omitted Authorize fails
//     closed at runtime and is flagged by the uowseal analyzer.
//   - An operation cannot reach the database except by returning a [Plan]
//     from Execute. Plan is a sealed interface (its only method is
//     unexported), so the sole constructors are [Save] / [Delete] / [Emit] /
//     [SaveAll] / [Sync], and the sole consumer is Run. There is no path to a
//     persisted aggregate that does not also write the domain event and audit
//     log in the same transaction.
//
// The Plan seal is the Go-idiomatic replacement for the older
// [usecase.Result] + internal/sealed token machinery: the unexported
// interface method does the same job as the token, without threading a
// witness value through every commit.
//
// # An operation
//
//	func CreateConnection(repo *connection.Repository) usecaseop.Operation[CreateCommand, ConnectionCreated] {
//	    return usecaseop.Operation[CreateCommand, ConnectionCreated]{
//	        Name:      "CreateConnection",
//	        Validate:  func(ctx context.Context, cmd CreateCommand) error { ... },
//	        Authorize: func(ctx context.Context, cmd CreateCommand) error {
//	            return auth.RequireAnchor(auth.FromContext(ctx))
//	        },
//	        Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionCreated], error) {
//	            // ... invariant checks, build aggregate + event ...
//	            return usecaseop.Save(c, repo, event), nil
//	        },
//	    }
//	}
//
// And the HTTP handler:
//
//	ec := usecase.NewExecutionContext(ac.PrincipalID)
//	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateConnection(s.Repo), cmd, ec)
//	if err != nil { return nil, err }
//	// event.ConnectionID is the committed id
package usecaseop

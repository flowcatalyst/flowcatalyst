// Package commit is the leaner sealed unit-of-work for FlowCatalyst
// platform use cases.
//
// It replaces the [usecase.Result] + [usecase.UseCase] machinery in
// [usecasepgx] with plain functions returning ([Committed], error).
// The seal — "no aggregate write reaches the DB without its domain
// event" — is preserved: the [Committed] type's event field is
// unexported, so external callers can only produce a non-zero
// [Committed] by going through [Save] / [Delete] / [SaveAll] / [Emit],
// each of which writes the event + audit log atomically with the
// aggregate change.
//
// # When to use this vs the older usecasepgx surface
//
// New use cases SHOULD use this package. The older
// [usecasepgx.Commit] / [usecasepgx.CommitDelete] / etc. surface
// remains available during the migration and will be removed once
// every aggregate is ported.
//
// # The contract
//
// A use case becomes a plain function:
//
//	func CreateEventType(ctx context.Context, deps *Deps, cmd CreateCommand, ac *auth.Context) (commit.Committed[EventTypeCreated], error) {
//	    if strings.TrimSpace(cmd.Code) == "" {
//	        return commit.Committed[EventTypeCreated]{}, usecase.Validation("CODE_REQUIRED", "...")
//	    }
//	    existing, err := deps.Repo.FindByCode(ctx, cmd.Code)
//	    if err != nil {
//	        return commit.Committed[EventTypeCreated]{}, usecase.Internal("REPO", "find_by_code failed", err)
//	    }
//	    if existing != nil {
//	        return commit.Committed[EventTypeCreated]{}, usecase.Conflict("CODE_EXISTS", "...")
//	    }
//	    et, _ := eventtype.New(cmd.Code, cmd.Name)
//	    event := EventTypeCreated{ /* ... */ }
//	    return commit.Save(ctx, deps.UoW, et, deps.Repo, event, cmd)
//	}
//
// And the HTTP handler:
//
//	committed, err := operations.CreateEventType(r.Context(), s.deps, cmd, ac)
//	if err != nil { httperror.Write(w, err); return }
//	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: committed.Event().EventTypeID})
//
// # The seal in one paragraph
//
// External code can syntactically construct a zero [Committed][E] via
// `Committed[E]{}` (Go's zero-value rule), but the resulting [Event]
// method returns the zero value of E — no event ID, no event type, no
// data. Nothing was written to the database. The invariant we care
// about ("aggregate row exists ⇒ event row exists") holds vacuously
// when no aggregate row was written. External code cannot put a real
// event into [Committed][E] because the `event` field is unexported;
// only [Save] / [Delete] / [SaveAll] / [Emit] can — and each of those
// writes the event row in the same transaction as the aggregate row.
package commit

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sealed"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Committed[E] is the sealed witness that domain event E was written
// atomically with its aggregate. See the package doc for the seal
// rationale.
type Committed[E any] struct {
	// _ holds a [sealed.Token]. It carries no information at runtime
	// (the zero value is the same as sealed.New()) but signals at
	// import time that this type belongs to the sealed unit-of-work
	// family — see [internal/sealed].
	_     sealed.Token
	event E
}

// Event returns the committed domain event. For an externally
// zero-constructed Committed[E] this returns the zero value of E.
func (c Committed[E]) Event() E { return c.event }

// Save persists agg via repo, writes event and audit log in the same
// transaction, and returns Committed[E] on success.
//
// Delegates to [usecasepgx.Commit] for the transaction machinery; the
// only difference between the two surfaces is return shape.
func Save[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
	command C,
) (Committed[E], error) {
	return unwrap(usecasepgx.Commit(ctx, uow, agg, repo, event, command))
}

// Delete removes agg via repo, writes event and audit log atomically.
func Delete[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
	command C,
) (Committed[E], error) {
	return unwrap(usecasepgx.CommitDelete(ctx, uow, agg, repo, event, command))
}

// SaveAll persists multiple aggregates of the same type along with one
// summary event + audit log in a single transaction. For batch
// operations (e.g. toggling client→application enablement) where one
// logical command touches many rows.
func SaveAll[A usecase.HasID, E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	aggs []A,
	repo usecasepgx.Persist[A],
	event E,
	command C,
) (Committed[E], error) {
	return unwrap(usecasepgx.CommitAll(ctx, uow, aggs, repo, event, command))
}

// Emit writes a domain event + audit log with no aggregate write. For
// events that don't modify any entity directly (e.g. login attempts).
func Emit[E usecase.DomainEvent, C any](
	ctx context.Context,
	uow *usecasepgx.UnitOfWork,
	event E,
	command C,
) (Committed[E], error) {
	return unwrap(usecasepgx.EmitEvent(ctx, uow, event, command))
}

// unwrap converts a usecasepgx Result[E] into the (Committed[E], error)
// shape this package exposes. The conversion is the only path through
// which the unexported event field gets populated, which is what makes
// it the seal point.
func unwrap[E any](r usecase.Result[E]) (Committed[E], error) {
	e, err := usecase.Into(r)
	if err != nil {
		return Committed[E]{}, err
	}
	return Committed[E]{event: e}, nil
}

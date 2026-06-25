package usecaseop

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Plan describes a pending change and the domain event it produces, built
// during an operation's Execute phase but NOT yet committed. [Run] is the
// only thing that applies a Plan, and it does so inside one transaction —
// writing the aggregate change, the domain event, and the audit log
// atomically.
//
// Plan is a sealed interface: its sole method is unexported, so only this
// package can implement it. The only constructors are [Save], [Delete],
// [Emit], [SaveAll], and [Sync]. An operation therefore cannot reach the
// database except by returning a Plan and letting Run apply it after Validate
// and Authorize have run — which is what makes "aggregate written ⇒ event +
// audit written, atomically" hold by construction.
type Plan[E usecase.DomainEvent] interface {
	// apply performs the planned change in the unit of work's transaction and
	// returns the committed event. command is the audit subject (the original
	// command), threaded in by Run. Unexported: this is the seal.
	apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error)
}

// Save plans an aggregate upsert with its domain event. Use for create and
// update operations.
func Save[A usecase.HasID, E usecase.DomainEvent](
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return savePlan[A, E]{agg: agg, repo: repo, event: event}
}

// Delete plans an aggregate deletion with its domain event.
func Delete[A usecase.HasID, E usecase.DomainEvent](
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return deletePlan[A, E]{agg: agg, repo: repo, event: event}
}

// Emit plans a domain event with no aggregate change (e.g. UserLoggedIn).
func Emit[E usecase.DomainEvent](event E) Plan[E] {
	return emitPlan[E]{event: event}
}

// SaveAll plans an upsert of many aggregates of one type with a single
// summary event. For one logical command that touches many rows.
func SaveAll[A usecase.HasID, E usecase.DomainEvent](
	aggs []A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return saveAllPlan[A, E]{aggs: aggs, repo: repo, event: event}
}

// Sync plans a batch of per-row saves and deletes plus a rollup event, all
// in one transaction. For sync / bulk-upsert endpoints whose consumers
// project the per-row events. The rollup is the returned event.
func Sync[A usecase.HasID, RE usecase.DomainEvent](
	repo usecasepgx.Persist[A],
	saves []usecasepgx.SyncSaveItem[A],
	deletes []usecasepgx.SyncDeleteItem[A],
	rollup RE,
) Plan[RE] {
	return syncPlan[A, RE]{repo: repo, saves: saves, deletes: deletes, rollup: rollup}
}

type savePlan[A usecase.HasID, E usecase.DomainEvent] struct {
	agg   *A
	repo  usecasepgx.Persist[A]
	event E
}

func (p savePlan[A, E]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error) {
	return usecase.Into(usecasepgx.Commit(ctx, uow, p.agg, p.repo, p.event, command))
}

type deletePlan[A usecase.HasID, E usecase.DomainEvent] struct {
	agg   *A
	repo  usecasepgx.Persist[A]
	event E
}

func (p deletePlan[A, E]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error) {
	return usecase.Into(usecasepgx.CommitDelete(ctx, uow, p.agg, p.repo, p.event, command))
}

type emitPlan[E usecase.DomainEvent] struct {
	event E
}

func (p emitPlan[E]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error) {
	return usecase.Into(usecasepgx.EmitEvent(ctx, uow, p.event, command))
}

type saveAllPlan[A usecase.HasID, E usecase.DomainEvent] struct {
	aggs  []A
	repo  usecasepgx.Persist[A]
	event E
}

func (p saveAllPlan[A, E]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error) {
	return usecase.Into(usecasepgx.CommitAll(ctx, uow, p.aggs, p.repo, p.event, command))
}

type syncPlan[A usecase.HasID, RE usecase.DomainEvent] struct {
	repo    usecasepgx.Persist[A]
	saves   []usecasepgx.SyncSaveItem[A]
	deletes []usecasepgx.SyncDeleteItem[A]
	rollup  RE
}

func (p syncPlan[A, RE]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (RE, error) {
	return usecase.Into(usecasepgx.CommitSync(ctx, uow, p.repo, p.saves, p.deletes, p.rollup, command))
}

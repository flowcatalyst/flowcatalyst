package usecaseop

import (
	"context"
	"slices"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Plan describes a pending change and the domain event it produces, built
// during an operation's Execute phase but NOT yet committed. [Run] is the
// only thing that applies a Plan, and it does so inside one transaction —
// writing the aggregate change, the domain event, the plan's dispatch jobs
// and the audit log atomically.
//
// Plan is a sealed interface: its apply method is unexported, so only this
// package can implement it. The only constructors are [Save], [Delete],
// [Emit], [SaveAll], and [Sync]. An operation therefore cannot reach the
// database except by returning a Plan and letting Run apply it after Validate
// and Authorize have run — which is what makes "aggregate written ⇒ event +
// audit written, atomically" hold by construction.
type Plan[E usecase.DomainEvent] interface {
	// Dispatch returns a copy of the plan that also writes these dispatch
	// jobs (framework write-path contract 7a): DISPATCH_JOB outbox rows in
	// the same transaction, after the plan's events and before its audit
	// row, in the order given (calls accumulate). The platform's outbox
	// processor delivers them to POST /api/dispatch-jobs/batch. The plan it
	// is called on is not changed.
	//
	// Run validates every job (outbox.DispatchJob.Validate: the 7a rules,
	// with their texts) before the transaction opens; an invalid job fails
	// the operation as INTERNAL DISPATCH_JOB_INVALID (Message = the rule's
	// text, Details["index"] = the job's position) and nothing is written.
	// Check values that come from the command in Validate, so the caller
	// gets a 400 instead. A job without a correlation id takes the plan's
	// event's (the returned event; a Sync plan's rollup). Nothing else is
	// inherited: the job's client is what it names (WithClientCode or
	// WithPlatformClientID; neither leaves it unscoped), and its eventId is
	// set only when the job sets it. The UnitOfWork's sink must implement
	// usecasepgx.DispatchJobSink (outboxpgx.Sink does); otherwise Run fails
	// with INTERNAL DISPATCH_JOB_UNSUPPORTED, again before any write.
	Dispatch(jobs ...outbox.DispatchJob) Plan[E]

	// apply performs the planned change in the unit of work's transaction and
	// returns the committed event. command is the audit subject (the original
	// command), threaded in by Run. Unexported: this is the seal.
	apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error)
}

// commitFunc is one constructor's commit, with the plan's commit options.
type commitFunc[E usecase.DomainEvent] func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (E, error)

// plan is the only Plan implementation: what to commit, and the dispatch jobs
// to write with it.
type plan[E usecase.DomainEvent] struct {
	commit commitFunc[E]
	jobs   []outbox.DispatchJob
}

func (p plan[E]) Dispatch(jobs ...outbox.DispatchJob) Plan[E] {
	// Clip so that two Dispatch calls on one plan never share a backing array.
	p.jobs = append(slices.Clip(p.jobs), jobs...)
	return p
}

func (p plan[E]) apply(ctx context.Context, uow *usecasepgx.UnitOfWork, command any) (E, error) {
	if len(p.jobs) == 0 {
		return p.commit(ctx, uow, command)
	}
	return p.commit(ctx, uow, command, usecasepgx.WithDispatchJobs(p.jobs...))
}

// Save plans an aggregate upsert with its domain event. Use for create and
// update operations.
func Save[A usecase.HasID, E usecase.DomainEvent](
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return plan[E]{commit: func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (E, error) {
		return usecase.Into(usecasepgx.Commit(ctx, uow, agg, repo, event, command, opts...))
	}}
}

// Delete plans an aggregate deletion with its domain event.
func Delete[A usecase.HasID, E usecase.DomainEvent](
	agg *A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return plan[E]{commit: func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (E, error) {
		return usecase.Into(usecasepgx.CommitDelete(ctx, uow, agg, repo, event, command, opts...))
	}}
}

// Emit plans a domain event with no aggregate change (e.g. UserLoggedIn).
func Emit[E usecase.DomainEvent](event E) Plan[E] {
	return plan[E]{commit: func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (E, error) {
		return usecase.Into(usecasepgx.EmitEvent(ctx, uow, event, command, opts...))
	}}
}

// SaveAll plans an upsert of many aggregates of one type with a single
// summary event. For one logical command that touches many rows.
func SaveAll[A usecase.HasID, E usecase.DomainEvent](
	aggs []A,
	repo usecasepgx.Persist[A],
	event E,
) Plan[E] {
	return plan[E]{commit: func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (E, error) {
		return usecase.Into(usecasepgx.CommitAll(ctx, uow, aggs, repo, event, command, opts...))
	}}
}

// Sync plans a batch of per-row saves and deletes plus a rollup event, all
// in one transaction. For sync / bulk-upsert endpoints whose consumers
// project the per-row events. The rollup is the returned event. Dispatch
// jobs are written after the rollup event, before its audit row.
func Sync[A usecase.HasID, RE usecase.DomainEvent](
	repo usecasepgx.Persist[A],
	saves []usecasepgx.SyncSaveItem[A],
	deletes []usecasepgx.SyncDeleteItem[A],
	rollup RE,
) Plan[RE] {
	return plan[RE]{commit: func(ctx context.Context, uow *usecasepgx.UnitOfWork, command any, opts ...usecasepgx.CommitOption) (RE, error) {
		return usecase.Into(usecasepgx.CommitSync(ctx, uow, repo, saves, deletes, rollup, command, opts...))
	}}
}

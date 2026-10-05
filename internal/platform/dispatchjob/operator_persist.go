package dispatchjob

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ErrTransitionRefused is returned by an operator action's persister when its
// guarded transition matched no row (the job is not in a status the action may
// move from): the use-case transaction rolls back, so the event and audit rows
// are not written for a change that did not happen.
var ErrTransitionRefused = errors.New("dispatch job transition refused: job not in an allowed status")

// operatorPersist adapts one lifecycle operation to usecasepgx.Persist, so the
// operator use cases (cancel / complete / resend) keep committing the change,
// event and audit atomically in the unit of work's transaction while the status
// write itself is the lifecycle's. There is deliberately no generic "save the
// job" persister: none of these writes a status the lifecycle did not name.
type operatorPersist struct {
	lc *Lifecycle
	do func(ctx context.Context, lc *Lifecycle, j *DispatchJob) (bool, error)
	// strict makes a refused transition an error (cancel/complete); a requeue
	// of an unknown id stays the silent no-op it has always been.
	strict bool
}

func (p operatorPersist) Persist(ctx context.Context, j *DispatchJob, tx *usecasepgx.DbTx) error {
	ok, err := p.do(ctx, p.lc.In(tx.Inner()), j)
	if err != nil {
		return err
	}
	if !ok && p.strict {
		return ErrTransitionRefused
	}
	return nil
}

func (operatorPersist) Delete(context.Context, *DispatchJob, *usecasepgx.DbTx) error {
	return errors.New("dispatch jobs are never deleted through the use-case envelope")
}

// CancelPersister commits an operator cancel: FAILED -> CANCELLED, checked in SQL.
func (r *Repository) CancelPersister() usecasepgx.Persist[DispatchJob] {
	return operatorPersist{lc: r.lc, strict: true, do: func(ctx context.Context, lc *Lifecycle, j *DispatchJob) (bool, error) {
		return lc.OperatorCancel(ctx, j.ID, j.CreatedAt)
	}}
}

// CompletePersister commits an operator complete: FAILED -> COMPLETED, checked in SQL.
func (r *Repository) CompletePersister() usecasepgx.Persist[DispatchJob] {
	return operatorPersist{lc: r.lc, strict: true, do: func(ctx context.Context, lc *Lifecycle, j *DispatchJob) (bool, error) {
		return lc.OperatorComplete(ctx, j.ID, j.CreatedAt)
	}}
}

// RequeuePersister commits an operator requeue: any status -> PENDING.
func (r *Repository) RequeuePersister() usecasepgx.Persist[DispatchJob] {
	return operatorPersist{lc: r.lc, do: func(ctx context.Context, lc *Lifecycle, j *DispatchJob) (bool, error) {
		return lc.Requeue(ctx, j.ID, j.CreatedAt)
	}}
}

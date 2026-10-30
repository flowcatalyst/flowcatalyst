package usecasepgx

import (
	"context"
	"slices"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// DispatchJobSink is a Sink that also writes dispatch jobs (outbox rows of
// type DISPATCH_JOB, which the platform's outbox processor delivers to POST
// /api/dispatch-jobs/batch). outboxpgx.Sink implements it. A commit that
// carries dispatch jobs through a Sink without it fails before anything is
// written (INTERNAL DISPATCH_JOB_UNSUPPORTED).
type DispatchJobSink interface {
	Sink
	// WriteDispatchJobs writes the jobs, in order, in the open transaction.
	// The jobs are already validated and resolved (each has its id). Must
	// not commit or roll back the tx.
	WriteDispatchJobs(ctx context.Context, tx *DbTx, jobs []outbox.DispatchJob) error
}

// CommitOption adds to what a commit writes besides the aggregate change, its
// event and its audit row.
type CommitOption func(*commitOptions)

type commitOptions struct {
	jobs []outbox.DispatchJob
}

// WithDispatchJobs makes the commit also write these dispatch jobs, in the
// same transaction: after the event, before the audit row (framework
// write-path contract 7a). Every job is validated (outbox.DispatchJob.Validate,
// the 7a rules and texts) before the transaction opens, so an invalid job
// fails the commit as INTERNAL DISPATCH_JOB_INVALID with nothing written. A
// job without a correlation id takes the committed event's. usecaseop's
// Plan.Dispatch is the usual way to pass jobs; this option is for the
// lower-level Commit* / *Scoped helpers.
func WithDispatchJobs(jobs ...outbox.DispatchJob) CommitOption {
	jobs = slices.Clone(jobs)
	return func(o *commitOptions) { o.jobs = append(o.jobs, jobs...) }
}

// preparedJobs are a commit's dispatch jobs, validated and resolved, and the
// sink that writes them.
type preparedJobs struct {
	jobs []outbox.DispatchJob
	sink DispatchJobSink
}

// prepareJobs validates and resolves the commit's jobs against event (the
// event the commit returns) before anything is written.
func prepareJobs(sink Sink, event usecase.DomainEvent, opts []CommitOption) (preparedJobs, *usecase.Error) {
	var o commitOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if len(o.jobs) == 0 {
		return preparedJobs{}, nil
	}
	resolved := make([]outbox.DispatchJob, len(o.jobs))
	for i, j := range o.jobs {
		if j.CorrelationID == "" {
			j.CorrelationID = event.CorrelationID()
		}
		r, err := j.Resolved()
		if err != nil {
			return preparedJobs{}, usecase.Internal("DISPATCH_JOB_INVALID", err.Error(), nil).
				WithDetails(map[string]any{"index": i})
		}
		resolved[i] = r
	}
	js, ok := sink.(DispatchJobSink)
	if !ok {
		return preparedJobs{}, usecase.Internal("DISPATCH_JOB_UNSUPPORTED",
			"the configured sink cannot write dispatch jobs (it does not implement usecasepgx.DispatchJobSink)", nil)
	}
	return preparedJobs{jobs: resolved, sink: js}, nil
}

// write writes the jobs; a no-op when there are none.
func (p preparedJobs) write(ctx context.Context, tx *DbTx) *usecase.Error {
	if len(p.jobs) == 0 {
		return nil
	}
	if err := p.sink.WriteDispatchJobs(ctx, tx, p.jobs); err != nil {
		return usecase.Internal("DISPATCH_JOB_WRITE", "could not write dispatch jobs", err)
	}
	return nil
}

// writeOutbox writes event, then the jobs, then the audit row of
// (event, command): the order of write-path contract 7a.
func writeOutbox(ctx context.Context, sink Sink, tx *DbTx, event usecase.DomainEvent, jobs preparedJobs, command any) *usecase.Error {
	if err := sink.WriteEvent(ctx, tx, event); err != nil {
		return usecase.Internal("EVENT_WRITE", "could not write domain event", err)
	}
	if err := jobs.write(ctx, tx); err != nil {
		return err
	}
	if err := sink.WriteAudit(ctx, tx, event, command); err != nil {
		return usecase.Internal("AUDIT_WRITE", "could not write audit log", err)
	}
	return nil
}

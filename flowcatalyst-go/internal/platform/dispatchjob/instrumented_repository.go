package dispatchjob

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/common/repository"
)

const collectionName = "dispatch_jobs"

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindByID", func() (*DispatchJob, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByIdempotencyKey(ctx context.Context, key string) (*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindByIdempotencyKey", func() (*DispatchJob, error) {
		return r.inner.FindByIdempotencyKey(ctx, key)
	})
}

func (r *instrumentedRepository) FindByEventID(ctx context.Context, eventID string) ([]*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindByEventID", func() ([]*DispatchJob, error) {
		return r.inner.FindByEventID(ctx, eventID)
	})
}

func (r *instrumentedRepository) FindBySubscription(ctx context.Context, subscriptionID string, skip, limit int64) ([]*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindBySubscription", func() ([]*DispatchJob, error) {
		return r.inner.FindBySubscription(ctx, subscriptionID, skip, limit)
	})
}

func (r *instrumentedRepository) FindPending(ctx context.Context, limit int64) ([]*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindPending", func() ([]*DispatchJob, error) {
		return r.inner.FindPending(ctx, limit)
	})
}

func (r *instrumentedRepository) FindPendingByPool(ctx context.Context, poolID string, limit int64) ([]*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindPendingByPool", func() ([]*DispatchJob, error) {
		return r.inner.FindPendingByPool(ctx, poolID, limit)
	})
}

func (r *instrumentedRepository) FindStaleQueued(ctx context.Context, threshold time.Duration) ([]*DispatchJob, error) {
	return repository.Instrument(ctx, collectionName, "FindStaleQueued", func() ([]*DispatchJob, error) {
		return r.inner.FindStaleQueued(ctx, threshold)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, job *DispatchJob) error {
	return repository.InstrumentVoid(ctx, collectionName, "Insert", func() error {
		return r.inner.Insert(ctx, job)
	})
}

func (r *instrumentedRepository) InsertMany(ctx context.Context, jobs []*DispatchJob) error {
	return repository.InstrumentVoid(ctx, collectionName, "InsertMany", func() error {
		return r.inner.InsertMany(ctx, jobs)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, job *DispatchJob) error {
	return repository.InstrumentVoid(ctx, collectionName, "Update", func() error {
		return r.inner.Update(ctx, job)
	})
}

func (r *instrumentedRepository) UpdateStatus(ctx context.Context, id string, status DispatchStatus) error {
	return repository.InstrumentVoid(ctx, collectionName, "UpdateStatus", func() error {
		return r.inner.UpdateStatus(ctx, id, status)
	})
}

func (r *instrumentedRepository) MarkQueued(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "MarkQueued", func() error {
		return r.inner.MarkQueued(ctx, id)
	})
}

func (r *instrumentedRepository) MarkInProgress(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "MarkInProgress", func() error {
		return r.inner.MarkInProgress(ctx, id)
	})
}

func (r *instrumentedRepository) MarkCompleted(ctx context.Context, id string, durationMillis int64) error {
	return repository.InstrumentVoid(ctx, collectionName, "MarkCompleted", func() error {
		return r.inner.MarkCompleted(ctx, id, durationMillis)
	})
}

func (r *instrumentedRepository) MarkError(ctx context.Context, id string, errorMsg string) error {
	return repository.InstrumentVoid(ctx, collectionName, "MarkError", func() error {
		return r.inner.MarkError(ctx, id, errorMsg)
	})
}

func (r *instrumentedRepository) RecordAttempt(ctx context.Context, id string, attempt DispatchAttempt) error {
	return repository.InstrumentVoid(ctx, collectionName, "RecordAttempt", func() error {
		return r.inner.RecordAttempt(ctx, id, attempt)
	})
}

func (r *instrumentedRepository) ResetToPending(ctx context.Context, id string, scheduledFor time.Time) error {
	return repository.InstrumentVoid(ctx, collectionName, "ResetToPending", func() error {
		return r.inner.ResetToPending(ctx, id, scheduledFor)
	})
}

func (r *instrumentedRepository) CountByStatus(ctx context.Context, status DispatchStatus) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountByStatus", func() (int64, error) {
		return r.inner.CountByStatus(ctx, status)
	})
}

func (r *instrumentedRepository) CountByGroupAndStatus(ctx context.Context, messageGroup string, status DispatchStatus) (int64, error) {
	return repository.Instrument(ctx, collectionName, "CountByGroupAndStatus", func() (int64, error) {
		return r.inner.CountByGroupAndStatus(ctx, messageGroup, status)
	})
}

func (r *instrumentedRepository) HasErrorJobsInGroup(ctx context.Context, messageGroup string) (bool, error) {
	return repository.Instrument(ctx, collectionName, "HasErrorJobsInGroup", func() (bool, error) {
		return r.inner.HasErrorJobsInGroup(ctx, messageGroup)
	})
}

func (r *instrumentedRepository) GetBlockedMessageGroups(ctx context.Context, groups []string) (map[string]bool, error) {
	return repository.Instrument(ctx, collectionName, "GetBlockedMessageGroups", func() (map[string]bool, error) {
		return r.inner.GetBlockedMessageGroups(ctx, groups)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionName, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}

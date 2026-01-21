package dispatchjob

import (
	"context"
	"time"
)

// Repository defines the interface for dispatch job data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	FindByID(ctx context.Context, id string) (*DispatchJob, error)
	FindByIdempotencyKey(ctx context.Context, key string) (*DispatchJob, error)
	FindByEventID(ctx context.Context, eventID string) ([]*DispatchJob, error)
	FindBySubscription(ctx context.Context, subscriptionID string, skip, limit int64) ([]*DispatchJob, error)
	FindPending(ctx context.Context, limit int64) ([]*DispatchJob, error)
	FindPendingByPool(ctx context.Context, poolID string, limit int64) ([]*DispatchJob, error)
	FindStaleQueued(ctx context.Context, threshold time.Duration) ([]*DispatchJob, error)
	Insert(ctx context.Context, job *DispatchJob) error
	InsertMany(ctx context.Context, jobs []*DispatchJob) error
	Update(ctx context.Context, job *DispatchJob) error
	UpdateStatus(ctx context.Context, id string, status DispatchStatus) error
	MarkQueued(ctx context.Context, id string) error
	MarkInProgress(ctx context.Context, id string) error
	MarkCompleted(ctx context.Context, id string, durationMillis int64) error
	MarkError(ctx context.Context, id string, errorMsg string) error
	RecordAttempt(ctx context.Context, id string, attempt DispatchAttempt) error
	ResetToPending(ctx context.Context, id string, scheduledFor time.Time) error
	CountByStatus(ctx context.Context, status DispatchStatus) (int64, error)
	CountByGroupAndStatus(ctx context.Context, messageGroup string, status DispatchStatus) (int64, error)
	HasErrorJobsInGroup(ctx context.Context, messageGroup string) (bool, error)
	GetBlockedMessageGroups(ctx context.Context, groups []string) (map[string]bool, error)
	Delete(ctx context.Context, id string) error
}

package outbox

import (
	"context"
	"time"
)

// Repository defines the interface for outbox data access.
// Uses a single-poller, status-based pattern with NO row locking.
// This works identically across PostgreSQL, MySQL, and MongoDB.
type Repository interface {
	// FetchPending fetches pending items (status=0) ordered by message_group, created_at.
	// Does NOT lock or modify the items - caller must call MarkAsInProgress after.
	// This is safe because only one poller runs (enforced by leader election).
	FetchPending(ctx context.Context, itemType OutboxItemType, limit int) ([]*OutboxItem, error)

	// MarkAsInProgress marks items as in-progress (status=9).
	// Must be called immediately after FetchPending, before distributing to queues.
	// This prevents re-polling of items that are being processed.
	MarkAsInProgress(ctx context.Context, itemType OutboxItemType, ids []string) error

	// MarkWithStatus updates items to the specified status code.
	// Used for both success (status=1) and various error types (status=2-6).
	MarkWithStatus(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus) error

	// MarkWithStatusAndError updates items to the specified status with an error message.
	MarkWithStatusAndError(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus, errorMessage string) error

	// FetchStuckItems fetches items stuck in in-progress status (status=9).
	// Used on startup for crash recovery.
	FetchStuckItems(ctx context.Context, itemType OutboxItemType) ([]*OutboxItem, error)

	// ResetStuckItems resets stuck items back to pending (status=0).
	// Used on startup for crash recovery.
	ResetStuckItems(ctx context.Context, itemType OutboxItemType, ids []string) error

	// IncrementRetryCount increments the retry count for items and resets to pending.
	// Used when an item fails but should be retried.
	IncrementRetryCount(ctx context.Context, itemType OutboxItemType, ids []string) error

	// FetchRecoverableItems fetches items eligible for periodic recovery.
	// Returns items with error statuses (IN_PROGRESS, BAD_REQUEST, INTERNAL_ERROR,
	// UNAUTHORIZED, FORBIDDEN, GATEWAY_ERROR) that have been in that status
	// longer than the specified timeout.
	FetchRecoverableItems(ctx context.Context, itemType OutboxItemType, timeoutSeconds int, limit int) ([]*OutboxItem, error)

	// ResetRecoverableItems resets recoverable items back to PENDING status for retry.
	// Does NOT reset retry count - items will be retried with existing count.
	ResetRecoverableItems(ctx context.Context, itemType OutboxItemType, ids []string) error

	// CountPending returns the count of pending items (for metrics).
	CountPending(ctx context.Context, itemType OutboxItemType) (int64, error)

	// GetTableName returns the table/collection name for the item type.
	GetTableName(itemType OutboxItemType) string

	// CreateSchema creates the outbox tables/collections if they don't exist.
	CreateSchema(ctx context.Context) error
}

// RepositoryConfig holds configuration for the outbox repository
type RepositoryConfig struct {
	// EventsTable is the table name for event outbox items
	EventsTable string

	// DispatchJobsTable is the table name for dispatch job outbox items
	DispatchJobsTable string

	// DatabaseType is the type of database
	DatabaseType DatabaseType
}

// DefaultRepositoryConfig returns sensible defaults
func DefaultRepositoryConfig() *RepositoryConfig {
	return &RepositoryConfig{
		EventsTable:       "outbox_events",
		DispatchJobsTable: "outbox_dispatch_jobs",
		DatabaseType:      DatabaseTypeMongoDB,
	}
}

// BatchResult represents the result of a batch API call
type BatchResult struct {
	// SuccessIDs are the IDs that were successfully processed
	SuccessIDs []string

	// FailedItems maps item ID to the status code for that failure
	FailedItems map[string]OutboxStatus

	// Error is set if the entire batch failed
	Error error
}

// NewBatchResult creates a new BatchResult
func NewBatchResult() *BatchResult {
	return &BatchResult{
		SuccessIDs:  make([]string, 0),
		FailedItems: make(map[string]OutboxStatus),
	}
}

// ProcessorStats represents processing statistics
type ProcessorStats struct {
	// Status is "UP" or "DOWN"
	Status string

	// Healthy indicates if the processor is functioning normally
	Healthy bool

	// LastPollTime is when the last successful poll occurred
	LastPollTime time.Time

	// ActiveMessageGroups is the number of active message group queues
	ActiveMessageGroups int

	// InFlightPermits is the number of available in-flight permits
	InFlightPermits int

	// TotalInFlightCapacity is the maximum in-flight capacity
	TotalInFlightCapacity int

	// BufferedItems is the total number of items in message group queues
	BufferedItems int

	// TotalProcessed is the total number of items processed
	TotalProcessed int64

	// TotalSuccess is the total number of successful items
	TotalSuccess int64

	// TotalFailed is the total number of failed items
	TotalFailed int64
}

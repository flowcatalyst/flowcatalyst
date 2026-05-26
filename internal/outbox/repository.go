// Package outbox implements the outbox processor: it polls the
// consumer application's outbox table, batches by message group, and
// forwards to the FlowCatalyst platform API. Mirrors fc-outbox/src/*.
//
// Multi-backend: Postgres, SQLite, MySQL, MongoDB. The Repository
// interface abstracts the storage; each backend lives in its own
// subpackage and registers a factory at init time.
package outbox

import (
	"context"
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Item is one outbox row. Wire shape matches the Rust OutboxItem.
type Item struct {
	ID            string             `json:"id"`
	ItemType      common.OutboxItemType `json:"itemType"`
	MessageGroup  *string            `json:"messageGroup,omitempty"`
	Payload       json.RawMessage    `json:"payload"`
	Status        common.OutboxStatus
	StatusMessage string             `json:"statusMessage,omitempty"`
	AttemptCount  int                `json:"attemptCount"`
	NextRetryAt   *time.Time         `json:"nextRetryAt,omitempty"`
	CreatedAt     time.Time          `json:"createdAt"`
	UpdatedAt     time.Time          `json:"updatedAt"`
}

// Repository is the per-backend storage interface.
type Repository interface {
	// ClaimPending claims up to batchSize PENDING items, marks them IN_PROGRESS,
	// and returns them. Each backend implements this with a backend-appropriate
	// claim semantic (FOR UPDATE SKIP LOCKED for SQL, findAndUpdate for Mongo).
	ClaimPending(ctx context.Context, batchSize int) ([]Item, error)
	// MarkSuccess sets the items SUCCESS.
	MarkSuccess(ctx context.Context, ids []string) error
	// MarkFailed sets items to a failed status with an attempt-count bump and
	// next-retry-at backoff.
	MarkFailed(ctx context.Context, ids []string, status common.OutboxStatus, msg string, nextRetry time.Time) error
	// Healthy reports whether the backend can be reached.
	Healthy(ctx context.Context) bool
	// InitSchema ensures the outbox table/collection exists.
	InitSchema(ctx context.Context) error
}

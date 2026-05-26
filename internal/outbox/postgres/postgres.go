// Package postgres is the Postgres-backed outbox repository.
//
// Schema (matches fc-outbox/src/postgres.rs):
//
//	CREATE TABLE outbox_messages (
//	    id              TEXT PRIMARY KEY,
//	    item_type       TEXT NOT NULL,
//	    message_group   TEXT,
//	    payload         JSONB NOT NULL,
//	    status          INT NOT NULL DEFAULT 0,
//	    status_message  TEXT,
//	    attempt_count   INT NOT NULL DEFAULT 0,
//	    next_retry_at   TIMESTAMPTZ,
//	    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//	CREATE INDEX outbox_messages_pending_idx
//	    ON outbox_messages (status, next_retry_at)
//	    WHERE status IN (0, 3, 4, 6); -- PENDING + retryable
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
)

// Repository is the Postgres outbox repository.
type Repository struct {
	pool *pgxpool.Pool
}

// New wires a repository against an existing pool.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// InitSchema creates the outbox table and indexes if missing.
func (r *Repository) InitSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS outbox_messages (
    id             TEXT PRIMARY KEY,
    item_type      TEXT NOT NULL,
    message_group  TEXT,
    payload        JSONB NOT NULL,
    status         INT NOT NULL DEFAULT 0,
    status_message TEXT,
    attempt_count  INT NOT NULL DEFAULT 0,
    next_retry_at  TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS outbox_messages_pending_idx
    ON outbox_messages (status, next_retry_at)
    WHERE status IN (0, 3, 4, 6);
`
	_, err := r.pool.Exec(ctx, ddl)
	return err
}

// ClaimPending claims a batch of pending items via FOR UPDATE SKIP LOCKED.
func (r *Repository) ClaimPending(ctx context.Context, batchSize int) ([]outbox.Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
WITH claimed AS (
  SELECT id FROM outbox_messages
   WHERE status IN (0, 3, 4, 6)
     AND (next_retry_at IS NULL OR next_retry_at <= NOW())
   ORDER BY created_at
   LIMIT $1
   FOR UPDATE SKIP LOCKED
)
UPDATE outbox_messages m
   SET status = 9, updated_at = NOW()
  FROM claimed
 WHERE m.id = claimed.id
 RETURNING m.id, m.item_type, m.message_group, m.payload, m.status, m.status_message,
           m.attempt_count, m.next_retry_at, m.created_at, m.updated_at
`, batchSize)
	if err != nil {
		return nil, fmt.Errorf("claim: %w", err)
	}
	defer rows.Close()

	var out []outbox.Item
	for rows.Next() {
		var item outbox.Item
		var itemType string
		var msgGroup *string
		var payload []byte
		var statusInt int
		var statusMsg *string
		var nextRetry *time.Time
		if err := rows.Scan(&item.ID, &itemType, &msgGroup, &payload, &statusInt, &statusMsg,
			&item.AttemptCount, &nextRetry, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ItemType = common.OutboxItemType(itemType)
		item.MessageGroup = msgGroup
		item.Payload = json.RawMessage(payload)
		item.Status = common.FromOutboxCode(statusInt)
		if statusMsg != nil {
			item.StatusMessage = *statusMsg
		}
		item.NextRetryAt = nextRetry
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return out, nil
}

// MarkSuccess flips items to SUCCESS.
func (r *Repository) MarkSuccess(ctx context.Context, ids []string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages SET status = 1, updated_at = NOW() WHERE id = ANY($1)`,
		ids)
	return err
}

// MarkFailed flips items to a failed status + records the message and next retry.
func (r *Repository) MarkFailed(ctx context.Context, ids []string, status common.OutboxStatus, msg string, nextRetry time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages
		    SET status = $1, status_message = $2, attempt_count = attempt_count + 1,
		        next_retry_at = $3, updated_at = NOW()
		  WHERE id = ANY($4)`,
		status.Code(), msg, nextRetry, ids)
	return err
}

// Healthy pings the pool.
func (r *Repository) Healthy(ctx context.Context) bool {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.pool.Ping(c) == nil
}

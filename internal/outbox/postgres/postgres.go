// Package postgres is the Postgres-backed outbox repository. It is
// schema-compatible with the SDK customer outbox migration
// (clients/*/migrations/postgresql/001_create_outbox_messages.sql), so the Go
// processor can be pointed at a real customer database created by the SDK.
//
// Schema (created by InitSchema; matches the SDK migration exactly):
//
//	CREATE TABLE outbox_messages (
//	    id            VARCHAR(26) PRIMARY KEY,
//	    type          VARCHAR(20) NOT NULL,
//	    message_group VARCHAR(255),
//	    payload       TEXT NOT NULL,
//	    status        SMALLINT NOT NULL DEFAULT 0,
//	    retry_count   SMALLINT NOT NULL DEFAULT 0,
//	    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    error_message TEXT,
//	    client_id     VARCHAR(26),     -- SDK-owned; processor reads around it
//	    payload_size  INTEGER,         -- SDK-owned
//	    headers       JSONB            -- SDK-owned
//	);
//
// status 0 = PENDING, 9 = IN_PROGRESS (claimed). The processor claims
// PENDING rows (status=0), DELETEs them on success, and on failure bumps
// retry_count + records error_message (retryable -> back to PENDING, always;
// non-retryable -> keeps its status code: 2 BAD_REQUEST, 5 FORBIDDEN).
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
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
    id            VARCHAR(26) PRIMARY KEY,
    type          VARCHAR(20) NOT NULL,
    message_group VARCHAR(255),
    payload       TEXT NOT NULL,
    status        SMALLINT NOT NULL DEFAULT 0,
    retry_count   SMALLINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error_message TEXT,
    client_id     VARCHAR(26),
    payload_size  INTEGER,
    headers       JSONB
);
CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending
    ON outbox_messages (status, message_group, created_at) WHERE status = 0;
CREATE INDEX IF NOT EXISTS idx_outbox_messages_stuck
    ON outbox_messages (status, created_at) WHERE status = 9;
CREATE INDEX IF NOT EXISTS idx_outbox_client_pending
    ON outbox_messages (client_id, status, created_at);
`
	_, err := r.pool.Exec(ctx, ddl)
	return err
}

// ClaimPending claims a batch of pending items via FOR UPDATE SKIP LOCKED,
// passing over the rows skip names (held, paused or blocked).
func (r *Repository) ClaimPending(ctx context.Context, batchSize int, skip outbox.ClaimFilter) ([]outbox.Item, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// A NULL array would make every NOT IN unknown and claim nothing: pass empty ones.
	types := make([]string, 0, len(skip.Types))
	for _, t := range skip.Types {
		types = append(types, string(t))
	}
	rows, err := tx.Query(ctx, `
WITH claimed AS (
  SELECT id FROM outbox_messages
   WHERE status = 0
     AND (COALESCE(message_group, '') = '' OR message_group NOT IN (SELECT unnest($2::text[])))
     AND (COALESCE(message_group, '') <> '' OR type NOT IN (SELECT unnest($3::text[])))
     AND id NOT IN (SELECT unnest($4::text[]))
   ORDER BY message_group, created_at, id
   LIMIT $1
   FOR UPDATE SKIP LOCKED
)
UPDATE outbox_messages m
   SET status = 9, updated_at = NOW()
  FROM claimed
 WHERE m.id = claimed.id
 RETURNING m.id, m.type, m.message_group, m.payload, m.status, m.retry_count,
           m.error_message, m.created_at, m.updated_at
`, batchSize, nonNil(skip.Groups), types, nonNil(skip.IDs))
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
		var errMsg *string
		if err := rows.Scan(&item.ID, &itemType, &msgGroup, &payload, &statusInt, &item.AttemptCount,
			&errMsg, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ItemType = common.OutboxItemType(itemType)
		item.MessageGroup = msgGroup
		item.Payload = json.RawMessage(payload)
		item.Status = common.FromOutboxCode(statusInt)
		if errMsg != nil {
			item.StatusMessage = *errMsg
		}
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

// MarkSuccess deletes successfully dispatched rows (the upstream
// model DELETEs on success to keep the customer outbox table bounded).
func (r *Repository) MarkSuccess(ctx context.Context, ids []string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM outbox_messages WHERE id = ANY($1)`,
		ids)
	return err
}

// MarkFailed bumps retry_count, records error_message, and sets the status.
// Requeued rows are returned to PENDING (0) so a later poll re-claims them;
// the others keep their code so they are not re-claimed. There is no
// next_retry_at column upstream. retry_count stops at 32767, the SDK
// migration's SMALLINT ceiling: a row retried that often is still retried,
// and the bump must not fail the update.
func (r *Repository) MarkFailed(ctx context.Context, ids []string, status common.OutboxStatus, msg string, requeue bool) error {
	newStatus := status.Code()
	if requeue {
		newStatus = int(common.OutboxPending)
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages
		    SET status = $1, error_message = $2, retry_count = LEAST(retry_count + 1, 32767), updated_at = NOW()
		  WHERE id = ANY($3)`,
		newStatus, msg, ids)
	return err
}

// RecoverStuck resets IN_PROGRESS (9) rows older than olderThan back to
// PENDING (0) so a crash that left rows claimed-but-unresolved self-heals.
// Release returns claimed (IN_PROGRESS) rows to PENDING without a failure
// penalty (no retry bump / error). Used by block-on-error to re-run a group's
// undispatched items in order behind a failed one.
func (r *Repository) Release(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages SET status = 0, updated_at = NOW()
		  WHERE id = ANY($1) AND status = 9`, ids)
	return err
}

// Requeue resets rows to PENDING from ANY status, clearing retry_count + error
// for a fresh attempt (the state machine's Unblock-retry of a poison item).
func (r *Repository) Requeue(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages SET status = 0, retry_count = 0, error_message = NULL, updated_at = NOW()
		  WHERE id = ANY($1)`, ids)
	return err
}

// MarkSkipped puts outbox.SkippedMarker in place of a leading
// outbox.BlockingMarker in the rows' error_message (once), leaving their
// status alone.
func (r *Repository) MarkSkipped(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.pool.Exec(ctx, `
UPDATE outbox_messages
   SET error_message = $2::text || CASE WHEN starts_with(COALESCE(error_message, ''), $3::text)
                                        THEN substr(error_message, char_length($3::text) + 1)
                                        ELSE COALESCE(error_message, '') END,
       updated_at = NOW()
 WHERE id = ANY($1) AND (error_message IS NULL OR NOT starts_with(error_message, $2::text))`,
		ids, outbox.SkippedMarker, outbox.BlockingMarker)
	return err
}

// MarkLegacyRefusals prefixes outbox.SkippedBeforeUpgradeMarker to the
// error_message of rows in a non-retryable failure status (2 BAD_REQUEST,
// 5 FORBIDDEN) that carry no marker.
func (r *Repository) MarkLegacyRefusals(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `
UPDATE outbox_messages
   SET error_message = $2::text || COALESCE(error_message, ''), updated_at = NOW()
 WHERE status = ANY($1)
   AND (error_message IS NULL OR NOT (starts_with(error_message, $2::text)
        OR starts_with(error_message, $3::text) OR starts_with(error_message, $4::text)))`,
		terminalFailureCodes(), outbox.SkippedBeforeUpgradeMarker, outbox.BlockingMarker, outbox.SkippedMarker)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// RequeueRetryableFailures returns rows in a retryable failure status
// (3 INTERNAL_ERROR, 4 UNAUTHORIZED, 6 GATEWAY_ERROR) to PENDING, keeping
// retry_count and error_message.
func (r *Repository) RequeueRetryableFailures(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages SET status = 0, updated_at = NOW() WHERE status = ANY($1)`,
		retryableFailureCodes())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// BlockingFailures returns each message group's oldest row in a non-retryable
// failure status (2 BAD_REQUEST, 5 FORBIDDEN) marked outbox.BlockingMarker.
func (r *Repository) BlockingFailures(ctx context.Context) ([]outbox.Item, error) {
	rows, err := r.pool.Query(ctx, `
SELECT DISTINCT ON (message_group) id, type, message_group, status, retry_count, error_message, created_at, updated_at
  FROM outbox_messages
 WHERE status = ANY($1) AND COALESCE(message_group, '') <> ''
   AND starts_with(error_message, $2::text)
 ORDER BY message_group, created_at, id`, terminalFailureCodes(), outbox.BlockingMarker)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []outbox.Item
	for rows.Next() {
		var item outbox.Item
		var itemType string
		var statusInt int
		var errMsg *string
		if err := rows.Scan(&item.ID, &itemType, &item.MessageGroup, &statusInt, &item.AttemptCount,
			&errMsg, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.ItemType = common.OutboxItemType(itemType)
		item.Status = common.FromOutboxCode(statusInt)
		if errMsg != nil {
			item.StatusMessage = *errMsg
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// retryableFailureCodes / terminalFailureCodes are the stored failure statuses
// by retryability (IN_PROGRESS and SUCCESS are not failures).
func retryableFailureCodes() []int {
	return []int{common.OutboxInternalError.Code(), common.OutboxUnauthorized.Code(), common.OutboxGatewayError.Code()}
}

func terminalFailureCodes() []int {
	return []int{common.OutboxBadRequest.Code(), common.OutboxForbidden.Code()}
}

// nonNil returns s, or an empty slice for nil (pgx sends nil as NULL).
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (r *Repository) RecoverStuck(ctx context.Context, olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan)
	tag, err := r.pool.Exec(ctx,
		`UPDATE outbox_messages SET status = 0, updated_at = NOW()
		  WHERE status = 9 AND updated_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// Healthy pings the pool.
func (r *Repository) Healthy(ctx context.Context) bool {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.pool.Ping(c) == nil
}

// Listen implements outbox.Waker: it LISTENs on outbox.ChannelPush on a connection of its own and
// calls wake for every notification, and once after each (re)connection. It returns when ctx ends.
func (r *Repository) Listen(ctx context.Context, wake func()) {
	pause := time.Second
	for ctx.Err() == nil {
		err := r.listenOnce(ctx, wake, func() { pause = time.Second })
		if ctx.Err() != nil {
			return
		}
		slog.Warn("outbox push listener lost; polling until it reconnects", "err", err, "in", pause)
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
		pause = min(pause*2, 30*time.Second)
	}
}

func (r *Repository) listenOnce(ctx context.Context, wake, connected func()) error {
	pooled, err := r.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	conn := pooled.Hijack()
	defer func() { _ = conn.Close(context.WithoutCancel(ctx)) }()
	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{outbox.ChannelPush}.Sanitize()); err != nil {
		return err
	}
	connected()
	wake() // catch up on what was written while the listener was down
	for {
		if _, err := conn.WaitForNotification(ctx); err != nil {
			return err
		}
		wake()
	}
}

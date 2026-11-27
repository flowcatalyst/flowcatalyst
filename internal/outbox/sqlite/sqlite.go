// Package sqlite is the SQLite-backed outbox repository.
//
// It implements [outbox.Repository] and [outbox.ClaimRecoverer] against pure-Go
// SQLite (modernc.org/sqlite via database/sql), matching the schema and claim,
// block-on-error, marker, and crash-recovery semantics of the Postgres backend.
//
// Concurrency model: SQLite permits one writer at a time. Repository serialises
// in-process writes with writeMu and wraps multi-statement claims and stuck-row
// sweeps in BEGIN IMMEDIATE transactions so cross-process writers wait cleanly
// under PRAGMA busy_timeout instead of failing mid-transaction with SQLITE_BUSY
// on a read-to-write lock upgrade.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" database/sql driver

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
)

const (
	// maxIDBatch bounds the number of IDs bound in a single IN (?, ...) clause,
	// well under SQLite's SQLITE_MAX_VARIABLE_NUMBER (32766).
	maxIDBatch = 900

	// timeLayout is a fixed-width UTC RFC3339Nano format (trailing zeros kept)
	// written to updated_at/created_at by the repository itself.
	timeLayout = "2006-01-02T15:04:05.000000000Z"
)

var (
	_ outbox.Repository     = (*Repository)(nil)
	_ outbox.ClaimRecoverer = (*Repository)(nil)
)

// CreateOutboxTableSQL is the idempotent SQLite schema + indexes, matching the
// columns and partial-index structure of the Postgres outbox table.
const CreateOutboxTableSQL = `
CREATE TABLE IF NOT EXISTS outbox_messages (
    id            TEXT PRIMARY KEY,
    type          TEXT NOT NULL,
    message_group TEXT,
    payload       TEXT NOT NULL,
    status        INTEGER NOT NULL DEFAULT 0,
    retry_count   INTEGER NOT NULL DEFAULT 0,
    created_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at    DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    error_message TEXT,
    client_id     TEXT,
    payload_size  INTEGER,
    headers       TEXT
);
CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending
    ON outbox_messages (status, message_group, created_at) WHERE status = 0;
CREATE INDEX IF NOT EXISTS idx_outbox_messages_stuck
    ON outbox_messages (status, created_at) WHERE status = 9;
CREATE INDEX IF NOT EXISTS idx_outbox_client_pending
    ON outbox_messages (client_id, status, created_at);
`

// Repository is the SQLite-backed outbox repository.
type Repository struct {
	db      *sql.DB
	writeMu sync.Mutex
}

// New wraps an existing *sql.DB (such as one shared with pkg/fcsdk/outboxsql).
// The caller retains ownership of db's lifecycle.
func New(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Open opens a SQLite database at dsn (a file path, ":memory:", "file:...", or
// "sqlite://..."), configures WAL mode and a 5-second busy timeout, and returns
// a Repository whose Close method closes the underlying *sql.DB.
func Open(ctx context.Context, dsn string) (*Repository, error) {
	norm, inMemory := normalizeDSN(dsn)
	if norm == "" {
		return nil, errors.New("sqlite outbox: empty database path")
	}
	db, err := sql.Open("sqlite", norm)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	// Single pooled connection prevents :memory: from splitting into isolated
	// per-connection databases and avoids intra-process writer contention.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite busy_timeout: %w", err)
	}
	if !inMemory {
		if _, err := db.ExecContext(ctx, `PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL;`); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("sqlite wal: %w", err)
		}
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	return &Repository{db: db}, nil
}

// Close closes the underlying *sql.DB.
func (r *Repository) Close() error {
	if r == nil || r.db == nil {
		return nil
	}
	return r.db.Close()
}

func normalizeDSN(raw string) (dsn string, inMemory bool) {
	s := strings.TrimSpace(raw)
	for _, prefix := range []string{"sqlite3://", "sqlite://"} {
		if after, ok := strings.CutPrefix(s, prefix); ok {
			s = after
			break
		}
	}
	lower := strings.ToLower(s)
	inMemory = lower == ":memory:" || strings.HasPrefix(lower, "file::memory:") || strings.Contains(lower, "mode=memory")
	return s, inMemory
}

// InitSchema creates the outbox_messages table and indexes idempotently.
func (r *Repository) InitSchema(ctx context.Context) error {
	if r.db == nil {
		return errors.New("sqlite outbox: nil db")
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_, err := r.db.ExecContext(ctx, CreateOutboxTableSQL)
	return err
}

// ClaimPending claims up to batchSize PENDING items outside skip, marks them
// IN_PROGRESS (status = 9) inside a BEGIN IMMEDIATE transaction, and returns
// them in (message_group NULLS LAST, created_at, id) order.
//
// Like the Postgres backend, any message group that still has an IN_PROGRESS
// row is skipped whole so a later row cannot overtake an unresolved earlier row
// waiting for RecoverStuck.
func (r *Repository) ClaimPending(ctx context.Context, batchSize int, skip outbox.ClaimFilter) ([]outbox.Item, error) {
	if batchSize <= 0 {
		return nil, nil
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	var claimed []outbox.Item
	err := r.withWriteTx(ctx, func(conn *sql.Conn) error {
		query, args := buildClaimSelect(batchSize, skip)
		rows, err := conn.QueryContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("claim select: %w", err)
		}
		var ids []string
		for rows.Next() {
			item, err := scanClaimRow(rows)
			if err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, item.ID)
			claimed = append(claimed, item)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()

		if len(ids) == 0 {
			return nil
		}

		now := time.Now().UTC()
		nowStr := now.Format(timeLayout)
		if err := execInBatches(ctx, conn, ids, func(chunk []string) (string, []any) {
			args := make([]any, 0, len(chunk)+1)
			args = append(args, nowStr)
			for _, id := range chunk {
				args = append(args, id)
			}
			return `UPDATE outbox_messages SET status = 9, updated_at = ? WHERE id IN (` + placeholders(len(chunk)) + `)`, args
		}); err != nil {
			return fmt.Errorf("claim mark in_progress: %w", err)
		}
		for i := range claimed {
			claimed[i].Status = common.OutboxInProgress
			claimed[i].UpdatedAt = now
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func buildClaimSelect(batchSize int, skip outbox.ClaimFilter) (string, []any) {
	var b strings.Builder
	var args []any

	b.WriteString(`SELECT id, type, message_group, payload, retry_count, error_message, created_at
FROM outbox_messages
WHERE status = 0
  AND (COALESCE(message_group, '') = '' OR message_group NOT IN (
        SELECT message_group FROM outbox_messages
         WHERE status = 9 AND message_group IS NOT NULL AND message_group <> ''))`)

	if len(skip.Groups) > 0 {
		b.WriteString(` AND (COALESCE(message_group, '') = '' OR message_group NOT IN (` + placeholders(len(skip.Groups)) + `))`)
		for _, g := range skip.Groups {
			args = append(args, g)
		}
	}
	if len(skip.Types) > 0 {
		b.WriteString(` AND (COALESCE(message_group, '') <> '' OR type NOT IN (` + placeholders(len(skip.Types)) + `))`)
		for _, t := range skip.Types {
			args = append(args, string(t))
		}
	}
	if len(skip.IDs) > 0 {
		b.WriteString(` AND id NOT IN (` + placeholders(len(skip.IDs)) + `)`)
		for _, id := range skip.IDs {
			args = append(args, id)
		}
	}

	b.WriteString(` ORDER BY message_group NULLS LAST, unixepoch(created_at, 'subsec'), created_at, id LIMIT ?`)
	args = append(args, batchSize)
	return b.String(), args
}

func scanClaimRow(rows *sql.Rows) (outbox.Item, error) {
	var (
		item      outbox.Item
		itemType  string
		msgGroup  sql.NullString
		payload   string
		errMsg    sql.NullString
		createdAt any
	)
	if err := rows.Scan(&item.ID, &itemType, &msgGroup, &payload, &item.AttemptCount, &errMsg, &createdAt); err != nil {
		return outbox.Item{}, fmt.Errorf("claim scan: %w", err)
	}
	created, err := parseTime(createdAt)
	if err != nil {
		return outbox.Item{}, fmt.Errorf("claim parse created_at: %w", err)
	}
	item.ItemType = common.OutboxItemType(itemType)
	if msgGroup.Valid {
		g := msgGroup.String
		item.MessageGroup = &g
	}
	item.Payload = json.RawMessage(payload)
	if errMsg.Valid {
		item.StatusMessage = errMsg.String
	}
	item.CreatedAt = created
	return item, nil
}

// MarkSuccess deletes successfully dispatched rows to keep outbox_messages bounded.
func (r *Repository) MarkSuccess(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return execDBInBatches(ctx, r.db, ids, func(chunk []string) (string, []any) {
		return `DELETE FROM outbox_messages WHERE id IN (` + placeholders(len(chunk)) + `)`, toAny(chunk)
	})
}

// MarkFailed bumps retry_count (capped at 32767 to match the SDK SMALLINT
// contract), records error_message, and sets status (or PENDING when requeue is true).
func (r *Repository) MarkFailed(ctx context.Context, ids []string, status common.OutboxStatus, msg string, requeue bool) error {
	if len(ids) == 0 {
		return nil
	}
	newStatus := status.Code()
	if requeue {
		newStatus = int(common.OutboxPending)
	}
	nowStr := time.Now().UTC().Format(timeLayout)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return execDBInBatches(ctx, r.db, ids, func(chunk []string) (string, []any) {
		args := make([]any, 0, len(chunk)+3)
		args = append(args, newStatus, msg, nowStr)
		for _, id := range chunk {
			args = append(args, id)
		}
		q := `UPDATE outbox_messages
		         SET status = ?, error_message = ?, retry_count = MIN(retry_count + 1, 32767), updated_at = ?
		       WHERE id IN (` + placeholders(len(chunk)) + `)`
		return q, args
	})
}

// Release returns claimed (IN_PROGRESS) rows to PENDING without penalty.
func (r *Repository) Release(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	nowStr := time.Now().UTC().Format(timeLayout)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return execDBInBatches(ctx, r.db, ids, func(chunk []string) (string, []any) {
		args := make([]any, 0, len(chunk)+1)
		args = append(args, nowStr)
		for _, id := range chunk {
			args = append(args, id)
		}
		return `UPDATE outbox_messages SET status = 0, updated_at = ? WHERE id IN (` + placeholders(len(chunk)) + `) AND status = 9`, args
	})
}

// Requeue resets rows to PENDING from any status, clearing retry_count and error_message.
func (r *Repository) Requeue(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	nowStr := time.Now().UTC().Format(timeLayout)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return execDBInBatches(ctx, r.db, ids, func(chunk []string) (string, []any) {
		args := make([]any, 0, len(chunk)+1)
		args = append(args, nowStr)
		for _, id := range chunk {
			args = append(args, id)
		}
		return `UPDATE outbox_messages SET status = 0, retry_count = 0, error_message = NULL, updated_at = ? WHERE id IN (` + placeholders(len(chunk)) + `)`, args
	})
}

// MarkSkipped replaces a leading outbox.BlockingMarker with outbox.SkippedMarker
// (once), leaving the failure status intact.
func (r *Repository) MarkSkipped(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	nowStr := time.Now().UTC().Format(timeLayout)
	blockLen := len(outbox.BlockingMarker)
	skipLen := len(outbox.SkippedMarker)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	return execDBInBatches(ctx, r.db, ids, func(chunk []string) (string, []any) {
		args := make([]any, 0, len(chunk)+7)
		args = append(args,
			outbox.SkippedMarker,
			blockLen, outbox.BlockingMarker,
			blockLen+1,
			nowStr,
			skipLen, outbox.SkippedMarker,
		)
		for _, id := range chunk {
			args = append(args, id)
		}
		q := `UPDATE outbox_messages
		         SET error_message = ? || CASE WHEN substr(COALESCE(error_message, ''), 1, ?) = ?
		                                       THEN substr(error_message, ?)
		                                       ELSE COALESCE(error_message, '') END,
		             updated_at = ?
		       WHERE (error_message IS NULL OR substr(error_message, 1, ?) <> ?)
		         AND id IN (` + placeholders(len(chunk)) + `)`
		return q, args
	})
}

// MarkLegacyRefusals prefixes outbox.SkippedBeforeUpgradeMarker onto terminal
// failure rows (BAD_REQUEST, FORBIDDEN) that carry none of the processor markers.
func (r *Repository) MarkLegacyRefusals(ctx context.Context) (int, error) {
	nowStr := time.Now().UTC().Format(timeLayout)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	res, err := r.db.ExecContext(ctx, `
UPDATE outbox_messages
   SET error_message = ? || COALESCE(error_message, ''), updated_at = ?
 WHERE status IN (?, ?)
   AND (error_message IS NULL OR (
        substr(error_message, 1, ?) <> ?
    AND substr(error_message, 1, ?) <> ?
    AND substr(error_message, 1, ?) <> ?))`,
		outbox.SkippedBeforeUpgradeMarker, nowStr,
		common.OutboxBadRequest.Code(), common.OutboxForbidden.Code(),
		len(outbox.SkippedBeforeUpgradeMarker), outbox.SkippedBeforeUpgradeMarker,
		len(outbox.BlockingMarker), outbox.BlockingMarker,
		len(outbox.SkippedMarker), outbox.SkippedMarker,
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// RequeueRetryableFailures resets rows left in a retryable failure status
// (INTERNAL_ERROR, UNAUTHORIZED, GATEWAY_ERROR) back to PENDING.
func (r *Repository) RequeueRetryableFailures(ctx context.Context) (int, error) {
	nowStr := time.Now().UTC().Format(timeLayout)

	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	res, err := r.db.ExecContext(ctx, `
UPDATE outbox_messages
   SET status = 0, updated_at = ?
 WHERE status IN (?, ?, ?)`,
		nowStr,
		common.OutboxInternalError.Code(),
		common.OutboxUnauthorized.Code(),
		common.OutboxGatewayError.Code(),
	)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// BlockingFailures returns each message group's oldest terminal failure row
// carrying outbox.BlockingMarker (payload omitted).
func (r *Repository) BlockingFailures(ctx context.Context) ([]outbox.Item, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, type, message_group, status, retry_count, error_message, created_at, updated_at
  FROM outbox_messages
 WHERE status IN (?, ?)
   AND COALESCE(message_group, '') <> ''
   AND substr(error_message, 1, ?) = ?
 ORDER BY message_group, unixepoch(created_at, 'subsec'), created_at, id`,
		common.OutboxBadRequest.Code(), common.OutboxForbidden.Code(),
		len(outbox.BlockingMarker), outbox.BlockingMarker,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []outbox.Item
	seen := map[string]bool{}
	for rows.Next() {
		var (
			item      outbox.Item
			itemType  string
			msgGroup  sql.NullString
			statusInt int
			errMsg    sql.NullString
			createdAt any
			updatedAt any
		)
		if err := rows.Scan(&item.ID, &itemType, &msgGroup, &statusInt, &item.AttemptCount, &errMsg, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		if !msgGroup.Valid || seen[msgGroup.String] {
			continue
		}
		seen[msgGroup.String] = true
		created, err := parseTime(createdAt)
		if err != nil {
			return nil, fmt.Errorf("blocking_failures parse created_at: %w", err)
		}
		updated, err := parseTime(updatedAt)
		if err != nil {
			return nil, fmt.Errorf("blocking_failures parse updated_at: %w", err)
		}
		g := msgGroup.String
		item.ItemType = common.OutboxItemType(itemType)
		item.MessageGroup = &g
		item.Status = common.FromOutboxCode(statusInt)
		if errMsg.Valid {
			item.StatusMessage = errMsg.String
		}
		item.CreatedAt = created
		item.UpdatedAt = updated
		out = append(out, item)
	}
	return out, rows.Err()
}

// RecoverStuck resets IN_PROGRESS (9) rows whose updated_at is older than
// olderThan back to PENDING (0). Parsing updated_at in Go inside the write
// transaction handles every timestamp format an SDK or migration default can
// write (RFC3339, SQLite CURRENT_TIMESTAMP, Go time.Time.String()).
func (r *Repository) RecoverStuck(ctx context.Context, olderThan time.Duration) (int, error) {
	return r.RecoverClaimedBefore(ctx, time.Now().UTC().Add(-olderThan))
}

// DBNow implements [outbox.ClaimRecoverer]. SQLite runs in-process, so the
// database clock is the process UTC clock.
func (r *Repository) DBNow(_ context.Context) (time.Time, error) {
	return time.Now().UTC(), nil
}

// RecoverClaimedBefore implements [outbox.ClaimRecoverer].
func (r *Repository) RecoverClaimedBefore(ctx context.Context, before time.Time) (int, error) {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()

	recovered := 0
	err := r.withWriteTx(ctx, func(conn *sql.Conn) error {
		rows, err := conn.QueryContext(ctx, `SELECT id, updated_at FROM outbox_messages WHERE status = 9`)
		if err != nil {
			return err
		}
		var staleIDs []string
		for rows.Next() {
			var id string
			var rawUpdated any
			if err := rows.Scan(&id, &rawUpdated); err != nil {
				_ = rows.Close()
				return err
			}
			updated, err := parseTime(rawUpdated)
			if err != nil {
				_ = rows.Close()
				return fmt.Errorf("recover parse updated_at for %s: %w", id, err)
			}
			if updated.Before(before) {
				staleIDs = append(staleIDs, id)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return err
		}
		_ = rows.Close()

		if len(staleIDs) == 0 {
			return nil
		}
		nowStr := time.Now().UTC().Format(timeLayout)
		return execInBatches(ctx, conn, staleIDs, func(chunk []string) (string, []any) {
			recovered += len(chunk)
			args := make([]any, 0, len(chunk)+1)
			args = append(args, nowStr)
			for _, id := range chunk {
				args = append(args, id)
			}
			return `UPDATE outbox_messages SET status = 0, updated_at = ? WHERE status = 9 AND id IN (` + placeholders(len(chunk)) + `)`, args
		})
	})
	if err != nil {
		return 0, err
	}
	return recovered, nil
}

// Healthy pings the SQLite connection.
func (r *Repository) Healthy(ctx context.Context) bool {
	if r == nil || r.db == nil {
		return false
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return r.db.PingContext(c) == nil
}

// withWriteTx runs fn inside a BEGIN IMMEDIATE transaction on a dedicated conn.
func (r *Repository) withWriteTx(ctx context.Context, fn func(conn *sql.Conn) error) (err error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire conn: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return fmt.Errorf("begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), `ROLLBACK`)
		}
	}()

	if err := fn(conn); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	committed = true
	return nil
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func execInBatches(ctx context.Context, ex sqlExecer, ids []string, build func(chunk []string) (string, []any)) error {
	for start := 0; start < len(ids); start += maxIDBatch {
		end := min(start+maxIDBatch, len(ids))
		q, args := build(ids[start:end])
		if _, err := ex.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	return nil
}

func execDBInBatches(ctx context.Context, db *sql.DB, ids []string, build func(chunk []string) (string, []any)) error {
	return execInBatches(ctx, db, ids, build)
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05.999999999 -0700",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

func parseTime(v any) (time.Time, error) {
	switch x := v.(type) {
	case nil:
		return time.Time{}, nil
	case time.Time:
		return x.UTC(), nil
	case int64:
		return time.Unix(x, 0).UTC(), nil
	case []byte:
		return parseTimeString(string(x))
	case string:
		return parseTimeString(x)
	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp type %T", v)
	}
}

func parseTimeString(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	// Strip monotonic clock suffix (" m=+...") if Go's time.Time.String() wrote it.
	if idx := strings.Index(s, " m="); idx > 0 {
		s = s[:idx]
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp %q", s)
}

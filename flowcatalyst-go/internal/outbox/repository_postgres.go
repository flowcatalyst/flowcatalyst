package outbox

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PostgresRepository implements Repository for PostgreSQL.
// Uses simple SELECT/UPDATE with status codes - NO row locking.
// Safe because only one poller runs (enforced by leader election).
type PostgresRepository struct {
	db     *sql.DB
	config *RepositoryConfig
}

// NewPostgresRepository creates a new PostgreSQL outbox repository
func NewPostgresRepository(db *sql.DB, config *RepositoryConfig) *PostgresRepository {
	if config == nil {
		config = DefaultRepositoryConfig()
	}
	return &PostgresRepository{
		db:     db,
		config: config,
	}
}

// GetTableName returns the table name for the item type
func (r *PostgresRepository) GetTableName(itemType OutboxItemType) string {
	switch itemType {
	case OutboxItemTypeEvent:
		return r.config.EventsTable
	case OutboxItemTypeDispatchJob:
		return r.config.DispatchJobsTable
	default:
		return r.config.EventsTable
	}
}

// FetchPending fetches pending items (status=0) ordered by message_group, created_at.
// Simple SELECT with no locking - safe because only one poller runs.
func (r *PostgresRepository) FetchPending(ctx context.Context, itemType OutboxItemType, limit int) ([]*OutboxItem, error) {
	table := r.GetTableName(itemType)

	query := fmt.Sprintf(`
		SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
		FROM %s
		WHERE status = 0
		ORDER BY message_group, created_at
		LIMIT $1
	`, table)

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch pending: %w", err)
	}
	defer rows.Close()

	return r.scanItems(rows)
}

// MarkAsInProgress marks items as in-progress (status=9).
func (r *PostgresRepository) MarkAsInProgress(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 0)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, updated_at = NOW()
		WHERE id IN (%s)
	`, table, StatusInProgress, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark as in-progress: %w", err)
	}
	return nil
}

// MarkWithStatus updates items to the specified status code.
func (r *PostgresRepository) MarkWithStatus(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 0)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, updated_at = NOW()
		WHERE id IN (%s)
	`, table, status, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark with status %d: %w", status, err)
	}
	return nil
}

// MarkWithStatusAndError updates items to the specified status with an error message.
func (r *PostgresRepository) MarkWithStatusAndError(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus, errorMessage string) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 1)
	args = append([]interface{}{errorMessage}, args...)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, error_message = $1, updated_at = NOW()
		WHERE id IN (%s)
	`, table, status, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("mark with status %d and error: %w", status, err)
	}
	return nil
}

// FetchStuckItems fetches items stuck in in-progress status (status=9).
func (r *PostgresRepository) FetchStuckItems(ctx context.Context, itemType OutboxItemType) ([]*OutboxItem, error) {
	table := r.GetTableName(itemType)

	query := fmt.Sprintf(`
		SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
		FROM %s
		WHERE status = %d
		ORDER BY created_at
	`, table, StatusInProgress)

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("fetch stuck items: %w", err)
	}
	defer rows.Close()

	return r.scanItems(rows)
}

// ResetStuckItems resets stuck items back to pending (status=0).
func (r *PostgresRepository) ResetStuckItems(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 0)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, updated_at = NOW()
		WHERE id IN (%s)
	`, table, StatusPending, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reset stuck items: %w", err)
	}
	return nil
}

// IncrementRetryCount increments the retry count for items and resets to pending.
func (r *PostgresRepository) IncrementRetryCount(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 0)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, retry_count = retry_count + 1, updated_at = NOW()
		WHERE id IN (%s)
	`, table, StatusPending, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("increment retry count: %w", err)
	}
	return nil
}

// FetchRecoverableItems fetches items eligible for periodic recovery.
func (r *PostgresRepository) FetchRecoverableItems(ctx context.Context, itemType OutboxItemType, timeoutSeconds int, limit int) ([]*OutboxItem, error) {
	table := r.GetTableName(itemType)

	query := fmt.Sprintf(`
		SELECT id, type, message_group, payload, status, retry_count, created_at, updated_at, error_message
		FROM %s
		WHERE status IN (%d, %d, %d, %d, %d, %d)
		  AND updated_at < NOW() - INTERVAL '%d seconds'
		ORDER BY created_at
		LIMIT $1
	`, table,
		StatusInProgress,
		StatusBadRequest,
		StatusInternalError,
		StatusUnauthorized,
		StatusForbidden,
		StatusGatewayError,
		timeoutSeconds)

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("fetch recoverable items: %w", err)
	}
	defer rows.Close()

	return r.scanItems(rows)
}

// ResetRecoverableItems resets recoverable items back to PENDING status.
func (r *PostgresRepository) ResetRecoverableItems(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	table := r.GetTableName(itemType)
	placeholders, args := r.buildPlaceholders(ids, 0)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = %d, updated_at = NOW()
		WHERE id IN (%s)
	`, table, StatusPending, placeholders)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("reset recoverable items: %w", err)
	}
	return nil
}

// CountPending returns the count of pending items.
func (r *PostgresRepository) CountPending(ctx context.Context, itemType OutboxItemType) (int64, error) {
	table := r.GetTableName(itemType)

	query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE status = %d`, table, StatusPending)

	var count int64
	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count pending: %w", err)
	}
	return count, nil
}

// CreateSchema creates the outbox tables if they don't exist.
func (r *PostgresRepository) CreateSchema(ctx context.Context) error {
	for _, itemType := range []OutboxItemType{OutboxItemTypeEvent, OutboxItemTypeDispatchJob} {
		table := r.GetTableName(itemType)

		createTable := fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s (
				id VARCHAR(26) PRIMARY KEY,
				type VARCHAR(20) NOT NULL,
				message_group VARCHAR(255),
				payload TEXT NOT NULL,
				status SMALLINT NOT NULL DEFAULT 0,
				retry_count SMALLINT NOT NULL DEFAULT 0,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				error_message TEXT
			)
		`, table)

		if _, err := r.db.ExecContext(ctx, createTable); err != nil {
			return fmt.Errorf("create table %s: %w", table, err)
		}

		// Index for fetching pending items (status=0, ordered by message_group, created_at)
		createPendingIndex := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS idx_%s_pending
			ON %s(status, message_group, created_at)
			WHERE status = 0
		`, table, table)

		if _, err := r.db.ExecContext(ctx, createPendingIndex); err != nil {
			return fmt.Errorf("create pending index on %s: %w", table, err)
		}

		// Index for finding stuck items (status=9)
		createStuckIndex := fmt.Sprintf(`
			CREATE INDEX IF NOT EXISTS idx_%s_stuck
			ON %s(status, created_at)
			WHERE status = 9
		`, table, table)

		if _, err := r.db.ExecContext(ctx, createStuckIndex); err != nil {
			return fmt.Errorf("create stuck index on %s: %w", table, err)
		}
	}
	return nil
}

// buildPlaceholders builds PostgreSQL placeholders ($1, $2, ...) and args slice
func (r *PostgresRepository) buildPlaceholders(ids []string, offset int) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1+offset)
		args[i] = id
	}
	return strings.Join(placeholders, ", "), args
}

// scanItems scans rows into OutboxItem slice
func (r *PostgresRepository) scanItems(rows *sql.Rows) ([]*OutboxItem, error) {
	var items []*OutboxItem
	for rows.Next() {
		var item OutboxItem
		var messageGroup, errorMessage sql.NullString
		var updatedAt sql.NullTime

		err := rows.Scan(
			&item.ID,
			&item.Type,
			&messageGroup,
			&item.Payload,
			&item.Status,
			&item.RetryCount,
			&item.CreatedAt,
			&updatedAt,
			&errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("scan item: %w", err)
		}

		if messageGroup.Valid {
			item.MessageGroup = messageGroup.String
		}
		if updatedAt.Valid {
			item.UpdatedAt = updatedAt.Time
		} else {
			item.UpdatedAt = time.Time{}
		}
		if errorMessage.Valid {
			item.ErrorMessage = errorMessage.String
		}

		items = append(items, &item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}

	return items, nil
}

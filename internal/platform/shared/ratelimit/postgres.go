package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresStore appends one row per attempt to iam_rate_limit_events and
// counts rows in the window. The combined insert+count runs in a single
// CTE so the read sees its own write — no race between replicas both
// inserting, both counting "below limit", and both letting the request
// through.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore wires the store against pool.
func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

// CheckAndRecord inserts the attempt and returns the in-window total
// atomically, rejecting when it exceeds the limit.
func (s *PostgresStore) CheckAndRecord(ctx context.Context, bucket Bucket, key string, policy Policy) (Decision, error) {
	windowStart := time.Now().UTC().Add(-policy.Window)

	var count int64
	err := s.pool.QueryRow(ctx,
		`WITH ins AS (
			INSERT INTO iam_rate_limit_events (bucket, key, occurred_at)
			VALUES ($1, $2, NOW())
			RETURNING 1
		)
		SELECT COUNT(*) FROM iam_rate_limit_events
		WHERE bucket = $1 AND key = $2 AND occurred_at > $3`,
		string(bucket), key, windowStart).Scan(&count)
	if err != nil {
		return Decision{}, fmt.Errorf("rate-limit count: %w", err)
	}

	if uint64(count) > uint64(policy.Limit) {
		// Retry-after = time until the oldest in-window event ages out.
		var oldest *time.Time
		if err := s.pool.QueryRow(ctx,
			`SELECT MIN(occurred_at) FROM iam_rate_limit_events
			WHERE bucket = $1 AND key = $2 AND occurred_at > $3`,
			string(bucket), key, windowStart).Scan(&oldest); err != nil {
			return Decision{}, fmt.Errorf("rate-limit oldest: %w", err)
		}
		retryAfter := int64(policy.Window.Seconds())
		if oldest != nil {
			elapsed := int64(time.Since(*oldest).Seconds())
			if elapsed < 0 {
				elapsed = 0
			}
			retryAfter = int64(policy.Window.Seconds()) - elapsed
		}
		if retryAfter < 1 {
			retryAfter = 1
		}
		return Decision{Allowed: false, RetryAfterSecs: clampU32(retryAfter)}, nil
	}
	return Decision{Allowed: true}, nil
}

// Prune deletes events older than olderThan. Returns the number removed.
func (s *PostgresStore) Prune(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM iam_rate_limit_events WHERE occurred_at < $1`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

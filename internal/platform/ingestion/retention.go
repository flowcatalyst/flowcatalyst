package ingestion

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// How long the runtime API's records are kept (docs/ingestion-design.md
// section 3). Metrics and failures are the high-volume tables.
const (
	MetricsRetention      = 30 * 24 * time.Hour
	FailuresRetention     = 30 * 24 * time.Hour
	TestRequestRetention  = 24 * time.Hour
	RuntimeUnseenFor      = 7 * 24 * time.Hour
	retentionChunk        = 5000
	retentionMaxChunks    = 20
	DefaultRetentionDelay = 2 * time.Minute
	// DefaultRetentionInterval is hourly rather than daily: expired leases and
	// test requests are small but are not worth keeping for a day.
	DefaultRetentionInterval = time.Hour
)

// Pruned counts the rows one retention sweep deleted.
type Pruned struct{ Metrics, Failures, TestRequests, JobKeys, Leases, Runtimes int64 }

// Prune deletes the records past retention as of now: exchange metrics and
// failures after 30 days, test requests after a day, job keys after a week,
// leases that have expired, and runtimes unseen for 7 days. The high-volume tables go in bounded chunks;
// every delete is idempotent, so concurrent sweeps on several replicas are safe.
func (r *Repository) Prune(ctx context.Context, now time.Time) (Pruned, error) {
	var p Pruned
	var err error
	if p.Metrics, err = r.deleteChunked(ctx, `DELETE FROM ing_exchange_metrics WHERE ctid = ANY(ARRAY(
		SELECT ctid FROM ing_exchange_metrics WHERE window_start < $1 LIMIT $2))`, now.Add(-MetricsRetention)); err != nil {
		return p, fmt.Errorf("ingestion repo: prune metrics: %w", err)
	}
	if p.Failures, err = r.deleteChunked(ctx, `DELETE FROM ing_failures WHERE id IN (
		SELECT id FROM ing_failures WHERE occurred_at < $1 LIMIT $2)`, now.Add(-FailuresRetention)); err != nil {
		return p, fmt.Errorf("ingestion repo: prune failures: %w", err)
	}
	if p.TestRequests, err = r.deleteChunked(ctx, `DELETE FROM ing_test_requests WHERE id IN (
		SELECT id FROM ing_test_requests WHERE created_at < $1 LIMIT $2)`, now.Add(-TestRequestRetention)); err != nil {
		return p, fmt.Errorf("ingestion repo: prune test requests: %w", err)
	}
	if p.JobKeys, err = r.deleteChunked(ctx, `DELETE FROM ing_job_keys WHERE ctid = ANY(ARRAY(
		SELECT ctid FROM ing_job_keys WHERE created_at < $1 LIMIT $2))`, now.Add(-JobKeysRetention)); err != nil {
		return p, fmt.Errorf("ingestion repo: prune job keys: %w", err)
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM ing_leases WHERE expires_at < $1`, now)
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune leases: %w", err)
	}
	p.Leases = ct.RowsAffected()
	ct, err = r.pool.Exec(ctx, `DELETE FROM ing_runtimes WHERE last_seen_at < $1`, now.Add(-RuntimeUnseenFor))
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune runtimes: %w", err)
	}
	p.Runtimes = ct.RowsAffected()
	return p, nil
}

// deleteChunked runs a delete that takes the cutoff as $1 and its chunk size
// as $2 until a chunk comes back short, at most retentionMaxChunks times.
func (r *Repository) deleteChunked(ctx context.Context, sql string, before time.Time) (int64, error) {
	var total int64
	for range retentionMaxChunks {
		ct, err := r.pool.Exec(ctx, sql, before, retentionChunk)
		if err != nil {
			return total, err
		}
		total += ct.RowsAffected()
		if ct.RowsAffected() < retentionChunk {
			break
		}
	}
	return total, nil
}

// RunRetention prunes once after delay and then every interval, until ctx
// ends. Not leader-gated: the deletes are bounded and idempotent, the posture
// of the function trace-log retention it follows.
func RunRetention(ctx context.Context, repo *Repository, delay, interval time.Duration) {
	if delay <= 0 {
		delay = DefaultRetentionDelay
	}
	if interval <= 0 {
		interval = DefaultRetentionInterval
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		p, err := repo.Prune(ctx, time.Now().UTC())
		switch {
		case err != nil:
			slog.Warn("ingestion retention sweep failed", "err", err)
		case p != Pruned{}:
			slog.Info("ingestion retention sweep", "metrics", p.Metrics, "failures", p.Failures,
				"test_requests", p.TestRequests, "job_keys", p.JobKeys, "leases", p.Leases, "runtimes", p.Runtimes)
		}
		timer.Reset(interval)
	}
}

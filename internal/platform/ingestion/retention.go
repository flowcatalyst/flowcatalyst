package ingestion

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
)

// How long the runtime API's records are kept (docs/ingestion-design.md
// section 3). Metrics, failures and job keys are the high-volume tables.
const (
	MetricsRetention      = 30 * 24 * time.Hour
	FailuresRetention     = 30 * 24 * time.Hour
	TestRequestRetention  = 24 * time.Hour
	RuntimeUnseenFor      = 7 * 24 * time.Hour
	DefaultRetentionDelay = 2 * time.Minute
	// DefaultRetentionInterval is hourly rather than daily: expired leases and
	// test requests are small but are not worth keeping for a day.
	DefaultRetentionInterval = time.Hour
)

// The per-sweep budget of each high-volume table: up to retentionMaxChunks
// deletes of retentionChunk rows, every DefaultRetentionInterval.
//
// The arithmetic. ing_job_keys gets one row per record that carries an
// externalId, which the runtime now sends on every record it can (C5). A
// fleet ingesting a steady 1,000 records a second writes 3.6 million keys an
// hour; the old budget (20 chunks of 5,000, 100,000 rows an hour) fell behind
// at 28 records a second and never caught up. The default here is 500 chunks of
// 10,000, 5 million rows a table per hourly sweep: steady state for about
// 1,400 records a second, and a backlog left by a slower past drains at the
// difference. Exchange metrics are one row per ingestion, runtime and window
// (1,000 ingestions on 3 runtimes with one-minute windows is 180,000 an hour),
// well inside it. A delete of 10,000 rows by ctid through the created_at index
// takes tens of milliseconds, so a full budget is under a minute or two a
// table. FC_INGESTION_RETENTION_MAX_CHUNKS raises (or lowers) the chunk count
// for a fleet beyond that; every delete is bounded and idempotent, so more
// chunks cost time, not correctness.
const (
	retentionChunk            = 10_000
	DefaultRetentionMaxChunks = 500
)

// RetentionMaxChunks is the chunk count of one sweep:
// FC_INGESTION_RETENTION_MAX_CHUNKS, or DefaultRetentionMaxChunks when unset,
// unparseable or not positive.
func RetentionMaxChunks() int {
	if n := envutil.Int("FC_INGESTION_RETENTION_MAX_CHUNKS", DefaultRetentionMaxChunks); n > 0 {
		return n
	}
	return DefaultRetentionMaxChunks
}

// Pruned counts the rows one retention sweep deleted.
type Pruned struct {
	Metrics, Failures, TestRequests, JobKeys, Leases, RouteStates, SecretFetches, Runtimes int64
	TraceLog, TraceWindows                                                                 int64
}

// Prune deletes the records past retention as of now, with the chunk budget
// RetentionMaxChunks.
func (r *Repository) Prune(ctx context.Context, now time.Time) (Pruned, error) {
	return r.PruneWith(ctx, now, RetentionMaxChunks())
}

// PruneWith deletes the records past retention as of now: exchange metrics
// and failures after 30 days, test requests after a day, job keys after a
// week, leases that have expired, and the route states, secret fetch records
// and rows of runtimes unseen for 7 days, and trace records after
// FC_INGESTION_TRACE_RETENTION_DAYS (7) with the windows that ended before
// that. Each is judged by created_at, the time the platform
// wrote the row, never by a time the runtime supplied. The high-volume tables
// go in chunks of retentionChunk, at most maxChunks of them a table; every
// delete is idempotent, so concurrent sweeps on several replicas are safe.
func (r *Repository) PruneWith(ctx context.Context, now time.Time, maxChunks int) (Pruned, error) {
	var p Pruned
	var err error
	if p.Metrics, err = r.deleteChunked(ctx, `DELETE FROM ing_exchange_metrics WHERE ctid = ANY(ARRAY(
		SELECT ctid FROM ing_exchange_metrics WHERE created_at < $1 LIMIT $2))`, now.Add(-MetricsRetention), maxChunks); err != nil {
		return p, fmt.Errorf("ingestion repo: prune metrics: %w", err)
	}
	if p.Failures, err = r.deleteChunked(ctx, `DELETE FROM ing_failures WHERE id IN (
		SELECT id FROM ing_failures WHERE created_at < $1 LIMIT $2)`, now.Add(-FailuresRetention), maxChunks); err != nil {
		return p, fmt.Errorf("ingestion repo: prune failures: %w", err)
	}
	if p.TestRequests, err = r.deleteChunked(ctx, `DELETE FROM ing_test_requests WHERE id IN (
		SELECT id FROM ing_test_requests WHERE created_at < $1 LIMIT $2)`, now.Add(-TestRequestRetention), maxChunks); err != nil {
		return p, fmt.Errorf("ingestion repo: prune test requests: %w", err)
	}
	if p.JobKeys, err = r.deleteChunked(ctx, `DELETE FROM ing_job_keys WHERE ctid = ANY(ARRAY(
		SELECT ctid FROM ing_job_keys WHERE created_at < $1 LIMIT $2))`, now.Add(-JobKeysRetention), maxChunks); err != nil {
		return p, fmt.Errorf("ingestion repo: prune job keys: %w", err)
	}
	// Trace records by created_at, never the runtime's `at`; a window row is
	// kept for as long as its records could be, then goes (it is one row per
	// ingestion, so this only tidies ingestions nobody traces any more).
	if p.TraceLog, err = r.deleteChunked(ctx, `DELETE FROM ing_trace_log WHERE id IN (
		SELECT id FROM ing_trace_log WHERE created_at < $1 LIMIT $2)`, now.Add(-TraceRetention()), maxChunks); err != nil {
		return p, fmt.Errorf("ingestion repo: prune trace log: %w", err)
	}
	ct, err := r.pool.Exec(ctx, `DELETE FROM ing_trace_windows WHERE hard_stop < $1`, now.Add(-TraceRetention()))
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune trace windows: %w", err)
	}
	p.TraceWindows = ct.RowsAffected()
	ct, err = r.pool.Exec(ctx, `DELETE FROM ing_leases WHERE expires_at < $1`, now)
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune leases: %w", err)
	}
	p.Leases = ct.RowsAffected()
	// A runtime id is often a pod's name, new on every restart, so the route
	// states of runtimes long gone would otherwise pile up and read as running.
	unseen := now.Add(-RuntimeUnseenFor)
	ct, err = r.pool.Exec(ctx, `DELETE FROM ing_route_states rs WHERE NOT EXISTS (
		SELECT 1 FROM ing_runtimes rt WHERE rt.runtime_id = rs.runtime_id AND rt.last_seen_at >= $1)`, unseen)
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune route states: %w", err)
	}
	p.RouteStates = ct.RowsAffected()
	// The same for the record of which runtime last fetched which secret.
	ct, err = r.pool.Exec(ctx, `DELETE FROM ing_secret_fetches sf WHERE NOT EXISTS (
		SELECT 1 FROM ing_runtimes rt WHERE rt.runtime_id = sf.runtime_id AND rt.last_seen_at >= $1)`, unseen)
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune secret fetches: %w", err)
	}
	p.SecretFetches = ct.RowsAffected()
	ct, err = r.pool.Exec(ctx, `DELETE FROM ing_runtimes WHERE last_seen_at < $1`, unseen)
	if err != nil {
		return p, fmt.Errorf("ingestion repo: prune runtimes: %w", err)
	}
	p.Runtimes = ct.RowsAffected()
	return p, nil
}

// deleteChunked runs a delete that takes the cutoff as $1 and its chunk size
// as $2 until a chunk comes back short, at most maxChunks times.
func (r *Repository) deleteChunked(ctx context.Context, sql string, before time.Time, maxChunks int) (int64, error) {
	var total int64
	for range max(maxChunks, 1) {
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
				"test_requests", p.TestRequests, "job_keys", p.JobKeys, "leases", p.Leases,
				"route_states", p.RouteStates, "secret_fetches", p.SecretFetches, "runtimes", p.Runtimes,
				"trace_log", p.TraceLog, "trace_windows", p.TraceWindows)
		}
		timer.Reset(interval)
	}
}

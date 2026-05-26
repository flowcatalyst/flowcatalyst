package stream

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PartitionManager ensures the next month's RANGE partition exists for
// every partitioned table. Ports fc-stream/src/partition_manager.rs.
//
// Runs on a 60-minute tick. Idempotent: CREATE TABLE ... PARTITION OF
// ... IF NOT EXISTS doesn't exist in Postgres, so we check first.
type PartitionManager struct {
	pool *pgxpool.Pool
}

// PartitionedTables is the canonical list. Mirrors the Rust source.
// Matches migration 019/020/022 in flowcatalyst-rust/migrations.
var PartitionedTables = []string{
	"msg_events",
	"msg_events_read",
	"msg_dispatch_jobs",
	"msg_dispatch_jobs_read",
	"msg_dispatch_job_attempts",
	"msg_scheduled_job_instances",
	"msg_scheduled_job_instance_logs",
}

// NewPartitionManager wires a manager.
func NewPartitionManager(pool *pgxpool.Pool) *PartitionManager {
	return &PartitionManager{pool: pool}
}

// Run watches for new months and creates partitions. Returns when ctx is cancelled.
func (m *PartitionManager) Run(ctx context.Context) {
	// Initial ensure on startup.
	if err := m.ensureNext(ctx); err != nil {
		slog.Warn("partition manager initial ensure failed", "err", err)
	}
	tick := time.NewTicker(60 * time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("partition manager stopped")
			return
		case <-tick.C:
			if err := m.ensureNext(ctx); err != nil {
				slog.Warn("partition manager tick failed", "err", err)
			}
		}
	}
}

// ensureNext creates next-month partitions if missing.
func (m *PartitionManager) ensureNext(ctx context.Context) error {
	now := time.Now().UTC()
	// Current month, next month, month after next (gives a 60-day buffer).
	for _, offset := range []int{0, 1, 2} {
		first := time.Date(now.Year(), now.Month()+time.Month(offset), 1, 0, 0, 0, 0, time.UTC)
		next := time.Date(first.Year(), first.Month()+1, 1, 0, 0, 0, 0, time.UTC)
		for _, table := range PartitionedTables {
			if err := m.createPartition(ctx, table, first, next); err != nil {
				return fmt.Errorf("create partition for %s @ %s: %w", table, first.Format("2006-01"), err)
			}
		}
	}
	return nil
}

func (m *PartitionManager) createPartition(ctx context.Context, table string, from, to time.Time) error {
	name := fmt.Sprintf("%s_%s", table, from.Format("2006_01"))
	// Check existence first; CREATE TABLE ... PARTITION OF doesn't support IF NOT EXISTS.
	var exists bool
	if err := m.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_class WHERE relname = $1 AND relkind = 'r')`,
		name,
	).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err := m.pool.Exec(ctx, fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		name, table, from.Format("2006-01-02"), to.Format("2006-01-02"),
	))
	if err != nil {
		return err
	}
	slog.Info("partition created", "table", name)
	return nil
}

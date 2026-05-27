package scheduledjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InstanceRepository is the read-side repo for msg_scheduled_job_instances
// and msg_scheduled_job_instance_logs. The cron poller in
// internal/platform/scheduler writes these tables directly through the
// pool (no use-case envelope — instances aren't an aggregate, they're
// the history projection of cron firings).
type InstanceRepository struct {
	pool *pgxpool.Pool
}

// NewInstanceRepository wires the repo against the supplied pool.
func NewInstanceRepository(pool *pgxpool.Pool) *InstanceRepository {
	return &InstanceRepository{pool: pool}
}

const instanceSelect = `SELECT id, scheduled_job_id, client_id, job_code, trigger_kind,
    scheduled_for, fired_at, delivered_at, completed_at, status, delivery_attempts,
    delivery_error, completion_status, completion_result, correlation_id, created_at
    FROM msg_scheduled_job_instances`

// List returns instances matching the supplied filters, newest first
// (created_at DESC, id DESC for stable ordering when timestamps tie).
func (r *InstanceRepository) List(ctx context.Context, f InstanceListFilters) ([]ScheduledJobInstance, error) {
	q, args := buildInstanceQuery(instanceSelect, f, true)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("scheduled_job_instance list: %w", err)
	}
	defer rows.Close()
	var out []ScheduledJobInstance
	for rows.Next() {
		inst, err := scanInstance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *inst)
	}
	return out, rows.Err()
}

// Count returns the total number of instances matching the filters,
// ignoring Limit and Offset (so the caller can render "12 of 1,328").
func (r *InstanceRepository) Count(ctx context.Context, f InstanceListFilters) (int64, error) {
	q, args := buildInstanceQuery(`SELECT COUNT(*) FROM msg_scheduled_job_instances`, f, false)
	var count int64
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("scheduled_job_instance count: %w", err)
	}
	return count, nil
}

// FindByID loads a single instance. Returns (nil, nil) when not found.
func (r *InstanceRepository) FindByID(ctx context.Context, id string) (*ScheduledJobInstance, error) {
	row := r.pool.QueryRow(ctx, instanceSelect+` WHERE id = $1`, id)
	inst, err := scanInstance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scheduled_job_instance find_by_id: %w", err)
	}
	return inst, nil
}

// HasActiveInstance reports whether any instance of the supplied job is
// in a non-terminal state (QUEUED / IN_FLIGHT / DELIVERED). Used by the
// BFF "currently running" badge on the job list. Backed by a partial
// index — see migration 021.
func (r *InstanceRepository) HasActiveInstance(ctx context.Context, jobID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM msg_scheduled_job_instances
			WHERE scheduled_job_id = $1
			  AND status IN ('QUEUED', 'IN_FLIGHT', 'DELIVERED')
		)`, jobID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("scheduled_job_instance has_active: %w", err)
	}
	return exists, nil
}

// ListLogs returns up to `limit` log rows for the supplied instance,
// oldest first. A limit of <= 0 falls back to 500 (matches Rust).
func (r *InstanceRepository) ListLogs(ctx context.Context, instanceID string, limit int64) ([]ScheduledJobInstanceLog, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, instance_id, scheduled_job_id, client_id, level, message, metadata, created_at
		FROM msg_scheduled_job_instance_logs
		WHERE instance_id = $1
		ORDER BY created_at ASC, id ASC
		LIMIT $2`, instanceID, limit)
	if err != nil {
		return nil, fmt.Errorf("scheduled_job_instance_log list: %w", err)
	}
	defer rows.Close()
	var out []ScheduledJobInstanceLog
	for rows.Next() {
		log, err := scanInstanceLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *log)
	}
	return out, rows.Err()
}

// buildInstanceQuery composes the WHERE clause + args from filters.
// withPagination controls whether ORDER BY / LIMIT / OFFSET are
// appended — Count callers pass false.
func buildInstanceQuery(base string, f InstanceListFilters, withPagination bool) (string, []any) {
	q := base
	args := []any{}
	conds := []string{}
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.ScheduledJobID != nil {
		add("scheduled_job_id = $%d", *f.ScheduledJobID)
	}
	if f.ClientID != nil {
		add("client_id = $%d", *f.ClientID)
	}
	if f.Status != nil {
		add("status = $%d", string(*f.Status))
	}
	if f.TriggerKind != nil {
		add("trigger_kind = $%d", string(*f.TriggerKind))
	}
	if f.From != nil {
		add("created_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("created_at < $%d", *f.To)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	if withPagination {
		q += " ORDER BY created_at DESC, id DESC"
		if f.Limit != nil {
			args = append(args, *f.Limit)
			q += fmt.Sprintf(" LIMIT $%d", len(args))
		}
		if f.Offset != nil {
			args = append(args, *f.Offset)
			q += fmt.Sprintf(" OFFSET $%d", len(args))
		}
	}
	return q, args
}

// scanInstance reads a row into a ScheduledJobInstance. Works for both
// pgx.Rows and pgx.Row (both expose Scan).
type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstance(rs rowScanner) (*ScheduledJobInstance, error) {
	var (
		inst             ScheduledJobInstance
		status, trigger  string
		completionResult []byte
	)
	if err := rs.Scan(
		&inst.ID, &inst.ScheduledJobID, &inst.ClientID, &inst.JobCode, &trigger,
		&inst.ScheduledFor, &inst.FiredAt, &inst.DeliveredAt, &inst.CompletedAt,
		&status, &inst.DeliveryAttempts, &inst.DeliveryError, &inst.CompletionStatus,
		&completionResult, &inst.CorrelationID, &inst.CreatedAt,
	); err != nil {
		return nil, err
	}
	inst.Status = ParseInstanceStatus(status)
	inst.TriggerKind = ParseTriggerKind(trigger)
	if len(completionResult) > 0 {
		inst.CompletionResult = json.RawMessage(completionResult)
	}
	return &inst, nil
}

func scanInstanceLog(rs rowScanner) (*ScheduledJobInstanceLog, error) {
	var (
		log      ScheduledJobInstanceLog
		metadata []byte
	)
	if err := rs.Scan(
		&log.ID, &log.InstanceID, &log.ScheduledJobID, &log.ClientID,
		&log.Level, &log.Message, &metadata, &log.CreatedAt,
	); err != nil {
		return nil, err
	}
	if len(metadata) > 0 {
		log.Metadata = json.RawMessage(metadata)
	}
	return &log, nil
}

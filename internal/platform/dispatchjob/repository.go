package dispatchjob

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository owns msg_dispatch_jobs (the lean write table) plus the
// msg_dispatch_job_attempts history, and serves filtered reads from the
// denormalized msg_dispatch_jobs_read projection (owned by internal/stream's
// projector). The write table keeps only transactional indexes (migration
// 015), so the user-facing list / by-event / filter-options reads go to the
// projection — mirroring the events repo.
// The detail view (FindByID) and the debug raw view (FindRecentRaw) stay on
// the write table because they need the un-projected payload/metadata.
//
// FindWithFilters + DistinctValues + FindByEventID + FindRecentRaw +
// InsertBatch stay hand-rolled (dynamic SQL / pgx.Batch); everything else
// goes through *dbq.Queries.
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters + DistinctValues + InsertBatch
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FilterParams is the query DTO for /api/dispatch-jobs.
//
// The plural slice fields back the SPA's CSV multi-filters
// (clientIds/statuses/codes). `Source` is a free-text source filter.
// applications/subdomains/aggregates match the projection's real
// application/subdomain/aggregate columns (split_part of `code`), so the
// facets filter by indexed equality rather than code-prefix LIKEs.
type FilterParams struct {
	Status         *string
	ClientID       *string
	DispatchPoolID *string
	SubscriptionID *string
	Code           *string
	Source         *string
	Since          *time.Time
	Until          *time.Time
	// SortAscending flips the created_at ordering (default: newest first).
	SortAscending bool
	Limit         int
	Offset        int

	// CSV multi-filters from the SPA.
	ClientIDs    []string
	Statuses     []string
	Codes        []string
	Applications []string
	Subdomains   []string
	Aggregates   []string

	// AccessibleClientIDs: a non-nil pointer scopes results to
	// platform-scoped jobs (client_id IS NULL) plus jobs whose client_id is
	// in the set; nil means no access scoping (anchor). Mirrors
	// scheduledjob/event FilterParams — enforced in SQL so the caller's
	// clientId/clientIds filters can only narrow within the principal's own
	// tenants, never reach across them.
	AccessibleClientIDs *[]string
}

// FindByID loads a single job (write table).
func (r *Repository) FindByID(ctx context.Context, id string) (*DispatchJob, error) {
	res, err := r.q.DispatchJobFindByID(ctx, id)
	row, err := repocommon.One(res, err, "dispatch_job repo")
	if row == nil || err != nil {
		return nil, err
	}
	return findByIDRowToJob(*row)
}

// FindByEventID lists jobs spawned by a single event. Used for the
// frontend's "event detail → which dispatch jobs did this event create?"
// drill-down (GET /api/dispatch-jobs/event/{eventId}). Reads the projection
// (slim DispatchJobRead shape) — backed by idx_msg_dispatch_jobs_read_event_id.
func (r *Repository) FindByEventID(ctx context.Context, eventID string) ([]DispatchJob, error) {
	rows, err := r.pool.Query(ctx, readSelect+` WHERE event_id = $1 ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[readRow])
	if err != nil {
		return nil, err
	}
	// A corrupted kind/status/retry_strategy on any one row fails the WHOLE
	// list read (X-06: "a list containing the row fails too") rather than
	// silently skipping or coercing that row.
	out := make([]DispatchJob, 0, len(collected))
	for _, rr := range collected {
		j, err := readRowToJob(rr)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, nil
}

// readSelect is the slim projection column set shared by the filtered list
// and by-event reads. msg_dispatch_jobs_read omits payload / metadata /
// schema_id / payload_content_type / data_only — the DispatchJobRead wire
// shape doesn't surface them. Columns map to readRow by db tag (order cosmetic).
const readSelect = `SELECT id, external_id, source, kind, code, subject,
	event_id, correlation_id, target_url, protocol, service_account_id,
	client_id, subscription_id, mode, dispatch_pool_id, message_group,
	sequence, timeout_seconds, status, max_retries, retry_strategy,
	scheduled_for, expires_at, attempt_count, last_attempt_at, completed_at,
	duration_millis, last_error, idempotency_key, created_at, updated_at
	FROM msg_dispatch_jobs_read`

// FindWithFilters returns dispatch jobs matching non-nil filters, ordered
// most-recent first. Powers the frontend's job list view (GET
// /api/dispatch-jobs). Reads the msg_dispatch_jobs_read projection — the write
// table carries no query indexes (migration 015). Hand-rolled dynamic query.
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]DispatchJob, error) {
	var f repocommon.Filter
	f.EqPtr("status", p.Status)
	f.Any("status", p.Statuses)
	f.EqPtr("client_id", p.ClientID)
	f.Any("client_id", p.ClientIDs)
	if p.AccessibleClientIDs != nil {
		// SECURITY: SQL-level tenant scoping — platform-scoped rows plus the
		// principal's own tenants. Parenthesization matters: the OR group
		// must AND with the caller's other filters.
		f.Clause("(client_id IS NULL OR client_id = ANY($%d))", *p.AccessibleClientIDs)
	}
	f.EqPtr("dispatch_pool_id", p.DispatchPoolID)
	f.EqPtr("subscription_id", p.SubscriptionID)
	f.EqPtr("code", p.Code)
	f.Any("code", p.Codes)
	f.EqPtr("source", p.Source)
	// Facets filter the projection's real columns (split_part of code), backed
	// by their own indexes — replacing the old leading-wildcard code LIKEs.
	f.Any("application", p.Applications)
	f.Any("subdomain", p.Subdomains)
	f.Any("aggregate", p.Aggregates)
	if p.Since != nil {
		f.Clause("created_at >= $%d", *p.Since)
	}
	if p.Until != nil {
		f.Clause("created_at <= $%d", *p.Until)
	}
	order := " ORDER BY created_at DESC"
	if p.SortAscending {
		order = " ORDER BY created_at ASC"
	}
	q := readSelect + f.Where() + order
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q += fmt.Sprintf(" LIMIT $%d", f.Arg(limit))
	if p.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", f.Arg(p.Offset))
	}
	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[readRow])
	if err != nil {
		return nil, err
	}
	// A corrupted kind/status/retry_strategy on any one row fails the WHOLE
	// list read (X-06: "a list containing the row fails too") rather than
	// silently skipping or coercing that row.
	out := make([]DispatchJob, 0, len(collected))
	for _, rr := range collected {
		j, err := readRowToJob(rr)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, nil
}

// FindRecentRaw returns the most-recent `limit` jobs straight from the
// write-side msg_dispatch_jobs table, including payload + metadata. Powers the
// debug raw-job view (GET /bff/debug/dispatch-jobs), which needs the
// un-projected envelope the read projection drops. Mirrors the events repo's
// FindRecentRaw. Ordered most-recent first.
func (r *Repository) FindRecentRaw(ctx context.Context, limit int) ([]DispatchJob, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, external_id, source, kind, code, subject, event_id,
		        correlation_id, metadata, target_url, protocol, payload,
		        payload_content_type, data_only, service_account_id, client_id,
		        subscription_id, mode, dispatch_pool_id, message_group, sequence,
		        timeout_seconds, schema_id, status, max_retries, retry_strategy,
		        scheduled_for, expires_at, attempt_count, last_attempt_at,
		        completed_at, duration_millis, last_error, idempotency_key,
		        created_at, updated_at
		   FROM msg_dispatch_jobs
		  ORDER BY created_at DESC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.DispatchJobFindByIDRow])
	if err != nil {
		return nil, err
	}
	// A corrupted kind/status/retry_strategy on any one row fails the WHOLE
	// list read (X-06: "a list containing the row fails too") rather than
	// silently skipping or coercing that row.
	out := make([]DispatchJob, 0, len(collected))
	for _, row := range collected {
		j, err := findByIDRowToJob(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, nil
}

// DistinctValues lists distinct non-null values for a whitelisted column.
// Powers GET /api/dispatch-jobs/filter-options. Dynamic column name —
// stays hand-rolled (sqlc can't parameterise identifiers).
func (r *Repository) DistinctValues(ctx context.Context, column string, limit int) ([]string, error) {
	allowed := map[string]bool{
		"status": true, "code": true, "client_id": true,
		"dispatch_pool_id": true, "subscription_id": true, "kind": true,
	}
	if !allowed[column] {
		return nil, fmt.Errorf("dispatch_job repo: column %q not allowed", column)
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT DISTINCT %s FROM msg_dispatch_jobs_read
		              WHERE %s IS NOT NULL ORDER BY 1 LIMIT $1`, column, column),
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Insert writes a brand-new dispatch job (called by ingest + stream fan-out).
// No UoW commit — this is the infrastructure path.
func (r *Repository) Insert(ctx context.Context, j *DispatchJob) error {
	now := time.Now().UTC()
	if j.CreatedAt.IsZero() {
		j.CreatedAt = now
	}
	j.UpdatedAt = now
	metaJSON, err := json.Marshal(metadataOrEmpty(j.Metadata))
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	retry := string(j.RetryStrategy)
	pct := j.PayloadContentType
	return r.q.DispatchJobInsert(ctx, dbq.DispatchJobInsertParams{
		ID:                 j.ID,
		ExternalID:         j.ExternalID,
		Source:             j.Source,
		Kind:               string(j.Kind),
		Code:               j.Code,
		Subject:            j.Subject,
		EventID:            j.EventID,
		CorrelationID:      j.CorrelationID,
		Metadata:           metaJSON,
		TargetUrl:          j.TargetURL,
		Protocol:           string(j.Protocol),
		Payload:            j.Payload,
		PayloadContentType: &pct,
		DataOnly:           j.DataOnly,
		ServiceAccountID:   j.ServiceAccountID,
		ClientID:           j.ClientID,
		SubscriptionID:     j.SubscriptionID,
		Mode:               string(j.Mode),
		DispatchPoolID:     j.DispatchPoolID,
		MessageGroup:       j.MessageGroup,
		Sequence:           j.Sequence,
		TimeoutSeconds:     int32(j.TimeoutSeconds),
		SchemaID:           j.SchemaID,
		Status:             string(j.Status),
		MaxRetries:         int32(j.MaxRetries),
		RetryStrategy:      &retry,
		ScheduledFor:       j.ScheduledFor,
		ExpiresAt:          j.ExpiresAt,
		AttemptCount:       j.AttemptCount,
		LastAttemptAt:      j.LastAttemptAt,
		CompletedAt:        j.CompletedAt,
		DurationMillis:     j.DurationMillis,
		LastError:          j.LastError,
		IdempotencyKey:     j.IdempotencyKey,
		CreatedAt:          j.CreatedAt,
		UpdatedAt:          j.UpdatedAt,
	})
}

// InsertBatch writes many jobs in one round-trip via pgx Batch. Used by
// the stream processor's fan-out path. `ON CONFLICT (id, created_at)`
// matches the composite PK introduced by partitioning (migration 019).
// Hand-rolled because sqlc has no batch wrapper.
func (r *Repository) InsertBatch(ctx context.Context, jobs []DispatchJob) error {
	if len(jobs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	now := time.Now().UTC()
	for _, j := range jobs {
		if j.CreatedAt.IsZero() {
			j.CreatedAt = now
		}
		metaJSON, _ := json.Marshal(metadataOrEmpty(j.Metadata))
		batch.Queue(
			`INSERT INTO msg_dispatch_jobs
			     (id, external_id, source, kind, code, subject, event_id, correlation_id,
			      metadata, target_url, protocol, payload, payload_content_type, data_only,
			      service_account_id, client_id, subscription_id, mode, dispatch_pool_id,
			      message_group, sequence, timeout_seconds, schema_id, status, max_retries,
			      retry_strategy, scheduled_for, expires_at, attempt_count, last_attempt_at,
			      completed_at, duration_millis, last_error, idempotency_key, created_at, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31,$32,$33,$34,$35,$36)
			 ON CONFLICT (id, created_at) DO NOTHING`,
			j.ID, j.ExternalID, j.Source, string(j.Kind), j.Code, j.Subject, j.EventID,
			j.CorrelationID, metaJSON, j.TargetURL, string(j.Protocol), j.Payload,
			j.PayloadContentType, j.DataOnly, j.ServiceAccountID, j.ClientID,
			j.SubscriptionID, string(j.Mode), j.DispatchPoolID, j.MessageGroup,
			j.Sequence, j.TimeoutSeconds, j.SchemaID, string(j.Status), j.MaxRetries,
			string(j.RetryStrategy), j.ScheduledFor, j.ExpiresAt, j.AttemptCount,
			j.LastAttemptAt, j.CompletedAt, j.DurationMillis, j.LastError,
			j.IdempotencyKey, j.CreatedAt, now)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range jobs {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

// The status-flip methods all take the job's createdAt alongside its id:
// msg_dispatch_jobs is partitioned by created_at, and the extra equality
// lets the planner prune to the row's own partition instead of probing all
// of them. Callers have the loaded job in hand, so it's free.

// MarkInProgress flips status to PROCESSING and stamps last_attempt_at.
// Called by the router immediately before the first attempt.
func (r *Repository) MarkInProgress(ctx context.Context, id string, createdAt time.Time) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkInProgress(ctx, dbq.DispatchJobMarkInProgressParams{
		ID: id, LastAttemptAt: &now, CreatedAt: createdAt,
	})
}

// MarkCompleted flips status to COMPLETED and stamps completed_at +
// duration_millis (end-to-end). Called after a successful delivery.
func (r *Repository) MarkCompleted(ctx context.Context, id string, createdAt time.Time, durationMillis int64) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkCompleted(ctx, dbq.DispatchJobMarkCompletedParams{
		ID: id, CompletedAt: &now, DurationMillis: &durationMillis, CreatedAt: createdAt,
	})
}

// MarkFailed flips status to FAILED and stops retries. Terminal.
// Stamps last_error + completed_at + duration_millis.
func (r *Repository) MarkFailed(ctx context.Context, id string, createdAt time.Time, lastError *string, durationMillis int64) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkFailed(ctx, dbq.DispatchJobMarkFailedParams{
		ID: id, CompletedAt: &now, DurationMillis: &durationMillis, LastError: lastError,
		CreatedAt: createdAt,
	})
}

// ScheduleRetry bumps attempt_count, stamps last_error, and sets
// scheduled_for. Status stays PENDING so the poller picks it up once
// scheduled_for falls due.
func (r *Repository) ScheduleRetry(ctx context.Context, id string, createdAt time.Time, scheduledFor time.Time, lastError *string) error {
	return r.q.DispatchJobScheduleRetry(ctx, dbq.DispatchJobScheduleRetryParams{
		ID: id, ScheduledFor: &scheduledFor, LastError: lastError, CreatedAt: createdAt,
	})
}

// Reschedule sets a job back to PENDING with a future scheduled_for WITHOUT
// bumping attempt_count. For cooperative back-pressure — a subscriber that
// returned ack=false, or an HTTP 429 — which are "try again later" signals,
// not delivery failures, so they must not consume the retry budget. The
// poller re-dispatches once scheduled_for falls due.
func (r *Repository) Reschedule(ctx context.Context, id string, createdAt time.Time, scheduledFor time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE msg_dispatch_jobs
		    SET status = 'PENDING', scheduled_for = $2, updated_at = NOW()
		  WHERE id = $1 AND created_at = $3`, id, scheduledFor.UTC(), createdAt)
	return err
}

// GroupHeldBefore reports whether an EARLIER job in the message group is
// holding it up — failed, or sitting out a retry backoff (see
// GroupHoldingStatusSQL). It is the delivery-time half of the scheduler's
// claim-time hold-back: the poller stops QUEUEING a group's jobs once one is
// held, but messages already on the queue at that moment still arrive here and
// would deliver past it.
//
// "Earlier" is positional — the (sequence, created_at, id) the poller claims
// by. Asking merely whether the group contains a held job would also catch the
// held job itself the moment it became deliverable again, and the group would
// never move.
func (r *Repository) GroupHeldBefore(ctx context.Context, group string, sequence int32, createdAt time.Time, id string) (bool, error) {
	var held bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM msg_dispatch_jobs
		      WHERE message_group = $1
		        AND (`+GroupHoldingStatusSQL+`)
		        AND (sequence, created_at, id) < ($2, $3, $4))`,
		group, sequence, createdAt, id).Scan(&held)
	return held, err
}

// FindByIDs batch-loads jobs by id from the write table (full envelope,
// including payload/metadata). Used by the ResendDispatchJobs operation to
// reload the aggregates it commits via usecaseop.SaveAll. Ids that don't
// exist are silently omitted — mirrors the pre-envelope bare-UPDATE
// Requeue's behaviour of no-op'ing on unknown ids.
func (r *Repository) FindByIDs(ctx context.Context, ids []string) ([]DispatchJob, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.q.DispatchJobFindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	// A corrupted kind/status/retry_strategy on any one row fails the WHOLE
	// list read (X-06: "a list containing the row fails too") rather than
	// silently skipping or coercing that row.
	out := make([]DispatchJob, 0, len(rows))
	for _, row := range rows {
		j, err := findByIDsRowToJob(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[DispatchJob] for the human-initiated
// status overrides that go through the use-case envelope (Cancel / Complete
// / Resend — see entity.go's package doc and operations/). It only writes
// the fields those operations ever change (status, attempt_count,
// scheduled_for, completed_at, duration_millis, last_error, updated_at) —
// payload/metadata/target_url/etc are write-once at ingest and never
// revisited here. created_at is carried alongside id for partition pruning,
// matching every other status-flip query in this file.
func (r *Repository) Persist(ctx context.Context, j *DispatchJob, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).DispatchJobPersist(ctx, dbq.DispatchJobPersistParams{
		ID:             j.ID,
		Status:         string(j.Status),
		AttemptCount:   j.AttemptCount,
		ScheduledFor:   j.ScheduledFor,
		CompletedAt:    j.CompletedAt,
		DurationMillis: j.DurationMillis,
		LastError:      j.LastError,
		UpdatedAt:      time.Now().UTC(),
		CreatedAt:      j.CreatedAt,
	})
}

// Delete satisfies usecasepgx.Persist[DispatchJob], which requires both
// Persist and Delete. No operation in this module deletes a dispatch job
// today — Cancel/Complete/Resend all commit via Save/SaveAll, never Delete —
// so this is currently unreachable through any use case, but it's a real
// delete (not a stub) in case that changes.
func (r *Repository) Delete(ctx context.Context, j *DispatchJob, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).DispatchJobDelete(ctx, dbq.DispatchJobDeleteParams{
		ID: j.ID, CreatedAt: j.CreatedAt,
	})
}

// SettleAcked is the router→platform settled-message hook's repo call (see
// the settled package): resets the given ids to PENDING and records reason
// in last_error, scoped to QUEUED/PROCESSING so a row a concurrent path
// already advanced is left alone. Returns the ids actually reset (a subset
// of ids: some may not exist, or may already be past QUEUED/PROCESSING).
func (r *Repository) SettleAcked(ctx context.Context, ids []string, reason string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return r.q.DispatchJobSettleAcked(ctx, dbq.DispatchJobSettleAckedParams{
		Ids: ids, Reason: reason,
	})
}

// RecordAttempt inserts a row into msg_dispatch_job_attempts —
// generates an untyped TSID for the row id and
// derives the `status` column from the entity's Success bool
// (SUCCESS / FAILURE).
func (r *Repository) RecordAttempt(ctx context.Context, jobID string, a *Attempt) error {
	status := "FAILURE"
	if a.Success {
		status = "SUCCESS"
	}
	var responseCode *int32
	if a.ResponseCode != nil {
		v := int32(*a.ResponseCode)
		responseCode = &v
	}
	var errType *string
	if a.ErrorType != nil {
		v := string(*a.ErrorType)
		errType = &v
	}
	return r.q.DispatchJobAttemptInsert(ctx, dbq.DispatchJobAttemptInsertParams{
		ID:             tsid.GenerateUntyped(),
		DispatchJobID:  jobID,
		AttemptNumber:  &a.AttemptNumber,
		Status:         &status,
		ResponseCode:   responseCode,
		ResponseBody:   a.ResponseBody,
		ErrorMessage:   a.ErrorMessage,
		ErrorType:      errType,
		DurationMillis: a.DurationMillis,
		AttemptedAt:    &a.AttemptedAt,
		CompletedAt:    a.CompletedAt,
		CreatedAt:      time.Now().UTC(),
	})
}

// AttemptsByJob returns all attempts for a job, oldest first. The DB
// stores `status` (SUCCESS / FAILURE); entity exposes the derived
// Success bool to match the wire shape.
//
// A corrupted error_type on any one attempt fails the WHOLE list read
// (X-06: "a list containing the row fails too") rather than silently
// coercing it to UNKNOWN. The row carries no id of its own (see
// DispatchJobAttemptsByJobRow), so the job id + attempt number identify
// it in the log/error instead.
func (r *Repository) AttemptsByJob(ctx context.Context, jobID string) ([]Attempt, error) {
	rows, err := r.q.DispatchJobAttemptsByJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	out := make([]Attempt, 0, len(rows))
	for _, row := range rows {
		a := Attempt{
			CompletedAt:    row.CompletedAt,
			DurationMillis: row.DurationMillis,
			ResponseBody:   row.ResponseBody,
			ErrorMessage:   row.ErrorMessage,
		}
		if row.AttemptNumber != nil {
			a.AttemptNumber = *row.AttemptNumber
		}
		if row.AttemptedAt != nil {
			a.AttemptedAt = *row.AttemptedAt
		}
		if row.ResponseCode != nil {
			v := int(*row.ResponseCode)
			a.ResponseCode = &v
		}
		if row.Status != nil {
			a.Success = *row.Status == "SUCCESS"
		}
		if row.ErrorType != nil {
			et, ok := ParseErrorType(*row.ErrorType)
			if !ok {
				var attemptNumber int32
				if row.AttemptNumber != nil {
					attemptNumber = *row.AttemptNumber
				}
				slog.Error("dispatch job attempt row has unrecognised error type",
					"dispatch_job_id", jobID, "attempt_number", attemptNumber, "error_type", *row.ErrorType)
				return nil, usecase.Internal("CORRUPT_DISPATCH_JOB_ATTEMPT_ERROR_TYPE",
					fmt.Sprintf("dispatch job %s attempt has an unrecognised error type", jobID), nil)
			}
			a.ErrorType = &et
		}
		out = append(out, a)
	}
	return out, nil
}

// ── row → entity adapters ──────────────────────────────────────────────

func findByIDRowToJob(r dbq.DispatchJobFindByIDRow) (*DispatchJob, error) {
	return rowToJob(rawRow{
		ID: r.ID, ExternalID: r.ExternalID, Source: r.Source, Kind: r.Kind,
		Code: r.Code, Subject: r.Subject, EventID: r.EventID,
		CorrelationID: r.CorrelationID, Metadata: r.Metadata,
		TargetUrl: r.TargetUrl, Protocol: r.Protocol, Payload: r.Payload,
		PayloadContentType: r.PayloadContentType, DataOnly: r.DataOnly,
		ServiceAccountID: r.ServiceAccountID, ClientID: r.ClientID,
		SubscriptionID: r.SubscriptionID, Mode: r.Mode,
		DispatchPoolID: r.DispatchPoolID, MessageGroup: r.MessageGroup,
		Sequence: r.Sequence, TimeoutSeconds: r.TimeoutSeconds,
		SchemaID: r.SchemaID, Status: r.Status, MaxRetries: r.MaxRetries,
		RetryStrategy: r.RetryStrategy, ScheduledFor: r.ScheduledFor,
		ExpiresAt: r.ExpiresAt, AttemptCount: r.AttemptCount,
		LastAttemptAt: r.LastAttemptAt, CompletedAt: r.CompletedAt,
		DurationMillis: r.DurationMillis, LastError: r.LastError,
		IdempotencyKey: r.IdempotencyKey, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	})
}

// findByIDsRowToJob adapts DispatchJobFindByIDsRow — column-for-column
// identical to DispatchJobFindByIDRow, but sqlc mints a distinct Go type per
// query — onto the same rawRow mapper.
func findByIDsRowToJob(r dbq.DispatchJobFindByIDsRow) (*DispatchJob, error) {
	return findByIDRowToJob(dbq.DispatchJobFindByIDRow(r))
}

// readRow is the slim msg_dispatch_jobs_read column set scanned by the
// filtered list + by-event reads (see readSelect). db tags match the
// projection's columns so pgx.RowToStructByName can map them. Payload /
// metadata / schema_id / content-type / data_only are intentionally absent —
// the projection drops them and the DispatchJobRead wire shape doesn't carry
// them.
type readRow struct {
	ID               string     `db:"id"`
	ExternalID       *string    `db:"external_id"`
	Source           *string    `db:"source"`
	Kind             string     `db:"kind"`
	Code             string     `db:"code"`
	Subject          *string    `db:"subject"`
	EventID          *string    `db:"event_id"`
	CorrelationID    *string    `db:"correlation_id"`
	TargetUrl        string     `db:"target_url"`
	Protocol         string     `db:"protocol"`
	ServiceAccountID *string    `db:"service_account_id"`
	ClientID         *string    `db:"client_id"`
	SubscriptionID   *string    `db:"subscription_id"`
	Mode             string     `db:"mode"`
	DispatchPoolID   *string    `db:"dispatch_pool_id"`
	MessageGroup     *string    `db:"message_group"`
	Sequence         int32      `db:"sequence"`
	TimeoutSeconds   int32      `db:"timeout_seconds"`
	Status           string     `db:"status"`
	MaxRetries       int32      `db:"max_retries"`
	RetryStrategy    *string    `db:"retry_strategy"`
	ScheduledFor     *time.Time `db:"scheduled_for"`
	ExpiresAt        *time.Time `db:"expires_at"`
	AttemptCount     int32      `db:"attempt_count"`
	LastAttemptAt    *time.Time `db:"last_attempt_at"`
	CompletedAt      *time.Time `db:"completed_at"`
	DurationMillis   *int64     `db:"duration_millis"`
	LastError        *string    `db:"last_error"`
	IdempotencyKey   *string    `db:"idempotency_key"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

func readRowToJob(r readRow) (*DispatchJob, error) {
	return rowToJob(rawRow{
		ID: r.ID, ExternalID: r.ExternalID, Source: r.Source, Kind: r.Kind,
		Code: r.Code, Subject: r.Subject, EventID: r.EventID,
		CorrelationID: r.CorrelationID,
		TargetUrl:     r.TargetUrl, Protocol: r.Protocol,
		ServiceAccountID: r.ServiceAccountID, ClientID: r.ClientID,
		SubscriptionID: r.SubscriptionID, Mode: r.Mode,
		DispatchPoolID: r.DispatchPoolID, MessageGroup: r.MessageGroup,
		Sequence: r.Sequence, TimeoutSeconds: r.TimeoutSeconds,
		Status: r.Status, MaxRetries: r.MaxRetries,
		RetryStrategy: r.RetryStrategy, ScheduledFor: r.ScheduledFor,
		ExpiresAt: r.ExpiresAt, AttemptCount: r.AttemptCount,
		LastAttemptAt: r.LastAttemptAt, CompletedAt: r.CompletedAt,
		DurationMillis: r.DurationMillis, LastError: r.LastError,
		IdempotencyKey: r.IdempotencyKey, CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
		// Payload / Metadata / SchemaID / PayloadContentType / DataOnly absent.
	})
}

// rawRow is the union of every sqlc-generated row's field set — lets the
// small adapters above forward to a single canonical mapper.
type rawRow struct {
	ID                 string
	ExternalID         *string
	Source             *string
	Kind               string
	Code               string
	Subject            *string
	EventID            *string
	CorrelationID      *string
	Metadata           json.RawMessage
	TargetUrl          string
	Protocol           string
	Payload            *string
	PayloadContentType *string
	DataOnly           bool
	ServiceAccountID   *string
	ClientID           *string
	SubscriptionID     *string
	Mode               string
	DispatchPoolID     *string
	MessageGroup       *string
	Sequence           int32
	TimeoutSeconds     int32
	SchemaID           *string
	Status             string
	MaxRetries         int32
	RetryStrategy      *string
	ScheduledFor       *time.Time
	ExpiresAt          *time.Time
	AttemptCount       int32
	LastAttemptAt      *time.Time
	CompletedAt        *time.Time
	DurationMillis     *int64
	LastError          *string
	IdempotencyKey     *string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// rowToJob hydrates the entity from its row. A kind, status, or
// retry_strategy value that isn't one of the known constants (junk written
// before write-boundary validation existed, or a hand-edited row) is a
// loud read error — never round-tripped as-is and never coerced to a
// default, per the X-06 ruling. The row id is logged so the bad row can be
// found and fixed without a debugger.
//
// Mode stays lenient (common.ParseDispatchMode, ruled X-01) — deliberately
// not converted here.
func rowToJob(r rawRow) (*DispatchJob, error) {
	kind, ok := ParseKind(r.Kind)
	if !ok {
		slog.Error("dispatch job row has unrecognised kind", "id", r.ID, "kind", r.Kind)
		return nil, usecase.Internal("CORRUPT_DISPATCH_JOB_KIND",
			fmt.Sprintf("dispatch job %s has an unrecognised kind", r.ID), nil)
	}
	status, ok := common.ParseDispatchStatus(r.Status)
	if !ok {
		slog.Error("dispatch job row has unrecognised status", "id", r.ID, "status", r.Status)
		return nil, usecase.Internal("CORRUPT_DISPATCH_JOB_STATUS",
			fmt.Sprintf("dispatch job %s has an unrecognised status", r.ID), nil)
	}
	j := &DispatchJob{
		ID:               r.ID,
		ExternalID:       r.ExternalID,
		Kind:             kind,
		Code:             r.Code,
		Source:           r.Source,
		Subject:          r.Subject,
		TargetURL:        r.TargetUrl,
		Protocol:         ProtocolHTTPWebhook,
		Payload:          r.Payload,
		DataOnly:         r.DataOnly,
		EventID:          r.EventID,
		CorrelationID:    r.CorrelationID,
		ClientID:         r.ClientID,
		SubscriptionID:   r.SubscriptionID,
		ServiceAccountID: r.ServiceAccountID,
		DispatchPoolID:   r.DispatchPoolID,
		MessageGroup:     r.MessageGroup,
		Mode:             common.ParseDispatchMode(r.Mode),
		Sequence:         r.Sequence,
		TimeoutSeconds:   uint32(r.TimeoutSeconds),
		SchemaID:         r.SchemaID,
		MaxRetries:       uint32(r.MaxRetries),
		Status:           status,
		AttemptCount:     r.AttemptCount,
		LastError:        r.LastError,
		IdempotencyKey:   r.IdempotencyKey,
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		ScheduledFor:     r.ScheduledFor,
		ExpiresAt:        r.ExpiresAt,
		LastAttemptAt:    r.LastAttemptAt,
		CompletedAt:      r.CompletedAt,
		DurationMillis:   r.DurationMillis,
	}
	if r.PayloadContentType != nil {
		j.PayloadContentType = *r.PayloadContentType
	} else {
		j.PayloadContentType = "application/json"
	}
	if r.RetryStrategy != nil {
		retry, ok := ParseRetryStrategy(*r.RetryStrategy)
		if !ok {
			slog.Error("dispatch job row has unrecognised retry strategy",
				"id", r.ID, "retry_strategy", *r.RetryStrategy)
			return nil, usecase.Internal("CORRUPT_DISPATCH_JOB_RETRY_STRATEGY",
				fmt.Sprintf("dispatch job %s has an unrecognised retry strategy", r.ID), nil)
		}
		j.RetryStrategy = retry
	} else {
		j.RetryStrategy = RetryExponentialBackoff
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &j.Metadata)
	}
	_ = r.Protocol // single protocol today
	return j, nil
}

// metadataOrEmpty returns an empty slice for nil so the JSONB column
// stores `[]` (matches the column
// `DEFAULT '[]'::jsonb`).
func metadataOrEmpty(m []Metadata) []Metadata {
	if m == nil {
		return []Metadata{}
	}
	return m
}

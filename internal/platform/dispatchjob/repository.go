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
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
// FindWithFilters + DistinctValues + FindByEventID + FindRecentRaw stay
// hand-rolled (dynamic SQL); the reads otherwise go through *dbq.Queries. Every
// WRITE of msg_dispatch_jobs goes through the Lifecycle (lifecycle.go).
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters + DistinctValues + the reads
	q    *dbq.Queries
	lc   *Lifecycle // the one writer of msg_dispatch_jobs
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool), lc: NewLifecycle(pool)}
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
	// MessageGroup narrows to one ordered group — the grid's way of following
	// an aggregate's jobs in sequence (2026-09-22). Exact match; the read
	// projection indexes message_group.
	MessageGroup *string
	Since        *time.Time
	Until        *time.Time
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
// and by-event reads. msg_dispatch_jobs_read omits payload / schema_id /
// payload_content_type / data_only — the DispatchJobRead wire shape doesn't
// surface them. metadata and descriptor ARE projected (migration 057): the
// grid shows both. Columns map to readRow by db tag (order cosmetic).
const readSelect = `SELECT id, external_id, source, kind, code, subject,
	event_id, correlation_id, target_url, protocol, service_account_id,
	client_id, subscription_id, mode, dispatch_pool_id, message_group,
	sequence, timeout_seconds, status, max_retries, retry_strategy,
	scheduled_for, expires_at, attempt_count, last_attempt_at, completed_at,
	duration_millis, last_error, idempotency_key, descriptor, metadata,
	created_at, updated_at
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
	f.EqPtr("message_group", p.MessageGroup)
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
		        queue, descriptor, created_at, updated_at
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

// Lifecycle returns the dispatch-job lifecycle bound to this repository's
// pool: the one owner of msg_dispatch_jobs.status. Everything below that
// writes a job delegates to it; callers holding a transaction use
// Lifecycle().In(tx).
func (r *Repository) Lifecycle() *Lifecycle { return r.lc }

// InsertBatch creates jobs as PENDING (API ingest). A job whose (id,
// created_at) already exists is skipped.
func (r *Repository) InsertBatch(ctx context.Context, jobs []DispatchJob) error {
	_, err := r.lc.CreateBatch(ctx, jobs)
	return err
}

// The status-flip methods all take the job's createdAt alongside its id:
// msg_dispatch_jobs is partitioned by created_at, and the extra equality
// lets the planner prune to the row's own partition instead of probing all
// of them. Callers have the loaded job in hand, so it's free.

// ClaimForDelivery atomically claims a job for one delivery attempt. It
// reports whether this caller won the claim; false means another delivery of
// this job is already in flight (or the job finished), and the caller must not
// call the subscriber.
func (r *Repository) ClaimForDelivery(ctx context.Context, id string, createdAt time.Time) (bool, error) {
	return r.lc.ClaimForDelivery(ctx, id, createdAt)
}

// ReclaimStaleDelivery takes over a delivery whose attempt died (the process
// was killed mid-delivery, so the job stayed PROCESSING with no outcome): it
// re-stamps the claim time, only when the current claim was made before
// claimedBefore. Reports whether this caller won.
func (r *Repository) ReclaimStaleDelivery(ctx context.Context, id string, createdAt, claimedBefore time.Time) (bool, error) {
	return r.lc.ReclaimStaleDelivery(ctx, id, createdAt, claimedBefore)
}

// MarkCompleted records a successful delivery (live -> COMPLETED). A job that
// is already settled is left alone (counted as a refused transition).
func (r *Repository) MarkCompleted(ctx context.Context, id string, createdAt time.Time, durationMillis int64) error {
	_, err := r.lc.Complete(ctx, id, createdAt, durationMillis)
	return err
}

// MarkFailed records a terminal failure (live -> FAILED), stamping last_error.
func (r *Repository) MarkFailed(ctx context.Context, id string, createdAt time.Time, lastError *string, durationMillis int64) error {
	_, err := r.lc.Fail(ctx, id, createdAt, lastError, durationMillis)
	return err
}

// ScheduleRetry bumps attempt_count, stamps last_error and scheduled_for and
// returns a live job to PENDING for the poller.
func (r *Repository) ScheduleRetry(ctx context.Context, id string, createdAt time.Time, scheduledFor time.Time, lastError *string) error {
	_, err := r.lc.Retry(ctx, id, createdAt, scheduledFor, lastError)
	return err
}

// Reschedule defers a live job back to PENDING at scheduledFor WITHOUT bumping
// attempt_count (cooperative back-pressure: ack=false, HTTP 429).
func (r *Repository) Reschedule(ctx context.Context, id string, createdAt time.Time, scheduledFor time.Time) error {
	_, err := r.lc.Defer(ctx, id, createdAt, scheduledFor)
	return err
}

// Hold puts a job back to PENDING because an earlier job of its
// BLOCK_ON_ERROR group is holding the group.
func (r *Repository) Hold(ctx context.Context, id string, createdAt time.Time, scheduledFor time.Time) error {
	_, err := r.lc.Hold(ctx, id, createdAt, scheduledFor)
	return err
}

// GroupHeldBefore reports whether an EARLIER job in the message group is
// holding it up — failed, or sitting out a retry backoff (see group_hold.go). It
// is the delivery-time half of the scheduler's claim-time hold-back: the poller
// stops QUEUEING a group's jobs once one is held, but messages already on the
// queue at that moment still arrive here and would deliver past it.
//
// "Earlier" is positional — the (sequence, created_at, id) the poller claims
// by. Asking merely whether the group contains a held job would also catch the
// held job itself the moment it became deliverable again, and the group would
// never move.
func (r *Repository) GroupHeldBefore(ctx context.Context, group string, sequence int32, createdAt time.Time, id string) (bool, error) {
	var held bool
	err := r.pool.QueryRow(ctx, GroupHeldBeforeSQL, GroupHoldingStatuses, group, sequence, createdAt, id).Scan(&held)
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

// SettleAcked is the router->platform settled-message hook's repo call (see
// the settled package): resets the given ids to PENDING and records reason in
// last_error, scoped to QUEUED/PROCESSING. Returns the ids actually reset.
func (r *Repository) SettleAcked(ctx context.Context, ids []string, reason string) ([]string, error) {
	return r.lc.SettleAcked(ctx, ids, reason)
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
	var requestInfo json.RawMessage
	if a.Request != nil {
		requestInfo, _ = json.Marshal(a.Request)
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
		RequestInfo:    requestInfo,
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
		if len(row.RequestInfo) > 0 {
			var req RequestSummary
			if err := json.Unmarshal(row.RequestInfo, &req); err == nil {
				a.Request = &req
			}
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
		ServiceAccountID: r.ServiceAccountID, ClientID: ids.StringPtr(r.ClientID),
		SubscriptionID: r.SubscriptionID, Mode: r.Mode,
		DispatchPoolID: r.DispatchPoolID, MessageGroup: r.MessageGroup,
		Sequence: r.Sequence, TimeoutSeconds: r.TimeoutSeconds,
		SchemaID: r.SchemaID, Status: r.Status, MaxRetries: r.MaxRetries,
		RetryStrategy: r.RetryStrategy, ScheduledFor: r.ScheduledFor,
		ExpiresAt: r.ExpiresAt, AttemptCount: r.AttemptCount,
		LastAttemptAt: r.LastAttemptAt, CompletedAt: r.CompletedAt,
		DurationMillis: r.DurationMillis, LastError: r.LastError,
		IdempotencyKey: r.IdempotencyKey, Queue: r.Queue, Descriptor: r.Descriptor,
		CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
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
	ID               string          `db:"id"`
	ExternalID       *string         `db:"external_id"`
	Source           *string         `db:"source"`
	Kind             string          `db:"kind"`
	Code             string          `db:"code"`
	Subject          *string         `db:"subject"`
	EventID          *string         `db:"event_id"`
	CorrelationID    *string         `db:"correlation_id"`
	TargetUrl        string          `db:"target_url"`
	Protocol         string          `db:"protocol"`
	ServiceAccountID *string         `db:"service_account_id"`
	ClientID         *string         `db:"client_id"`
	SubscriptionID   *string         `db:"subscription_id"`
	Mode             string          `db:"mode"`
	DispatchPoolID   *string         `db:"dispatch_pool_id"`
	MessageGroup     *string         `db:"message_group"`
	Sequence         int32           `db:"sequence"`
	TimeoutSeconds   int32           `db:"timeout_seconds"`
	Status           string          `db:"status"`
	MaxRetries       int32           `db:"max_retries"`
	RetryStrategy    *string         `db:"retry_strategy"`
	ScheduledFor     *time.Time      `db:"scheduled_for"`
	ExpiresAt        *time.Time      `db:"expires_at"`
	AttemptCount     int32           `db:"attempt_count"`
	LastAttemptAt    *time.Time      `db:"last_attempt_at"`
	CompletedAt      *time.Time      `db:"completed_at"`
	DurationMillis   *int64          `db:"duration_millis"`
	LastError        *string         `db:"last_error"`
	IdempotencyKey   *string         `db:"idempotency_key"`
	Descriptor       *string         `db:"descriptor"`
	Metadata         json.RawMessage `db:"metadata"`
	CreatedAt        time.Time       `db:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at"`
}

func readRowToJob(r readRow) (*DispatchJob, error) {
	return rowToJob(rawRow{
		Descriptor: r.Descriptor, Metadata: r.Metadata,
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
		// Payload / SchemaID / PayloadContentType / DataOnly absent.
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
	Queue              *string
	Descriptor         *string
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
		Queue:            r.Queue,
		Descriptor:       r.Descriptor,
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

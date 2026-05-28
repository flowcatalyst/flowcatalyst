package dispatchjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Repository owns msg_dispatch_jobs (write table) and the
// msg_dispatch_job_attempts attempt history. The denormalized read table
// msg_dispatch_jobs_read is owned by internal/stream's projector;
// callers wanting fast filtered reads use the api routes below.
//
// FindWithFilters + DistinctValues + InsertBatch stay hand-rolled
// (dynamic SQL / pgx.Batch); everything else goes through *dbq.Queries.
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
// applications/subdomains/aggregates have no dedicated columns on the
// write-side msg_dispatch_jobs table, so they're matched as colon-
// delimited prefixes of the `code` column (code = "app:subdomain:agg:..").
type FilterParams struct {
	Status         *string
	ClientID       *string
	DispatchPoolID *string
	SubscriptionID *string
	Code           *string
	Source         *string
	Since          *time.Time
	Until          *time.Time
	Limit          int
	Offset         int

	// CSV multi-filters from the SPA.
	ClientIDs    []string
	Statuses     []string
	Codes        []string
	Applications []string
	Subdomains   []string
	Aggregates   []string
}

// FindByID loads a single job (write table).
func (r *Repository) FindByID(ctx context.Context, id string) (*DispatchJob, error) {
	row, err := r.q.DispatchJobFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dispatch_job repo: %w", err)
	}
	return findByIDRowToJob(row), nil
}

// FindByExternalID loads by external_id (used by idempotent ingest).
func (r *Repository) FindByExternalID(ctx context.Context, externalID string) (*DispatchJob, error) {
	row, err := r.q.DispatchJobFindByExternalID(ctx, &externalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dispatch_job repo: %w", err)
	}
	return findByExternalIDRowToJob(row), nil
}

// FindByEventID lists jobs spawned by a single event. Used for the
// frontend's "event detail → which dispatch jobs did this event create?"
// drill-down (GET /api/dispatch-jobs/event/{eventId}).
func (r *Repository) FindByEventID(ctx context.Context, eventID string) ([]DispatchJob, error) {
	rows, err := r.q.DispatchJobFindByEventID(ctx, &eventID)
	if err != nil {
		return nil, err
	}
	out := make([]DispatchJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, *findByEventIDRowToJob(row))
	}
	return out, nil
}

// FindPendingForPool returns up to limit PENDING jobs for the given pool,
// ordered by created_at. Claimed via FOR UPDATE SKIP LOCKED in tx.
func (r *Repository) FindPendingForPool(ctx context.Context, poolID string, limit int) ([]DispatchJob, error) {
	rows, err := r.q.DispatchJobFindPendingForPool(ctx, dbq.DispatchJobFindPendingForPoolParams{
		DispatchPoolID: &poolID,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]DispatchJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, *findPendingRowToJob(row))
	}
	return out, nil
}

// FindWithFilters returns dispatch jobs matching non-nil filters, ordered
// most-recent first. Powers the frontend's job list view. Hand-rolled
// dynamic query (mirrors the application repo pattern).
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]DispatchJob, error) {
	const baseSelect = `SELECT id, external_id, source, kind, code, subject,
		event_id, correlation_id, metadata, target_url, protocol, payload,
		payload_content_type, data_only, service_account_id, client_id,
		subscription_id, mode, dispatch_pool_id, message_group, sequence,
		timeout_seconds, schema_id, status, max_retries, retry_strategy,
		scheduled_for, expires_at, attempt_count, last_attempt_at, completed_at,
		duration_millis, last_error, idempotency_key, created_at, updated_at
		FROM msg_dispatch_jobs`
	q := baseSelect
	args := []any{}
	conds := []string{}
	add := func(col string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	addAny := func(col string, vs []string) {
		args = append(args, vs)
		conds = append(conds, fmt.Sprintf("%s = ANY($%d)", col, len(args)))
	}
	// codePrefix matches code segments (app:subdomain:aggregate:...) for
	// the facet filters that have no dedicated column.
	codePrefix := func(vs []string, depth int) {
		if len(vs) == 0 {
			return
		}
		ors := make([]string, 0, len(vs))
		for _, v := range vs {
			prefix := v
			for i := 0; i < depth; i++ {
				prefix = "%:" + prefix
			}
			args = append(args, prefix+":%")
			// depth 0 → "v:%", depth 1 → "%:v:%", etc.
			ors = append(ors, fmt.Sprintf("code LIKE $%d", len(args)))
		}
		conds = append(conds, "("+strings.Join(ors, " OR ")+")")
	}
	if p.Status != nil {
		add("status", *p.Status)
	}
	if len(p.Statuses) > 0 {
		addAny("status", p.Statuses)
	}
	if p.ClientID != nil {
		add("client_id", *p.ClientID)
	}
	if len(p.ClientIDs) > 0 {
		addAny("client_id", p.ClientIDs)
	}
	if p.DispatchPoolID != nil {
		add("dispatch_pool_id", *p.DispatchPoolID)
	}
	if p.SubscriptionID != nil {
		add("subscription_id", *p.SubscriptionID)
	}
	if p.Code != nil {
		add("code", *p.Code)
	}
	if len(p.Codes) > 0 {
		addAny("code", p.Codes)
	}
	if p.Source != nil {
		add("source", *p.Source)
	}
	codePrefix(p.Applications, 0)
	codePrefix(p.Subdomains, 1)
	codePrefix(p.Aggregates, 2)
	if p.Since != nil {
		args = append(args, *p.Since)
		conds = append(conds, fmt.Sprintf("created_at >= $%d", len(args)))
	}
	if p.Until != nil {
		args = append(args, *p.Until)
		conds = append(conds, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC"
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	args = append(args, limit)
	q += fmt.Sprintf(" LIMIT $%d", len(args))
	if p.Offset > 0 {
		args = append(args, p.Offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DispatchJob{}
	for rows.Next() {
		j, err := scanFilteredRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
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
		fmt.Sprintf(`SELECT DISTINCT %s FROM msg_dispatch_jobs
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
// No UoW commit — this is the infrastructure path. Column order matches
// the Rust insert in `dispatch_job/repository.rs::insert`.
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

// MarkInProgress flips status to PROCESSING and stamps last_attempt_at.
// Called by the router immediately before the first attempt.
func (r *Repository) MarkInProgress(ctx context.Context, id string) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkInProgress(ctx, dbq.DispatchJobMarkInProgressParams{
		ID: id, LastAttemptAt: &now,
	})
}

// MarkCompleted flips status to COMPLETED and stamps completed_at +
// duration_millis (end-to-end). Called after a successful delivery.
func (r *Repository) MarkCompleted(ctx context.Context, id string, durationMillis int64) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkCompleted(ctx, dbq.DispatchJobMarkCompletedParams{
		ID: id, CompletedAt: &now, DurationMillis: &durationMillis,
	})
}

// MarkFailed flips status to FAILED and stops retries. Terminal.
// Stamps last_error + completed_at + duration_millis.
func (r *Repository) MarkFailed(ctx context.Context, id string, lastError *string, durationMillis int64) error {
	now := time.Now().UTC()
	return r.q.DispatchJobMarkFailed(ctx, dbq.DispatchJobMarkFailedParams{
		ID: id, CompletedAt: &now, DurationMillis: &durationMillis, LastError: lastError,
	})
}

// ScheduleRetry bumps attempt_count, stamps last_error, and sets
// scheduled_for. Status stays PENDING so the poller picks it up once
// scheduled_for falls due.
func (r *Repository) ScheduleRetry(ctx context.Context, id string, scheduledFor time.Time, lastError *string) error {
	return r.q.DispatchJobScheduleRetry(ctx, dbq.DispatchJobScheduleRetryParams{
		ID: id, ScheduledFor: &scheduledFor, LastError: lastError,
	})
}

// RecordAttempt inserts a row into msg_dispatch_job_attempts. Mirrors
// Rust's insert_attempt — generates an untyped TSID for the row id and
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
// Success bool to match the Rust wire shape.
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
			et := ParseErrorType(*row.ErrorType)
			a.ErrorType = &et
		}
		out = append(out, a)
	}
	return out, nil
}

// ── row → entity adapters ──────────────────────────────────────────────

func findByIDRowToJob(r dbq.DispatchJobFindByIDRow) *DispatchJob {
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

func findByExternalIDRowToJob(r dbq.DispatchJobFindByExternalIDRow) *DispatchJob {
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

func findByEventIDRowToJob(r dbq.DispatchJobFindByEventIDRow) *DispatchJob {
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

func findPendingRowToJob(r dbq.DispatchJobFindPendingForPoolRow) *DispatchJob {
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

// rawRow is the union of every sqlc-generated row's field set — lets the
// four small adapters above forward to a single canonical mapper.
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

func rowToJob(r rawRow) *DispatchJob {
	j := &DispatchJob{
		ID:               r.ID,
		ExternalID:       r.ExternalID,
		Kind:             ParseKind(r.Kind),
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
		Status:           common.ParseDispatchStatus(r.Status),
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
		j.RetryStrategy = ParseRetryStrategy(*r.RetryStrategy)
	} else {
		j.RetryStrategy = RetryExponentialBackoff
	}
	if len(r.Metadata) > 0 {
		_ = json.Unmarshal(r.Metadata, &j.Metadata)
	}
	_ = r.Protocol // single protocol today — see Rust DispatchProtocol::from_str
	return j
}

// scanFilteredRow is the FindWithFilters scan path — kept hand-rolled
// because the dynamic SELECT can't be sqlc-generated.
func scanFilteredRow(rows pgx.Rows) (*DispatchJob, error) {
	var r rawRow
	if err := rows.Scan(&r.ID, &r.ExternalID, &r.Source, &r.Kind, &r.Code,
		&r.Subject, &r.EventID, &r.CorrelationID, &r.Metadata, &r.TargetUrl,
		&r.Protocol, &r.Payload, &r.PayloadContentType, &r.DataOnly,
		&r.ServiceAccountID, &r.ClientID, &r.SubscriptionID, &r.Mode,
		&r.DispatchPoolID, &r.MessageGroup, &r.Sequence, &r.TimeoutSeconds,
		&r.SchemaID, &r.Status, &r.MaxRetries, &r.RetryStrategy,
		&r.ScheduledFor, &r.ExpiresAt, &r.AttemptCount, &r.LastAttemptAt,
		&r.CompletedAt, &r.DurationMillis, &r.LastError, &r.IdempotencyKey,
		&r.CreatedAt, &r.UpdatedAt); err != nil {
		return nil, err
	}
	return rowToJob(r), nil
}

// metadataOrEmpty returns an empty slice for nil so the JSONB column
// stores `[]` (matches Rust's `Vec::new()` default and the column
// `DEFAULT '[]'::jsonb`).
func metadataOrEmpty(m []Metadata) []Metadata {
	if m == nil {
		return []Metadata{}
	}
	return m
}

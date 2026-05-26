package scheduledjob

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Table: msg_scheduled_jobs.
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*ScheduledJob, error) {
	row, err := r.q.ScheduledJobFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scheduled_job repo: %w", err)
	}
	return rowToScheduledJob(row), nil
}

// FindByCode loads by (code, client_id). clientID may be nil for platform-scoped.
func (r *Repository) FindByCode(ctx context.Context, code string, clientID *string) (*ScheduledJob, error) {
	var (
		row dbq.MsgScheduledJob
		err error
	)
	if clientID != nil {
		row, err = r.q.ScheduledJobFindByCodeClient(ctx, dbq.ScheduledJobFindByCodeClientParams{
			Code: code, ClientID: clientID,
		})
	} else {
		row, err = r.q.ScheduledJobFindByCodePlatform(ctx, code)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scheduled_job repo: %w", err)
	}
	return rowToScheduledJob(row), nil
}

// FindWithFilters returns jobs matching non-nil filters. Hand-rolled
// dynamic query (mirrors the application repo pattern).
func (r *Repository) FindWithFilters(ctx context.Context, status, clientID *string) ([]ScheduledJob, error) {
	const baseSelect = `SELECT id, client_id, code, name, description, status, crons, timezone,
		payload, concurrent, tracks_completion, timeout_seconds,
		delivery_max_attempts, target_url, last_fired_at, created_at, updated_at,
		created_by, updated_by, version FROM msg_scheduled_jobs`
	q := baseSelect
	args := []any{}
	conds := []string{}
	if status != nil {
		args = append(args, *status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if clientID != nil {
		args = append(args, *clientID)
		conds = append(conds, fmt.Sprintf("client_id = $%d", len(args)))
	}
	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY code`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduledJob
	for rows.Next() {
		var row dbq.MsgScheduledJob
		if err := rows.Scan(
			&row.ID, &row.ClientID, &row.Code, &row.Name, &row.Description, &row.Status,
			&row.Crons, &row.Timezone, &row.Payload, &row.Concurrent, &row.TracksCompletion,
			&row.TimeoutSeconds, &row.DeliveryMaxAttempts, &row.TargetUrl, &row.LastFiredAt,
			&row.CreatedAt, &row.UpdatedAt, &row.CreatedBy, &row.UpdatedBy, &row.Version,
		); err != nil {
			return nil, err
		}
		out = append(out, *rowToScheduledJob(row))
	}
	return out, rows.Err()
}

// FindActive lists ACTIVE jobs; used by the scheduler poller.
func (r *Repository) FindActive(ctx context.Context) ([]ScheduledJob, error) {
	rows, err := r.q.ScheduledJobFindActive(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ScheduledJob, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToScheduledJob(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[ScheduledJob].
func (r *Repository) Persist(ctx context.Context, j *ScheduledJob, tx *usecasepgx.DbTx) error {
	var payloadBytes []byte
	if len(j.Payload) > 0 {
		payloadBytes = []byte(j.Payload)
	}
	return r.q.WithTx(tx.Inner()).ScheduledJobUpsert(ctx, dbq.ScheduledJobUpsertParams{
		ID:                  j.ID,
		ClientID:            j.ClientID,
		Code:                j.Code,
		Name:                j.Name,
		Description:         j.Description,
		Status:              string(j.Status),
		Crons:               j.Crons,
		Timezone:            j.Timezone,
		Payload:             payloadBytes,
		Concurrent:          j.Concurrent,
		TracksCompletion:    j.TracksCompletion,
		TimeoutSeconds:      j.TimeoutSeconds,
		DeliveryMaxAttempts: j.DeliveryMaxAttempts,
		TargetUrl:           j.TargetURL,
		LastFiredAt:         j.LastFiredAt,
		CreatedAt:           j.CreatedAt,
		UpdatedAt:           time.Now().UTC(),
		CreatedBy:           j.CreatedBy,
		UpdatedBy:           j.UpdatedBy,
		Version:             j.Version,
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, j *ScheduledJob, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ScheduledJobDelete(ctx, j.ID)
}

func rowToScheduledJob(row dbq.MsgScheduledJob) *ScheduledJob {
	j := ScheduledJob{
		ID:                  row.ID,
		ClientID:            row.ClientID,
		Code:                row.Code,
		Name:                row.Name,
		Description:         row.Description,
		Status:              ParseStatus(row.Status),
		Crons:               row.Crons,
		Timezone:            row.Timezone,
		Concurrent:          row.Concurrent,
		TracksCompletion:    row.TracksCompletion,
		TimeoutSeconds:      row.TimeoutSeconds,
		DeliveryMaxAttempts: row.DeliveryMaxAttempts,
		TargetURL:           row.TargetUrl,
		LastFiredAt:         row.LastFiredAt,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		CreatedBy:           row.CreatedBy,
		UpdatedBy:           row.UpdatedBy,
		Version:             row.Version,
	}
	if j.Crons == nil {
		j.Crons = []string{}
	}
	if len(row.Payload) > 0 {
		j.Payload = json.RawMessage(row.Payload)
	}
	return &j
}

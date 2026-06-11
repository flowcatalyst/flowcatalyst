package process

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Table: msg_processes. The
// schema has no created_by column (matches Rust's None default); the
// entity's CreatedBy field stays for API-shape compat but never
// round-trips through the DB.
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Process, error) {
	res, err := r.q.ProcessFindByID(ctx, id)
	row, err := repocommon.One(res, err, "process repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToProcess(*row), nil
}

// FindByCode loads by unique code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*Process, error) {
	res, err := r.q.ProcessFindByCode(ctx, code)
	row, err := repocommon.One(res, err, "process repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToProcess(*row), nil
}

// FindWithFilters returns processes matching non-nil filters. Hand-rolled
// dynamic query — see docs/sqlc.md.
func (r *Repository) FindWithFilters(ctx context.Context, application, subdomain, status *string) ([]Process, error) {
	var f repocommon.Filter
	f.EqPtr("application", application)
	f.EqPtr("subdomain", subdomain)
	f.EqPtr("status", status)

	q := `SELECT id, code, name, description, status, source, application,
		subdomain, process_name, body, diagram_type, tags, created_at, updated_at FROM msg_processes` +
		f.Where() + ` ORDER BY code`

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.MsgProcess])
	if err != nil {
		return nil, err
	}
	out := make([]Process, 0, len(collected))
	for _, row := range collected {
		out = append(out, *rowToProcess(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[Process]. CreatedBy is dropped
// — the schema has no column for it (matches Rust).
func (r *Repository) Persist(ctx context.Context, p *Process, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ProcessUpsert(ctx, dbq.ProcessUpsertParams{
		ID:          p.ID,
		Code:        p.Code,
		Name:        p.Name,
		Description: p.Description,
		Status:      string(p.Status),
		Source:      string(p.Source),
		Application: p.Application,
		Subdomain:   p.Subdomain,
		ProcessName: p.ProcessName,
		Body:        p.Body,
		DiagramType: p.DiagramType,
		// msg_processes.tags is NOT NULL; normalize here so a nil slice
		// from any caller (reload round-trip, sync update) can't 23502.
		Tags:      append([]string{}, p.Tags...),
		CreatedAt: p.CreatedAt,
		UpdatedAt: time.Now().UTC(),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, p *Process, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ProcessDelete(ctx, p.ID)
}

func rowToProcess(row dbq.MsgProcess) *Process {
	return &Process{
		ID:          row.ID,
		Code:        row.Code,
		Name:        row.Name,
		Description: row.Description,
		Status:      ParseStatus(row.Status),
		Source:      ParseSource(row.Source),
		Application: row.Application,
		Subdomain:   row.Subdomain,
		ProcessName: row.ProcessName,
		Body:        row.Body,
		DiagramType: row.DiagramType,
		Tags:        append([]string{}, row.Tags...),
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}

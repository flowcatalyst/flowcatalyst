package openapispecs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the Postgres-backed repo for
// app_application_openapi_specs.
//
// The sync use case does *direct* writes through this repo (Insert,
// ArchiveCurrent) — domain-event emission happens in a tail transaction
// via usecaseop.Emit. The partial UNIQUE index on
// (application_id) WHERE status='CURRENT' serialises concurrent syncs;
// the loser observes a unique-violation error and the caller retries.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wires the repo against the supplied pool.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const specSelect = `SELECT id, application_id, version, status, spec, spec_hash,
    change_notes, change_notes_text, synced_at, synced_by, created_at, updated_at
    FROM app_application_openapi_specs`

// FindByID loads a single spec. Returns (nil, nil) when not found.
func (r *Repository) FindByID(ctx context.Context, id string) (*OpenApiSpec, error) {
	row := r.pool.QueryRow(ctx, specSelect+` WHERE id = $1`, id)
	spec, err := scanSpec(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("openapi_spec find_by_id: %w", err)
	}
	return spec, nil
}

// FindCurrentByApplication returns the application's single CURRENT
// row, or nil if no spec has ever been synced.
func (r *Repository) FindCurrentByApplication(ctx context.Context, applicationID string) (*OpenApiSpec, error) {
	row := r.pool.QueryRow(ctx,
		specSelect+` WHERE application_id = $1 AND status = 'CURRENT'`, applicationID)
	spec, err := scanSpec(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("openapi_spec find_current: %w", err)
	}
	return spec, nil
}

// FindAllByApplication returns every spec for the application, newest
// CURRENT first then archived versions in synced_at DESC order.
func (r *Repository) FindAllByApplication(ctx context.Context, applicationID string) ([]OpenApiSpec, error) {
	rows, err := r.pool.Query(ctx,
		specSelect+`
		WHERE application_id = $1
		ORDER BY (status = 'CURRENT') DESC, synced_at DESC, id DESC`,
		applicationID)
	if err != nil {
		return nil, fmt.Errorf("openapi_spec find_all_by_app: %w", err)
	}
	defer rows.Close()
	var out []OpenApiSpec
	for rows.Next() {
		spec, err := scanSpec(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *spec)
	}
	return out, rows.Err()
}

// ExistsByApplicationAndVersion reports whether ANY (CURRENT or
// ARCHIVED) row already has (application_id, version). Drives the
// sync use case's "+timestamp" disambiguation when info.version
// collides with an archived row.
func (r *Repository) ExistsByApplicationAndVersion(ctx context.Context, applicationID, version string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM app_application_openapi_specs
			WHERE application_id = $1 AND version = $2
		)`, applicationID, version).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("openapi_spec exists_by_version: %w", err)
	}
	return exists, nil
}

// Insert writes a new spec row. Caller mints id + status + timestamps
// via New(); this function just persists. Returns a wrap of any error
// from the unique-violation on (application_id, version) or the
// partial unique on CURRENT-per-app.
func (r *Repository) Insert(ctx context.Context, s *OpenApiSpec) error {
	var notesJSON []byte
	if s.ChangeNotes != nil {
		j, err := json.Marshal(s.ChangeNotes)
		if err != nil {
			return fmt.Errorf("openapi_spec marshal change_notes: %w", err)
		}
		notesJSON = j
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO app_application_openapi_specs
			(id, application_id, version, status, spec, spec_hash,
			 change_notes, change_notes_text, synced_at, synced_by,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		s.ID, s.ApplicationID, s.Version, string(s.Status), []byte(s.Spec), s.SpecHash,
		notesJSON, s.ChangeNotesText, s.SyncedAt, s.SyncedBy, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("openapi_spec insert: %w", err)
	}
	return nil
}

// ArchiveCurrent flips the application's current spec to ARCHIVED and
// stamps the computed diff onto it. Returns (false, nil) when no
// CURRENT row exists; (true, nil) on a successful flip.
func (r *Repository) ArchiveCurrent(ctx context.Context, applicationID string, notes ChangeNotes, summary string) (bool, error) {
	notesJSON, err := json.Marshal(notes)
	if err != nil {
		return false, fmt.Errorf("openapi_spec marshal change_notes: %w", err)
	}
	now := time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE app_application_openapi_specs
		SET status = 'ARCHIVED',
		    change_notes = $1,
		    change_notes_text = $2,
		    updated_at = $3
		WHERE application_id = $4 AND status = 'CURRENT'`,
		notesJSON, summary, now, applicationID)
	if err != nil {
		return false, fmt.Errorf("openapi_spec archive_current: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ── helpers ──────────────────────────────────────────────────────────────

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSpec(rs rowScanner) (*OpenApiSpec, error) {
	var (
		spec       OpenApiSpec
		status     string
		specBytes  []byte
		notesBytes []byte
	)
	if err := rs.Scan(
		&spec.ID, &spec.ApplicationID, &spec.Version, &status, &specBytes, &spec.SpecHash,
		&notesBytes, &spec.ChangeNotesText, &spec.SyncedAt, &spec.SyncedBy,
		&spec.CreatedAt, &spec.UpdatedAt,
	); err != nil {
		return nil, err
	}
	spec.Status = ParseStatus(status)
	if len(specBytes) > 0 {
		spec.Spec = json.RawMessage(specBytes)
	}
	if len(notesBytes) > 0 {
		var n ChangeNotes
		if err := json.Unmarshal(notesBytes, &n); err == nil {
			spec.ChangeNotes = &n
		}
	}
	return &spec, nil
}

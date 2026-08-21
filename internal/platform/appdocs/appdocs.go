// Package appdocs stores application-synced documentation: Markdown pages
// an application pushes through the SDK sync surface
// (POST /api/applications/{appCode}/docs/sync), read by administrators in
// Platform → Documentation. The sync is declarative — each call replaces
// the application's whole doc set, so the app's repo remains the source of
// truth exactly like event-type and OpenAPI-spec sync.
package appdocs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Doc is one synced documentation page.
type Doc struct {
	ID            string    `json:"id"`
	ApplicationID string    `json:"applicationId"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Content       string    `json:"content"`
	Position      int       `json:"position"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Summary is a Doc without its content (list surfaces).
type Summary struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// Input is one page in a sync payload, in payload order.
type Input struct {
	Slug    string
	Title   string
	Content string
}

// Repository is the pgx-backed store for app_docs.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires the repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// ListByApplication returns the app's doc summaries in sync order.
func (r *Repository) ListByApplication(ctx context.Context, applicationID string) ([]Summary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT slug, title FROM app_docs WHERE application_id = $1 ORDER BY position, slug`,
		applicationID)
	if err != nil {
		return nil, fmt.Errorf("app_docs list: %w", err)
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var s Summary
		if err := rows.Scan(&s.Slug, &s.Title); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ApplicationIDsWithDocs returns the distinct application ids that have at
// least one synced page — the grouped documentation index's spine.
func (r *Repository) ApplicationIDsWithDocs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT application_id FROM app_docs`)
	if err != nil {
		return nil, fmt.Errorf("app_docs apps: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetByApplicationSlug loads one page. Returns (nil, nil) when absent.
func (r *Repository) GetByApplicationSlug(ctx context.Context, applicationID, slug string) (*Doc, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, application_id, slug, title, content, position, created_at, updated_at
		 FROM app_docs WHERE application_id = $1 AND slug = $2`,
		applicationID, slug)
	var d Doc
	if err := row.Scan(&d.ID, &d.ApplicationID, &d.Slug, &d.Title, &d.Content,
		&d.Position, &d.CreatedAt, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("app_docs get: %w", err)
	}
	return &d, nil
}

// ReplaceResult reports what a declarative replace did.
type ReplaceResult struct {
	Created int
	Updated int
	Deleted int
	Slugs   []string
}

// ReplaceForApplication makes the app's stored doc set equal the payload, in
// payload order, inside one transaction. Existing slugs keep their id and
// created_at (updated in place); unlisted slugs are deleted — the sync IS
// the set.
func (r *Repository) ReplaceForApplication(ctx context.Context, applicationID string, docs []Input) (ReplaceResult, error) {
	var res ReplaceResult
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return res, fmt.Errorf("app_docs replace begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	existing := map[string]bool{}
	rows, err := tx.Query(ctx, `SELECT slug FROM app_docs WHERE application_id = $1`, applicationID)
	if err != nil {
		return res, fmt.Errorf("app_docs replace scan: %w", err)
	}
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			rows.Close()
			return res, err
		}
		existing[slug] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	listed := map[string]bool{}
	now := time.Now().UTC()
	for i, d := range docs {
		listed[d.Slug] = true
		res.Slugs = append(res.Slugs, d.Slug)
		if existing[d.Slug] {
			res.Updated++
		} else {
			res.Created++
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO app_docs (id, application_id, slug, title, content, position, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $7)
			 ON CONFLICT (application_id, slug) DO UPDATE SET
			     title = EXCLUDED.title,
			     content = EXCLUDED.content,
			     position = EXCLUDED.position,
			     updated_at = EXCLUDED.updated_at`,
			tsid.Generate(tsid.AppDoc), applicationID, d.Slug, d.Title, d.Content, i, now); err != nil {
			return res, fmt.Errorf("app_docs replace upsert %q: %w", d.Slug, err)
		}
	}
	for slug := range existing {
		if !listed[slug] {
			if _, err := tx.Exec(ctx,
				`DELETE FROM app_docs WHERE application_id = $1 AND slug = $2`,
				applicationID, slug); err != nil {
				return res, fmt.Errorf("app_docs replace delete %q: %w", slug, err)
			}
			res.Deleted++
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("app_docs replace commit: %w", err)
	}
	return res, nil
}

// Package migrate is a focused, no-deps SQL migration runner.
//
// Reads every *.sql file from the supplied embed.FS (or directory),
// applies them in filename order, and tracks applied migrations in a
// _fc_migrations table.
//
// Mirrors the Rust fc-platform/src/shared/database.rs::run_migrations
// behaviour: production profile applies everything in lexicographic order
// once each. No down migrations — this is forward-only by design.
package migrate

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Source is anything that can list + read .sql files. The two impls in
// this package are FS (embed.FS) and Dir (filesystem directory).
type Source interface {
	List() ([]string, error)
	Read(name string) ([]byte, error)
}

// FS wraps an embed.FS rooted at "migrations/".
type FS struct {
	FS   embed.FS
	Root string // e.g. "migrations"
}

func (s FS) List() ([]string, error) {
	entries, err := fs.ReadDir(s.FS, s.Root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s FS) Read(name string) ([]byte, error) {
	return s.FS.ReadFile(s.Root + "/" + name)
}

// Run applies every pending migration in order. Idempotent — already-
// applied migrations (by filename) are skipped.
func Run(ctx context.Context, pool *pgxpool.Pool, src Source) error {
	if err := ensureTrackerTable(ctx, pool); err != nil {
		return fmt.Errorf("ensure tracker: %w", err)
	}

	applied, err := loadApplied(ctx, pool)
	if err != nil {
		return fmt.Errorf("load applied: %w", err)
	}

	names, err := src.List()
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	for _, name := range names {
		if applied[name] {
			continue
		}
		body, err := src.Read(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if err := applyOne(ctx, pool, name, body); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func ensureTrackerTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx,
		`CREATE TABLE IF NOT EXISTS _fc_migrations (
		    name VARCHAR(255) PRIMARY KEY,
		    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		 )`)
	return err
}

func loadApplied(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT name FROM _fc_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out[n] = true
	}
	return out, rows.Err()
}

// applyOne runs a migration in a transaction so partial application is
// impossible. The SQL file may contain multiple statements separated by
// semicolons — pgx accepts that via Exec.
func applyOne(ctx context.Context, pool *pgxpool.Pool, name string, body []byte) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, string(body)); err != nil {
		return fmt.Errorf("exec sql: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO _fc_migrations (name, applied_at) VALUES ($1, $2)`,
		name, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

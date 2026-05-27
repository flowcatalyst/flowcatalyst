// Package migrate applies the embedded schema migrations to a Postgres
// database using github.com/pressly/goose/v3 as the runner.
//
// Each migration is a numbered .sql file under internal/migrate/sql/
// prefixed with `-- +goose Up`. Forward-only by design — we don't write
// down migrations. New migrations: `internal/migrate/sql/NNN_subject.sql`
// where NNN is the next zero-padded sequence.
//
// Run is idempotent. A pre-goose database (one whose history is tracked
// in the legacy `_fc_migrations` table) is upgraded transparently: the
// applied versions are seeded into `goose_db_version` and the legacy
// table is dropped before goose runs.
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed all:sql
var migrationsFS embed.FS

// Run applies every pending migration to pool's database.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)

	if err := bootstrap(ctx, db); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	return goose.UpContext(ctx, db, "sql")
}

// bootstrap seeds goose_db_version from the legacy _fc_migrations
// tracker (if present) so existing databases skip re-application on the
// cutover. Drops _fc_migrations once seeded. Safe to call on fresh
// databases: returns immediately when no legacy tracker exists.
func bootstrap(ctx context.Context, db *sql.DB) error {
	var hasLegacy bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM information_schema.tables
		     WHERE table_name = '_fc_migrations'
		 )`,
	).Scan(&hasLegacy); err != nil {
		return fmt.Errorf("probe _fc_migrations: %w", err)
	}
	if !hasLegacy {
		return nil
	}

	if _, err := db.ExecContext(ctx, gooseSchemaDDL); err != nil {
		return fmt.Errorf("ensure goose_db_version: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT name FROM _fc_migrations`)
	if err != nil {
		return fmt.Errorf("read _fc_migrations: %w", err)
	}
	defer rows.Close()

	versionRe := regexp.MustCompile(`^(\d+)_`)
	var versions []int64
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		m := versionRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		v, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, v := range versions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO goose_db_version (version_id, is_applied)
			 SELECT $1, TRUE
			 WHERE NOT EXISTS (
			     SELECT 1 FROM goose_db_version WHERE version_id = $1
			 )`, v); err != nil {
			return fmt.Errorf("seed goose version %d: %w", v, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE _fc_migrations`); err != nil {
		return fmt.Errorf("drop _fc_migrations: %w", err)
	}
	return tx.Commit()
}

// gooseSchemaDDL matches the table goose creates lazily on first use.
// Declaring it here lets us populate the table during bootstrap without
// having to call goose.Up first (which would attempt to apply all
// migrations against a database that already has the schema).
const gooseSchemaDDL = `
CREATE TABLE IF NOT EXISTS goose_db_version (
    id serial NOT NULL,
    version_id bigint NOT NULL,
    is_applied boolean NOT NULL,
    tstamp timestamp NULL default now(),
    PRIMARY KEY(id)
);

INSERT INTO goose_db_version (version_id, is_applied)
SELECT 0, TRUE
WHERE NOT EXISTS (SELECT 1 FROM goose_db_version WHERE version_id = 0);
`

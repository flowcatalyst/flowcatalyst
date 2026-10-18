// Package migrate applies the embedded schema migrations to a Postgres
// database using github.com/pressly/goose/v3 as the runner.
//
// Each migration is a numbered .sql file under internal/migrate/sql/
// prefixed with `-- +goose Up`. Forward-only by design — we don't write
// down migrations. New migrations: `internal/migrate/sql/NNN_subject.sql`
// where NNN is the next zero-padded sequence.
//
// Run is idempotent. An already-migrated database is detected and its goose
// ledger is seeded so nothing re-runs (which matters because migrations
// 019/022 DROP and recreate the messaging tables). Go's own legacy
// `_fc_migrations` tracker, the previous platform's `_schema_migrations`
// tracker (never trusted by number), and a populated-but-untracked schema are
// all recognised. See bootstrap for the precedence.
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
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

	// AllowMissing: a foreign database is seeded by evidence, so a lower
	// migration may be pending beneath a higher one that is already visible.
	return goose.UpContext(ctx, db, "sql", goose.WithAllowMissing())
}

// bootstrap makes goose_db_version agree with the schema actually present, so
// an existing database is never re-migrated (migrations 019/022 DROP and
// recreate the messaging tables) and a migration is never recorded as applied
// without its effect existing. In order:
//
//  0. goose already tracks this database: its ledger is the truth. A foreign
//     tracker must never add to it. Entries whose effect probe fails are
//     removed (repairLedger) so goose re-applies them; that undoes an earlier
//     boot that seeded by number from another implementation's tracker.
//  1. `_fc_migrations` — Go's own pre-goose tracker (a `name` column), same
//     numbering as goose. Seed from it (probed versions only if their effect
//     is visible), then drop it.
//  2. `_schema_migrations` (another implementation's tracker, `migration_id`)
//     or a populated schema with no tracker (`tnt_clients` exists). The
//     tracker's NUMBERS are not used: the implementations number identically
//     only through maxSharedVersion and differently after, so seeding by
//     number marked our 059 (functions) applied because their 034 was. Seed
//     only versions whose effect is visible (effectProbes); everything else is
//     left for goose, which runs it (every migration from 031 is re-runnable).
//     The foreign tracker is left in place: its owner may still use it.
//
// A genuinely fresh database matches none of these and is left untouched.
// Safe + idempotent on every boot.
func bootstrap(ctx context.Context, db *sql.DB) error {
	if hasGoose, err := gooseHasMigrations(ctx, db); err != nil {
		return err
	} else if hasGoose {
		return repairLedger(ctx, db)
	}

	// (1) Go's own pre-goose tracker.
	if ok, err := tableExists(ctx, db, "_fc_migrations"); err != nil {
		return err
	} else if ok {
		versions, err := trackerVersions(ctx, db, "_fc_migrations", "name")
		if err != nil {
			return err
		}
		versions, err = withEvidence(ctx, db, versions, true)
		if err != nil {
			return err
		}
		slog.Info("migrate: seeding goose ledger from legacy _fc_migrations",
			"migrations", len(versions))
		return seedGoose(ctx, db, versions, "_fc_migrations")
	}

	// (2) Another implementation's tracker, or a populated untracked schema.
	foreign, err := tableExists(ctx, db, "_schema_migrations")
	if err != nil {
		return err
	}
	populated, err := tableExists(ctx, db, "tnt_clients")
	if err != nil {
		return err
	}
	if !foreign && !populated {
		return nil // (3) fresh database
	}
	if !populated {
		// A tracker with none of our tables behind it: nothing to seed.
		return nil
	}
	shipped, err := shippedVersions()
	if err != nil {
		return err
	}
	versions, err := withEvidence(ctx, db, shipped, false)
	if err != nil {
		return err
	}
	slog.Warn("migrate: existing schema not tracked by goose; seeding only the migrations whose "+
		"effect is present, goose applies the rest",
		"foreignTracker", foreign, "seeded", len(versions), "shipped", len(shipped))
	return seedGoose(ctx, db, versions, "")
}

// maxSharedVersion is the last migration number the platform implementations
// number identically; from the next one on, numbers mean different things.
const maxSharedVersion = 30

// withEvidence keeps the versions whose effect is visible in the database. A
// version with no probe and no implication is kept only when trustUnprobed
// (Go's own tracker); a foreign or untracked database never gets one seeded.
func withEvidence(ctx context.Context, db *sql.DB, versions []int64, trustUnprobed bool) ([]int64, error) {
	var out []int64
	for _, v := range versions {
		ok, known, err := effectVisible(ctx, db, v)
		if err != nil {
			return nil, err
		}
		if ok || (!known && trustUnprobed) {
			out = append(out, v)
		}
	}
	return out, nil
}

// effectVisible reports whether migration v's effect is present. known is
// false when v has no probe of its own and no implication.
func effectVisible(ctx context.Context, db *sql.DB, v int64) (visible, known bool, err error) {
	probe, ok := effectProbes[v]
	if !ok {
		w, implied := impliedBy[v]
		if !implied {
			return false, false, nil
		}
		probe, ok = effectProbes[w]
		if !ok {
			return false, false, nil
		}
	}
	if err := db.QueryRowContext(ctx, "SELECT "+probe).Scan(&visible); err != nil { //nolint:gosec // G202: probes are in-package constants
		return false, true, fmt.Errorf("probe migration %d: %w", v, err)
	}
	return visible, true, nil
}

// repairLedger removes goose entries whose effect probe fails, so goose
// re-applies them. It only ever un-marks; it never marks.
func repairLedger(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx,
		`SELECT DISTINCT version_id FROM goose_db_version WHERE version_id > 0 AND is_applied`)
	if err != nil {
		return fmt.Errorf("read goose ledger: %w", err)
	}
	var applied []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return err
		}
		applied = append(applied, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, v := range applied {
		if _, probed := effectProbes[v]; !probed {
			continue // implications and unprobed versions are never repaired
		}
		ok, _, err := effectVisible(ctx, db, v)
		if err != nil {
			return err
		}
		if ok {
			continue
		}
		slog.Warn("migrate: goose ledger records a migration whose effect is missing; "+
			"un-marking it so it is applied", "version", v)
		if _, err := db.ExecContext(ctx, `DELETE FROM goose_db_version WHERE version_id = $1`, v); err != nil {
			return fmt.Errorf("un-mark migration %d: %w", v, err)
		}
	}
	return nil
}

// tableExists reports whether a table of the given name is visible in the
// connected database (any schema, matching the original probe).
func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var ok bool
	if err := db.QueryRowContext(ctx,
		`SELECT EXISTS (
		     SELECT 1 FROM information_schema.tables WHERE table_name = $1
		 )`, name).Scan(&ok); err != nil {
		return false, fmt.Errorf("probe %s: %w", name, err)
	}
	return ok, nil
}

// gooseHasMigrations reports whether goose has already recorded at least one
// real migration (version_id > 0) for this database.
func gooseHasMigrations(ctx context.Context, db *sql.DB) (bool, error) {
	ok, err := tableExists(ctx, db, "goose_db_version")
	if err != nil || !ok {
		return false, err
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM goose_db_version WHERE version_id > 0`).Scan(&n); err != nil {
		return false, fmt.Errorf("count goose_db_version: %w", err)
	}
	return n > 0, nil
}

// trackerVersions reads a tracker table's id column and returns the numeric
// version prefix of each entry. table/column are trusted in-package
// constants, not user input.
func trackerVersions(ctx context.Context, db *sql.DB, table, column string) ([]int64, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+column+" FROM "+table) //nolint:gosec // G202: table/column are internal constants (see doc), not user input
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", table, err)
	}
	defer rows.Close()
	var versions []int64
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		if v, ok := versionPrefix(id); ok {
			versions = append(versions, v)
		}
	}
	return versions, rows.Err()
}

// shippedVersions returns the numeric prefix of every migration file embedded
// under sql/, so a tracker-less but populated database can be baselined to the
// exact set this binary ships.
func shippedVersions() ([]int64, error) {
	entries, err := migrationsFS.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	var versions []int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if v, ok := versionPrefix(e.Name()); ok {
			versions = append(versions, v)
		}
	}
	return versions, nil
}

// seedGoose ensures goose_db_version exists (with goose's zero row) and marks
// each version applied, skipping any already present. When dropTable is
// non-empty it is dropped in the same transaction (used to retire Go's own
// _fc_migrations after seeding). dropTable is a trusted in-package constant.
func seedGoose(ctx context.Context, db *sql.DB, versions []int64, dropTable string) error {
	if _, err := db.ExecContext(ctx, gooseSchemaDDL); err != nil {
		return fmt.Errorf("ensure goose_db_version: %w", err)
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
	if dropTable != "" {
		if _, err := tx.ExecContext(ctx, "DROP TABLE "+dropTable); err != nil {
			return fmt.Errorf("drop %s: %w", dropTable, err)
		}
	}
	return tx.Commit()
}

// versionPrefix extracts the leading NNN_ numeric version from a migration id
// or filename (e.g. "026_processes" or "026_processes.sql" → 26).
func versionPrefix(name string) (int64, bool) {
	m := versionRe.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

var versionRe = regexp.MustCompile(`^(\d+)_`)

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

//go:build integration

package migrate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Bootstrap tests (F27). Every database here is a throwaway database on the
// test binary's own embedded Postgres, dropped when the test ends.

const latestShipped = 82

// emptyDB returns a pool on a brand-new, completely empty database.
func emptyDB(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	base := testpg.Pool(t)
	_, err := base.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	require.NoError(t, err)
	_, err = base.Exec(ctx, "CREATE DATABASE "+name)
	require.NoError(t, err)
	cfg := base.Config().Copy()
	cfg.ConnConfig.Database = name
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		p.Close()
		_, _ = base.Exec(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})
	return p
}

func exists(t *testing.T, p *pgxpool.Pool, q string, args ...any) bool {
	t.Helper()
	var ok bool
	require.NoError(t, p.QueryRow(context.Background(), "SELECT "+q, args...).Scan(&ok))
	return ok
}

func hasTable(t *testing.T, p *pgxpool.Pool, name string) bool {
	return exists(t, p, `EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = $1)`, name)
}

func hasColumn(t *testing.T, p *pgxpool.Pool, table, col string) bool {
	return exists(t, p, `EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = $1 AND column_name = $2)`, table, col)
}

func gooseApplied(t *testing.T, p *pgxpool.Pool) map[int64]bool {
	t.Helper()
	rows, err := p.Query(context.Background(),
		`SELECT version_id FROM goose_db_version WHERE version_id > 0 AND is_applied`)
	require.NoError(t, err)
	defer rows.Close()
	got := map[int64]bool{}
	for rows.Next() {
		var v int64
		require.NoError(t, rows.Scan(&v))
		got[v] = true
	}
	require.NoError(t, rows.Err())
	return got
}

// requireFullyMigrated asserts goose records every shipped migration and the
// effects that the number-seeding hazard used to lose are present.
func requireFullyMigrated(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	applied := gooseApplied(t, p)
	for _, v := range []int64{19, 22, 30, 31, 35, 40, 45, 48, 49, 59, 62, 68, 72, 81, 82} {
		assert.True(t, applied[v], "goose should record migration %d", v)
	}
	assert.True(t, applied[latestShipped])
	for _, tbl := range []string{"fng_functions", "fng_versions", "fng_routes", "oauth_identity_provider_allowed_roles", "portal_apps"} {
		assert.True(t, hasTable(t, p, tbl), "table %s", tbl)
	}
	assert.True(t, hasColumn(t, p, "msg_dispatch_jobs", "connection_id"))
	assert.True(t, hasColumn(t, p, "oauth_idp_role_mappings", "idp_type"))
	assert.False(t, hasTable(t, p, "msg_dispatch_queue"))
	assert.True(t, exists(t, p, `EXISTS (SELECT 1 FROM pg_partitioned_table pt JOIN pg_class c ON c.oid = pt.partrelid WHERE c.relname = 'iam_login_attempts')`))
}

// A fresh database is migrated from scratch.
func TestBootstrap_FreshDatabase(t *testing.T) {
	ctx := context.Background()
	p := emptyDB(t, "mig_boot_fresh")
	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
	require.NoError(t, migrate.Run(ctx, p), "second boot is a no-op")
}

// Every effect probe holds on a database migrated by goose itself, so a probe
// can never disagree with a migration's real final state (which would make
// repairLedger un-mark a healthy migration).
func TestProbesHoldOnFullyMigratedDatabase(t *testing.T) {
	p := testpg.Pool(t)
	for v, ok := range migrate.ProbeResults(t.Context(), t, p) {
		assert.True(t, ok, "probe for migration %d is false on a fully migrated database", v)
	}
}

// A Go-migrated database whose goose ledger was lost is re-baselined from the
// schema, not re-migrated: the destructive 019/022 never run again.
func TestBootstrap_GoDatabaseWithLostLedger(t *testing.T) {
	ctx := context.Background()
	p := emptyDB(t, "mig_boot_goledger")
	require.NoError(t, migrate.Run(ctx, p))
	_, err := p.Exec(ctx, `INSERT INTO msg_events (id, type, source, time, created_at) VALUES ('evt_bootsrv01','a:b:c:d','s', now(), now())`)
	require.NoError(t, err)
	_, err = p.Exec(ctx, `DROP TABLE goose_db_version`)
	require.NoError(t, err)

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
	assert.True(t, exists(t, p, `EXISTS (SELECT 1 FROM msg_events WHERE id = 'evt_bootsrv01')`),
		"data survived: partitioning migrations did not re-run")
}

// foreignTracker records the other implementation's migration ids in its own
// tracker table, as that implementation does.
func foreignTracker(t *testing.T, p *pgxpool.Pool, ids ...string) {
	t.Helper()
	ctx := context.Background()
	_, err := p.Exec(ctx, `CREATE TABLE IF NOT EXISTS _schema_migrations (
		migration_id VARCHAR(100) PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
		duration_ms INTEGER, checksum TEXT)`)
	require.NoError(t, err)
	for _, id := range ids {
		_, err = p.Exec(ctx, `INSERT INTO _schema_migrations (migration_id) VALUES ($1) ON CONFLICT DO NOTHING`, id)
		require.NoError(t, err)
	}
}

// foreignFingerprint builds a representative database that the other
// implementation migrated first: the shared 001-030 schema, then that
// implementation's own later effects (its 031+ numbering is NOT ours: its 034
// is the function registry, ours is 059). Hand-authored, not a copy of its
// files; TestBootstrap_RealForeignMigrations replays the real files when
// FC_FOREIGN_MIGRATIONS_DIR points at them.
func foreignFingerprint(t *testing.T, name string) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	p := emptyDB(t, name)
	require.NoError(t, migrate.UpTo(ctx, p, 30))
	for _, stmt := range []string{
		// effects of Go 034/039/042-ish migrations, created under its numbering
		`ALTER TABLE iam_principals ADD COLUMN all_applications boolean NOT NULL DEFAULT true`,
		`ALTER TABLE iam_service_accounts ADD COLUMN scope VARCHAR(20)`,
		`ALTER TABLE iam_service_accounts ADD COLUMN client_ids TEXT[]`,
		`ALTER TABLE msg_event_types ADD COLUMN created_by VARCHAR(17)`,
		`ALTER TABLE msg_subscriptions ADD COLUMN created_by VARCHAR(17)`,
		`ALTER TABLE msg_scheduled_jobs ADD COLUMN application_id VARCHAR(17)`,
		`ALTER TABLE oauth_clients ADD COLUMN previous_secret_ref TEXT`,
		`ALTER TABLE oauth_clients ADD COLUMN previous_secret_expires_at TIMESTAMPTZ`,
		`ALTER TABLE oauth_clients ADD COLUMN previous_secret_last_used_at TIMESTAMPTZ`,
		// its own function registry, a different table family from ours (fng_)
		`CREATE TABLE fnr_functions (id VARCHAR(17) PRIMARY KEY, address TEXT NOT NULL)`,
	} {
		_, err := p.Exec(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	ids := []string{}
	for i := 1; i <= 70; i++ {
		if i == 23 || i == 36 || i == 40 || i == 45 || i == 48 || i == 49 {
			continue // gaps in that implementation's numbering
		}
		ids = append(ids, fmt.Sprintf("%03d_theirs", i))
	}
	ids = append(ids, "034_functions")
	foreignTracker(t, p, ids...)
	return p
}

// Rust-migrated database (tracker up to 070): Go must apply its own 031+ and
// must not treat the other implementation's numbers as its own.
func TestBootstrap_ForeignMigratedDatabase(t *testing.T) {
	ctx := context.Background()
	p := foreignFingerprint(t, "mig_boot_foreign")
	require.False(t, hasTable(t, p, "fng_functions"))

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
	assert.True(t, hasTable(t, p, "fnr_functions"), "the other implementation's objects are left alone")
	assert.True(t, hasTable(t, p, "_schema_migrations"), "the foreign tracker is left in place")
	require.NoError(t, migrate.Run(ctx, p), "second boot is a no-op")
}

// A foreign database migrated only partway (tracker at 015, tables 001-015
// present): Go seeds only what is visible and applies the rest, including
// the destructive messaging partitioning exactly once.
func TestBootstrap_ForeignDatabaseAtEarlyVersion(t *testing.T) {
	ctx := context.Background()
	p := emptyDB(t, "mig_boot_early")
	require.NoError(t, migrate.UpTo(ctx, p, 15))
	var ids []string
	for i := 1; i <= 15; i++ {
		ids = append(ids, fmt.Sprintf("%03d_theirs", i))
	}
	foreignTracker(t, p, ids...)

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
}

// A tracker that claims far more than the database contains is not believed:
// numbers are never copied, only visible effects.
func TestBootstrap_ForeignTrackerOverclaims(t *testing.T) {
	ctx := context.Background()
	p := emptyDB(t, "mig_boot_overclaim")
	require.NoError(t, migrate.UpTo(ctx, p, 30))
	var ids []string
	for i := 1; i <= 82; i++ {
		ids = append(ids, fmt.Sprintf("%03d_theirs", i))
	}
	foreignTracker(t, p, ids...)

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
}

// A ledger poisoned by the old number-copying bootstrap (goose marks 1-70
// applied on a database that lacks our 031+ effects) is repaired: the marks
// whose effect is missing are removed and the migrations applied.
func TestBootstrap_RepairsLedgerSeededByNumber(t *testing.T) {
	ctx := context.Background()
	p := foreignFingerprint(t, "mig_boot_poisoned")
	// UpTo(30) left goose at 30; the old bootstrap would have copied 31-70.
	_, err := p.Exec(ctx, `INSERT INTO goose_db_version (version_id, is_applied) SELECT g, true FROM generate_series(31, 70) g`)
	require.NoError(t, err)
	require.False(t, hasTable(t, p, "fng_functions"))

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
}

// Rust at 067-070 (tracker complete) on a Go database whose ledger is older
// than 072: goose's own ledger rules; the other tracker adds nothing, and the
// pending Go migrations (072+) are applied.
func TestBootstrap_GoLedgerBehindForeignTracker(t *testing.T) {
	ctx := context.Background()
	p := emptyDB(t, "mig_boot_behind")
	require.NoError(t, migrate.UpTo(ctx, p, 71))
	foreignTracker(t, p, "067_theirs", "068_theirs", "069_theirs", "070_theirs")
	require.False(t, hasColumn(t, p, "msg_dispatch_jobs", "connection_id"))

	require.NoError(t, migrate.Run(ctx, p))
	requireFullyMigrated(t, p)
	assert.True(t, gooseApplied(t, p)[72])
}

// Replays the real files of the other implementation, when available
// (read-only). Skipped unless FC_FOREIGN_MIGRATIONS_DIR is set.
func TestBootstrap_RealForeignMigrations(t *testing.T) {
	dir := os.Getenv("FC_FOREIGN_MIGRATIONS_DIR")
	if dir == "" {
		t.Skip("FC_FOREIGN_MIGRATIONS_DIR not set")
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)

	for _, upTo := range []int{30, 45, 62, 1000} {
		t.Run(fmt.Sprintf("through_%d", upTo), func(t *testing.T) {
			ctx := context.Background()
			p := emptyDB(t, fmt.Sprintf("mig_boot_real_%d", upTo))
			var ids []string
			for _, f := range files {
				name := strings.TrimSuffix(filepath.Base(f), ".sql")
				var n int
				_, _ = fmt.Sscanf(name, "%d_", &n)
				if n > upTo {
					break
				}
				b, err := os.ReadFile(f)
				require.NoError(t, err)
				_, err = p.Exec(ctx, string(b))
				require.NoError(t, err, name)
				ids = append(ids, name)
			}
			foreignTracker(t, p, ids...)
			require.NoError(t, migrate.Run(ctx, p))
			requireFullyMigrated(t, p)
			require.NoError(t, migrate.Run(ctx, p))
		})
	}
}

//go:build integration

// External test package (not `package migrate`): testpg imports migrate to
// run the migration set, so a test file augmenting package migrate itself
// cannot also import testpg without an import cycle.
package migrate_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestMigration056DuplicatePreCheck_Fires proves migration 056's pre-check
// DO block actually detects a collision under the new
// (application_code, client_id, code) key and raises a message naming the
// offending code, rather than the collision only ever surfacing later as a
// raw unique-violation out of CREATE UNIQUE INDEX.
//
// Not parallel-safe (mirrors testpg.WithConstraintDropped's own caveat): it
// drops the migration's two unique indexes table-wide for the window
// between seeding the duplicate and re-running the migration's Up section,
// on the one shared database this package's tests use.
func TestMigration056DuplicatePreCheck_Fires(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	raw, err := os.ReadFile("sql/056_connection_application_scope.sql")
	require.NoError(t, err)
	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")

	// The migration's own unique indexes already enforce this key, so a
	// duplicate insert against a normally-migrated database would never get
	// this far. Dropping them re-creates the scenario the pre-check exists
	// for: an operator re-running this migration against a database that
	// grew a collision since it was last attempted.
	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS uq_msg_connections_app_client_code`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `DROP INDEX IF EXISTS uq_msg_subscriptions_app_client_code`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM msg_connections WHERE id IN ('cnx_precheckdup01', 'cnx_precheckdup02')`)
		// Re-running the migration's own Up section restores both indexes
		// (CREATE UNIQUE INDEX IF NOT EXISTS) and re-applies every other
		// statement in it idempotently, leaving the schema exactly as every
		// other test sharing this database expects it.
		if _, err := pool.Exec(cleanupCtx, up); err != nil {
			t.Errorf("restore migration 056 schema state: %v", err)
		}
	})

	const code = "migration056-precheck-dupe"
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ('cnx_precheckdup01', $1, 'First', 'ACTIVE', 'sva_precheck1')`, code)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_connections (id, code, name, status, service_account_id)
		 VALUES ('cnx_precheckdup02', $1, 'Second', 'ACTIVE', 'sva_precheck2')`, code)
	require.NoError(t, err, "the index is dropped, so this duplicate insert must succeed")

	_, err = pool.Exec(ctx, up)
	require.Error(t, err, "the migration's pre-check must fire before it ever reaches CREATE UNIQUE INDEX")
	assert.Contains(t, err.Error(), "msg_connections has rows that collide",
		"the raised message must identify which table has the collision")
	assert.Contains(t, err.Error(), code, "the raised message must name the offending code")
}

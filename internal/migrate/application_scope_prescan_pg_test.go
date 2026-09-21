//go:build integration

package migrate_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestMigration056PrescanExecutes runs the operator's production pre-scan
// (scripts/ops/056-application-scope-prescan.sql) for real. An ops query that
// has only ever been read is how a deploy discovers a typo at the worst
// moment; this proves it parses, runs, and — the new unique indexes being in
// place here — reports no colliding groups. It deliberately references no
// column migration 056 adds, because its job is to run BEFORE 056.
func TestMigration056PrescanExecutes(t *testing.T) {
	raw, err := os.ReadFile("../../scripts/ops/056-application-scope-prescan.sql")
	require.NoError(t, err)

	rows, err := testpg.Pool(t).Query(context.Background(), string(raw))
	require.NoError(t, err)
	defer rows.Close()

	var groups int
	for rows.Next() {
		groups++
	}
	require.NoError(t, rows.Err())
	require.Zero(t, groups, "a migrated schema enforces the key, so the pre-scan finds nothing")
}

//go:build integration

package seed

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

// TestSeededSchemasAreVersionOnePointZero pins the seeded schema version to
// "1.0" — the initial version the create operation and the SDKs use — and
// walks the upgrade path for a database the seeder populated back when it
// wrote 'v1': a re-seed must not attach a second copy next to the legacy row,
// and migration 055 must rename it in place.
func TestSeededSchemasAreVersionOnePointZero(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := NewSeeder(pool)

	versions := func() map[string]int {
		rows, err := pool.Query(ctx,
			`SELECT sv.version, COUNT(*)
			   FROM msg_event_type_spec_versions sv
			   JOIN msg_event_types et ON et.id = sv.event_type_id
			  WHERE et.code LIKE 'platform:%'
			  GROUP BY sv.version`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]int{}
		for rows.Next() {
			var v string
			var n int
			require.NoError(t, rows.Scan(&v, &n))
			out[v] = n
		}
		require.NoError(t, rows.Err())
		return out
	}

	require.NoError(t, s.seedPlatformEventTypes(ctx))
	fresh := versions()
	require.NotZero(t, fresh["1.0"], "a fresh seed attaches schemas as 1.0")
	assert.Len(t, fresh, 1, "and as nothing else: %v", fresh)

	// Rewind to what an older seeder left behind.
	_, err := pool.Exec(ctx,
		`UPDATE msg_event_type_spec_versions sv SET version = 'v1'
		   FROM msg_event_types et
		  WHERE et.id = sv.event_type_id AND et.code LIKE 'platform:%'`)
	require.NoError(t, err)

	require.NoError(t, s.seedPlatformEventTypes(ctx))
	assert.Equal(t, map[string]int{"v1": fresh["1.0"]}, versions(),
		"a re-seed over legacy v1 rows must not attach the schema a second time")

	raw, err := os.ReadFile("../../migrate/sql/055_platform_event_schema_version.sql")
	require.NoError(t, err)
	up, _, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Equal(t, fresh, versions(), "migration 055 renames every legacy v1 to 1.0")
}

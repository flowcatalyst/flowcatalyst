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

const (
	oldFunctionDeleted = "platform:admin:function:deleted"
	newFunctionDeleted = "platform:function:function:deleted"
)

func migration073(t *testing.T) (up, down string) {
	t.Helper()
	raw, err := os.ReadFile("../../migrate/sql/073_function_event_type_codes.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")
	return up, down
}

// A database an older seed populated holds platform:admin:function:deleted,
// with its schema and subscription bindings. Migration 073 renames it to the
// emitted platform:function:function:deleted (same id, schema kept, subdomain
// and aggregate recomputed), the bindings naming it follow, a wildcard and
// other function codes stay, the seed adds no second row, re-running changes
// nothing, and Down renames it back with its bindings.
func TestMigration073_RenamesFunctionDeleted(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_fn_deleted_rename")
	up, down := migration073(t)

	for _, row := range [][2]string{
		{"etc_f073_0000001", oldFunctionDeleted},
		{"etc_f073_0000002", "platform:admin:function:created"},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Seeded', 'CURRENT', 'UI', 'platform', 'admin', 'function')`, row[0], row[1])
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
			 VALUES ($1, $2, '1.0', 'application/schema+json', $3::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
			strings.Replace(row[0], "etc_", "sch_", 1), row[0], string(platformEventSchemas()[newFunctionDeleted]))
		require.NoError(t, err)
	}
	// Bindings: by row and code, by code only, a wildcard, another function code.
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (id, subscription_id, event_type_id, event_type_code, spec_version) VALUES
		 (907301, 'sub_f073_0000001', 'etc_f073_0000001', 'platform:admin:function:deleted', '1.0'),
		 (907302, 'sub_f073_0000001', 'etc_f073_0000002', 'platform:admin:function:created', '1.0'),
		 (907303, 'sub_f073_0000002', NULL, 'platform:admin:function:deleted', NULL),
		 (907304, 'sub_f073_0000003', NULL, 'platform:admin:function:*', NULL)`)
	require.NoError(t, err)

	type row struct{ ID, Subdomain, Aggregate string }
	types := func() map[string]row {
		rows, err := pool.Query(ctx,
			`SELECT code, id, subdomain, aggregate FROM msg_event_types WHERE code LIKE 'platform:%function%'`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]row{}
		for rows.Next() {
			var code string
			var r row
			require.NoError(t, rows.Scan(&code, &r.ID, &r.Subdomain, &r.Aggregate))
			out[code] = r
		}
		require.NoError(t, rows.Err())
		return out
	}
	bindings := func() map[int][2]string {
		rows, err := pool.Query(ctx,
			`SELECT id, COALESCE(event_type_id, ''), event_type_code FROM msg_subscription_event_types
			  WHERE subscription_id LIKE 'sub_f073_%'`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[int][2]string{}
		for rows.Next() {
			var id int
			var etID, code string
			require.NoError(t, rows.Scan(&id, &etID, &code))
			out[id] = [2]string{etID, code}
		}
		require.NoError(t, rows.Err())
		return out
	}
	schemaCount := func(id string) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE event_type_id = $1`, id).Scan(&n))
		return n
	}

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	after := types()
	assert.NotContains(t, after, oldFunctionDeleted)
	assert.Equal(t, row{"etc_f073_0000001", "function", "function"}, after[newFunctionDeleted], "keeps its id")
	assert.Equal(t, row{"etc_f073_0000002", "admin", "function"}, after["platform:admin:function:created"])
	assert.Equal(t, 1, schemaCount("etc_f073_0000001"), "schema kept")
	wantBindings := map[int][2]string{
		907301: {"etc_f073_0000001", newFunctionDeleted},
		907302: {"etc_f073_0000002", "platform:admin:function:created"},
		907303: {"", newFunctionDeleted},
		907304: {"", "platform:admin:function:*"},
	}
	assert.Equal(t, wantBindings, bindings())

	// The seed adds the other function codes but no second deleted row, and
	// attaches no second schema.
	require.NoError(t, NewSeeder(pool).seedPlatformEventTypes(ctx))
	seeded := types()
	assert.Equal(t, "etc_f073_0000001", seeded[newFunctionDeleted].ID)
	assert.NotContains(t, seeded, oldFunctionDeleted)
	assert.Contains(t, seeded, "platform:admin:function:promoted")
	assert.Equal(t, 1, schemaCount("etc_f073_0000001"))

	// Running the migration again changes nothing.
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Equal(t, seeded, types())
	assert.Equal(t, wantBindings, bindings())

	// Down renames it back; the bindings follow.
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	reverted := types()
	assert.NotContains(t, reverted, newFunctionDeleted)
	assert.Equal(t, row{"etc_f073_0000001", "admin", "function"}, reverted[oldFunctionDeleted])
	assert.Equal(t, map[int][2]string{
		907301: {"etc_f073_0000001", oldFunctionDeleted},
		907302: {"etc_f073_0000002", "platform:admin:function:created"},
		907303: {"", oldFunctionDeleted},
		907304: {"", "platform:admin:function:*"},
	}, bindings())
}

// When the new code is already an event type (inserted by this build's seeder
// or a Java/Rust seeder), migration 073 moves the old row's bindings to it,
// drops a binding that would duplicate one, and deletes the old row with its
// schema versions, without a unique-constraint failure.
func TestMigration073_MergesIntoAnExistingNewRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_fn_deleted_clash")
	up, _ := migration073(t)

	for _, row := range [][3]string{
		{"etc_f073clash001", oldFunctionDeleted, "admin"},
		{"etc_f073clash002", newFunctionDeleted, "function"},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Function Deleted', 'CURRENT', 'UI', 'platform', $3, 'function')`, row[0], row[1], row[2])
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
			 VALUES ($1, $2, '1.0', 'application/schema+json', '{"type":"object"}'::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
			strings.Replace(row[0], "etc_", "sch_", 1), row[0])
		require.NoError(t, err)
	}
	// sub 1: old by row and code; sub 2: old by code only; sub 3: bound to
	// both (the old binding would duplicate the new one).
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (id, subscription_id, event_type_id, event_type_code) VALUES
		 (907311, 'sub_f073clash001', 'etc_f073clash001', 'platform:admin:function:deleted'),
		 (907312, 'sub_f073clash002', NULL, 'platform:admin:function:deleted'),
		 (907313, 'sub_f073clash003', 'etc_f073clash001', 'platform:admin:function:deleted'),
		 (907314, 'sub_f073clash003', 'etc_f073clash002', 'platform:function:function:deleted')`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_types WHERE id = 'etc_f073clash001' OR code = $1`, oldFunctionDeleted).Scan(&n))
	assert.Zero(t, n, "the old row is gone")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE event_type_id = 'etc_f073clash001'`).Scan(&n))
	assert.Zero(t, n, "with its schema versions")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE event_type_id = 'etc_f073clash002'`).Scan(&n))
	assert.Equal(t, 1, n, "the new row keeps its schema")

	rows, err := pool.Query(ctx,
		`SELECT id, subscription_id, event_type_id, event_type_code FROM msg_subscription_event_types
		  WHERE subscription_id LIKE 'sub_f073clash%' ORDER BY id`)
	require.NoError(t, err)
	type bound struct {
		ID                     int
		Sub, EventTypeID, Code string
	}
	var got []bound
	for rows.Next() {
		var b bound
		require.NoError(t, rows.Scan(&b.ID, &b.Sub, &b.EventTypeID, &b.Code))
		got = append(got, b)
	}
	rows.Close()
	require.NoError(t, rows.Err())
	assert.Equal(t, []bound{
		{907311, "sub_f073clash001", "etc_f073clash002", newFunctionDeleted},
		{907312, "sub_f073clash002", "etc_f073clash002", newFunctionDeleted},
		{907314, "sub_f073clash003", "etc_f073clash002", newFunctionDeleted},
	}, got)

	// Running it again changes nothing.
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_subscription_event_types WHERE subscription_id LIKE 'sub_f073clash%'`).Scan(&n))
	assert.Equal(t, 3, n)
}

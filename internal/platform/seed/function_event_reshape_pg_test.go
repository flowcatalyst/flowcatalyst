//go:build integration

package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// reshapedFunctionCodes are migration 074's renames: Go's older code, the
// Java/Rust code, and the schema every seed before 074 attached to the old one.
var reshapedFunctionCodes = []struct {
	old, new  string
	oldSchema json.RawMessage
}{
	{
		"platform:admin:function:created", "platform:function:function:created",
		obj(reqStr("functionId"), reqStr("address"), reqStr("name")),
	},
	{
		"platform:admin:function:updated", "platform:function:function:updated",
		obj(reqStr("functionId"), reqStr("address")),
	},
	{
		"platform:admin:function-version:published", "platform:function:version:published",
		obj(reqStr("functionId"), reqStr("address"), reqU32("number"), reqStr("digest"), reqStr("runtime")),
	},
	{
		"platform:admin:function-version:retired", "platform:function:version:retired",
		obj(reqStr("functionId"), reqStr("address"), reqU32("number")),
	},
	{
		"platform:admin:function-alias:set", "platform:function:alias:changed",
		obj(reqStr("functionId"), reqStr("address"), reqStr("alias"), reqU32("number")),
	},
	{
		"platform:admin:function-alias:deleted", "platform:function:alias:removed",
		obj(reqStr("functionId"), reqStr("address"), reqStr("alias")),
	},
	{
		"platform:admin:function-domain:claimed", "platform:function:domain:claimed",
		obj(reqStr("domainId"), reqStr("zone"), optStr("clientId")),
	},
	{
		"platform:admin:function-domain:released", "platform:function:domain:released",
		obj(reqStr("domainId"), reqStr("zone")),
	},
}

func migration074(t *testing.T) (up, down string) {
	t.Helper()
	raw, err := os.ReadFile("../../migrate/sql/074_function_event_type_reshape.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")
	return up, down
}

type fnRow struct{ ID, Subdomain, Aggregate string }

func fnTypes(t *testing.T, pool *pgxpool.Pool) map[string]fnRow {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT code, id, subdomain, aggregate FROM msg_event_types WHERE code LIKE 'platform:%function%'`)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]fnRow{}
	for rows.Next() {
		var code string
		var r fnRow
		require.NoError(t, rows.Scan(&code, &r.ID, &r.Subdomain, &r.Aggregate))
		out[code] = r
	}
	require.NoError(t, rows.Err())
	return out
}

// fnBindings returns each binding of the subscriptions whose id starts with
// prefix as "subscription|eventTypeId|code", sorted.
func fnBindings(t *testing.T, pool *pgxpool.Pool, prefix string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT subscription_id || '|' || COALESCE(event_type_id, '') || '|' || event_type_code
		   FROM msg_subscription_event_types WHERE subscription_id LIKE $1 || '%'`, prefix)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	sort.Strings(out)
	return out
}

func schemaOfRow(t *testing.T, pool *pgxpool.Pool, id string) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT schema_content::text FROM msg_event_type_spec_versions WHERE event_type_id = $1 ORDER BY version`, id)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out = append(out, s)
	}
	require.NoError(t, rows.Err())
	return out
}

func insertEventType(t *testing.T, pool *pgxpool.Pool, id, code string, schema json.RawMessage) {
	t.Helper()
	parts := strings.Split(code, ":")
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
		 VALUES ($1, $2, 'Seeded', 'CURRENT', 'UI', 'platform', $3, $4)`, id, code, parts[1], parts[2])
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
		 VALUES ($1, $2, '1.0', 'application/schema+json', $3::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
		strings.Replace(id, "etc_", "sch_", 1), id, string(schema))
	require.NoError(t, err)
}

// Migration 074's schema rows are the old seeded schema and the seed's new
// one for each renamed code.
func TestMigration074_SchemasAreTheSeeds(t *testing.T) {
	up, down := migration074(t)
	row := regexp.MustCompile(`\('(platform:[a-z:-]+)',\s*'(\{[^']+)',\s*'(\{[^']+)'\)`)
	seeded := platformEventSchemas()
	upRows := row.FindAllStringSubmatch(up, -1)
	downRows := row.FindAllStringSubmatch(down, -1)
	require.Len(t, upRows, len(reshapedFunctionCodes))
	require.Len(t, downRows, len(reshapedFunctionCodes))
	for i, c := range reshapedFunctionCodes {
		assert.Equal(t, c.old, upRows[i][1])
		assert.Equal(t, c.new, downRows[i][1])
		for _, m := range [][]string{upRows[i], downRows[i]} {
			assert.JSONEq(t, string(c.oldSchema), m[2], c.old)
			assert.JSONEq(t, string(seeded[c.new]), m[3], c.new)
		}
	}
}

// A database an older seed populated holds the eight platform:admin:function*
// codes Go now emits under platform:function:*, with their seeded schemas and
// subscription bindings, plus the setting codes. Migration 074 renames each
// (id kept, subdomain and aggregate recomputed, the seeded schema replaced, an
// edited one kept), the bindings naming them follow, a subscription to a
// setting code also gets the secret and config codes, a wildcard and the kept
// codes stay, the seed adds no second row or schema, re-running changes
// nothing, and Down renames back with the bindings and seeded schemas.
func TestMigration074_RenamesTheSharedFunctionEvents(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_fn_event_reshape")
	up, down := migration074(t)

	ids := map[string]string{}
	for i, c := range reshapedFunctionCodes {
		ids[c.old] = fmt.Sprintf("etc_f074_%07d", i+1)
		insertEventType(t, pool, ids[c.old], c.old, c.oldSchema)
	}
	edited := `{"type":"object","description":"edited"}`
	_, err := pool.Exec(ctx, `UPDATE msg_event_type_spec_versions SET schema_content = $1::jsonb WHERE event_type_id = $2`,
		edited, ids["platform:admin:function-alias:deleted"])
	require.NoError(t, err)
	for i, code := range []string{
		"platform:admin:function-setting:set", "platform:admin:function-setting:deleted", "platform:admin:function:promoted",
	} {
		ids[code] = fmt.Sprintf("etc_f074_%07d", 20+i)
		insertEventType(t, pool, ids[code], code, platformEventSchemas()[code])
	}

	// sub 1: created by row and code, alias:set by code only, setting:set by code.
	// sub 2: setting:set and setting:deleted by row (config:updated added once).
	// sub 3: a wildcard, and the kept promoted code.
	// sub 4: setting:set, already bound to secret:set.
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (id, subscription_id, event_type_id, event_type_code, spec_version) VALUES
		 (907401, 'sub_f074_0000001', $1, 'platform:admin:function:created', '1.0'),
		 (907402, 'sub_f074_0000001', NULL, 'platform:admin:function-alias:set', NULL),
		 (907403, 'sub_f074_0000001', NULL, 'platform:admin:function-setting:set', NULL),
		 (907404, 'sub_f074_0000002', $2, 'platform:admin:function-setting:set', '1.0'),
		 (907405, 'sub_f074_0000002', $3, 'platform:admin:function-setting:deleted', '1.0'),
		 (907406, 'sub_f074_0000003', NULL, 'platform:admin:function:*', NULL),
		 (907407, 'sub_f074_0000003', $4, 'platform:admin:function:promoted', NULL),
		 (907408, 'sub_f074_0000004', NULL, 'platform:admin:function-setting:set', NULL),
		 (907409, 'sub_f074_0000004', NULL, 'platform:function:secret:set', NULL)`,
		ids["platform:admin:function:created"], ids["platform:admin:function-setting:set"],
		ids["platform:admin:function-setting:deleted"], ids["platform:admin:function:promoted"])
	require.NoError(t, err)
	before := fnBindings(t, pool, "sub_f074_")

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	after := fnTypes(t, pool)
	seeded := platformEventSchemas()
	for _, c := range reshapedFunctionCodes {
		assert.NotContains(t, after, c.old)
		parts := strings.Split(c.new, ":")
		assert.Equal(t, fnRow{ids[c.old], parts[1], parts[2]}, after[c.new], "%s keeps its id", c.new)
		got := schemaOfRow(t, pool, ids[c.old])
		require.Len(t, got, 1, c.new)
		if c.old == "platform:admin:function-alias:deleted" {
			assert.JSONEq(t, edited, got[0], "an edited schema is kept")
		} else {
			assert.JSONEq(t, string(seeded[c.new]), got[0], c.new)
		}
	}
	for _, code := range []string{"platform:admin:function-setting:set", "platform:admin:function-setting:deleted", "platform:admin:function:promoted"} {
		assert.Equal(t, ids[code], after[code].ID, "%s is kept", code)
	}
	wantBindings := []string{
		"sub_f074_0000001||platform:admin:function-setting:set",
		"sub_f074_0000001||platform:function:alias:changed",
		"sub_f074_0000001||platform:function:config:updated",
		"sub_f074_0000001||platform:function:secret:set",
		"sub_f074_0000001|" + ids["platform:admin:function:created"] + "|platform:function:function:created",
		"sub_f074_0000002||platform:function:config:updated",
		"sub_f074_0000002||platform:function:secret:deleted",
		"sub_f074_0000002||platform:function:secret:set",
		"sub_f074_0000002|" + ids["platform:admin:function-setting:deleted"] + "|platform:admin:function-setting:deleted",
		"sub_f074_0000002|" + ids["platform:admin:function-setting:set"] + "|platform:admin:function-setting:set",
		"sub_f074_0000003||platform:admin:function:*",
		"sub_f074_0000003|" + ids["platform:admin:function:promoted"] + "|platform:admin:function:promoted",
		"sub_f074_0000004||platform:admin:function-setting:set",
		"sub_f074_0000004||platform:function:config:updated",
		"sub_f074_0000004||platform:function:secret:set",
	}
	sort.Strings(wantBindings)
	assert.Equal(t, wantBindings, fnBindings(t, pool, "sub_f074_"))

	// The seed adds the codes no row holds yet, but no second row for a
	// renamed one and no second schema.
	require.NoError(t, NewSeeder(pool).seedPlatformEventTypes(ctx))
	reseeded := fnTypes(t, pool)
	for _, c := range reshapedFunctionCodes {
		assert.Equal(t, ids[c.old], reseeded[c.new].ID)
		assert.Len(t, schemaOfRow(t, pool, ids[c.old]), 1)
	}
	assert.Contains(t, reseeded, "platform:function:secret:set")
	assert.NotContains(t, reseeded, "platform:admin:function:created")

	// Running the migration again changes nothing.
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Equal(t, reseeded, fnTypes(t, pool))
	assert.Equal(t, wantBindings, fnBindings(t, pool, "sub_f074_"))

	// Down renames back with the bindings and the seeded schemas; the
	// setting bindings Up added stay.
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	reverted := fnTypes(t, pool)
	for _, c := range reshapedFunctionCodes {
		assert.NotContains(t, reverted, c.new)
		parts := strings.Split(c.old, ":")
		assert.Equal(t, fnRow{ids[c.old], parts[1], parts[2]}, reverted[c.old])
		got := schemaOfRow(t, pool, ids[c.old])
		require.Len(t, got, 1)
		if c.old == "platform:admin:function-alias:deleted" {
			assert.JSONEq(t, edited, got[0])
		} else {
			assert.JSONEq(t, string(c.oldSchema), got[0], c.old)
		}
	}
	revertedBindings := fnBindings(t, pool, "sub_f074_")
	for _, b := range before {
		assert.Contains(t, revertedBindings, b)
	}
	assert.Len(t, revertedBindings, len(wantBindings))
}

// When a new code is already an event type (this build's seeder or a
// Java/Rust seeder put it there, with its own schema), migration 074 moves the
// old row's bindings to it, drops a binding that would duplicate one, and
// deletes the old row with its schema versions, leaving the new row's schema.
func TestMigration074_MergesIntoAnExistingNewRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_fn_event_reshape_clash")
	up, _ := migration074(t)

	javaCreated := obj(reqStr("functionId"), reqStr("address"), reqStr("applicationId"), optStr("clientId"), reqStr("runtime"))
	insertEventType(t, pool, "etc_f074clash001", "platform:admin:function:created", reshapedFunctionCodes[0].oldSchema)
	insertEventType(t, pool, "etc_f074clash002", "platform:function:function:created", javaCreated)
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (id, subscription_id, event_type_id, event_type_code) VALUES
		 (907411, 'sub_f074clash001', 'etc_f074clash001', 'platform:admin:function:created'),
		 (907412, 'sub_f074clash002', NULL, 'platform:admin:function:created'),
		 (907413, 'sub_f074clash003', 'etc_f074clash001', 'platform:admin:function:created'),
		 (907414, 'sub_f074clash003', 'etc_f074clash002', 'platform:function:function:created')`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	types := fnTypes(t, pool)
	assert.NotContains(t, types, "platform:admin:function:created")
	assert.Equal(t, "etc_f074clash002", types["platform:function:function:created"].ID)
	assert.Empty(t, schemaOfRow(t, pool, "etc_f074clash001"), "the old row's schema is gone")
	got := schemaOfRow(t, pool, "etc_f074clash002")
	require.Len(t, got, 1)
	assert.JSONEq(t, string(javaCreated), got[0], "the existing row keeps its own schema")
	assert.Equal(t, []string{
		"sub_f074clash001|etc_f074clash002|platform:function:function:created",
		"sub_f074clash002|etc_f074clash002|platform:function:function:created",
		"sub_f074clash003|etc_f074clash002|platform:function:function:created",
	}, fnBindings(t, pool, "sub_f074clash"))

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Len(t, fnBindings(t, pool, "sub_f074clash"), 3)
}

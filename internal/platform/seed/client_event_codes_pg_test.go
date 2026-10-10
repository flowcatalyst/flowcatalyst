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

// A database an older seed populated holds the client events as
// platform:iam:client:*, which nothing emits. Migration 070 renames them to the
// emitted platform:admin:client:* codes (subscription bindings follow, a
// schema still as seeded is replaced by one describing the emitted data), the
// seed then adds the rest, and running either again changes nothing.
func TestMigration070_RenamesTheSeededClientEventTypes(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_client_codes")

	raw, err := os.ReadFile("../../migrate/sql/070_client_event_type_codes.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")

	// What an older seed wrote: the six iam codes, each with its 1.0 schema.
	oldSchemas := map[string]string{
		"created":    `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"description":{"type":["string","null"]},"identifier":{"type":"string"},"name":{"type":"string"}},"required":["clientId","name","identifier"],"type":"object"}`,
		"updated":    `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"description":{"type":["string","null"]},"name":{"type":["string","null"]}},"required":["clientId"],"type":"object"}`,
		"activated":  `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"previousStatus":{"type":"string"}},"required":["clientId","previousStatus"],"type":"object"}`,
		"suspended":  `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"reason":{"type":"string"}},"required":["clientId","reason"],"type":"object"}`,
		"deleted":    `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"identifier":{"type":"string"},"name":{"type":"string"}},"required":["clientId","name","identifier"],"type":"object"}`,
		"note-added": `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"author":{"type":"string"},"category":{"type":"string"},"clientId":{"type":"string"},"text":{"type":"string"}},"required":["clientId","category","text","author"],"type":"object"}`,
	}
	i := 0
	for ev, schema := range oldSchemas {
		i++
		id := "etc_c070_" + string(rune('a'+i)) + "0000000"
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Client ' || $3, 'CURRENT', 'UI', 'platform', 'iam', 'client')`,
			id, "platform:iam:client:"+ev, ev)
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
			 VALUES ($1, $2, '1.0', 'application/schema+json', $3::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
			"sch_c070_"+string(rune('a'+i))+"0000000", id, schema)
		require.NoError(t, err)
	}
	// A subscription made to "Client Created" from the catalogue.
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_id, event_type_code, spec_version)
		 SELECT 'sub_c070_0000001', id, code, '1.0' FROM msg_event_types WHERE code = 'platform:iam:client:created'`)
	require.NoError(t, err)
	// Someone edited the "updated" schema: an edited schema is kept.
	edited := `{"type":"object","properties":{"clientId":{"type":"string"}},"required":["clientId"]}`
	_, err = pool.Exec(ctx,
		`UPDATE msg_event_type_spec_versions sv SET schema_content = $1::jsonb
		   FROM msg_event_types et WHERE et.id = sv.event_type_id AND et.code = 'platform:iam:client:updated'`, edited)
	require.NoError(t, err)

	codes := func() map[string]string {
		rows, err := pool.Query(ctx,
			`SELECT code, id FROM msg_event_types WHERE code LIKE 'platform:%:client:%'`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[string]string{}
		for rows.Next() {
			var code, id string
			require.NoError(t, rows.Scan(&code, &id))
			out[code] = id
		}
		require.NoError(t, rows.Err())
		return out
	}
	schemaOf := func(code string) string {
		var s string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT sv.schema_content::text FROM msg_event_type_spec_versions sv
			   JOIN msg_event_types et ON et.id = sv.event_type_id
			  WHERE et.code = $1 AND sv.version = '1.0'`, code).Scan(&s))
		return s
	}
	before := codes()

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	after := codes()
	for ev := range oldSchemas {
		assert.NotContains(t, after, "platform:iam:client:"+ev)
		assert.Equal(t, before["platform:iam:client:"+ev], after["platform:admin:client:"+ev],
			"%s keeps its id", ev)
	}
	var subdomain string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT subdomain FROM msg_event_types WHERE code = 'platform:admin:client:created'`).Scan(&subdomain))
	assert.Equal(t, "admin", subdomain)
	var bound string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type_code FROM msg_subscription_event_types WHERE subscription_id = 'sub_c070_0000001'`).Scan(&bound))
	assert.Equal(t, "platform:admin:client:created", bound)

	// The seeded schemas now describe the emitted data; an edited one is kept.
	schemas := platformEventSchemas()
	for _, ev := range []string{"created", "activated", "suspended", "deleted", "note-added"} {
		assert.JSONEq(t, string(schemas["platform:admin:client:"+ev]), schemaOf("platform:admin:client:"+ev), ev)
	}
	assert.JSONEq(t, edited, schemaOf("platform:admin:client:updated"))

	// The seed then adds what is missing, once, and attaches no second schema.
	s := NewSeeder(pool)
	require.NoError(t, s.seedPlatformEventTypes(ctx))
	seeded := codes()
	assert.Contains(t, seeded, "platform:admin:client:deactivated")
	assert.Contains(t, seeded, "platform:iam:client:applications-updated")
	assert.Len(t, seeded, 8, "seven admin client types and applications-updated, nothing twice: %v", seeded)
	var versions int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_type_spec_versions sv JOIN msg_event_types et ON et.id = sv.event_type_id
		  WHERE et.code LIKE 'platform:admin:client:%'`).Scan(&versions))
	assert.Equal(t, 7, versions)

	// Running the migration again changes nothing.
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Equal(t, seeded, codes())
	assert.JSONEq(t, edited, schemaOf("platform:admin:client:updated"))

	// Down renames the six back; the binding follows.
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	reverted := codes()
	for ev := range oldSchemas {
		assert.Equal(t, before["platform:iam:client:"+ev], reverted["platform:iam:client:"+ev], ev)
	}
	assert.Contains(t, reverted, "platform:admin:client:deactivated")
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type_code FROM msg_subscription_event_types WHERE subscription_id = 'sub_c070_0000001'`).Scan(&bound))
	assert.Equal(t, "platform:iam:client:created", bound)
}

// When the emitted code is already an event type, migration 070 leaves both
// rows (and the old one's bindings) as they are rather than fail on the
// unique code.
func TestMigration070_LeavesAClashAlone(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_client_clash")
	raw, err := os.ReadFile("../../migrate/sql/070_client_event_type_codes.sql")
	require.NoError(t, err)
	up, _, _ := strings.Cut(string(raw), "-- +goose Down")

	for _, row := range [][2]string{
		{"etc_c070clash001", "platform:iam:client:created"},
		{"etc_c070clash002", "platform:admin:client:created"},
	} {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Client Created', 'CURRENT', 'UI', 'platform', $3, 'client')`,
			row[0], row[1], strings.Split(row[1], ":")[1])
		require.NoError(t, err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (subscription_id, event_type_id, event_type_code)
		 VALUES ('sub_c070clash001', 'etc_c070clash001', 'platform:iam:client:created')`)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	var oldCode, bound string
	require.NoError(t, pool.QueryRow(ctx, `SELECT code FROM msg_event_types WHERE id = 'etc_c070clash001'`).Scan(&oldCode))
	assert.Equal(t, "platform:iam:client:created", oldCode)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT event_type_code FROM msg_subscription_event_types WHERE subscription_id = 'sub_c070clash001'`).Scan(&bound))
	assert.Equal(t, "platform:iam:client:created", bound)
}

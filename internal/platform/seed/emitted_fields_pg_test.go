//go:build integration

package seed

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 126 replaces the seeded 1.0 schemas of
// platform:iam:application:updated and platform:ingestion:ingestion:config-set
// with the seed's (which admit type, previousType and secretsSet) only while
// each is still what a seed wrote, changes nothing when run again, leaves a
// changed schema and an older seed's form alone, and its Down puts the old
// schemas back.
func TestMigration126_EmittedFieldsInTheSchemas(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_emitted_fields")
	raw, err := os.ReadFile("../../migrate/sql/126_event_schemas_application_type_config_secrets.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok)

	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	// What every seed since 072 wrote for each.
	old := map[string]string{
		"platform:iam:application:updated":        `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"name":{"type":"string"}},"required":["applicationId","name"],"type":"object"}`,
		"platform:ingestion:ingestion:config-set": `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"code":{"type":"string"},"ingestionId":{"type":"string"},"ingestions":{"items":{"type":"string"},"type":"array"},"key":{"type":"string"},"settings":{"items":{"type":"string"},"type":"array"}},"required":["ingestionId","code","key","settings","ingestions"],"type":"object"}`,
	}
	ids := map[string]string{}
	i := 0
	for code, schema := range old {
		i++
		parts := strings.Split(code, ":")
		id := fmt.Sprintf("etc_f126_%07d", i)
		ids[code] = fmt.Sprintf("sch_f126_%07d", i)
		exec(`DELETE FROM msg_event_type_spec_versions sv USING msg_event_types et WHERE et.id = sv.event_type_id AND et.code = $1`, code)
		exec(`DELETE FROM msg_event_types WHERE code = $1`, code)
		exec(`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
		      VALUES ($1, $2, 'Seeded', 'CURRENT', 'UI', 'platform', $3, $4)`, id, code, parts[1], parts[2])
		exec(`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
		      VALUES ($1, $2, '1.0', 'application/schema+json', $3::jsonb, 'JSON_SCHEMA', 'CURRENT')`, ids[code], id, schema)
	}
	schema := func(code string) string {
		t.Helper()
		var s string
		require.NoError(t, pool.QueryRow(ctx, `SELECT schema_content::text FROM msg_event_type_spec_versions WHERE id = $1`, ids[code]).Scan(&s))
		return s
	}

	for range 2 {
		exec(up)
		for code := range old {
			assert.JSONEq(t, string(platformEventSchemas()[code]), schema(code), code)
		}
	}
	exec(down)
	for code, s := range old {
		assert.JSONEq(t, s, schema(code), code)
	}

	// A schema someone changed is theirs; the form a seed wrote before 072 is
	// 072's to replace, not this migration's.
	const theirs = `{"type":"object"}`
	const before072 = `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"description":{"type":["string","null"]},"name":{"type":["string","null"]}},"required":["applicationId"],"type":"object"}`
	exec(`UPDATE msg_event_type_spec_versions SET schema_content = $1::jsonb WHERE id = $2`, theirs, ids["platform:ingestion:ingestion:config-set"])
	exec(`UPDATE msg_event_type_spec_versions SET schema_content = $1::jsonb WHERE id = $2`, before072, ids["platform:iam:application:updated"])
	exec(up)
	assert.JSONEq(t, theirs, schema("platform:ingestion:ingestion:config-set"))
	assert.JSONEq(t, before072, schema("platform:iam:application:updated"))
}

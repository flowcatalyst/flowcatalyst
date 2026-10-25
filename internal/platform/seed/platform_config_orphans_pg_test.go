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

// Migration 091 removes the platform configuration of applications that no
// longer exist, keeps the platform's own and the ambiguous rows, replaces the
// seeded application:deleted schema with the seed's (which carries the
// deletion's platform-config counts), changes nothing when run again, and its
// Down puts the old schema back.
func TestMigration091_PlatformConfigOrphans(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_pc_orphans")
	raw, err := os.ReadFile("../../migrate/sql/091_platform_config_orphans.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok)

	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO app_applications (id, code, name) VALUES ('app_pc091_live01', 'live', 'Live')`)
	for id, code := range map[string]string{
		"cfg_pc091_live01": "live",     // its application exists
		"cfg_pc091_gone01": "gone",     // orphan
		"cfg_pc091_plat01": "platform", // the platform's own
		"cfg_pc091_case01": "LIVE ",    // ambiguous: case and space
	} {
		exec(`INSERT INTO app_platform_configs (id, application_code, section, property, scope, value_type, value, created_at, updated_at)
		      VALUES ($1, $2, 's', 'p', 'GLOBAL', 'PLAIN', 'v', now(), now())`, id, code)
	}
	exec(`INSERT INTO app_platform_config_access (id, application_code, role_code, can_read, can_write, created_at)
	      VALUES ('pca_pc091_gone01', 'gone', 'r', true, false, now()), ('pca_pc091_live01', 'live', 'r', true, false, now())`)

	const oldSchema = `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"applicationId":{"type":"string"},"code":{"type":"string"}},"required":["applicationId","code"],"type":"object"}`
	exec(`DELETE FROM msg_event_type_spec_versions sv USING msg_event_types et
	       WHERE et.id = sv.event_type_id AND et.code = 'platform:iam:application:deleted'`)
	exec(`DELETE FROM msg_event_types WHERE code = 'platform:iam:application:deleted'`)
	exec(`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
	      VALUES ('etc_pc091_000001', 'platform:iam:application:deleted', 'Seeded', 'CURRENT', 'UI', 'platform', 'iam', 'application')`)
	exec(`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
	      VALUES ('sch_pc091_000001', 'etc_pc091_000001', '1.0', 'application/schema+json', $1::jsonb, 'JSON_SCHEMA', 'CURRENT')`, oldSchema)

	ids := func(table string) []string {
		rows, err := pool.Query(ctx, `SELECT id FROM `+table+` WHERE id LIKE '%pc091%' ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var id string
			require.NoError(t, rows.Scan(&id))
			out = append(out, id)
		}
		return out
	}
	schema := func() string {
		var s string
		require.NoError(t, pool.QueryRow(ctx, `SELECT schema_content::text FROM msg_event_type_spec_versions WHERE id = 'sch_pc091_000001'`).Scan(&s))
		return s
	}

	for range 2 {
		exec(up)
		assert.Equal(t, []string{"cfg_pc091_case01", "cfg_pc091_live01", "cfg_pc091_plat01"}, ids("app_platform_configs"))
		assert.Equal(t, []string{"pca_pc091_live01"}, ids("app_platform_config_access"))
		assert.JSONEq(t, string(platformEventSchemas()["platform:iam:application:deleted"]), schema())
	}
	exec(down)
	assert.JSONEq(t, oldSchema, schema())
}

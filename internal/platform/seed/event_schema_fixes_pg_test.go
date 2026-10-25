//go:build integration

package seed

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A database an older seed populated holds 1.0 schemas that do not describe
// the emitted data. Migration 072 replaces each with the seed's corrected
// schema while it is still exactly what a seed wrote, leaves an edited schema
// alone, changes nothing when run again, and Down puts the seeded ones back.
func TestMigration072_ReplacesTheSeededSchemas(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_schema_fixes")

	raw, err := os.ReadFile("../../migrate/sql/072_platform_event_schema_fixes.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")

	// The migration's (code, seeded, corrected) rows; each corrected schema is
	// the seed's.
	row := regexp.MustCompile(`\('(platform:[a-z:-]+)',\s*'([^']+)',\s*'([^']+)'\)`)
	type fix struct{ old, new string }
	fixes := map[string]fix{}
	for _, m := range row.FindAllStringSubmatch(up, -1) {
		fixes[m[1]] = fix{m[2], m[3]}
	}
	require.Len(t, fixes, 28)
	schemas := platformEventSchemas()
	// Codes whose seeded schema a later migration amended again (091: the
	// application deletion's platform-config counts); their seed is that
	// migration's, not 072's.
	amendedLater := map[string]bool{"platform:iam:application:deleted": true}
	for code, f := range fixes {
		require.Contains(t, schemas, code)
		if amendedLater[code] {
			continue
		}
		assert.JSONEq(t, string(schemas[code]), f.new, code)
	}
	// Two seeded schemas spelled out, as every seed before this one wrote them.
	assert.JSONEq(t, `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":["string","null"]},"code":{"type":"string"},"dispatchPoolId":{"type":"string"},"name":{"type":"string"}},"required":["dispatchPoolId","code","name"],"type":"object"}`,
		fixes["platform:admin:dispatch-pool:created"].old)
	assert.JSONEq(t, `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"clientId":{"type":"string"},"disabledRemoved":{"items":{"type":"string"},"type":"array"},"enabledAdded":{"items":{"type":"string"},"type":"array"},"enabledApplicationIds":{"items":{"type":"string"},"type":"array"}},"required":["clientId","enabledApplicationIds","enabledAdded","disabledRemoved"],"type":"object"}`,
		fixes["platform:iam:client:applications-updated"].old)

	// What an older seed wrote: every one of the 28 with its seeded 1.0.
	i := 0
	for code, f := range fixes {
		i++
		parts := strings.Split(code, ":")
		id := fmt.Sprintf("etc_f072_%07d", i)
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Seeded', 'CURRENT', 'UI', 'platform', $3, $4)`,
			id, code, parts[1], parts[2])
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
			 VALUES ($1, $2, '1.0', 'application/schema+json', $3::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
			fmt.Sprintf("sch_f072_%07d", i), id, f.old)
		require.NoError(t, err)
	}
	// Someone edited one; an edited schema is theirs.
	edited := `{"type":"object","properties":{"poolId":{"type":"string"}}}`
	_, err = pool.Exec(ctx,
		`UPDATE msg_event_type_spec_versions sv SET schema_content = $1::jsonb
		   FROM msg_event_types et WHERE et.id = sv.event_type_id AND et.code = 'platform:admin:dispatch-pool:deleted'`, edited)
	require.NoError(t, err)

	schemaOf := func(code string) string {
		var s string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT sv.schema_content::text FROM msg_event_type_spec_versions sv
			   JOIN msg_event_types et ON et.id = sv.event_type_id
			  WHERE et.code = $1 AND sv.version = '1.0'`, code).Scan(&s))
		return s
	}

	for range 2 { // the second run changes nothing
		_, err = pool.Exec(ctx, up)
		require.NoError(t, err)
		for code, f := range fixes {
			if code == "platform:admin:dispatch-pool:deleted" {
				assert.JSONEq(t, edited, schemaOf(code))
				continue
			}
			assert.JSONEq(t, f.new, schemaOf(code), code)
		}
	}

	// The seed attaches no second schema to them.
	require.NoError(t, NewSeeder(pool).seedPlatformEventTypes(ctx))
	var versions int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE id LIKE 'sch_f072_%'`).Scan(&versions))
	assert.Equal(t, 28, versions)
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_event_type_spec_versions sv JOIN msg_event_types et ON et.id = sv.event_type_id
		  WHERE et.code = 'platform:admin:dispatch-pool:created'`).Scan(&versions))
	assert.Equal(t, 1, versions)

	// Down puts the seeded schemas back; the edited one stays.
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	for code, f := range fixes {
		if code == "platform:admin:dispatch-pool:deleted" {
			assert.JSONEq(t, edited, schemaOf(code))
			continue
		}
		assert.JSONEq(t, f.old, schemaOf(code), code)
	}
}

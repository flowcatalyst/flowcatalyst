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

// Migration 123 replaces the seeded 1.0 schema of
// platform:messaging:dispatch-job:payload-edited with the seed's (which
// carries the optional payload ids) only while it is still what a seed wrote,
// changes nothing when run again, leaves a changed schema alone, and its Down
// puts the old schema back.
func TestMigration123_PayloadEditedPayloadIDs(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_payload_edited_ids")
	raw, err := os.ReadFile("../../migrate/sql/123_payload_edited_payload_ids.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok)

	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	const code = "platform:messaging:dispatch-job:payload-edited"
	const oldSchema = `{"$schema":"http://json-schema.org/draft-07/schema#","additionalProperties":false,"properties":{"contentType":{"type":"string"},"dispatchJobId":{"type":"string"},"previousContentType":{"type":"string"},"previousSha256":{"type":"string"},"previousSize":{"minimum":0,"type":"integer"},"sha256":{"type":"string"},"size":{"minimum":0,"type":"integer"},"status":{"type":"string"}},"required":["dispatchJobId","status","previousSize","previousSha256","size","sha256","contentType","previousContentType"],"type":"object"}`
	exec(`DELETE FROM msg_event_type_spec_versions sv USING msg_event_types et
	       WHERE et.id = sv.event_type_id AND et.code = $1`, code)
	exec(`DELETE FROM msg_event_types WHERE code = $1`, code)
	exec(`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
	      VALUES ('etc_pe123_000001', $1, 'Seeded', 'CURRENT', 'UI', 'platform', 'messaging', 'dispatch-job')`, code)
	exec(`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
	      VALUES ('sch_pe123_000001', 'etc_pe123_000001', '1.0', 'application/schema+json', $1::jsonb, 'JSON_SCHEMA', 'CURRENT')`, oldSchema)

	schema := func() string {
		var s string
		require.NoError(t, pool.QueryRow(ctx, `SELECT schema_content::text FROM msg_event_type_spec_versions WHERE id = 'sch_pe123_000001'`).Scan(&s))
		return s
	}

	for range 2 {
		exec(up)
		assert.JSONEq(t, string(platformEventSchemas()[code]), schema())
	}
	exec(down)
	assert.JSONEq(t, oldSchema, schema())

	// A schema someone changed is theirs.
	const theirs = `{"type":"object"}`
	exec(`UPDATE msg_event_type_spec_versions SET schema_content = $1::jsonb WHERE id = 'sch_pe123_000001'`, theirs)
	exec(up)
	assert.JSONEq(t, theirs, schema())
}

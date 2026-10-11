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

// A database an older seed populated holds 16 event types nothing emits.
// Migration 071 removes them and their schema versions, moves subscription
// bindings that name them into the archive (a wildcard binding and bindings to
// emitted types stay), leaves emitted types alone, and the seed does not add
// them back. Running it again changes nothing; Down restores the bindings.
func TestMigration071_PrunesTheUnemittedEventTypes(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "seed_prune_unemitted")

	raw, err := os.ReadFile("../../migrate/sql/071_prune_unemitted_event_types.sql")
	require.NoError(t, err)
	up, down, ok := strings.Cut(string(raw), "-- +goose Down")
	require.True(t, ok, "migration has a Down section")

	// What an older seed wrote: two unemitted types and one emitted, each with
	// its 1.0 schema.
	for _, row := range [][3]string{
		{"etc_p071_0000001", "platform:iam:role:created", "iam"},
		{"etc_p071_0000002", "platform:admin:idp:updated", "admin"},
		{"etc_p071_0000003", "platform:admin:role:created", "admin"},
	} {
		parts := strings.Split(row[1], ":")
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, source, application, subdomain, aggregate)
			 VALUES ($1, $2, 'Seeded', 'CURRENT', 'UI', 'platform', $3, $4)`,
			row[0], row[1], row[2], parts[2])
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_content, schema_type, status)
			 VALUES ($1, $2, '1.0', 'application/schema+json', '{"type":"object"}'::jsonb, 'JSON_SCHEMA', 'CURRENT')`,
			strings.Replace(row[0], "etc_", "sch_", 1), row[0])
		require.NoError(t, err)
	}
	// Subscription bindings: one by row and code to an unemitted type next to
	// one to an emitted type; one by code only; one wildcard.
	_, err = pool.Exec(ctx,
		`INSERT INTO msg_subscription_event_types (id, subscription_id, event_type_id, event_type_code, spec_version) VALUES
		 (907101, 'sub_p071_0000001', 'etc_p071_0000001', 'platform:iam:role:created', '1.0'),
		 (907102, 'sub_p071_0000001', 'etc_p071_0000003', 'platform:admin:role:created', '1.0'),
		 (907103, 'sub_p071_0000002', NULL, 'platform:admin:idp:updated', NULL),
		 (907104, 'sub_p071_0000003', NULL, 'platform:iam:role:*', NULL)`)
	require.NoError(t, err)

	countOf := func(query string, args ...any) int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, query, args...).Scan(&n))
		return n
	}
	bindings := func() map[int]string {
		rows, err := pool.Query(ctx, `SELECT id, event_type_code FROM msg_subscription_event_types WHERE subscription_id LIKE 'sub_p071_%'`)
		require.NoError(t, err)
		defer rows.Close()
		out := map[int]string{}
		for rows.Next() {
			var id int
			var code string
			require.NoError(t, rows.Scan(&id, &code))
			out[id] = code
		}
		require.NoError(t, rows.Err())
		return out
	}

	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)

	assert.Equal(t, 0, countOf(`SELECT COUNT(*) FROM msg_event_types WHERE id IN ('etc_p071_0000001', 'etc_p071_0000002')`))
	assert.Equal(t, 0, countOf(`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE event_type_id IN ('etc_p071_0000001', 'etc_p071_0000002')`))
	assert.Equal(t, 1, countOf(`SELECT COUNT(*) FROM msg_event_types WHERE id = 'etc_p071_0000003'`), "an emitted type stays")
	assert.Equal(t, 1, countOf(`SELECT COUNT(*) FROM msg_event_type_spec_versions WHERE event_type_id = 'etc_p071_0000003'`))
	assert.Equal(t, map[int]string{907102: "platform:admin:role:created", 907104: "platform:iam:role:*"}, bindings())

	type archived struct {
		ID             int
		SubscriptionID string
		EventTypeID    *string
		Code           string
		SpecVersion    *string
		Reason         string
	}
	readArchive := func() []archived {
		rows, err := pool.Query(ctx,
			`SELECT id, subscription_id, event_type_id, event_type_code, spec_version, archived_reason
			   FROM msg_subscription_event_types_archive WHERE subscription_id LIKE 'sub_p071_%' ORDER BY id`)
		require.NoError(t, err)
		defer rows.Close()
		var out []archived
		for rows.Next() {
			var a archived
			require.NoError(t, rows.Scan(&a.ID, &a.SubscriptionID, &a.EventTypeID, &a.Code, &a.SpecVersion, &a.Reason))
			out = append(out, a)
		}
		require.NoError(t, rows.Err())
		return out
	}
	etID, spec := "etc_p071_0000001", "1.0"
	want := []archived{
		{907101, "sub_p071_0000001", &etID, "platform:iam:role:created", &spec, "071_event_type_not_emitted"},
		{907103, "sub_p071_0000002", nil, "platform:admin:idp:updated", nil, "071_event_type_not_emitted"},
	}
	assert.Equal(t, want, readArchive())

	// The seed does not add them back, and running the migration again
	// changes nothing.
	require.NoError(t, NewSeeder(pool).seedPlatformEventTypes(ctx))
	for _, code := range []string{"platform:iam:role:created", "platform:admin:idp:updated", "platform:iam:user:password-reset-requested"} {
		assert.Equal(t, 0, countOf(`SELECT COUNT(*) FROM msg_event_types WHERE code = $1`, code), code)
	}
	_, err = pool.Exec(ctx, up)
	require.NoError(t, err)
	assert.Equal(t, want, readArchive())
	assert.Equal(t, map[int]string{907102: "platform:admin:role:created", 907104: "platform:iam:role:*"}, bindings())

	// Down puts the bindings back under their ids and drops the archive.
	_, err = pool.Exec(ctx, down)
	require.NoError(t, err)
	assert.Equal(t, map[int]string{
		907101: "platform:iam:role:created",
		907102: "platform:admin:role:created",
		907103: "platform:admin:idp:updated",
		907104: "platform:iam:role:*",
	}, bindings())
	assert.Equal(t, 0, countOf(`SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'msg_subscription_event_types_archive'`))
}

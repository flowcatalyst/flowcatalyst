//go:build integration

package eventtype_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindByID_CorruptStatusFailsLoudly is the X-06 read boundary: a row
// whose status column holds a value that isn't one of the known Status
// constants (junk written before write-boundary validation existed, or a
// hand-edited row) must fail the read with a distinct error, never
// round-trip as that literal string and never silently coerce to CURRENT.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := eventtype.NewRepository(pool)

	const id = "evt_corrupt_test1"
	testpg.WithConstraintDropped(t, pool, "msg_event_types", "chk_msg_event_types_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, application, subdomain, aggregate)
		 VALUES ($1, 'corrupt:code:one:test', 'Corrupt', 'NOT_A_REAL_STATUS', 'corrupt', 'sub', 'agg')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_event_types WHERE id = $1`, id)
		})
	})

	et, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it")
	assert.Nil(t, et)
	assert.Contains(t, err.Error(), "CORRUPT_EVENT_TYPE_STATUS")
}

// TestFindByID_CorruptSpecVersionSchemaTypeFailsLoudly pins the nested
// spec-version list: a bad schema_type on a spec version must fail the
// whole event-type read, not silently drop that version.
func TestFindByID_CorruptSpecVersionSchemaTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := eventtype.NewRepository(pool)

	const id = "evt_corrupt_test2"
	const svID = "esv_corrupt_test1"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_event_types (id, code, name, application, subdomain, aggregate)
		 VALUES ($1, 'corrupt:code:two:test', 'Corrupt', 'corrupt', 'sub', 'agg')`, id)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_event_type_spec_versions", "chk_msg_event_type_spec_versions_schema_type", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions (id, event_type_id, version, mime_type, schema_type)
		 VALUES ($1, $2, 'v1', 'application/json', 'NOT_A_SCHEMA_TYPE')`, svID, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_event_type_spec_versions WHERE id = $1`, svID)
		})
	})

	et, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt spec-version schema type must fail the read, not drop the version")
	assert.Nil(t, et)
	assert.Contains(t, err.Error(), "CORRUPT_SPEC_VERSION_SCHEMA_TYPE")
}

// TestFindWithFilters_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly: "a list containing the row fails too" — one bad row
// must not be silently skipped or coerced while the rest of the list is
// returned successfully.
func TestFindWithFilters_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := eventtype.NewRepository(pool)

	const goodID = "evt_cl_good012345"
	const badID = "evt_cl_bad0012345"
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_event_types (id, code, name, status, application, subdomain, aggregate)
		 VALUES ($1, 'corruptlist:good:test:one', 'Good', 'CURRENT', 'corruptlist', 'sub', 'agg')`, goodID)
	require.NoError(t, err)
	testpg.WithConstraintDropped(t, pool, "msg_event_types", "chk_msg_event_types_status", func() {
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_event_types (id, code, name, status, application, subdomain, aggregate)
		 VALUES ($1, 'corruptlist:bad:test:one', 'Bad', 'NOT_A_REAL_STATUS', 'corruptlist', 'sub', 'agg')`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_event_types WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, nil, nil, nil, nil, nil)
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_EVENT_TYPE_STATUS")
}

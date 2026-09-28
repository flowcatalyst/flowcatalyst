//go:build integration

package migrate_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestMigration059_FunctionTablesExistAndCascade proves the six new
// function-runner tables exist with the expected write-boundary CHECK
// constraints, and that deleting a function cascades to its versions,
// aliases, and settings (migration 059's FK ON DELETE CASCADE).
func TestMigration059_FunctionTablesExistAndCascade(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	const fnID = "fnc_mig059cascad1"
	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ('app_mig059cascad', 'mig059cascadeapp', 'Mig059 App')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_functions
		(id, application_id, name, address, limits)
		VALUES ($1, 'app_mig059cascad', 'cascade-fn', 'mig059cascadeapp.cascade-fn', '{}'::jsonb)`, fnID)
	require.NoError(t, err)
	t.Cleanup(func() {
		cCtx := context.Background()
		_, _ = pool.Exec(cCtx, `DELETE FROM fng_functions WHERE id = $1`, fnID)
		_, _ = pool.Exec(cCtx, `DELETE FROM app_applications WHERE id = 'app_mig059cascad'`)
	})

	_, err = pool.Exec(ctx, `INSERT INTO fng_versions
		(id, function_id, number, digest, size_bytes, abi, describe, status)
		VALUES ('fnv_mig059cascad1', $1, 1, repeat('a', 64), 10, 1, '{}'::jsonb, 'PUBLISHED')`, fnID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_aliases (function_id, name, version_id, updated_at)
		VALUES ($1, 'live', 'fnv_mig059cascad1', NOW())`, fnID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_settings (function_id, kind, key, value, updated_at)
		VALUES ($1, 'CONFIG', 'GREETING', 'hi', NOW())`, fnID)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `DELETE FROM fng_functions WHERE id = $1`, fnID)
	require.NoError(t, err, "deleting the function must succeed")

	var count int
	row := pool.QueryRow(ctx, `SELECT COUNT(*) FROM fng_versions WHERE function_id = $1`, fnID)
	require.NoError(t, row.Scan(&count))
	assert.Zero(t, count, "fng_versions must cascade-delete")
	row = pool.QueryRow(ctx, `SELECT COUNT(*) FROM fng_aliases WHERE function_id = $1`, fnID)
	require.NoError(t, row.Scan(&count))
	assert.Zero(t, count, "fng_aliases must cascade-delete")
	row = pool.QueryRow(ctx, `SELECT COUNT(*) FROM fng_settings WHERE function_id = $1`, fnID)
	require.NoError(t, row.Scan(&count))
	assert.Zero(t, count, "fng_settings must cascade-delete")
}

// TestMigration059_FngVersionsStatusCheck proves the status CHECK constraint
// rejects an unrecognised value.
func TestMigration059_FngVersionsStatusCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ('app_mig059status', 'mig059statusapp', 'Mig059 Status App')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_functions (id, application_id, name, address, limits)
		VALUES ('fnc_mig059status1', 'app_mig059status', 'status-fn', 'mig059statusapp.status-fn', '{}'::jsonb)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cCtx := context.Background()
		_, _ = pool.Exec(cCtx, `DELETE FROM fng_functions WHERE id = 'fnc_mig059status1'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM app_applications WHERE id = 'app_mig059status'`)
	})

	_, err = pool.Exec(ctx, `INSERT INTO fng_versions
		(id, function_id, number, digest, size_bytes, abi, describe, status)
		VALUES ('fnv_mig059status1', 'fnc_mig059status1', 1, repeat('b', 64), 10, 1, '{}'::jsonb, 'NOT_A_REAL_STATUS')`)
	require.Error(t, err, "an unrecognised status must be rejected at the write boundary")
}

// TestMigration059_FnSettingsKindCheck proves the kind CHECK constraint
// rejects an unrecognised value.
func TestMigration059_FnSettingsKindCheck(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ('app_mig059kind01', 'mig059kindapp', 'Mig059 Kind App')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_functions (id, application_id, name, address, limits)
		VALUES ('fnc_mig059kind001', 'app_mig059kind01', 'kind-fn', 'mig059kindapp.kind-fn', '{}'::jsonb)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cCtx := context.Background()
		_, _ = pool.Exec(cCtx, `DELETE FROM fng_functions WHERE id = 'fnc_mig059kind001'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM app_applications WHERE id = 'app_mig059kind01'`)
	})

	_, err = pool.Exec(ctx, `INSERT INTO fng_settings (function_id, kind, key, value, updated_at)
		VALUES ('fnc_mig059kind001', 'NOT_A_REAL_KIND', 'k', 'v', NOW())`)
	require.Error(t, err, "an unrecognised setting kind must be rejected at the write boundary")
}

// TestMigration059_SubscriptionSourceAcceptsFunction proves the widened
// chk_msg_subscriptions_source constraint admits 'FUNCTION' alongside the
// existing CODE/API/UI values, and that function_id round-trips.
func TestMigration059_SubscriptionSourceAcceptsFunction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ('app_mig059subfn01', 'mig059subfnapp', 'Mig059 Sub Fn App')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_functions (id, application_id, name, address, limits)
		VALUES ('fnc_mig059subfn01', 'app_mig059subfn01', 'sub-fn', 'mig059subfnapp.sub-fn', '{}'::jsonb)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cCtx := context.Background()
		_, _ = pool.Exec(cCtx, `DELETE FROM msg_subscriptions WHERE id = 'sub_mig059subfn01'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM fng_functions WHERE id = 'fnc_mig059subfn01'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM app_applications WHERE id = 'app_mig059subfn01'`)
	})

	_, err = pool.Exec(ctx, `INSERT INTO msg_subscriptions
		(id, code, name, target, source, function_id)
		VALUES ('sub_mig059subfn01', 'mig059-sub-fn', 'Mig059 Sub Fn', 'https://example.test/hook', 'FUNCTION', 'fnc_mig059subfn01')`)
	require.NoError(t, err, "source FUNCTION must be accepted")

	rows, err := pool.Query(ctx, `SELECT function_id FROM msg_subscriptions WHERE id = 'sub_mig059subfn01'`)
	require.NoError(t, err)
	fid, err := pgx.CollectOneRow(rows, pgx.RowTo[*string])
	require.NoError(t, err)
	require.NotNil(t, fid)
	assert.Equal(t, "fnc_mig059subfn01", *fid)

	_, err = pool.Exec(ctx, `INSERT INTO msg_subscriptions (id, code, name, target, source)
		VALUES ('sub_mig059bad0001', 'mig059-bad', 'Mig059 Bad', 'https://example.test/hook', 'NOT_A_REAL_SOURCE')`)
	require.Error(t, err, "an unrecognised source must still be rejected")
	if err != nil {
		_, _ = pool.Exec(ctx, `DELETE FROM msg_subscriptions WHERE id = 'sub_mig059bad0001'`)
	}
}

// TestMigration059_ScheduledJobFunctionID proves msg_scheduled_jobs gained
// a nullable function_id column that round-trips.
func TestMigration059_ScheduledJobFunctionID(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)

	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name)
		VALUES ('app_mig059sjfn01', 'mig059sjfnapp', 'Mig059 SJ Fn App')
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO fng_functions (id, application_id, name, address, limits)
		VALUES ('fnc_mig059sjfn001', 'app_mig059sjfn01', 'sj-fn', 'mig059sjfnapp.sj-fn', '{}'::jsonb)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		cCtx := context.Background()
		_, _ = pool.Exec(cCtx, `DELETE FROM msg_scheduled_jobs WHERE id = 'sjb_mig059sjfn01'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM fng_functions WHERE id = 'fnc_mig059sjfn001'`)
		_, _ = pool.Exec(cCtx, `DELETE FROM app_applications WHERE id = 'app_mig059sjfn01'`)
	})

	_, err = pool.Exec(ctx, `INSERT INTO msg_scheduled_jobs (id, code, name, crons, function_id)
		VALUES ('sjb_mig059sjfn01', 'mig059-sj-fn', 'Mig059 SJ Fn', ARRAY['*/5 * * * *'], 'fnc_mig059sjfn001')`)
	require.NoError(t, err)

	rows, err := pool.Query(ctx, `SELECT function_id FROM msg_scheduled_jobs WHERE id = 'sjb_mig059sjfn01'`)
	require.NoError(t, err)
	fid, err := pgx.CollectOneRow(rows, pgx.RowTo[*string])
	require.NoError(t, err)
	require.NotNil(t, fid)
	assert.Equal(t, "fnc_mig059sjfn001", *fid)
}

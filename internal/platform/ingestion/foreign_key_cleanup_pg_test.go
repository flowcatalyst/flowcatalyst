//go:build integration

package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// runtimeRows writes a lease, a route state, a secret fetch record and a state
// value for the ingestion, as a runtime would.
func (e *env) runtimeRows(t *testing.T, ingestionID, runtime string, now time.Time) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at) VALUES ($1, $2, $3, $3::timestamptz + interval '1 minute')`,
		`INSERT INTO ing_route_states (ingestion_id, runtime_id, version, status, updated_at) VALUES ($1, $2, 1, 'STARTED', $3)`,
		`INSERT INTO ing_secret_fetches (ingestion_id, secret_name, runtime_id, version, last_fetched_at) VALUES ($1, 'sftp-password', $2, 1, $3)`,
		`INSERT INTO ing_state (ingestion_id, key, value, updated_at) VALUES ($1, 'watermark', $2, $3)`,
	} {
		_, err := e.pool.Exec(context.Background(), q, ingestionID, runtime, now)
		require.NoError(t, err)
	}
}

// The lease, route states and secret fetch records carry no foreign key
// (migration 114, docs/foreign-keys.md), so deleting an ingestion removes them
// itself; another ingestion's rows are untouched.
func TestRepository_DeleteRemovesTheRuntimeRows(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	now := time.Now().UTC()
	rt := "rt-fkdel-" + unique()
	gone := e.live(t, "fk-gone", e.sftp(t))
	kept := e.live(t, "fk-kept", e.sftp(t))
	e.runtimeRows(t, gone.ID, rt, now)
	e.runtimeRows(t, kept.ID, rt, now)

	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Delete(ctx, gone, tx) }))

	for _, table := range []string{"ing_leases", "ing_route_states", "ing_secret_fetches", "ing_state"} {
		assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM `+table+` WHERE ingestion_id = $1`, gone.ID), "%s of the deleted ingestion", table)
		assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM `+table+` WHERE ingestion_id = $1`, kept.ID), "%s of the other ingestion", table)
	}
}

// A runtime report that races an ingestion's delete can write a route state or
// secret fetch record after the delete removed the rest; with no foreign key
// to refuse it, the retention sweep removes it, even while the runtime is live.
func TestPrune_RemovesRowsOfIngestionsThatNoLongerExist(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	now := time.Now().UTC()
	rt := "rt-fkorphan-" + unique()
	_, err := e.pool.Exec(ctx, `INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, $2, $2)`, rt, now)
	require.NoError(t, err)
	kept := e.live(t, "fk-orphan-kept", e.sftp(t))
	missing := "ing_" + unique()
	for _, id := range []string{kept.ID, missing} {
		_, err := e.pool.Exec(ctx, `INSERT INTO ing_route_states (ingestion_id, runtime_id, version, status, updated_at) VALUES ($1, $2, 1, 'FAILING', $3)`, id, rt, now)
		require.NoError(t, err)
		_, err = e.pool.Exec(ctx, `INSERT INTO ing_secret_fetches (ingestion_id, secret_name, runtime_id, version, last_fetched_at) VALUES ($1, 'sftp-password', $2, 1, $3)`, id, rt, now)
		require.NoError(t, err)
	}

	_, err = e.repo.Prune(ctx, now)
	require.NoError(t, err)

	for _, table := range []string{"ing_route_states", "ing_secret_fetches"} {
		assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM `+table+` WHERE ingestion_id = $1`, missing), "%s of an ingestion that no longer exists", table)
		assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM `+table+` WHERE ingestion_id = $1`, kept.ID), "%s of a live runtime and a live ingestion stays", table)
	}
}

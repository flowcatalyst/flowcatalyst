//go:build integration

package payloadstore_test

// An external test package: testpg imports the platform sink, which imports
// this package.

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func newPGService(t *testing.T, store payloadstore.ObjectStore) *payloadstore.Service {
	t.Helper()
	s, err := payloadstore.NewWithStore(payloadstore.DefaultConfig(), store, testpg.Pool(t))
	require.NoError(t, err)
	return s
}

// Rows from Prepare are inserted in a caller's transaction (Insert) and in a
// caller's pgx.Batch (payloadstore.QueueInsert), and read back by primary key, inline and
// offloaded.
func TestInsertAndGet(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadstore.NewMemoryStore()
	s := newPGService(t, store)
	createdAt := time.Now().UTC().Truncate(time.Microsecond)

	small := []byte(`{"order":"SO-1","note":"small"}`)
	large := payloadOf(50_000, 3)
	empty := []byte{}
	out := s.Prepare(ctx, []payloadstore.Item{
		{ClientID: "clt_PGTEST000001", CreatedAt: createdAt, Data: small},
		{ClientID: "clt_PGTEST000001", CreatedAt: createdAt, Data: large},
		{ClientID: "clt_PGTEST000001", CreatedAt: createdAt, Data: empty},
	})
	rows := make([]payloadstore.Row, 0, len(out))
	for _, p := range out {
		require.NoError(t, p.Err)
		rows = append(rows, p.Row)
	}
	require.False(t, rows[0].Offloaded())
	require.True(t, rows[1].Offloaded())

	// Insert on a transaction: nothing is visible until it commits.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, payloadstore.Insert(ctx, tx, rows[:2]))
	_, err = s.Get(ctx, rows[0].ID, createdAt)
	require.ErrorIs(t, err, payloadstore.ErrMissing, "not committed yet")
	require.NoError(t, tx.Commit(ctx))

	// payloadstore.QueueInsert beside a caller's own statement in one batch.
	b := &pgx.Batch{}
	b.Queue(`SELECT 1`)
	payloadstore.QueueInsert(b, rows[2])
	br := pool.SendBatch(ctx, b)
	for range b.Len() {
		_, err := br.Exec()
		require.NoError(t, err)
	}
	require.NoError(t, br.Close())

	got, err := s.Get(ctx, rows[0].ID, createdAt)
	require.NoError(t, err)
	assert.Equal(t, small, got)
	got, err = s.Get(ctx, rows[1].ID, createdAt)
	require.NoError(t, err)
	assert.Equal(t, large, got)
	got, err = s.Get(ctx, rows[2].ID, createdAt)
	require.NoError(t, err)
	assert.Empty(t, got)

	// What the table holds: data or ref, and the raw size.
	var data, ref *string
	var size int
	require.NoError(t, pool.QueryRow(ctx, `SELECT data, ref, size FROM msg_payloads WHERE id = $1 AND created_at = $2`,
		rows[1].ID, createdAt).Scan(&data, &ref, &size))
	assert.Nil(t, data)
	require.NotNil(t, ref)
	assert.Equal(t, rows[1].Ref.String(), *ref)
	assert.Equal(t, len(large), size)

	// Resolve, as a reader that selected the columns itself.
	got, err = s.Resolve(ctx, payloadstore.Stored{ID: rows[1].ID, Data: data, Ref: ref, Size: size})
	require.NoError(t, err)
	assert.Equal(t, large, got)

	// Read by primary key: another created_at finds nothing.
	_, err = s.Get(ctx, rows[0].ID, createdAt.Add(time.Second))
	assert.ErrorIs(t, err, payloadstore.ErrMissing)
}

// Exactly one of data and ref: the CHECK refuses neither and both.
func TestPayloadRowCheck(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, payloadstore.InsertSQL, tsid.GenerateUntyped(), now, nil, nil, 0)
	require.Error(t, err, "neither data nor ref")
	assert.Contains(t, err.Error(), "chk_msg_payloads_data_or_ref")
	_, err = pool.Exec(ctx, payloadstore.InsertSQL, tsid.GenerateUntyped(), now, "{}", "v1:k:0:1:zstd:00000000", 2)
	require.Error(t, err, "both data and ref")
	assert.Contains(t, err.Error(), "chk_msg_payloads_data_or_ref")
	_, err = pool.Exec(ctx, payloadstore.InsertSQL, tsid.GenerateUntyped(), now, "{}", nil, 2)
	require.NoError(t, err, "data only")
}

// A failed insert in a batch is reported with the payload's id.
func TestInsert_DuplicateKeyFails(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	row := payloadstore.Row{ID: tsid.GenerateUntyped(), CreatedAt: time.Now().UTC(), Data: []byte(`1`), Size: 1}
	require.NoError(t, payloadstore.Insert(ctx, pool, []payloadstore.Row{row}))
	err := payloadstore.Insert(ctx, pool, []payloadstore.Row{row})
	require.Error(t, err)
	assert.Contains(t, err.Error(), row.ID)
}

// payloadOf makes an n-byte JSON-ish payload that differs by seed (as the
// package's own tests do).
func payloadOf(n int, seed byte) []byte {
	b := bytes.Repeat([]byte{'a' + seed%26}, n)
	if n >= 2 {
		b[0], b[n-1] = '"', '"'
	}
	return b
}

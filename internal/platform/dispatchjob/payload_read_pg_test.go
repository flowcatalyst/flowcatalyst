//go:build integration

package dispatchjob_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The readers of a job's payload accept both shapes
// (docs/payload-offload-design.md, section 10, step 1). These tests write the
// rows of either shape themselves, so each reader is tested on exactly the
// shape it meets; the writers (behind FC_PAYLOAD_TABLE_WRITES) have tests of
// their own.

// jobOddPayload is a payload JSONB would have changed; a job's payload is
// text and is returned byte for byte, in both shapes.
const jobOddPayload = `{"z": 1,"a":  [3, 1] , "z":2}`

func jobLargePayload() string {
	return `{"z": 1, "pad":"` + strings.Repeat("q", 3*payloadtest.OffloadMinBytes) + `" ,"a":2}`
}

// putJob writes a PENDING job under createdAt: legacy (payload set,
// payloadID nil) or new-shape (payload NULL, payloadID set).
func putJob(t *testing.T, pool *pgxpool.Pool, createdAt time.Time, payload, payloadID *string) string {
	t.Helper()
	id := tsid.GenerateUntyped()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, payload, payload_id, created_at, updated_at)
		 VALUES ($1, 'payloadread:job:test:made', 'http://subscriber.invalid/hook', 'PENDING', $2, $3, $4, $4)`,
		id, payload, payloadID, createdAt)
	require.NoError(t, err)
	return id
}

// jobAt is a created_at newer than anything else in the shared database, so
// FindRecentRaw's page holds the test's rows.
func jobAt() time.Time { return time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond) }

func findRecent(t *testing.T, repo *dispatchjob.Repository, id string) *dispatchjob.DispatchJob {
	t.Helper()
	jobs, err := repo.FindRecentRaw(context.Background(), 100, nil)
	require.NoError(t, err)
	for i := range jobs {
		if jobs[i].ID == id {
			return &jobs[i]
		}
	}
	t.Fatalf("job %s not in the recent raw page", id)
	return nil
}

func TestJobReads_LegacyRowReadsAsBefore(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	repo := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.Service(t, store))
	p := jobOddPayload
	id := putJob(t, pool, jobAt(), &p, nil)

	j, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, j)
	require.NotNil(t, j.Payload)
	assert.Equal(t, jobOddPayload, *j.Payload)
	assert.Nil(t, j.StoredPayload)
	assert.Equal(t, len(jobOddPayload), j.PayloadLength())
	assert.False(t, j.PayloadOffloaded())
	require.NoError(t, repo.ResolvePayload(ctx, j), "a legacy job needs nothing resolved")
	assert.Equal(t, jobOddPayload, *j.Payload)

	got := findRecent(t, repo, id)
	require.NotNil(t, got.Payload)
	assert.Equal(t, jobOddPayload, *got.Payload)
	assert.Equal(t, len(jobOddPayload), got.PayloadLength())

	many, err := repo.FindByIDs(ctx, []string{id})
	require.NoError(t, err)
	require.Len(t, many, 1)
	assert.Equal(t, jobOddPayload, *many[0].Payload)
	assert.Zero(t, store.Gets())
}

func TestJobReads_NewShape(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	at := jobAt()

	inlineRow := payloadtest.Store(t, svc, pool, "", at, []byte(jobOddPayload))
	require.False(t, inlineRow.Offloaded())
	large := jobLargePayload()
	bigRow := payloadtest.Store(t, svc, pool, "", at, []byte(large))
	require.True(t, bigRow.Offloaded())

	inline := putJob(t, pool, at, nil, &inlineRow.ID)
	// Two jobs of one event share its payload row.
	shareA := putJob(t, pool, at, nil, &bigRow.ID)
	shareB := putJob(t, pool, at, nil, &bigRow.ID)

	t.Run("held in the database", func(t *testing.T) {
		j, err := repo.FindByID(ctx, inline)
		require.NoError(t, err)
		require.NotNil(t, j.Payload)
		assert.Equal(t, jobOddPayload, *j.Payload)
		assert.Equal(t, len(jobOddPayload), j.PayloadLength())
		assert.False(t, j.PayloadOffloaded())
		got := findRecent(t, repo, inline)
		require.NotNil(t, got.Payload)
		assert.Equal(t, jobOddPayload, *got.Payload)
		assert.Zero(t, store.Gets())
	})

	t.Run("offloaded: no read resolves by itself", func(t *testing.T) {
		j, err := repo.FindByID(ctx, shareA)
		require.NoError(t, err)
		assert.Nil(t, j.Payload)
		assert.True(t, j.PayloadOffloaded())
		assert.Equal(t, len(large), j.PayloadLength())

		got := findRecent(t, repo, shareA)
		assert.Nil(t, got.Payload)
		assert.True(t, got.PayloadOffloaded())
		assert.Equal(t, len(large), got.PayloadLength())

		many, err := repo.FindByIDs(ctx, []string{shareA, shareB, inline})
		require.NoError(t, err)
		require.Len(t, many, 3)
		assert.Zero(t, store.Gets(), "FindByID, FindByIDs and the raw list make no store call")
	})

	t.Run("offloaded: resolved on demand, both jobs of the event", func(t *testing.T) {
		for _, id := range []string{shareA, shareB} {
			j, err := repo.FindByID(ctx, id)
			require.NoError(t, err)
			require.NoError(t, repo.ResolvePayload(ctx, j))
			require.NotNil(t, j.Payload)
			assert.Equal(t, large, *j.Payload)
			assert.False(t, j.PayloadOffloaded())
		}
		assert.EqualValues(t, 2, store.Gets(), "one read per resolve (the test service caches nothing)")

		j, err := repo.FindByID(ctx, shareB)
		require.NoError(t, err)
		repo.ResolvePayloadForView(ctx, j)
		require.NotNil(t, j.Payload)
		assert.Equal(t, large, *j.Payload)
		assert.False(t, j.PayloadUnavailable)
	})

	t.Run("store failing", func(t *testing.T) {
		store.FailReads(errors.New("connection reset"))
		t.Cleanup(func() { store.FailReads(nil) })
		j, err := repo.FindByID(ctx, shareA)
		require.NoError(t, err)
		err = repo.ResolvePayload(ctx, j)
		require.ErrorIs(t, err, payloadstore.ErrStoreUnavailable)
		assert.Nil(t, j.Payload)

		repo.ResolvePayloadForView(ctx, j)
		assert.True(t, j.PayloadUnavailable)
		assert.Nil(t, j.Payload)
		assert.Equal(t, len(large), j.PayloadLength())
	})

	t.Run("payload missing", func(t *testing.T) {
		goneRow := payloadtest.Store(t, svc, pool, "", at, []byte(large))
		gone := putJob(t, pool, at, nil, &goneRow.ID)
		store.Delete(goneRow.Ref.Key)
		j, err := repo.FindByID(ctx, gone)
		require.NoError(t, err)
		require.ErrorIs(t, repo.ResolvePayload(ctx, j), payloadstore.ErrMissing)

		// A payload_id with no payload row under the job's created_at.
		orphanID := tsid.GenerateUntyped()
		orphan := putJob(t, pool, at, nil, &orphanID)
		j, err = repo.FindByID(ctx, orphan)
		require.NoError(t, err)
		assert.Nil(t, j.Payload)
		assert.False(t, j.PayloadOffloaded())
		require.ErrorIs(t, repo.ResolvePayload(ctx, j), payloadstore.ErrMissing)
	})

	t.Run("no payload service", func(t *testing.T) {
		bare := dispatchjob.NewRepository(pool)
		j, err := bare.FindByID(ctx, inline)
		require.NoError(t, err)
		require.NoError(t, bare.ResolvePayload(ctx, j))
		assert.Equal(t, jobOddPayload, *j.Payload, "held in the database: readable without a service")
		j, err = bare.FindByID(ctx, shareA)
		require.NoError(t, err)
		require.ErrorIs(t, bare.ResolvePayload(ctx, j), payloadstore.ErrStoreNotConfigured)
	})
}

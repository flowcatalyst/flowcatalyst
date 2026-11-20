//go:build integration

package dispatchjob_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The job writers with the payload table written (docs/payload-offload-design.md,
// sections 3.1 and 6): PreparePayloads readies a payload row of the job's own
// under its created_at, and the lifecycle's create inserts it with the job in
// one statement, only when the job row is inserted.

func newJob(payload *string) dispatchjob.DispatchJob {
	return dispatchjob.DispatchJob{
		ID: tsid.GenerateUntyped(), Kind: dispatchjob.KindTask, Code: "pwtest:job:made",
		TargetURL: "http://example.invalid/hook", Protocol: dispatchjob.ProtocolHTTPWebhook,
		Payload: payload, PayloadContentType: "application/json", Mode: common.DispatchImmediate,
		TimeoutSeconds: 30, MaxRetries: 3, RetryStrategy: dispatchjob.RetryExponentialBackoff,
	}
}

func TestCreateBatch_NewShapeWithItsPayloadRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, nil, nil))
	require.True(t, repo.WritesPayloadTable())

	jobs := []dispatchjob.DispatchJob{newJob(new(jobOddPayload)), newJob(nil)}
	errs := repo.PreparePayloads(ctx, jobs)
	require.Equal(t, []error{nil, nil}, errs)
	require.NotNil(t, jobs[0].NewPayload)
	assert.False(t, jobs[0].CreatedAt.IsZero(), "the job's created_at is fixed before its payload is prepared")
	assert.Nil(t, jobs[1].NewPayload, "a job with no payload gets no payload row")
	require.NoError(t, repo.InsertBatch(ctx, jobs))

	got, err := repo.FindByID(ctx, jobs[0].ID)
	require.NoError(t, err)
	require.NotNil(t, got.StoredPayload, "the new shape")
	assert.Equal(t, jobs[0].NewPayload.ID, got.StoredPayload.ID)
	require.NotNil(t, got.Payload)
	assert.Equal(t, jobOddPayload, *got.Payload)
	var legacy *string
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM msg_dispatch_jobs WHERE id = $1`, jobs[0].ID).Scan(&legacy))
	assert.Nil(t, legacy)

	none, err := repo.FindByID(ctx, jobs[1].ID)
	require.NoError(t, err)
	assert.Nil(t, none.StoredPayload)
	assert.Nil(t, none.Payload)

	// The same job again (same id and created_at) with a new payload row:
	// the job insert's ON CONFLICT drops it, and its payload row with it.
	again := []dispatchjob.DispatchJob{jobs[0]}
	again[0].NewPayload = nil
	again[0].Payload = new(`{"second":true}`)
	require.Equal(t, []error{nil}, repo.PreparePayloads(ctx, again))
	require.NoError(t, repo.InsertBatch(ctx, again))
	var n int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM msg_payloads WHERE id = $1`, again[0].NewPayload.ID).Scan(&n))
	assert.Zero(t, n, "no payload row without its job row")
}

func TestPreparePayloads_RefusalsAndTheSwitchOff(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	store.FailWrites(errors.New("down"))
	repo := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, store,
		func(c *payloadstore.Config) { c.InlineMaxBytes = 8192; c.MaxBytes = 16384 }))
	str := func(n int) *string { s := `"` + strings.Repeat("r", n-2) + `"`; return &s }
	jobs := []dispatchjob.DispatchJob{newJob(str(100)), newJob(str(6000)), newJob(str(10000)), newJob(str(20000)), newJob(new("a\x00b"))}
	errs := repo.PreparePayloads(ctx, jobs)
	assert.NoError(t, errs[0])
	assert.NoError(t, errs[1], "kept in the database while the store is down")
	assert.ErrorIs(t, errs[2], payloadstore.ErrStoreUnavailable)
	assert.ErrorIs(t, errs[3], payloadstore.ErrTooLarge)
	assert.ErrorIs(t, errs[4], payloadstore.ErrNotText)

	off := dispatchjob.NewRepository(pool).UsePayloads(payloadtest.Service(t, store))
	legacy := []dispatchjob.DispatchJob{newJob(str(20000))}
	assert.Equal(t, []error{nil}, off.PreparePayloads(ctx, legacy), "no new limit with the switch off")
	assert.Nil(t, legacy[0].NewPayload)
	assert.True(t, legacy[0].CreatedAt.IsZero(), "nothing is touched")
}

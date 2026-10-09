//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func newState(pool *pgxpool.Pool) *State {
	return &State{Repo: dispatchjob.NewRepository(pool), UoW: usecasepgx.New(pool, platformsink.New())}
}

func anchor() context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_rbftestanchor", Scope: auth.ScopeAnchor, Permissions: []string{viewPerm},
	})
}

func clientCaller(clients ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_rbftestclient", Scope: auth.ScopeClient, Clients: clients, Permissions: []string{viewPerm},
	})
}

// seedJobs writes n jobs into BOTH the write table (what the use case reads and
// resets) and the read projection (what the filter counts and walks) — the
// projector does not run in these tests. Ids are prefix + a zero-padded
// counter, so their id order is their seeding order. Returns the ids.
func seedJobs(t *testing.T, pool *pgxpool.Pool, prefix, code, status string, clientID *string, n int) []string {
	t.Helper()
	require.Len(t, prefix, 5, "ids are VARCHAR(13): 5-char prefix + 8 digits")
	ctx := context.Background()
	created := time.Now().UTC().Add(-time.Hour)
	_, err := pool.Exec(ctx, `
		INSERT INTO msg_dispatch_jobs (id, code, target_url, status, client_id, attempt_count, last_error, created_at, updated_at)
		SELECT $1 || lpad(n::text, 8, '0'), $2, 'http://example.invalid/hook', $3, $4, 3, 'boom', $6, $6
		  FROM generate_series(1, $5::int) n`, prefix, code, status, clientID, n, created)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO msg_dispatch_jobs_read (id, code, target_url, client_id, kind, protocol, mode, status, max_retries, attempt_count, created_at, updated_at)
		SELECT $1 || lpad(n::text, 8, '0'), $2, 'http://example.invalid/hook', $4, 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', $3, 3, 3, $6, $6
		  FROM generate_series(1, $5::int) n`, prefix, code, status, clientID, n, created)
	require.NoError(t, err)
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("%s%08d", prefix, i+1)
	}
	return ids
}

// syncProjection does the projector's job for the given code: the read rows
// take the write rows' status.
func syncProjection(t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		UPDATE msg_dispatch_jobs_read r SET status = j.status, attempt_count = j.attempt_count
		  FROM msg_dispatch_jobs j WHERE r.id = j.id AND r.created_at = j.created_at AND r.code = $1`, code)
	require.NoError(t, err)
}

func statusesOf(t *testing.T, pool *pgxpool.Pool, ids []string) map[string]int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT status FROM msg_dispatch_jobs WHERE id = ANY($1)`, ids)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		out[s]++
	}
	require.NoError(t, rows.Err())
	return out
}

func preview(t *testing.T, s *State, ctx context.Context, f DispatchJobFilter) RequeueByFilterPreviewResponse {
	t.Helper()
	out, err := s.requeueByFilterPreview(ctx, &apicommon.In[RequeueByFilterPreviewRequest]{Body: RequeueByFilterPreviewRequest{f}})
	require.NoError(t, err)
	return out.Body
}

func run(s *State, ctx context.Context, f DispatchJobFilter, confirm int64, limit int) (RequeueByFilterResponse, error) {
	out, err := s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: f, ConfirmCount: confirm, Limit: limit,
	}})
	if err != nil {
		return RequeueByFilterResponse{}, err
	}
	return out.Body, nil
}

func TestRequeueByFilterPreview_CountsAndBreakdown(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:preview:job:x"
	a, b := "clt_rbfprev0000a", "clt_rbfprev0000b"
	seedJobs(t, pool, "rbpaf", code, "FAILED", &a, 3)
	seedJobs(t, pool, "rbpac", code, "COMPLETED", &a, 2)
	seedJobs(t, pool, "rbpap", code, "PENDING", &a, 1)
	seedJobs(t, pool, "rbppf", code, "FAILED", nil, 2)
	seedJobs(t, pool, "rbpbf", code, "FAILED", &b, 1)

	got := preview(t, s, anchor(), DispatchJobFilter{Codes: code})
	assert.Equal(t, RequeueByFilterPreviewResponse{
		Count: 9, ByStatus: map[string]int64{"FAILED": 6, "COMPLETED": 2, "PENDING": 1}, EligibleCount: 9,
	}, got)

	got = preview(t, s, anchor(), DispatchJobFilter{Codes: code, Statuses: "FAILED"})
	assert.Equal(t, int64(6), got.Count)
	assert.Equal(t, int64(6), got.EligibleCount)

	// A tenant's caller sees its own jobs plus platform-scoped ones (the list's
	// rule) but may requeue only its own (the use case's rule).
	got = preview(t, s, clientCaller(a), DispatchJobFilter{Codes: code})
	assert.Equal(t, int64(8), got.Count)
	assert.Equal(t, map[string]int64{"FAILED": 5, "COMPLETED": 2, "PENDING": 1}, got.ByStatus)
	assert.Equal(t, int64(6), got.EligibleCount)

	// Asking for another tenant's jobs finds none.
	got = preview(t, s, clientCaller(a), DispatchJobFilter{Codes: code, ClientIDs: b})
	assert.Equal(t, RequeueByFilterPreviewResponse{ByStatus: map[string]int64{}}, got)
}

func TestRequeueByFilter_ConfirmCountMismatchIs409(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:conflict:job:x"
	ids := seedJobs(t, pool, "rbcfl", code, "FAILED", nil, 5)
	f := DispatchJobFilter{Codes: code, Statuses: "FAILED"}

	_, err := run(s, anchor(), f, 4, 0)
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "FILTER_MATCH_CHANGED")
	assert.Equal(t, int64(5), usecase.AsError(err).Details["eligibleCount"])
	assert.Equal(t, map[string]int{"FAILED": 5}, statusesOf(t, pool, ids), "a refused run resets nothing")

	// Fewer matches than confirmed is fine (some were handled meanwhile).
	res, err := run(s, anchor(), f, 7, 0)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 5}, res)
}

// 1,250 matches, limit 1,000: two batches of 500 run, 250 are left, and each
// batch is one ResendDispatchJobs run with its own event and audit row.
func TestRequeueByFilter_BatchesAndTruncates(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:batch:job:x"
	ids := seedJobs(t, pool, "rbbat", code, "FAILED", nil, 1250)
	f := DispatchJobFilter{Codes: code, Statuses: "FAILED"}

	pv := preview(t, s, anchor(), f)
	require.Equal(t, int64(1250), pv.EligibleCount)
	res, err := run(s, anchor(), f, pv.EligibleCount, 1000)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 1000, Truncated: true, Remaining: 250}, res)
	// The walk is in id order: the first 1,000 ids were reset, the rest not.
	assert.Equal(t, map[string]int{"PENDING": 1000}, statusesOf(t, pool, ids[:1000]))
	assert.Equal(t, map[string]int{"FAILED": 250}, statusesOf(t, pool, ids[1000:]))

	var events, audits int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_events WHERE type = $1 AND data->'ids' ?| $2`, operations.DispatchJobsResentType, ids).Scan(&events))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM aud_logs WHERE operation = 'ResendCommand' AND operation_json->'ids' ?| $1`, ids).Scan(&audits))
	assert.Equal(t, 2, events, "one DispatchJobsResent event per batch")
	assert.Equal(t, 2, audits, "one audit row per batch")

	var batchSizes []int
	rows, err := pool.Query(context.Background(),
		`SELECT jsonb_array_length(data->'ids') FROM msg_events WHERE type = $1 AND data->'ids' ?| $2`, operations.DispatchJobsResentType, ids)
	require.NoError(t, err)
	for rows.Next() {
		var n int
		require.NoError(t, rows.Scan(&n))
		batchSizes = append(batchSizes, n)
	}
	rows.Close()
	assert.Equal(t, []int{500, 500}, batchSizes)

	// Once the projection has caught up, running again finishes the rest.
	syncProjection(t, pool, code)
	pv = preview(t, s, anchor(), f)
	require.Equal(t, int64(250), pv.EligibleCount)
	res, err = run(s, anchor(), f, pv.EligibleCount, 1000)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 250}, res)
	assert.Equal(t, map[string]int{"PENDING": 1250}, statusesOf(t, pool, ids))
}

func TestRequeueByFilter_OnlyMatchingRowsReset(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code, other = "rbftest:only:job:x", "rbftest:only:job:other"
	failed := seedJobs(t, pool, "rbof1", code, "FAILED", nil, 4)
	completed := seedJobs(t, pool, "rbof2", code, "COMPLETED", nil, 3)
	otherCode := seedJobs(t, pool, "rbof3", other, "FAILED", nil, 2)

	f := DispatchJobFilter{Codes: code, Statuses: "FAILED"}
	pv := preview(t, s, anchor(), f)
	res, err := run(s, anchor(), f, pv.EligibleCount, 0)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 4}, res)

	assert.Equal(t, map[string]int{"PENDING": 4}, statusesOf(t, pool, failed))
	assert.Equal(t, map[string]int{"COMPLETED": 3}, statusesOf(t, pool, completed))
	assert.Equal(t, map[string]int{"FAILED": 2}, statusesOf(t, pool, otherCode))

	var reset int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_dispatch_jobs WHERE id = ANY($1) AND attempt_count = 0 AND last_error IS NULL`, failed).Scan(&reset))
	assert.Equal(t, 4, reset, "the use case's full reset ran on every matched job")
}

func TestRequeueByFilter_TenantScoping(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:tenant:job:x"
	a, b := "clt_rbftenant00a", "clt_rbftenant00b"
	own := seedJobs(t, pool, "rbtna", code, "FAILED", &a, 3)
	theirs := seedJobs(t, pool, "rbtnb", code, "FAILED", &b, 3)
	platform := seedJobs(t, pool, "rbtnp", code, "FAILED", nil, 3)

	ctx := clientCaller(a)
	f := DispatchJobFilter{Codes: code}
	pv := preview(t, s, ctx, f)
	require.Equal(t, int64(6), pv.Count)
	require.Equal(t, int64(3), pv.EligibleCount)
	res, err := run(s, ctx, f, pv.EligibleCount, 0)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 3}, res)
	assert.Equal(t, map[string]int{"PENDING": 3}, statusesOf(t, pool, own))
	assert.Equal(t, map[string]int{"FAILED": 3}, statusesOf(t, pool, theirs))
	assert.Equal(t, map[string]int{"FAILED": 3}, statusesOf(t, pool, platform))

	// Naming the other tenant explicitly reaches nothing either.
	res, err = run(s, ctx, DispatchJobFilter{Codes: code, ClientIDs: b}, 0, 0)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{}, res)
	assert.Equal(t, map[string]int{"FAILED": 3}, statusesOf(t, pool, theirs))
}

func TestRequeueByFilter_PermissionDenied(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:denied:job:x"
	ids := seedJobs(t, pool, "rbden", code, "FAILED", nil, 2)
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_rbftestnoperm", Scope: auth.ScopeAnchor, Permissions: []string{"platform:messaging:event:view"},
	})

	_, err := s.requeueByFilterPreview(ctx, &apicommon.In[RequeueByFilterPreviewRequest]{})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = run(s, ctx, DispatchJobFilter{Codes: code}, 2, 0)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	assert.Equal(t, map[string]int{"FAILED": 2}, statusesOf(t, pool, ids))
}

func TestRequeueByFilter_RejectsBadTimestampAndLimit(t *testing.T) {
	t.Parallel()
	s := newState(testpg.Pool(t))
	_, err := s.requeueByFilterPreview(anchor(), &apicommon.In[RequeueByFilterPreviewRequest]{
		Body: RequeueByFilterPreviewRequest{DispatchJobFilter{Since: "yesterday"}},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_TIMESTAMP")
	_, err = run(s, anchor(), DispatchJobFilter{Until: "2026-13-01"}, 0, 0)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_TIMESTAMP")
	_, err = run(s, anchor(), DispatchJobFilter{}, 0, operations.ResendByFilterMaxLimit+1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_LIMIT")
}

// The wire: both route prefixes, the JSON shapes, body validation (limit cap,
// required confirmCount) and the 409 envelope with the new counts.
func TestRequeueByFilter_Wire(t *testing.T) {
	httpcompat.Init()
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "rbftest:wire:job:x"
	ids := seedJobs(t, pool, "rbwir", code, "FAILED", nil, 3)

	_, api := humatest.New(t)
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithContext(ctx, auth.WithContext(ctx.Context(), auth.FromContext(anchor()))))
	})
	Register(api, s)
	httpcompat.RelaxRequestBodies(api)

	decode := func(r *httptest.ResponseRecorder) map[string]any {
		var m map[string]any
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &m), r.Body.String())
		return m
	}

	r := api.Post("/bff/dispatch-jobs/requeue-by-filter/preview", map[string]any{"codes": code, "statuses": "FAILED"})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, map[string]any{"count": 3.0, "byStatus": map[string]any{"FAILED": 3.0}, "eligibleCount": 3.0}, decode(r))

	r = api.Post("/api/dispatch-jobs/requeue-by-filter", map[string]any{"codes": code})
	assert.Equal(t, http.StatusBadRequest, r.Code, "confirmCount is required: %s", r.Body.String())
	r = api.Post("/api/dispatch-jobs/requeue-by-filter", map[string]any{"codes": code, "confirmCount": 3, "limit": 10001})
	assert.Equal(t, http.StatusBadRequest, r.Code, "limit is capped at 10000: %s", r.Body.String())

	r = api.Post("/api/dispatch-jobs/requeue-by-filter", map[string]any{"codes": code, "confirmCount": 2})
	require.Equal(t, http.StatusConflict, r.Code, r.Body.String())
	body := decode(r)
	assert.Equal(t, "FILTER_MATCH_CHANGED", body["error"])
	assert.Equal(t, map[string]any{"count": 3.0, "eligibleCount": 3.0, "byStatus": map[string]any{"FAILED": 3.0}}, body["details"])

	r = api.Post("/bff/dispatch-jobs/requeue-by-filter", map[string]any{"codes": code, "confirmCount": 3})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, map[string]any{"requeued": 3.0, "truncated": false, "remaining": 0.0}, decode(r))
	assert.Equal(t, map[string]int{"PENDING": 3}, statusesOf(t, pool, ids))
}

// A filter with no restricting field is "every job": it works, under the same
// confirmCount check. Own database, so "every job" is only this test's.
func TestRequeueByFilter_NoFilter(t *testing.T) {
	pool := testpg.ScratchDB(t, "rbf_nofilter")
	s := newState(pool)
	failed := seedJobs(t, pool, "rbnf1", "rbftest:nofilter:job:a", "FAILED", nil, 20)
	done := seedJobs(t, pool, "rbnf2", "rbftest:nofilter:job:b", "COMPLETED", nil, 10)

	pv := preview(t, s, anchor(), DispatchJobFilter{})
	assert.Equal(t, RequeueByFilterPreviewResponse{
		Count: 30, ByStatus: map[string]int64{"FAILED": 20, "COMPLETED": 10}, EligibleCount: 30,
	}, pv)
	_, err := run(s, anchor(), DispatchJobFilter{}, 29, 0)
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "FILTER_MATCH_CHANGED")

	res, err := run(s, anchor(), DispatchJobFilter{}, pv.EligibleCount, 0)
	require.NoError(t, err)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 30}, res)
	assert.Equal(t, map[string]int{"PENDING": 30}, statusesOf(t, pool, append(failed, done...)))
}

// The scheduler keeps claiming PENDING jobs and marking them QUEUED while a run
// resets the same jobs. Requeue moves a job from any status, so every job is
// reset; mark-QUEUED is optimistic on updated_at, so a claim the reset
// overtook is refused rather than applied to the reset job. Every job ends
// PENDING (reset, not yet claimed again) or QUEUED (claimed after its reset),
// always with a fresh attempt budget, and nothing errors.
func TestRequeueByFilter_ConcurrentSchedulerClaim(t *testing.T) {
	pool := testpg.ScratchDB(t, "rbf_concurrent")
	s := newState(pool)
	const code = "rbftest:concurrent:job:x"
	ids := seedJobs(t, pool, "rbcon", code, "PENDING", nil, 1500)
	lc := dispatchjob.NewLifecycle(pool)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var marked atomic.Int64
	var claimErr error
	wg.Go(func() {
		for ctx.Err() == nil {
			claimed, err := lc.ClaimPending(ctx, 200, nil, nil, nil)
			if err != nil {
				if ctx.Err() == nil {
					claimErr = err
				}
				return
			}
			if len(claimed) == 0 {
				continue
			}
			cids := make([]string, len(claimed))
			cas := make([]time.Time, len(claimed))
			ups := make([]time.Time, len(claimed))
			for i, c := range claimed {
				cids[i], cas[i], ups[i] = c.ID, c.CreatedAt, c.UpdatedAt
			}
			n, err := lc.MarkQueued(ctx, cids, cas, ups)
			if err != nil {
				if ctx.Err() == nil {
					claimErr = err
				}
				return
			}
			marked.Add(n)
		}
	})

	f := DispatchJobFilter{Codes: code}
	pv := preview(t, s, anchor(), f)
	require.Equal(t, int64(1500), pv.EligibleCount)
	res, err := run(s, anchor(), f, pv.EligibleCount, 0)
	cancel()
	wg.Wait()
	require.NoError(t, err)
	require.NoError(t, claimErr)
	assert.Equal(t, RequeueByFilterResponse{Requeued: 1500}, res)
	assert.Positive(t, marked.Load(), "the scheduler must actually have raced the run")

	got := statusesOf(t, pool, ids)
	assert.Equal(t, 1500, got["PENDING"]+got["QUEUED"], "every job is PENDING or QUEUED: %v", got)
	var fresh int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_dispatch_jobs WHERE id = ANY($1) AND attempt_count = 0`, ids).Scan(&fresh))
	assert.Equal(t, 1500, fresh)
}

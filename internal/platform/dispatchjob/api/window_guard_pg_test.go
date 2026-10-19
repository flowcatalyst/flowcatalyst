//go:build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func callerWith(perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_wgtestcaller", Scope: auth.ScopeAnchor, Permissions: perms,
	})
}

func serveAs(t *testing.T, s *State, perms ...string) humatest.TestAPI {
	t.Helper()
	httpcompat.Init()
	_, api := humatest.New(t)
	c := auth.FromContext(callerWith(perms...))
	api.UseMiddleware(func(h huma.Context, next func(huma.Context)) {
		next(huma.WithContext(h, auth.WithContext(h.Context(), c)))
	})
	Register(api, s)
	httpcompat.RelaxRequestBodies(api)
	return api
}

func rfc(t time.Time) string { return url.QueryEscape(t.UTC().Format(time.RFC3339)) }

func idsOf(t *testing.T, body []byte) []string {
	t.Helper()
	var rows []struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &rows), string(body))
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out
}

// ── the time window ────────────────────────────────────────────────────

// A list that names no time bound is held to the last 24 hours and says so;
// an explicit window is honoured as given; a window beyond the maximum is a
// 400 WINDOW_TOO_LARGE; lookups keyed by one job or event are not windowed.
func TestWindow_ListDefaultsHonoursAndLimits(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "wgwin:list:job:x"
	recent := seedJobs(t, pool, "wgwr1", code, "FAILED", nil, 2)
	old := seedJobsAt(t, pool, "wgwo1", code, "FAILED", nil, 1, time.Now().UTC().Add(-72*time.Hour))
	api := serveAs(t, s, viewPerm, auth.DispatchJobRequeuePerm)

	// No bounds: the last 24 hours, echoed in headers on both mounts.
	for _, base := range []string{"/api/dispatch-jobs", "/bff/dispatch-jobs"} {
		r := api.Get(base + "?code=" + code)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.ElementsMatch(t, recent, idsOf(t, r.Body.Bytes()), "the 72-hour-old job is outside the default window")
		assert.Equal(t, "true", r.Header().Get("X-Window-Defaulted"))
		from, err := time.Parse(time.RFC3339, r.Header().Get("X-Window-From"))
		require.NoError(t, err)
		to, err := time.Parse(time.RFC3339, r.Header().Get("X-Window-To"))
		require.NoError(t, err)
		assert.InDelta(t, 24*time.Hour.Seconds(), to.Sub(from).Seconds(), 2)
		assert.WithinDuration(t, time.Now(), to, time.Minute)
	}

	// An explicit window that holds the old job: honoured, not defaulted.
	r := api.Get("/api/dispatch-jobs?code=" + code + "&since=" + rfc(time.Now().Add(-5*24*time.Hour)) + "&until=" + rfc(time.Now().Add(-2*24*time.Hour)))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Equal(t, old, idsOf(t, r.Body.Bytes()))
	assert.Equal(t, "false", r.Header().Get("X-Window-Defaulted"))

	// since only: since .. now.
	r = api.Get("/api/dispatch-jobs?code=" + code + "&since=" + rfc(time.Now().Add(-5*24*time.Hour)))
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Len(t, idsOf(t, r.Body.Bytes()), 3)
	assert.Equal(t, "false", r.Header().Get("X-Window-Defaulted"))

	// Too large: the default maximum is 31 days.
	r = api.Get("/api/dispatch-jobs?code=" + code + "&since=" + rfc(time.Now().Add(-60*24*time.Hour)))
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "WINDOW_TOO_LARGE")
	r = api.Get("/bff/dispatch-jobs?since=" + rfc(time.Now().Add(-60*24*time.Hour)) + "&until=" + rfc(time.Now().Add(-1*24*time.Hour)))
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())

	// ... and a configured maximum applies.
	narrow := newState(pool)
	narrow.MaxWindowDays = 2
	r = serveAs(t, narrow, viewPerm).Get("/api/dispatch-jobs?since=" + rfc(time.Now().Add(-3*24*time.Hour)))
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "WINDOW_TOO_LARGE")
	r = serveAs(t, narrow, viewPerm).Get("/api/dispatch-jobs?code=" + code + "&since=" + rfc(time.Now().Add(-47*time.Hour)))
	assert.Equal(t, http.StatusOK, r.Code, r.Body.String())

	// until before since.
	r = api.Get("/api/dispatch-jobs?since=" + rfc(time.Now().Add(-1*time.Hour)) + "&until=" + rfc(time.Now().Add(-2*time.Hour)))
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "INVALID_WINDOW")
}

func TestWindow_ListRawIsWindowedToo(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "wgwin:raw:job:x"
	recent := seedJobs(t, pool, "wgrr1", code, "FAILED", nil, 1)
	seedJobsAt(t, pool, "wgro1", code, "FAILED", nil, 1, time.Now().UTC().Add(-72*time.Hour))
	api := serveAs(t, s, viewRawPerm)
	for _, path := range []string{"/api/dispatch-jobs/list-raw"} {
		r := api.Get(path + "?code=" + code)
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		assert.Equal(t, recent, idsOf(t, r.Body.Bytes()))
		assert.Equal(t, "true", r.Header().Get("X-Window-Defaulted"))
	}
	r := api.Get("/api/dispatch-jobs/list-raw?since=" + rfc(time.Now().Add(-90*24*time.Hour)))
	assert.Equal(t, http.StatusBadRequest, r.Code)
	assert.Contains(t, r.Body.String(), "WINDOW_TOO_LARGE")
}

func TestWindow_ExemptLookupsAreNotWindowed(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	ctx := context.Background()
	old := seedJobsAt(t, pool, "wgex1", "wgwin:exempt:job:x", "FAILED", nil, 1, time.Now().UTC().Add(-72*time.Hour))[0]
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs_read SET event_id = 'evt_wgex0001' WHERE id = $1`, old)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET payload = '{}' WHERE id = $1`, old)
	require.NoError(t, err)
	api := serveAs(t, s, viewPerm, viewRawPerm)

	for _, path := range []string{
		"/api/dispatch-jobs/" + old,
		"/api/dispatch-jobs/" + old + "/raw",
		"/api/dispatch-jobs/" + old + "/attempts",
		"/api/dispatch-jobs/event/evt_wgex0001",
		"/api/dispatch-jobs/by-event/evt_wgex0001",
		"/bff/dispatch-jobs/" + old,
	} {
		r := api.Get(path)
		assert.Equal(t, http.StatusOK, r.Code, "%s: %s", path, r.Body.String())
		assert.Empty(t, r.Header().Get("X-Window-From"), "%s is keyed, not windowed", path)
	}
	r := api.Get("/api/dispatch-jobs/event/evt_wgex0001")
	assert.Equal(t, []string{old}, idsOf(t, r.Body.Bytes()))
}

func TestWindow_FilterOptionsAreWindowed(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	seedJobs(t, pool, "wgfo1", "wgfilt:recent:job:x", "FAILED", nil, 1)
	seedJobsAt(t, pool, "wgfo2", "wgfilt:old:job:x", "FAILED", nil, 1, time.Now().UTC().Add(-72*time.Hour))

	out, err := s.filterOptions(callerWith(viewPerm), &filterOptionsInput{})
	require.NoError(t, err)
	assert.Contains(t, out.Body.Codes, "wgfilt:recent:job:x")
	assert.NotContains(t, out.Body.Codes, "wgfilt:old:job:x", "outside the default 24 hours")
	assert.True(t, out.Body.Window.Defaulted)
	assert.InDelta(t, 24*time.Hour.Seconds(), out.Body.Window.To.Sub(out.Body.Window.From).Seconds(), 2)

	out, err = s.filterOptions(callerWith(viewPerm), &filterOptionsInput{
		Since: time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339),
	})
	require.NoError(t, err)
	assert.Contains(t, out.Body.Codes, "wgfilt:old:job:x")
	assert.Contains(t, out.Body.Codes, "wgfilt:recent:job:x")
	assert.False(t, out.Body.Window.Defaulted)

	_, err = s.filterOptions(callerWith(viewPerm), &filterOptionsInput{
		Since: time.Now().Add(-60 * 24 * time.Hour).UTC().Format(time.RFC3339),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "WINDOW_TOO_LARGE")
	_, err = s.filterOptions(callerWith(viewPerm), &filterOptionsInput{Since: "last tuesday"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_TIMESTAMP")
}

func TestWindow_PreviewAndRequeueByFilterAreWindowed(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	const code = "wgwin:bulk:job:x"
	recent := seedJobs(t, pool, "wgbr1", code, "FAILED", nil, 2)
	old := seedJobsAt(t, pool, "wgbo1", code, "FAILED", nil, 3, time.Now().UTC().Add(-72*time.Hour))
	ctx := callerWith(auth.DispatchJobRequeuePerm)

	// Default window: only the two recent jobs are counted and reset.
	pv := preview(t, s, ctx, DispatchJobFilter{Codes: code})
	assert.Equal(t, int64(2), pv.Count)
	assert.Equal(t, int64(2), pv.EligibleCount)
	assert.True(t, pv.Window.Defaulted)
	assert.InDelta(t, 24*time.Hour.Seconds(), pv.Window.To.Sub(pv.Window.From).Seconds(), 2)

	out, err := s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: DispatchJobFilter{Codes: code}, ConfirmCount: 2,
	}})
	require.NoError(t, err)
	assert.EqualValues(t, 2, out.Body.Requeued)
	assert.True(t, out.Body.Window.Defaulted)
	assert.Equal(t, map[string]int{"PENDING": 2}, statusesOf(t, pool, recent))
	assert.Equal(t, map[string]int{"FAILED": 3}, statusesOf(t, pool, old), "outside the window: untouched")

	// An explicit window reaches the old ones.
	f := DispatchJobFilter{
		Codes: code, Since: time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339),
		Until: time.Now().Add(-2 * 24 * time.Hour).UTC().Format(time.RFC3339),
	}
	pv = preview(t, s, ctx, f)
	assert.Equal(t, int64(3), pv.Count)
	assert.False(t, pv.Window.Defaulted)
	out, err = s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{DispatchJobFilter: f, ConfirmCount: 3}})
	require.NoError(t, err)
	assert.EqualValues(t, 3, out.Body.Requeued)
	assert.Equal(t, map[string]int{"PENDING": 3}, statusesOf(t, pool, old))

	// Too large, on both.
	wide := DispatchJobFilter{Codes: code, Since: time.Now().Add(-60 * 24 * time.Hour).UTC().Format(time.RFC3339)}
	_, err = s.requeueByFilterPreview(ctx, &apicommon.In[RequeueByFilterPreviewRequest]{Body: RequeueByFilterPreviewRequest{wide}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "WINDOW_TOO_LARGE")
	_, err = s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{DispatchJobFilter: wide, ConfirmCount: 1}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "WINDOW_TOO_LARGE")
}

// ── the status guard ───────────────────────────────────────────────────

type guardJobs struct {
	pending, failed, cancelled, expired, queued, processing, completed string
	requeueable                                                        []string
}

func seedGuard(t *testing.T, pool *pgxpool.Pool, prefix string) guardJobs {
	t.Helper()
	one := func(suffix, status string) string {
		return seedJobs(t, pool, prefix+suffix, "wgguard:"+prefix+":job:x", status, nil, 1)[0]
	}
	g := guardJobs{
		pending: one("p", "PENDING"), failed: one("f", "FAILED"), cancelled: one("c", "CANCELLED"), expired: one("e", "EXPIRED"),
		queued: one("q", "QUEUED"), processing: one("r", "PROCESSING"), completed: one("d", "COMPLETED"),
	}
	g.requeueable = []string{g.pending, g.failed, g.cancelled, g.expired}
	return g
}

func (g guardJobs) all() []string {
	return append(append([]string{}, g.requeueable...), g.queued, g.processing, g.completed)
}

func requeueIDs(s *State, ctx context.Context, req RequeueRequest) (RequeueResponse, error) {
	out, err := s.requeue(ctx, &apicommon.In[RequeueRequest]{Body: req})
	if err != nil {
		return RequeueResponse{}, err
	}
	return out.Body, nil
}

func statusOf(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var st string
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&st))
	return st
}

// QUEUED and PROCESSING are never requeued, COMPLETED only by a doubly
// confirmed redrive; skipped jobs are reported, never an error for the batch.
func TestRequeueGuard_ByIDs(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	g := seedGuard(t, pool, "wgg1")
	ctx := callerWith(auth.DispatchJobRequeuePerm)

	res, err := requeueIDs(s, ctx, RequeueRequest{IDs: g.all()})
	require.NoError(t, err)
	assert.EqualValues(t, 4, res.Requeued)
	assert.EqualValues(t, 0, res.Redriven)
	assert.EqualValues(t, 2, res.SkippedInFlight)
	assert.EqualValues(t, 1, res.SkippedCompleted)
	reasons := map[string]string{}
	for _, sk := range res.Skipped {
		reasons[sk.ID] = sk.Reason
		assert.NotEmpty(t, sk.Status)
	}
	assert.Equal(t, map[string]string{
		g.queued: dispatchjob.SkipInFlight, g.processing: dispatchjob.SkipInFlight, g.completed: dispatchjob.SkipCompletedNotConfirmed,
	}, reasons)
	for _, id := range g.requeueable {
		assert.Equal(t, "PENDING", statusOf(t, pool, id))
	}
	assert.Equal(t, "QUEUED", statusOf(t, pool, g.queued))
	assert.Equal(t, "PROCESSING", statusOf(t, pool, g.processing))
	assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))

	// An unrelated, in-flight-only batch is not an error either.
	res, err = requeueIDs(s, ctx, RequeueRequest{IDs: []string{g.queued}})
	require.NoError(t, err)
	assert.EqualValues(t, 0, res.Requeued)
	assert.EqualValues(t, 1, res.SkippedInFlight)
}

func TestRequeueGuard_RedriveNeedsBothFieldsAndThePermission(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	g := seedGuard(t, pool, "wgg2")
	both := callerWith(auth.DispatchJobRequeuePerm, auth.DispatchJobRedriveCompletedPerm)

	// The flag alone, or the word alone: COMPLETED is skipped and reported.
	for _, req := range []RequeueRequest{
		{IDs: []string{g.completed}, RedriveCompleted: true},
		{IDs: []string{g.completed}, ConfirmRedriveCompleted: "REDRIVE"},
		{IDs: []string{g.completed}, RedriveCompleted: true, ConfirmRedriveCompleted: "redrive"},
	} {
		res, err := requeueIDs(s, both, req)
		require.NoError(t, err)
		assert.EqualValues(t, 0, res.Requeued)
		assert.EqualValues(t, 1, res.SkippedCompleted)
		assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))
	}

	// Asking to redrive without the redrive permission is a 403, not a quiet skip.
	_, err := requeueIDs(s, callerWith(auth.DispatchJobRequeuePerm),
		RequeueRequest{IDs: []string{g.completed}, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	// ... and redrive-completed alone is not a requeue permission.
	_, err = requeueIDs(s, callerWith(auth.DispatchJobRedriveCompletedPerm),
		RequeueRequest{IDs: []string{g.completed}, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))

	// Both fields and both permissions: redriven, with the in-flight still skipped.
	res, err := requeueIDs(s, both, RequeueRequest{
		IDs: append(g.all(), "nosuchjob"), RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE",
	})
	require.NoError(t, err)
	assert.EqualValues(t, 5, res.Requeued, "4 ordinary + 1 redriven")
	assert.EqualValues(t, 1, res.Redriven)
	assert.EqualValues(t, 2, res.SkippedInFlight)
	assert.EqualValues(t, 0, res.SkippedCompleted)
	assert.Equal(t, "PENDING", statusOf(t, pool, g.completed))
	assert.Equal(t, "QUEUED", statusOf(t, pool, g.queued))
	assert.Equal(t, "PROCESSING", statusOf(t, pool, g.processing))
	var attempts int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT attempt_count FROM msg_dispatch_jobs WHERE id = $1`, g.completed).Scan(&attempts))
	assert.Zero(t, attempts, "a redriven job gets a fresh retry budget")

	// The audit trail tells a redrive apart: the event and the audit row say so.
	var redrive bool
	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT (data->>'redrive')::bool, (data->>'redrivenCount')::int FROM msg_events
		  WHERE type = $1 AND data->'ids' ? $2`, operations.DispatchJobsResentType, g.completed).Scan(&redrive, &count))
	assert.True(t, redrive)
	assert.Equal(t, 1, count)
	var flagged bool
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT (operation_json->>'redriveCompleted')::bool FROM aud_logs
		  WHERE operation = 'ResendCommand' AND operation_json->'ids' ? $1 AND operation_json->>'redriveCompleted' = 'true'`, g.completed).Scan(&flagged))
	assert.True(t, flagged, "the audit row carries redriveCompleted=true")
	// An ordinary requeue's event says redrive=false.
	r2, err := requeueIDs(s, both, RequeueRequest{IDs: []string{g.failed}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, r2.Requeued)
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT (data->>'redrive')::bool FROM msg_events WHERE type = $1 AND data->'ids' ? $2 ORDER BY id DESC LIMIT 1`,
		operations.DispatchJobsResentType, g.failed).Scan(&redrive))
	assert.False(t, redrive)
}

// A job that left the requeueable set after it was read is refused in SQL, not
// reset: the lifecycle guard is the last line.
func TestRequeueGuard_SQLRefusesInFlightAndCompleted(t *testing.T) {
	pool := testpg.Pool(t)
	g := seedGuard(t, pool, "wgg3")
	lc := dispatchjob.NewLifecycle(pool)
	created := func(id string) time.Time {
		var c time.Time
		require.NoError(t, pool.QueryRow(context.Background(), `SELECT created_at FROM msg_dispatch_jobs WHERE id = $1`, id).Scan(&c))
		return c
	}
	for _, id := range []string{g.queued, g.processing, g.completed} {
		ok, err := lc.Requeue(context.Background(), id, created(id))
		require.NoError(t, err)
		assert.False(t, ok, statusOf(t, pool, id))
	}
	ok, err := lc.Redrive(context.Background(), g.failed, created(g.failed))
	require.NoError(t, err)
	assert.False(t, ok, "redrive only moves COMPLETED jobs")
	ok, err = lc.Redrive(context.Background(), g.completed, created(g.completed))
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRequeueGuard_ByFilter(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	g := seedGuard(t, pool, "wgg4")
	f := DispatchJobFilter{Codes: "wgguard:wgg4:job:x"}
	ctx := callerWith(auth.DispatchJobRequeuePerm, auth.DispatchJobRedriveCompletedPerm)

	pv := preview(t, s, ctx, f)
	assert.EqualValues(t, 7, pv.Count)
	assert.EqualValues(t, 4, pv.EligibleCount, "pending, failed, cancelled, expired")
	assert.EqualValues(t, 1, pv.CompletedCount, "reported separately so a UI can ask twice")
	assert.EqualValues(t, 2, pv.InFlightCount)
	assert.EqualValues(t, 5, pv.RedriveEligibleCount)

	// An ordinary run: completed and in-flight are skipped and counted.
	res, err := s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{DispatchJobFilter: f, ConfirmCount: 4}})
	require.NoError(t, err)
	assert.EqualValues(t, 4, res.Body.Requeued)
	assert.EqualValues(t, 0, res.Body.Redriven)
	assert.EqualValues(t, 2, res.Body.SkippedInFlight)
	assert.EqualValues(t, 1, res.Body.SkippedCompleted)
	assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))
	assert.Equal(t, "QUEUED", statusOf(t, pool, g.queued))

	// Redrive asked for but only half-confirmed: completed stays COMPLETED.
	seedGuard2 := seedJobs(t, pool, "wgg4x", f.Codes, "FAILED", nil, 1)
	_ = seedGuard2
	res, err = s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: f, ConfirmCount: 10, RedriveCompleted: true,
	}})
	require.NoError(t, err)
	assert.EqualValues(t, 0, res.Body.Redriven)
	assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))

	// A confirmed redrive confirms eligible + completed; fewer than that is a 409.
	syncProjection(t, pool, "wgguard:wgg4:job:x")
	pv = preview(t, s, ctx, f)
	_, err = s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: f, ConfirmCount: pv.EligibleCount, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE",
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "FILTER_MATCH_CHANGED")
	assert.Equal(t, "COMPLETED", statusOf(t, pool, g.completed))
	res, err = s.requeueByFilter(ctx, &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: f, ConfirmCount: pv.RedriveEligibleCount, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE",
	}})
	require.NoError(t, err)
	assert.EqualValues(t, 1, res.Body.Redriven)
	assert.Equal(t, "PENDING", statusOf(t, pool, g.completed))
	assert.Equal(t, "QUEUED", statusOf(t, pool, g.queued), "in flight is never reset, even by a redrive")
	assert.Equal(t, "PROCESSING", statusOf(t, pool, g.processing))

	// Without the redrive permission, asking is a 403 and nothing runs.
	_, err = s.requeueByFilter(callerWith(auth.DispatchJobRequeuePerm), &apicommon.In[RequeueByFilterRequest]{Body: RequeueByFilterRequest{
		DispatchJobFilter: f, ConfirmCount: 99, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE",
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}

// ── the permission split ───────────────────────────────────────────────

// Each write permission, held alone, allows exactly its own action; view
// allows no write.
func TestPermissionMatrix(t *testing.T) {
	pool := testpg.Pool(t)
	s := newState(pool)
	n := 0
	seedFailed := func(string) string {
		n++
		prefix := fmt.Sprintf("wgp%02d", n)
		id := seedJobs(t, pool, prefix, "wgperm:"+prefix+":job:x", "FAILED", nil, 1)[0]
		_, err := pool.Exec(context.Background(), `UPDATE msg_dispatch_jobs SET payload = '{}' WHERE id = $1`, id)
		require.NoError(t, err)
		return id
	}
	const (
		requeue, redrive, cancel, complete, edit = "requeue", "redrive", "cancel", "complete", "edit-payload"
	)
	perms := map[string]string{
		requeue: auth.DispatchJobRequeuePerm, redrive: auth.DispatchJobRedriveCompletedPerm, cancel: auth.DispatchJobCancelPerm,
		complete: auth.DispatchJobCompletePerm, edit: auth.DispatchJobEditPayloadPerm,
	}
	// Each action is tried by a caller with a permission set; it reports whether
	// it was allowed (not 403).
	try := func(action string, ctx context.Context) bool {
		var err error
		switch action {
		case requeue:
			id := seedFailed("")
			_, err = requeueIDs(s, ctx, RequeueRequest{IDs: []string{id}})
		case redrive:
			id := seedFailed("")
			_, err = requeueIDs(s, ctx, RequeueRequest{IDs: []string{id}, RedriveCompleted: true, ConfirmRedriveCompleted: "REDRIVE"})
		case cancel:
			id := seedFailed("")
			_, err = s.cancel(ctx, &apicommon.IDInput{ID: id})
		case complete:
			id := seedFailed("")
			_, err = s.complete(ctx, &apicommon.IDInput{ID: id})
		case edit:
			id := seedFailed("")
			_, err = s.editPayload(ctx, &editPayloadInput{ID: id, Body: EditPayloadRequest{Payload: `{"a":1}`}})
		}
		if err == nil {
			return true
		}
		ue := usecase.AsError(err)
		require.NotNil(t, ue, err)
		require.Equal(t, usecase.KindAuthorization, ue.Kind, "%s failed for a reason other than permission: %v", action, err)
		return false
	}

	actions := []string{requeue, redrive, cancel, complete, edit}
	for _, holds := range actions {
		for _, action := range actions {
			allowed := try(action, callerWith(perms[holds]))
			// redrive-completed alone is not enough for a redrive request: it also
			// resets what is not COMPLETED, so it needs requeue too.
			assert.Equal(t, holds == action && action != redrive, allowed, "holding only %s, attempting %s", holds, action)
		}
	}
	assert.True(t, try(redrive, callerWith(perms[requeue], perms[redrive])), "requeue + redrive-completed allows a redrive")
	for _, action := range actions {
		assert.False(t, try(action, callerWith(viewPerm, viewRawPerm)), "view alone must not allow %s", action)
	}
	// The wildcard (super-admin) holds all of them.
	assert.True(t, try(redrive, callerWith("platform:*:*:*")))
}

// The preview is part of requeueing and is also a read of counts: requeue or
// view opens it, nothing else does.
func TestPermissionMatrix_Preview(t *testing.T) {
	s := newState(testpg.Pool(t))
	for _, p := range []string{auth.DispatchJobRequeuePerm, viewPerm} {
		_, err := s.requeueByFilterPreview(callerWith(p), &apicommon.In[RequeueByFilterPreviewRequest]{
			Body: RequeueByFilterPreviewRequest{DispatchJobFilter{Codes: "wgperm:none:job:x"}},
		})
		require.NoError(t, err, p)
	}
	for _, p := range []string{auth.DispatchJobCancelPerm, auth.DispatchJobCompletePerm, auth.DispatchJobEditPayloadPerm, auth.DispatchJobRedriveCompletedPerm} {
		_, err := s.requeueByFilterPreview(callerWith(p), &apicommon.In[RequeueByFilterPreviewRequest]{})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	}
}

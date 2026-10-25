//go:build integration

// trace_pg_test.go covers the trace endpoints
// (docs/function-trace-design.md section 7): permissions, the window caps and
// unknown-context errors, the record filters and keyset cursor, and the body
// redaction for a viewer without view-bodies.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func ctxWith(perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_traceapitest", Scope: auth.ScopeAnchor, Permissions: perms,
	})
}

const (
	permView   = "platform:function:trace:view"
	permManage = "platform:function:trace:manage"
	permBodies = "platform:function:trace:view-bodies"
)

type traceFixture struct {
	t  *testing.T
	s  *State
	fn *function.Function
}

func newTraceFixture(t *testing.T, seed string) *traceFixture {
	t.Helper()
	pool := testpg.Pool(t)
	repo := function.NewRepository(pool)
	appID, appCode := "app_trapi"+seed, "trapi"+seed
	_, err := pool.Exec(context.Background(), `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2) ON CONFLICT (id) DO NOTHING`, appID, appCode)
	require.NoError(t, err)
	fn := function.New(appID, appCode, "fn")
	tx, err := pool.Begin(context.Background())
	require.NoError(t, err)
	require.NoError(t, repo.Persist(context.Background(), fn, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(context.Background()))

	d := `{"abi":1,"endpoints":[{"method":"POST","path":"/x","auth":"webhook"}],"traceContexts":[{"name":"orders","description":"What the order mapping decided."}]}`
	require.NoError(t, repo.InsertVersion(context.Background(), &function.Version{
		ID: tsid.Generate(tsid.FunctionVersion), FunctionID: fn.ID, Number: 1, Digest: fmt.Sprintf("%064x", time.Now().UnixNano()),
		SizeBytes: 1, ABI: 1, Runtime: "js", Describe: []byte(d), Status: function.VersionReady, CreatedAt: time.Now().UTC(),
	}))
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = pool.Exec(bg, `DELETE FROM fng_trace_log WHERE function_id = $1`, fn.ID)
		_, _ = pool.Exec(bg, `DELETE FROM fng_trace_contexts WHERE function_id = $1`, fn.ID)
		_, _ = pool.Exec(bg, `DELETE FROM fng_versions WHERE function_id = $1`, fn.ID)
		_, _ = pool.Exec(bg, `DELETE FROM fng_functions WHERE id = $1`, fn.ID)
		_, _ = pool.Exec(bg, `DELETE FROM app_applications WHERE id = $1`, appID)
	})
	return &traceFixture{t: t, s: &State{Repo: repo, UoW: testpg.NewUoW(t)}, fn: fn}
}

func (f *traceFixture) put(ctx context.Context, name string, minutes, sample int) (*TraceContextResponse, error) {
	out, err := f.s.putTrace(ctx, &putTraceInput{ID: f.fn.ID, Context: name, Body: EnableTraceRequest{Minutes: minutes, SampleRate: sample}})
	if err != nil {
		return nil, err
	}
	return &out.Body, nil
}

func TestTraceAPI_ListContexts(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "list001")

	_, err := f.s.getTrace(ctxWith(permManage), &getTraceInput{ID: f.fn.ID})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	out, err := f.s.getTrace(ctxWith(permView), &getTraceInput{ID: f.fn.ID})
	require.NoError(t, err)
	b, _ := json.Marshal(out.Body)
	t.Logf("GET /trace -> %s", b)
	require.Len(t, out.Body.Contexts, 2)
	assert.Equal(t, "http", out.Body.Contexts[0].Name)
	assert.True(t, out.Body.Contexts[0].BuiltIn)
	assert.Equal(t, 15, out.Body.Contexts[0].MaxMinutes)
	assert.Nil(t, out.Body.Contexts[0].EnabledUntil)
	assert.Equal(t, "orders", out.Body.Contexts[1].Name)
	assert.Equal(t, 60, out.Body.Contexts[1].MaxMinutes)
	assert.False(t, out.Body.Contexts[1].Live)

	_, err = f.s.getTrace(ctxWith(permView), &getTraceInput{ID: "fnc_missing"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function_NOT_FOUND")
}

func TestTraceAPI_EnableAndDisable_Permissions(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "perm0001")

	_, err := f.put(ctxWith(permView), "orders", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	// manage alone switches a declared context on, not http.
	out, err := f.put(ctxWith(permManage), "orders", 5, 1)
	require.NoError(t, err)
	assert.True(t, out.Live)
	_, err = f.put(ctxWith(permManage), "http", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.put(ctxWith(permBodies), "http", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	out, err = f.put(ctxWith(permManage, permBodies), "http", 5, 3)
	require.NoError(t, err)
	b, _ := json.Marshal(out)
	t.Logf("PUT /trace/http -> %s", b)
	assert.Equal(t, 3, out.SampleRate)
	assert.True(t, out.BuiltIn)

	_, err = f.s.deleteTrace(ctxWith(permView), &deleteTraceInput{ID: f.fn.ID, Context: "http"})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.s.deleteTrace(ctxWith(permManage), &deleteTraceInput{ID: f.fn.ID, Context: "http"})
	require.NoError(t, err, "disabling needs manage only")
	_, err = f.s.deleteTrace(ctxWith(permManage), &deleteTraceInput{ID: f.fn.ID, Context: "http"})
	require.NoError(t, err, "and is idempotent")

	list, err := f.s.getTrace(ctxWith(permView), &getTraceInput{ID: f.fn.ID})
	require.NoError(t, err)
	assert.False(t, list.Body.Contexts[0].Live)
	require.NotNil(t, list.Body.Contexts[0].EnabledUntil, "closed at the switch-off time")
	assert.False(t, list.Body.Contexts[0].EnabledUntil.Underlying().After(time.Now()))
	assert.True(t, list.Body.Contexts[1].Live, "orders is still on")
}

func TestTraceAPI_Enable_Indefinite(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "indef001")
	const permIndefinite = "platform:function:trace:indefinite"
	put := func(ctx context.Context, name string) (*TraceContextResponse, error) {
		out, err := f.s.putTrace(ctx, &putTraceInput{ID: f.fn.ID, Context: name, Body: EnableTraceRequest{Indefinite: true}})
		if err != nil {
			return nil, err
		}
		return &out.Body, nil
	}

	_, err := put(ctxWith(permManage), "orders")
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = put(ctxWith(permManage, permBodies, permIndefinite), "http")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_INDEFINITE_NOT_ALLOWED")

	f.s.Trace = function.TraceLimits{IndefiniteMaxDays: 2}
	before := time.Now()
	out, err := put(ctxWith(permManage, permIndefinite), "orders")
	require.NoError(t, err)
	assert.True(t, out.Indefinite)
	assert.True(t, out.Live)
	assert.Equal(t, 2, out.IndefiniteMaxDays)
	require.NotNil(t, out.EnabledUntil)
	assert.WithinDuration(t, before.Add(48*time.Hour), out.EnabledUntil.Underlying(), 10*time.Second, "the configured ceiling")

	list, err := f.s.getTrace(ctxWith(permView), &getTraceInput{ID: f.fn.ID})
	require.NoError(t, err)
	assert.Equal(t, 0, list.Body.Contexts[0].IndefiniteMaxDays, "http never takes one")
	assert.Equal(t, 2, list.Body.Contexts[1].IndefiniteMaxDays)
	assert.True(t, list.Body.Contexts[1].Indefinite)
}

func TestTraceAPI_Enable_Errors(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "err00001")
	all := ctxWith(permManage, permBodies)

	_, err := f.put(all, "orders", 61, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_WINDOW_TOO_LONG")
	_, err = f.put(all, "http", 16, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_WINDOW_TOO_LONG")
	_, err = f.put(all, "unknown", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_CONTEXT_UNKNOWN")

	// The operator-set caps apply.
	f.s.Trace = function.TraceLimits{MaxMinutes: 10, HTTPMaxMinutes: 2, RetentionDays: 7}
	_, err = f.put(all, "orders", 11, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_WINDOW_TOO_LONG")
	out, err := f.put(all, "http", 2, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, out.MaxMinutes)
}

func TestTraceAPI_Enable_PrunesOldRecords(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "prune001")
	now := time.Now().UTC()
	require.NoError(t, f.s.Repo.InsertTraceRecords(context.Background(), []function.TraceLogRecord{
		{ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1, InvocationID: "old", Context: "http", Kind: "log", Level: "info", Message: "old", At: now, CreatedAt: now.Add(-31 * 24 * time.Hour)},
		{ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1, InvocationID: "new", Context: "http", Kind: "log", Level: "info", Message: "new", At: now.Add(-40 * 24 * time.Hour), CreatedAt: now.Add(-time.Hour)},
	}))
	_, err := f.put(ctxWith(permManage, permBodies), "http", 5, 1)
	require.NoError(t, err)
	// The prune runs in the background after the request.
	assert.Eventually(t, func() bool {
		n, err := f.s.Repo.CountTraceLog(context.Background(), f.fn.ID)
		return err == nil && n == 1
	}, 30*time.Second, 20*time.Millisecond, "an enable prunes the function's records stored past retention")
}

func (f *traceFixture) seedRecords() {
	f.t.Helper()
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	httpData := `{"request":{"method":"POST","path":"/orders","rawQuery":"tenant=acme","caller":{"kind":"principal","id":"prn_x"},"headers":{"content-type":["application/json"],"authorization":["[redacted]"]},"body":"{\"secret\":1}","bodyEncoding":"utf8","bodyBytes":12,"bodyTruncated":false},` +
		`"response":{"status":200,"headers":{},"body":"ok","bodyEncoding":"utf8","bodyBytes":2,"bodyTruncated":false},"durationMs":12,"outcome":"ok"}`
	corr, ref, runner := "corr-1", "TRIP-1", "runner-1"
	var recs []function.TraceLogRecord
	for i := range 7 {
		r := function.TraceLogRecord{
			ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1 + int32(i%2), InvocationID: fmt.Sprintf("inv-%d", i),
			Context: "orders", Kind: "trace", Level: "info", Message: fmt.Sprintf("m%d", i), Data: []byte(`{"i":` + fmt.Sprint(i) + `}`),
			At: base.Add(time.Duration(i) * time.Second), RunnerID: &runner, CreatedAt: time.Now().UTC(),
		}
		if i%2 == 0 {
			r.Reference, r.CorrelationID = &ref, &corr
		}
		if i == 3 {
			r.Level = "error"
		}
		if i == 5 {
			r.Context, r.Kind, r.Data = "http", "http", []byte(httpData)
		}
		recs = append(recs, r)
	}
	// Two records at the very same instant: the cursor must not skip or repeat either.
	for i := range 2 {
		recs = append(recs, function.TraceLogRecord{
			ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1, InvocationID: "tie",
			Context: "orders", Kind: "trace", Level: "info", Message: fmt.Sprintf("tie%d", i),
			At: base.Add(10 * time.Second), CreatedAt: time.Now().UTC(),
		})
	}
	require.NoError(f.t, f.s.Repo.InsertTraceRecords(context.Background(), recs))
}

func (f *traceFixture) log(ctx context.Context, mut func(*traceLogInput)) *TraceLogResponse {
	f.t.Helper()
	in := &traceLogInput{ID: f.fn.ID}
	if mut != nil {
		mut(in)
	}
	out, err := f.s.getTraceLog(ctx, in)
	require.NoError(f.t, err)
	return &out.Body
}

func messages(r *TraceLogResponse) []string {
	var out []string
	for _, rec := range r.Records {
		out = append(out, rec.Message)
	}
	return out
}

func TestTraceAPI_Log_NeedsViewPermission(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "logperm1")
	_, err := f.s.getTraceLog(ctxWith(permManage), &traceLogInput{ID: f.fn.ID})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	_, err = f.s.getTraceLog(ctxWith(permView), &traceLogInput{ID: "fnc_missing"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function_NOT_FOUND")
}

func TestTraceAPI_Log_FiltersNewestFirst(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "logflt01")
	f.seedRecords()
	v := ctxWith(permView, permBodies)

	all := f.log(v, nil)
	require.Len(t, all.Records, 9)
	assert.Nil(t, all.NextCursor)
	for i := 1; i < len(all.Records); i++ {
		assert.False(t, all.Records[i].At.Underlying().After(all.Records[i-1].At.Underlying()), "newest first")
	}

	assert.ElementsMatch(t, []string{"m0", "m2", "m4", "m6"}, messages(f.log(v, func(in *traceLogInput) { in.Reference = "TRIP-1" })))
	assert.ElementsMatch(t, []string{"m0", "m2", "m4", "m6"}, messages(f.log(v, func(in *traceLogInput) { in.CorrelationID = "corr-1" })))
	assert.Equal(t, []string{"m3"}, messages(f.log(v, func(in *traceLogInput) { in.Level = "error" })))
	assert.Equal(t, []string{"m5"}, messages(f.log(v, func(in *traceLogInput) { in.Kind = "http" })))
	assert.Equal(t, []string{"m4"}, messages(f.log(v, func(in *traceLogInput) { in.InvocationID = "inv-4" })))
	assert.ElementsMatch(t, []string{"m5"}, messages(f.log(v, func(in *traceLogInput) { in.Context = "http" })))
	assert.ElementsMatch(t, []string{"m1", "m3", "m5"}, messages(f.log(v, func(in *traceLogInput) { in.Version = 2 })))
	assert.ElementsMatch(t, []string{"tie0", "tie1"}, messages(f.log(v, func(in *traceLogInput) { in.InvocationID = "tie" })))

	base := time.Now().UTC().Add(-time.Hour)
	since := base.Add(4500 * time.Millisecond)
	for name, mut := range map[string]func(*traceLogInput){
		"rfc3339": func(in *traceLogInput) { in.Since = since.Format(time.RFC3339Nano) },
		"unixms":  func(in *traceLogInput) { in.Since = fmt.Sprint(since.UnixMilli()) },
	} {
		assert.ElementsMatch(t, []string{"m5", "m6", "tie0", "tie1"}, messages(f.log(v, mut)), name)
	}
	assert.ElementsMatch(t, []string{"m0", "m1", "m2", "m3", "m4"}, messages(f.log(v, func(in *traceLogInput) { in.Until = since.Format(time.RFC3339Nano) })))

	_, err := f.s.getTraceLog(v, &traceLogInput{ID: f.fn.ID, Since: "yesterday"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_TIME_INVALID")
	_, err = f.s.getTraceLog(v, &traceLogInput{ID: f.fn.ID, Cursor: "garbage!"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_CURSOR_INVALID")
}

func TestTraceAPI_Log_KeysetCursorPagesWithoutSkippingOrRepeating(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "logcur01")
	f.seedRecords()
	v := ctxWith(permView, permBodies)

	var seen []string
	cursor := ""
	pages := 0
	for {
		page := f.log(v, func(in *traceLogInput) { in.Limit = 2; in.Cursor = cursor })
		pages++
		assert.LessOrEqual(t, len(page.Records), 2)
		for _, r := range page.Records {
			seen = append(seen, r.ID)
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
		require.Less(t, pages, 20)
	}
	assert.Equal(t, 5, pages, "9 records in pages of 2")
	assert.Len(t, seen, 9)
	uniq := map[string]bool{}
	for _, id := range seen {
		uniq[id] = true
	}
	assert.Len(t, uniq, 9, "no record repeated or skipped, including the two at the same instant")

	full := f.log(v, nil)
	var want []string
	for _, r := range full.Records {
		want = append(want, r.ID)
	}
	assert.Equal(t, want, seen, "paging yields the same order as one big page")

	assert.Len(t, f.log(v, func(in *traceLogInput) { in.Limit = 9 }).Records, 9)
	assert.Nil(t, f.log(v, func(in *traceLogInput) { in.Limit = 9 }).NextCursor, "an exactly-full last page has no next cursor")
	assert.Len(t, f.log(v, func(in *traceLogInput) { in.Limit = 100000 }).Records, 9, "limit is capped, not an error")
}

func TestTraceAPI_Log_WithholdsSensitiveDetailWithoutViewBodies(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "logred01")
	f.seedRecords()
	require.NoError(t, f.s.Repo.InsertTraceRecords(context.Background(), []function.TraceLogRecord{{
		ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1, InvocationID: "inv-log", Context: "http",
		Kind: "log", Level: "warn", Message: "card 4111-1111", Data: []byte(`{"user":"u1"}`),
		At: time.Now().UTC().Add(-time.Minute), CreatedAt: time.Now().UTC(),
	}}))

	find := func(r *TraceLogResponse, kind string) TraceRecordResponse {
		for _, rec := range r.Records {
			if rec.Kind == kind {
				return rec
			}
		}
		t.Fatal("no " + kind + " record")
		return TraceRecordResponse{}
	}

	full := find(f.log(ctxWith(permView, permBodies), func(in *traceLogInput) { in.Kind = "http" }), "http")
	assert.False(t, full.BodiesRedacted)
	assert.False(t, full.Redacted)
	var fd map[string]any
	require.NoError(t, json.Unmarshal(full.Data, &fd))
	assert.Equal(t, `{"secret":1}`, fd["request"].(map[string]any)["body"])
	assert.Equal(t, "tenant=acme", fd["request"].(map[string]any)["rawQuery"])
	assert.Equal(t, "ok", fd["response"].(map[string]any)["body"])
	assert.Equal(t, "card 4111-1111", find(f.log(ctxWith(permView, permBodies), nil), "log").Message)

	red := find(f.log(ctxWith(permView), func(in *traceLogInput) { in.Kind = "http" }), "http")
	assert.True(t, red.BodiesRedacted)
	assert.True(t, red.Redacted)
	var rd map[string]any
	require.NoError(t, json.Unmarshal(red.Data, &rd))
	req := rd["request"].(map[string]any)
	resp := rd["response"].(map[string]any)
	assert.Nil(t, req["body"])
	assert.Nil(t, resp["body"])
	assert.Contains(t, req, "body", "the key stays, its value is null")
	for _, k := range []string{"headers", "rawQuery", "caller", "bodyEncoding"} {
		assert.NotContains(t, req, k, "withheld from trace:view alone")
	}
	assert.NotContains(t, resp, "headers")
	assert.Equal(t, "POST", req["method"])
	assert.Equal(t, "/orders", req["path"])
	assert.EqualValues(t, 12, req["bodyBytes"])
	assert.EqualValues(t, 200, resp["status"])
	assert.EqualValues(t, 2, resp["bodyBytes"])
	assert.EqualValues(t, 12, rd["durationMs"])
	assert.Equal(t, "ok", rd["outcome"])

	// A log record keeps its metadata, not the printed line; an author trace
	// record keeps its message, not its data.
	logRec := find(f.log(ctxWith(permView), nil), "log")
	assert.True(t, logRec.Redacted)
	assert.False(t, logRec.BodiesRedacted)
	assert.Empty(t, logRec.Message)
	assert.JSONEq(t, "null", string(logRec.Data))
	assert.Equal(t, "warn", logRec.Level)
	assert.Equal(t, "inv-log", logRec.InvocationID)
	for _, rec := range f.log(ctxWith(permView), func(in *traceLogInput) { in.Kind = "trace" }).Records {
		assert.NotEmpty(t, rec.Message)
		assert.JSONEq(t, "null", string(rec.Data), rec.Message)
		assert.False(t, rec.BodiesRedacted, rec.Message)
	}
	// A bare record with no data has nothing to withhold.
	for _, rec := range f.log(ctxWith(permView), func(in *traceLogInput) { in.InvocationID = "tie" }).Records {
		assert.JSONEq(t, "null", string(rec.Data))
		assert.False(t, rec.Redacted)
	}
}

// ── Over HTTP ───────────────────────────────────────────────────────────

func traceServer(t *testing.T, s *State, perms ...string) *httptest.Server {
	t.Helper()
	httpcompat.Init()
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), &auth.AuthContext{
				PrincipalID: "prn_traceapihttp", Scope: auth.ScopeAnchor, Permissions: perms,
			})))
		})
	})
	Register(humachi.New(r, apiroute.PlatformAPIConfig("fn-trace-http", "test")), s)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func do(t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTraceAPI_OverHTTP_StatusesAndShapes(t *testing.T) {
	t.Parallel()
	f := newTraceFixture(t, "http0001")
	srv := traceServer(t, f.s, permView, permManage, permBodies)
	base := srv.URL + "/api/functions/" + f.fn.ID

	code, body := do(t, "PUT", base+"/trace/orders", `{"minutes":5,"sampleRate":2}`)
	t.Logf("PUT /trace/orders %d %s", code, body)
	assert.Equal(t, 200, code)
	assert.Contains(t, body, `"live":true`)

	code, body = do(t, "PUT", base+"/trace/orders", `{"minutes":500}`)
	t.Logf("PUT too long %d %s", code, body)
	assert.Equal(t, 400, code)
	assert.Contains(t, body, "TRACE_WINDOW_TOO_LONG")

	code, body = do(t, "PUT", base+"/trace/nope", `{"minutes":5}`)
	t.Logf("PUT unknown %d %s", code, body)
	assert.Equal(t, 400, code)
	assert.Contains(t, body, "TRACE_CONTEXT_UNKNOWN")

	code, _ = do(t, "PUT", base+"/trace/orders", `{}`)
	assert.Equal(t, 400, code, "minutes is required")

	code, body = do(t, "GET", base+"/trace", "")
	t.Logf("GET /trace %d %s", code, body)
	assert.Equal(t, 200, code)

	code, body = do(t, "DELETE", base+"/trace/orders", "")
	t.Logf("DELETE %d %q", code, body)
	assert.Equal(t, 204, code)

	f.seedRecords()
	code, body = do(t, "GET", base+"/trace-log?kind=http&limit=5", "")
	t.Logf("GET /trace-log %d %s", code, body)
	assert.Equal(t, 200, code)
	code, body = do(t, "GET", base+"/trace-log?limit=2", "")
	t.Logf("GET /trace-log page %d %s", code, body)
	assert.Contains(t, body, `"nextCursor":"`)

	viewOnly := traceServer(t, f.s, permView)
	code, body = do(t, "GET", viewOnly.URL+"/api/functions/"+f.fn.ID+"/trace-log?kind=http", "")
	t.Logf("GET /trace-log (no view-bodies) %d %s", code, body)
	assert.Equal(t, 200, code)
	assert.Contains(t, body, `"bodiesRedacted":true`)
	assert.NotContains(t, body, `secret`)

	noPerms := traceServer(t, f.s)
	code, body = do(t, "GET", noPerms.URL+"/api/functions/"+f.fn.ID+"/trace", "")
	t.Logf("no permission %d %s", code, body)
	assert.Equal(t, 403, code)
	code, _ = do(t, "GET", noPerms.URL+"/api/functions/"+f.fn.ID+"/trace-log", "")
	assert.Equal(t, 403, code)
	code, _ = do(t, "PUT", noPerms.URL+"/api/functions/"+f.fn.ID+"/trace/orders", `{"minutes":5}`)
	assert.Equal(t, 403, code)
	code, _ = do(t, "DELETE", noPerms.URL+"/api/functions/"+f.fn.ID+"/trace/orders", "")
	assert.Equal(t, 403, code)
}

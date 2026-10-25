//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// The trace switches and the trace log end to end against Postgres, through
// the use cases the API runs: enable and disable (the window stored, audited
// by the use case), the permission matrix on forged posts, and the bodies of
// http records withheld from a viewer without view-bodies.
func TestFunctionTraceAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	repo := function.NewRepository(pool)
	limits := function.DefaultTraceLimits()

	appID := testpg.EnsureApplication(t, "uitraceapp")
	f := function.New(appID, "uitraceapp", "trace-page")
	tx, err := uow.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.Persist(ctx, f, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))

	const secret = "pg-SECRET-BODY-7"
	now := time.Now().UTC().Truncate(time.Microsecond)
	data := []byte(`{"request":{"method":"POST","path":"/p","headers":{},"body":"{\"k\":\"` + secret + `\"}"},"response":{"status":201,"headers":{},"body":"` + secret + `-out"},"durationMs":3,"outcome":"ok"}`)
	require.NoError(t, repo.InsertTraceRecords(ctx, []function.TraceLogRecord{
		{ID: tsid.Generate(tsid.Application), FunctionID: f.ID, Version: 1, InvocationID: "inv-1", Context: "http", Kind: "http", Level: "info", Message: "http", Data: data, At: now, CreatedAt: now},
		{ID: tsid.Generate(tsid.Application), FunctionID: f.ID, Version: 1, InvocationID: "inv-1", Context: "http", Kind: "log", Level: "error", Message: "pg-log-line", At: now.Add(-time.Second), CreatedAt: now},
	}))

	page := &FunctionTrace{
		Store: TraceReader{Repository: repo, Limits: limits},
		Ops:   TraceOps{Repo: repo, UoW: uow, Limits: limits},
		Shell: shellForTest(),
	}
	as := func(ac *auth.AuthContext, q req) (int, string) {
		t.Helper()
		rec := serveMounted(t, ac, q, page)
		return rec.Code, rec.Body.String()
	}
	admin := anchorAdmin()
	viewer := traceUser(permTraceView)
	manager := traceUser(permTraceView, permTraceManage)
	logPath := "/ui/functions/" + f.ID + "/traces"
	enablePath := "/ui/functions/" + f.ID + "/trace/http/enable"
	window := func() *function.TraceContext {
		t.Helper()
		c, err := repo.GetTraceContext(ctx, f.ID, "http")
		require.NoError(t, err)
		return c
	}

	// The log: http is the one context (no version declares others), the
	// viewer's bodies and log lines are withheld and the admin's are shown.
	code, body := as(viewer, req{target: logPath})
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "POST /p → 201", "Bodies, headers and query string hidden", "Details hidden", `<option value="http">http</option>`)
	mustNotContain(t, body, secret, "pg-log-line")
	_, body = as(viewer, req{target: logPath, datastar: true})
	mustNotContain(t, body, secret)
	_, body = as(admin, req{target: logPath})
	assert.Contains(t, body, secret)
	_, body = as(admin, req{target: logPath + "?kind=log"})
	mustContain(t, body, "pg-log-line")
	mustNotContain(t, body, "POST /p")
	assert.Nil(t, window(), "nothing is on until an operator switches it on")

	// Forged posts change nothing.
	form := url.Values{"minutes": {"5"}, "sampleRate": {"1"}}
	for name, ac := range map[string]*auth.AuthContext{
		"view only":             viewer,
		"manage without bodies": manager,
		"bodies without manage": traceUser(permTraceView, permTraceBodies),
		"no permissions at all": traceUser(),
	} {
		as(ac, tracePost(enablePath, form))
		assert.Nil(t, window(), name)
	}

	// Over the http cap (15 minutes): refused on the field, nothing stored.
	_, body = as(admin, tracePost(enablePath, url.Values{"minutes": {"60"}, "sampleRate": {"1"}}))
	mustContain(t, body, "over the 15-minute limit")
	assert.Nil(t, window())

	// Enable for 5 minutes at 1 in 3: stored, live, with a countdown.
	_, body = as(admin, tracePost(enablePath, url.Values{"minutes": {"5"}, "sampleRate": {"3"}, "page": {"traces"}}))
	mustContain(t, body, "http is live for 5 minutes", `id="fn-trace-switches"`)
	w := window()
	require.NotNil(t, w)
	require.NotNil(t, w.EnabledUntil)
	assert.True(t, w.Live(time.Now()))
	assert.False(t, w.Indefinite)
	assert.EqualValues(t, 3, w.SampleRate)
	_, body = as(viewer, req{target: logPath})
	mustContain(t, body, "Live", "1 in 3", "Auto refresh every 5 s", "Pause")

	// http never stays on until switched off, even for the super-admin.
	_, body = as(admin, tracePost(enablePath, url.Values{"indefinite": {"1"}, "indefiniteConfirm": {"1"}, "sampleRate": {"1"}}))
	mustContain(t, body, "The http context always turns itself off")
	assert.False(t, window().Indefinite)
	assert.EqualValues(t, 3, window().SampleRate, "the window is unchanged")

	// A view-only caller cannot disable; the manager can, and the window then
	// ends at the switch-off time.
	disablePath := "/ui/functions/" + f.ID + "/trace/http/disable"
	as(viewer, tracePost(disablePath, nil))
	assert.True(t, window().Live(time.Now()))
	_, body = as(manager, tracePost(disablePath, nil))
	mustContain(t, body, "http is off")
	require.NotNil(t, window().EnabledUntil)
	assert.False(t, window().Live(time.Now().Add(time.Second)))
	_, body = as(viewer, req{target: logPath})
	mustNotContain(t, body, "Auto refresh")
}

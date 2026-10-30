//go:build integration

package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The Activity page against the real queries and use cases: the polls and last
// poll a report stored, the switch run through the unit of work (the use case's
// own permission checks included), the records the repository reads back
// grouped by capture, and the redaction of a reader without view-bodies.
func TestIngestionActivityPageAgainstPostgres(t *testing.T) {
	e := newUitPG(t)
	i := e.create("act-"+uitUnique(), "q-act", false)
	ctx := e.ctx()
	now := time.Now().UTC().Truncate(time.Second)
	rt := "rt-" + uitUnique()

	_, err := e.repo.Record(ctx, ingestion.Report{
		RuntimeID: rt, Caller: e.admin(), Now: now,
		Routes: []ingestion.RouteReport{{IngestionID: i.ID, Version: 1, Status: "STARTED"}},
		Metrics: []ingestion.WindowReport{{
			IngestionID: i.ID, From: now.Add(-time.Hour), To: now.Add(-59 * time.Minute), Polls: 7, LastPollAt: new(now.Add(-61 * time.Minute)),
		}},
	})
	require.NoError(t, err)

	tabs := e.tabs()
	act := &IngestionActivity{Store: e.repo, Ops: IngestionTraceOps{Repo: e.repo, UoW: e.uow, Grace: time.Minute}, Shell: shellForTest()}
	tabs.Page = act
	trace := func(perms ...string) *auth.AuthContext {
		ac := e.admin()
		ac.Permissions = append([]string{uitPermView}, perms...)
		return ac
	}
	operator := trace(iatTraceView, iatTraceManage, iatTraceBodies)
	get := func(ac *auth.AuthContext, target string, datastar bool) (int, string) {
		rec := serveMounted(t, ac, req{target: target, datastar: datastar}, tabs, act)
		return rec.Code, rec.Body.String()
	}
	post := func(ac *auth.AuthContext, path string, form url.Values) (int, string) {
		rec := serveMounted(t, ac, req{method: http.MethodPost, target: "/ui/ingestions/" + i.ID + "/trace/" + path, form: form, datastar: true, multipart: true}, tabs, act)
		return rec.Code, rec.Body.String()
	}

	// Polls without a trace: "polling, nothing new" instead of nothing.
	code, body := get(operator, "/ui/ingestions/"+i.ID+"/activity", false)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "7 polls, 0 completed", "Last poll", `datetime="`+now.Add(-61*time.Minute).Format(time.RFC3339)+`"`, rt, `id="ing-trace-off"`, "No trace records yet")

	// Switched on through the use case, as the page's form posts it.
	code, body = post(operator, "enable", url.Values{"windowSecs": {"300"}, "sampleEverySecs": {"10"}})
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "Tracing on", `id="ing-trace-live"`, "data-on-interval__duration.5s", "5 minutes per runtime, one capture every 10 s")
	w, err := e.repo.GetTraceWindow(ctx, i.ID)
	require.NoError(t, err)
	require.NotNil(t, w)
	assert.Equal(t, 300*time.Second+time.Minute, w.HardStop.Sub(w.EnabledAt), "window plus the grace")

	// A caller without view-bodies cannot: the use case refuses, nothing changes.
	code, body = post(trace(iatTraceView, iatTraceManage), "enable", url.Values{"windowSecs": {"60"}})
	assert.Equal(t, http.StatusOK, code, "a Datastar answer carries the refusal as a flash")
	mustContain(t, body, "platform:ingestion:trace:view-bodies")
	mustNotContain(t, body, "Tracing on")
	again, err := e.repo.GetTraceWindow(ctx, i.ID)
	require.NoError(t, err)
	assert.Equal(t, w.WindowID, again.WindowID)

	// Records the runtimes shipped, read back and grouped by capture.
	at := time.Now().UTC()
	rec := func(id, capture, kind, message, data string, ago time.Duration) ingestion.TraceLogRecord {
		return ingestion.TraceLogRecord{
			ID: "itr_" + id + uitUnique(), IngestionID: i.ID, Version: 1, WindowID: w.WindowID, CaptureID: capture, RuntimeID: rt,
			Kind: kind, Level: "info", Message: message, Data: []byte(data), At: at.Add(-ago), CreatedAt: at,
		}
	}
	httpData := `{"client":"mix","request":{"method":"GET","url":"https://api.example.com/trips?token=SECRET-TOKEN","headers":{"Authorization":["[redacted]"]},"bodyBytes":0,"bodyTruncated":false},` +
		`"response":{"status":200,"headers":{},"body":"{\"trips\":[]}","bodyEncoding":"utf8","bodyBytes":12,"bodyTruncated":false},"durationMs":40,"attempt":1}`
	require.NoError(t, e.repo.InsertTraceRecords(ctx, []ingestion.TraceLogRecord{
		rec("a", "cap-"+uitUnique(), "http", "GET https://api.example.com/trips?token=SECRET-TOKEN", httpData, 3*time.Second),
		rec("b", "cap-b"+uitUnique(), "poll", "poll: 1 call, 0 payloads", `{"durationMs":50,"outcome":"ok"}`, 2*time.Second),
	}))
	code, body = get(operator, "/ui/ingestions/"+i.ID+"/activity/trace?kind=http", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "GET https://api.example.com/trips?token=SECRET-TOKEN → 200 · 40 ms · mix", "Authorization", "trips")
	assert.Equal(t, 1, strings.Count(body, `data-testid="ing-trace-capture"`), body)
	code, body = get(operator, "/ui/ingestions/"+i.ID+"/activity/trace", true)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, 2, strings.Count(body, `data-testid="ing-trace-capture"`), "two captures")

	// Without view-bodies: the path and the status, no query, headers or bodies.
	reader := trace(iatTraceView)
	code, body = get(reader, "/ui/ingestions/"+i.ID+"/activity/trace", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "GET /trips → 200 · 40 ms · mix")
	mustNotContain(t, body, "SECRET-TOKEN", "Authorization", "api.example.com", `trips&#34;`)

	// Switched off.
	code, body = post(operator, "disable", url.Values{})
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, "Tracing off", `id="ing-trace-switch"`)
	w, err = e.repo.GetTraceWindow(ctx, i.ID)
	require.NoError(t, err)
	assert.False(t, w.Live(time.Now()))
	code, body = get(operator, "/ui/ingestions/"+i.ID+"/activity/trace", true)
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, `id="ing-trace-off"`)
	mustNotContain(t, body, "data-on-interval__duration.5s")
}

package pages

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	iatTraceView   = "platform:ingestion:trace:view"
	iatTraceManage = "platform:ingestion:trace:manage"
	iatTraceBodies = "platform:ingestion:trace:view-bodies"

	iatSecret = "SECRET-grant-hunter2"
)

// fakeIngTraceOps records the switch's commands.
type fakeIngTraceOps struct {
	mu       sync.Mutex
	enabled  []operations.EnableTraceCommand
	disabled []operations.DisableTraceCommand
	err      error
}

func (f *fakeIngTraceOps) Enable(_ context.Context, cmd operations.EnableTraceCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.enabled = append(f.enabled, cmd)
	return f.err
}

func (f *fakeIngTraceOps) Disable(_ context.Context, cmd operations.DisableTraceCommand) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.disabled = append(f.disabled, cmd)
	return f.err
}

// iatRecords are two captures, newest first: a poll that called an API
// (an http call and an exchange nested under it) and, newer, a lone error.
func iatRecords() []ingestion.TraceLogRecord {
	httpData := `{"client":"mix","request":{"method":"POST","url":"https://api.example.com/token?grant=[redacted]",` +
		`"headers":{"Authorization":["[redacted]"],"Content-Type":["application/json"]},` +
		`"body":"{\"grant_type\":\"password\",\"password\":\"[redacted]\",\"note\":\"` + iatSecret + `\"}","bodyEncoding":"utf8","bodyBytes":70,"bodyTruncated":false},` +
		`"response":{"status":200,"headers":{"X-Request-Id":["abc"]},"body":"{\"access_token\":\"[redacted]\"}","bodyEncoding":"utf8","bodyBytes":9000,"bodyTruncated":true},"durationMs":85,"attempt":1}`
	exchData := `{"source":{"fileName":"orders.csv"},"payloadBytes":120,"payload":"` + iatSecret + `-payload","records":3,"jobs":3,"outcome":"completed"}`
	pollData := `{"startedAt":"2026-10-24T11:59:00Z","durationMs":90,"httpCalls":1,"payloads":1,"records":3,"jobs":3,"outcome":"ok"}`
	rec := func(id, capture, kind, message string, ago time.Duration, data string) ingestion.TraceLogRecord {
		r := ingestion.TraceLogRecord{
			ID: id, IngestionID: "ing_one", Version: 3, WindowID: "itw_1", CaptureID: capture, RuntimeID: "rt-1",
			Kind: kind, Level: "info", Message: message, At: uitNow.Add(-ago), CreatedAt: uitNow.Add(-ago),
		}
		if data != "" {
			r.Data = []byte(data)
		}
		return r
	}
	return []ingestion.TraceLogRecord{
		rec("itr_5", "cap-b", "error", "the source refused "+iatSecret, 5*time.Second, `{"error":"`+iatSecret+`-err","kind":"x"}`),
		rec("itr_4", "cap-a", "poll", "poll: 1 call, 1 payload", 10*time.Second, pollData),
		rec("itr_3", "cap-a", "exchange", "file orders.csv", 11*time.Second, exchData),
		rec("itr_2", "cap-a", "http", "POST https://api.example.com/token", 12*time.Second, httpData),
	}
}

// iatWindow is a live window (ending in 90 s), or an ended one.
func iatWindow(live bool) *ingestion.TraceWindow {
	by := "prn_ing"
	w := &ingestion.TraceWindow{
		IngestionID: "ing_one", WindowID: "itw_1", WindowSecs: 60, SampleEverySecs: 5,
		HardStop: uitNow.Add(90 * time.Second), EnabledBy: &by, EnabledAt: uitNow.Add(-30 * time.Second), UpdatedAt: uitNow,
	}
	if !live {
		w.HardStop = uitNow.Add(-time.Minute)
	}
	return w
}

func iatPages(store *fakeUitTabs, ops IngestionTraceCommands) (*IngestionTabs, *IngestionActivity) {
	act := &IngestionActivity{Store: store, Ops: ops, Shell: shellForTest(), now: func() time.Time { return uitNow }}
	tabs := &IngestionTabs{Store: store, Shell: shellForTest(), Page: act, now: func() time.Time { return uitNow }}
	return tabs, act
}

func iatServe(t *testing.T, store *fakeUitTabs, ops IngestionTraceCommands, ac *auth.AuthContext, q req) (int, string) {
	t.Helper()
	tabs, act := iatPages(store, ops)
	rec := serveMounted(t, ac, q, tabs, act)
	return rec.Code, rec.Body.String()
}

func iatStore(window *ingestion.TraceWindow, recs ...ingestion.TraceLogRecord) *fakeUitTabs {
	s := uitTabsStore(false)
	s.window, s.records = window, recs
	return s
}

func iatAdmin() *auth.AuthContext {
	return uitUser(uitPermView, iatTraceView, iatTraceManage, iatTraceBodies)
}

// A navigation to the activity address is the page: the counters, the last
// poll, exchange and failure, the runtimes and the trace panel, inside the
// shell; the drawer's Datastar request to the same address still gets its panel.
func TestIngestionActivityPage_Summary(t *testing.T) {
	store := iatStore(iatWindow(false), iatRecords()...)
	code, body := iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity"})
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body,
		"<title>", "Activity", "<code>orders</code>", "Orders", `href="/ui/ingestions/ing_one"`, `id="ing-activity-back"`,
		"1440 polls, 12 completed, 1 failed, 340 records, 12 jobs", "10080 polls, 80 completed, 4 failed, 2100 records, 80 jobs",
		"Last poll", `datetime="2026-10-24T11:59:30Z"`, "Last exchange", "Last failure",
		"rt-1", "rt-2", `fc-tag fc-tag-success">STARTED`, `fc-tag fc-tag-warn">FAILING`,
		`id="ing-trace"`, `id="ing-trace-records"`,
	)
	mustNotContain(t, body, "<b>by</b>")

	// The drawer's request to the same address gets the short summary.
	code, body = iatServe(t, store, nil, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity", datastar: true})
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body, `id="ing-activity"`, `id="ing-open-activity"`, `href="/ui/ingestions/ing_one/activity"`)
	mustNotContain(t, body, `id="ing-trace"`)
}

func TestIngestionActivityPage_PermissionsAndReach(t *testing.T) {
	cases := map[string]struct {
		ac   *auth.AuthContext
		want int
	}{
		"viewer":              {uitUser(uitPermView), http.StatusOK},
		"no view permission":  {uitUser(iatTraceView), http.StatusForbidden},
		"client of it":        {uitClientUser(uitPermView), http.StatusOK},
		"another client":      {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"}, Permissions: []string{uitPermView}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}, http.StatusForbidden},
		"another application": {&auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{uitPermView}, Applications: []string{"app_b"}, PrincipalType: auth.PrincipalTypeUser}, http.StatusForbidden},
	}
	for name, c := range cases {
		store := iatStore(iatWindow(true), iatRecords()...)
		code, body := iatServe(t, store, &fakeIngTraceOps{}, c.ac, req{target: "/ui/ingestions/ing_one/activity"})
		assert.Equal(t, c.want, code, name)
		if c.want != http.StatusOK {
			mustNotContain(t, body, "rt-1", "orders")
		}
	}
	code, _ := iatServe(t, iatStore(nil), &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_nothing/activity"})
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = iatServe(t, iatStore(nil), &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace"})
	assert.Equal(t, http.StatusSeeOther, code, "the trace panel is a fragment: a navigation goes to the page")
}

// The switch follows the trace permissions: manage and view-bodies switch it on;
// manage alone is told why not; a viewer sees the state only; without
// trace:view nothing is read.
func TestIngestionActivityPage_SwitchFollowsPermissions(t *testing.T) {
	get := func(ac *auth.AuthContext, window *ingestion.TraceWindow) (*fakeUitTabs, string) {
		store := iatStore(window, iatRecords()...)
		code, body := iatServe(t, store, &fakeIngTraceOps{}, ac, req{target: "/ui/ingestions/ing_one/activity"})
		require.Equal(t, http.StatusOK, code, body)
		return store, body
	}

	_, body := get(iatAdmin(), iatWindow(false))
	mustContain(t, body, `id="ing-trace-enable"`, `action="/ui/ingestions/ing_one/trace/enable"`, `name="windowSecs"`, `name="sampleEverySecs"`,
		"1 minute", "5 minutes", "15 minutes", "Switch on", `id="ing-trace-hint"`, "picks the window up")
	mustNotContain(t, body, `id="ing-trace-disable"`, `id="ing-trace-live"`)

	_, body = get(iatAdmin(), iatWindow(true))
	mustContain(t, body, `id="ing-trace-live"`, "Live", `id="ing-trace-disable"`, `action="/ui/ingestions/ing_one/trace/disable"`, "Start a new window")

	_, body = get(uitUser(uitPermView, iatTraceView, iatTraceManage), iatWindow(false))
	mustContain(t, body, `id="ing-trace-needs-bodies"`, "platform:ingestion:trace:view-bodies")
	mustNotContain(t, body, `id="ing-trace-enable"`)

	_, body = get(uitUser(uitPermView, iatTraceView), iatWindow(true))
	mustContain(t, body, `id="ing-trace-live"`, "ing-trace-records")
	mustNotContain(t, body, `id="ing-trace-enable"`, `id="ing-trace-disable"`, `id="ing-trace-needs-bodies"`)

	store, body := get(uitUser(uitPermView), iatWindow(true))
	mustContain(t, body, `id="ing-trace-no-permission"`, "platform:ingestion:trace:view")
	mustNotContain(t, body, `id="ing-trace-records"`, `id="ing-trace-enable"`, "the source refused")
	assert.Empty(t, store.filters, "a viewer without trace:view reads no record")
}

// Records are grouped by capture, newest capture first: a poll with its HTTP
// calls and exchanges nested; each call expands to its request and response.
func TestIngestionActivityPage_RecordsGroupedByCapture(t *testing.T) {
	store := iatStore(iatWindow(false), iatRecords()...)
	_, body := iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity"})

	assert.Equal(t, 2, strings.Count(body, `data-testid="ing-trace-capture"`), "two captures")
	capB, capA := strings.Index(body, `id="ing-cap-cap-b"`), strings.Index(body, `id="ing-cap-cap-a"`)
	require.True(t, capB > 0 && capA > 0, body)
	assert.Less(t, capB, capA, "newest capture first")

	groupA := body[capA:]
	poll, call, exch := strings.Index(groupA, `id="ing-rec-itr_4"`), strings.Index(groupA, `id="ing-rec-itr_2"`), strings.Index(groupA, `id="ing-rec-itr_3"`)
	require.True(t, poll > 0 && call > poll && exch > call, "the poll heads its capture, its call and exchange nested after it, oldest first")
	mustContain(t, groupA[poll:call], "poll: 1 call, 1 payload", `fc-tag fc-tag-success">ok`, "90 ms")
	mustContain(t, groupA[exch:], "file orders.csv", "completed", "payloadBytes")
	mustContain(t, groupA[call:exch],
		// The call's summary line and its panels.
		"POST https://api.example.com/token?grant=[redacted] → 200 · 85 ms · mix",
		"Request", "Response", "Status 200", "Content-Type", "application/json", "X-Request-Id",
		// A JSON body pretty-printed, redaction as the runtime wrote it, truncation marked.
		"grant_type", "[redacted]", "access_token", "truncated (9,000 bytes total)", "data-ignore-morph")
	mustContain(t, body, "the source refused", `fc-tag fc-tag-secondary">error`)
}

// Without view-bodies a record shows what the API shows and nothing sensitive
// is in the response.
func TestIngestionActivityPage_WithoutViewBodiesRecordsAreRedacted(t *testing.T) {
	store := iatStore(iatWindow(false), iatRecords()...)
	ac := uitUser(uitPermView, iatTraceView)
	_, body := iatServe(t, store, &fakeIngTraceOps{}, ac, req{target: "/ui/ingestions/ing_one/activity"})
	mustContain(t, body, ingBodiesHidden[:40], "POST /token → 200", "85 ms", "exchange: completed", "poll: 1 call, 1 payload")
	mustNotContain(t, body, iatSecret, "Authorization", "grant_type", "access_token", "Content-Type", "X-Request-Id", "orders.csv", "grant=", "api.example.com")

	// The same for the panel's own refresh.
	_, body = iatServe(t, store, &fakeIngTraceOps{}, ac, req{target: "/ui/ingestions/ing_one/activity/trace", datastar: true})
	mustNotContain(t, body, iatSecret, "Authorization", "grant_type", "orders.csv")
}

// While a window is live the panel asks for itself again every 5 seconds (and
// ticks its countdown); an ended window takes the timer away.
func TestIngestionActivityPage_RefreshesWhileLive(t *testing.T) {
	live := iatStore(iatWindow(true), iatRecords()...)
	_, body := iatServe(t, live, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity"})
	mustContain(t, body, `data-on-interval__duration.5s`, `data-on-interval__duration.1s`, `data-until="`, "Refreshing every 5 s", "1:30", "one capture every 5 s")

	_, body = iatServe(t, live, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace", datastar: true})
	mustContain(t, body, `data-on-interval__duration.5s`, `id="ing-trace-switch"`, `id="ing-trace-records"`, "ing-rec-itr_4")

	ended := iatStore(iatWindow(false), iatRecords()...)
	_, body = iatServe(t, ended, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace", datastar: true})
	mustNotContain(t, body, `data-on-interval__duration.5s`, `id="ing-trace-live"`)
	mustContain(t, body, `id="ing-trace-off"`)

	never := iatStore(nil)
	_, body = iatServe(t, never, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity"})
	mustContain(t, body, `id="ing-trace-off"`, "No trace records yet")
	mustNotContain(t, body, `data-on-interval__duration.5s`)
}

// Filters narrow the records; "load more" appends the next older page.
func TestIngestionActivityPage_FiltersAndLoadMore(t *testing.T) {
	store := iatStore(iatWindow(false), iatRecords()...)
	_, body := iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace?kind=http&runtime=rt-1", datastar: true})
	last := store.filters[len(store.filters)-1]
	assert.Equal(t, "http", last.Kind)
	assert.Equal(t, "rt-1", last.RuntimeID)
	mustContain(t, body, "ing-rec-itr_2")
	mustNotContain(t, body, "ing-rec-itr_4", "ing-rec-itr_5")
	_, body = iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace?kind=exchange&runtime=nobody", datastar: true})
	mustContain(t, body, "No records match these filters.")

	// More than a page: the button carries the cursor, and the next page lands
	// below, with group ids that cannot clash with the first page's.
	var many []ingestion.TraceLogRecord
	for n := range ingTracePageSize + 30 {
		many = append(many, ingestion.TraceLogRecord{
			ID: fmt.Sprintf("itr_%04d", 9999-n), IngestionID: "ing_one", WindowID: "itw_1", CaptureID: fmt.Sprintf("cap-%04d", n), RuntimeID: "rt-1",
			Kind: "poll", Level: "info", Message: fmt.Sprintf("poll %d", n), At: uitNow.Add(-time.Duration(n) * time.Second), Version: 3,
		})
	}
	store = iatStore(iatWindow(false), many...)
	_, body = iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity"})
	mustContain(t, body, `id="ing-trace-more"`, "Load more", "cursor=")
	mustNotContain(t, body, fmt.Sprintf("poll %d<", ingTracePageSize))
	start := strings.Index(body, `data-more="`)
	require.True(t, start > 0, body)
	more := strings.ReplaceAll(body[start+len(`data-more="`):start+len(`data-more="`)+strings.Index(body[start+len(`data-more="`):], `"`)], "&amp;", "&")
	require.Contains(t, more, "cursor=")
	_, appended := iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: more, datastar: true})
	mustContain(t, appended, fmt.Sprintf("poll %d<", ingTracePageSize), `id="ing-cap-cap-0100-p1"`, `id="ing-trace-more"`)
	mustNotContain(t, appended, `id="ing-trace-switch"`, "poll 0<")

	// A cursor that is not one is the first page.
	_, body = iatServe(t, store, &fakeIngTraceOps{}, iatAdmin(), req{target: "/ui/ingestions/ing_one/activity/trace?cursor=junk", datastar: true})
	mustContain(t, body, "poll 0<")
}

func TestIngestionActivityPage_EnableAndDisable(t *testing.T) {
	post := func(ops *fakeIngTraceOps, ac *auth.AuthContext, path string, form url.Values, datastar bool) (int, string, *fakeUitTabs) {
		store := iatStore(iatWindow(true), iatRecords()...)
		code, body := iatServe(t, store, ops, ac, req{method: http.MethodPost, target: "/ui/ingestions/ing_one/trace/" + path, form: form, datastar: datastar, multipart: datastar})
		return code, body, store
	}

	// A Datastar post runs the use case with the form's numbers, and answers
	// with the switch, the records and a flash.
	ops := &fakeIngTraceOps{}
	code, body, _ := post(ops, iatAdmin(), "enable", url.Values{"windowSecs": {"300"}, "sampleEverySecs": {"10"}}, true)
	require.Equal(t, http.StatusOK, code, body)
	require.Len(t, ops.enabled, 1)
	assert.Equal(t, operations.EnableTraceCommand{IngestionID: "ing_one", WindowSecs: 300, SampleEverySecs: 10}, ops.enabled[0])
	mustContain(t, body, `id="ing-trace-switch"`, "Tracing on", "requestSubmit()")
	mustNotContain(t, body, `id="ing-trace-records"`)

	// Blank fields leave the defaults to the use case.
	ops = &fakeIngTraceOps{}
	post(ops, iatAdmin(), "enable", url.Values{"windowSecs": {""}, "sampleEverySecs": {""}}, true)
	assert.Equal(t, operations.EnableTraceCommand{IngestionID: "ing_one"}, ops.enabled[0])

	// A native post goes back to the page.
	ops = &fakeIngTraceOps{}
	code, _, _ = post(ops, iatAdmin(), "enable", url.Values{"windowSecs": {"60"}}, false)
	assert.Equal(t, http.StatusSeeOther, code)
	require.Len(t, ops.enabled, 1)

	ops = &fakeIngTraceOps{}
	code, body, _ = post(ops, iatAdmin(), "disable", url.Values{}, true)
	require.Equal(t, http.StatusOK, code, body)
	assert.Equal(t, []operations.DisableTraceCommand{{IngestionID: "ing_one"}}, ops.disabled)
	mustContain(t, body, `id="ing-trace-switch"`, "Tracing off")

	// A forged post is refused without running anything: enable needs manage
	// and view-bodies, disable needs manage, both need the ingestion's reach.
	for name, c := range map[string]struct {
		ac   *auth.AuthContext
		path string
	}{
		"enable, view only":           {uitUser(uitPermView, iatTraceView), "enable"},
		"enable, manage without body": {uitUser(uitPermView, iatTraceView, iatTraceManage), "enable"},
		"enable, bodies without mng":  {uitUser(uitPermView, iatTraceView, iatTraceBodies), "enable"},
		"disable, view only":          {uitUser(uitPermView, iatTraceView), "disable"},
		"enable, another client": {&auth.AuthContext{
			PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_b"}, Roles: []string{"r"},
			Permissions: []string{uitPermView, iatTraceManage, iatTraceBodies}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
		}, "enable"},
	} {
		ops = &fakeIngTraceOps{}
		code, _, _ = post(ops, c.ac, c.path, url.Values{"windowSecs": {"60"}}, false)
		assert.Equal(t, http.StatusForbidden, code, name)
		assert.Empty(t, ops.enabled, name)
		assert.Empty(t, ops.disabled, name)
	}

	// A number that is not one is a 400; the use case's refusal is shown, no flash.
	ops = &fakeIngTraceOps{}
	code, _, _ = post(ops, iatAdmin(), "enable", url.Values{"windowSecs": {"a minute"}}, false)
	assert.Equal(t, http.StatusBadRequest, code)
	assert.Empty(t, ops.enabled)
	ops = &fakeIngTraceOps{err: usecase.Validation(operations.TraceWindowInvalid, "windowSecs must be 60, 300 or 900")}
	code, body, _ = post(ops, iatAdmin(), "enable", url.Values{"windowSecs": {"61"}}, true)
	assert.Equal(t, http.StatusOK, code, "a Datastar answer carries the error as a flash")
	mustContain(t, body, "windowSecs must be 60, 300 or 900")
	mustNotContain(t, body, "Tracing on")
	ops = &fakeIngTraceOps{err: errors.New("db down")}
	code, _, _ = post(ops, iatAdmin(), "disable", url.Values{}, false)
	assert.Equal(t, http.StatusInternalServerError, code)
}

// The trace records can hold customer data: never cached.
func TestIngestionActivityPage_RecordsAreNotCached(t *testing.T) {
	store := iatStore(iatWindow(true), iatRecords()...)
	tabs, act := iatPages(store, &fakeIngTraceOps{})
	for _, q := range []req{{target: "/ui/ingestions/ing_one/activity"}, {target: "/ui/ingestions/ing_one/activity/trace", datastar: true}} {
		rec := serveMounted(t, iatAdmin(), q, tabs, act)
		assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"), q.target)
	}
}

// The list's row menu links to the page.
func TestIngestionRowMenuLinksToActivity(t *testing.T) {
	v := ingsView{Perms: ingsPerms{Update: true}}
	row := ingsRow{ID: "ing_one", Code: "orders", DetailHref: "/ui/ingestions/ing_one", Active: true}
	p := v.rowActions(row)
	var found bool
	for _, a := range p.Menu {
		if a.Label == "Activity" {
			found = true
			assert.Equal(t, "/ui/ingestions/ing_one/activity", a.Href)
		}
	}
	assert.True(t, found, "an Activity action in the row menu")
}

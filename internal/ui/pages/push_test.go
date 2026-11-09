package pages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	pushViewPermTest   = "platform:admin:push:view"
	pushManagePermTest = "platform:admin:push:manage"
)

var pushNow = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// pushFakes is the Push pages' collaborators in memory. Settings mirrors sdksync.PushSettingsOps
// (anchor + manage, validation); the Postgres test runs the real one.
type pushFakes struct {
	apps      []pushgw.ConfiguredApp
	rules     map[string][]pushgw.RuleDetail
	keys      map[string][]pushgw.KeyDetail
	settings  map[string]pushgw.AppSettings
	live      pushgw.Live
	logRows   []pushgw.LogRow
	windows   map[string]*pushgw.TraceWindow
	published []string
	audits    []string
	available bool
	pubErr    error
	clients   []client.Client
}

func newPushFakes() *pushFakes {
	synced := pushNow.Add(-90 * time.Second)
	f := &pushFakes{
		apps: []pushgw.ConfiguredApp{
			{ApplicationID: "app_alpha", Code: "alpha", Name: "Alpha <App>", Rules: 2, Keys: 2, SyncedAt: &synced, Overrides: 1},
			{ApplicationID: "app_beta", Code: "beta", Name: "Beta", Paused: true},
		},
		rules: map[string][]pushgw.RuleDetail{"app_alpha": {
			{Pattern: "alpha:orders:{principal}", SyncedAt: synced},
			{Pattern: "alpha:public:news", Public: true, SyncedAt: synced},
			{Pattern: "alpha:admin:*", Requires: []string{"alpha:admin:view"}, SyncedAt: synced},
		}},
		keys:     map[string][]pushgw.KeyDetail{"app_alpha": {{Kid: "k1", Alg: "Ed25519 OKP", SyncedAt: synced}, {Kid: "k2", Alg: "Ed25519 OKP", SyncedAt: synced}}},
		settings: map[string]pushgw.AppSettings{"app_alpha": {MaxStreams: new(500)}, "app_beta": {Paused: true}},
		windows:  map[string]*pushgw.TraceWindow{},
		clients: []client.Client{
			{ID: "clt_one", Name: "Client One", Identifier: "one", Status: client.StatusActive},
			{ID: "clt_two", Name: "Client Two", Identifier: "two", Status: client.StatusActive},
		},
		available: true,
	}
	f.logRows = []pushgw.LogRow{
		{ID: 12, Topic: "alpha:orders:7", Type: "order.updated", Client: "clt_one", Data: `{"secret":"customer-data-12"}`, LoggedAt: pushNow.Add(-5 * time.Second)},
		{ID: 11, Topic: "alpha:public:news", Type: "news.posted", Client: "", Data: `{"secret":"customer-data-11"}`, LoggedAt: pushNow.Add(-9 * time.Second)},
	}
	return f
}

// ConfigStore
func (f *pushFakes) ListConfigured(context.Context) ([]pushgw.ConfiguredApp, error) {
	return f.apps, nil
}

func (f *pushFakes) RuleDetails(_ context.Context, id string) ([]pushgw.RuleDetail, error) {
	return f.rules[id], nil
}

func (f *pushFakes) KeyDetails(_ context.Context, id string) ([]pushgw.KeyDetail, error) {
	return f.keys[id], nil
}

// Settings
func (f *pushFakes) Read(_ context.Context, id, code string) (sdksync.PushSettingsResponse, error) {
	return sdksync.NewPushSettingsResponse(code, f.settings[id]), nil
}

func (f *pushFakes) Update(_ context.Context, ac *auth.AuthContext, id, code string, next sdksync.PushSettings) (sdksync.PushSettingsResponse, error) {
	if err := auth.CanManagePush(ac); err != nil {
		return sdksync.PushSettingsResponse{}, err
	}
	st := pushgw.AppSettings{MaxStreams: next.MaxStreams, PublishRate: next.PublishRate, PublishBurst: next.PublishBurst, MaxDataBytes: next.MaxDataBytes, Paused: next.Paused}
	if err := pushgw.ValidateSettings(st); err != nil {
		if ve, ok := errors.AsType[*pushgw.ValidationError](err); ok {
			return sdksync.PushSettingsResponse{}, usecase.Validation(ve.Code, ve.Message)
		}
		return sdksync.PushSettingsResponse{}, err
	}
	f.settings[id] = st
	f.audits = append(f.audits, "PUSH_SETTINGS_CHANGED:"+code)
	return sdksync.NewPushSettingsResponse(code, st), nil
}

// Apps
func (f *pushFakes) FindByCode(_ context.Context, code string) (*application.Application, error) {
	for _, a := range f.apps {
		if a.Code == code {
			return &application.Application{ID: ids.ApplicationID("app_" + a.Code), Code: a.Code, Name: a.Name}, nil
		}
	}
	if code == "gamma" {
		return &application.Application{ID: ids.ApplicationID("app_gamma"), Code: "gamma", Name: "Gamma"}, nil
	}
	return nil, nil
}

// Stats
func (f *pushFakes) Live(context.Context, time.Time) (pushgw.Live, error) { return f.live, nil }

// Log
func (f *pushFakes) Recent(_ context.Context, q pushgw.LogQuery) ([]pushgw.LogRow, error) {
	var out []pushgw.LogRow
	for _, r := range f.logRows {
		if strings.HasPrefix(r.Topic, q.Application+":") {
			out = append(out, r)
		}
	}
	return out, nil
}

// Traces
func (f *pushFakes) Get(_ context.Context, id string) (*pushgw.TraceWindow, error) {
	return f.windows[id], nil
}

func (f *pushFakes) Enable(_ context.Context, id string, secs int, by string, now time.Time) (*pushgw.TraceWindow, error) {
	if !pushgw.ValidTraceWindowSecs(secs) {
		return nil, &pushgw.ValidationError{Code: "TRACE_WINDOW_INVALID", Message: "the window is 60, 300 or 900 seconds"}
	}
	w := &pushgw.TraceWindow{ApplicationID: id, HardStop: now.Add(time.Duration(secs) * time.Second), WindowSecs: secs, EnabledBy: &by, EnabledAt: now}
	f.windows[id] = w
	return w, nil
}

func (f *pushFakes) Disable(_ context.Context, id string, now time.Time) (bool, error) {
	w := f.windows[id]
	if w == nil || !w.Live(now) {
		return false, nil
	}
	w.HardStop = now
	return true, nil
}

// Publisher
func (f *pushFakes) Available() bool { return f.available }

func (f *pushFakes) PublishTest(_ context.Context, topic, clientID string) error {
	if f.pubErr != nil {
		return f.pubErr
	}
	f.published = append(f.published, topic+"|"+clientID)
	return nil
}

// ClientStore
func (f *pushFakes) FindAll(context.Context) ([]client.Client, error) { return f.clients, nil }

func (f *pushFakes) FindByID(_ context.Context, id string) (*client.Client, error) {
	for i := range f.clients {
		if string(f.clients[i].ID) == id {
			return &f.clients[i], nil
		}
	}
	return nil, nil
}

func (f *pushFakes) console() *PushConsole {
	return &PushConsole{
		Config: f, Settings: f, Apps: f, Clients: f, Stats: f, Log: f, Traces: f, Publisher: f,
		Audit: func(_ context.Context, _ *auth.AuthContext, appID, op string, _ map[string]any) {
			f.audits = append(f.audits, op+":"+appID)
		},
		Now: func() time.Time { return pushNow }, Shell: shellForTest(),
	}
}

func (f *pushFakes) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, f.console())
}

func pushViewer() *auth.AuthContext { return connUser(pushViewPermTest) }
func pushManager() *auth.AuthContext {
	return connUser(pushViewPermTest, pushManagePermTest)
}

func post(target string, form url.Values) req {
	return req{method: http.MethodPost, target: target, datastar: true, multipart: true, form: form}
}

func TestPushApplicationsList(t *testing.T) {
	f := newPushFakes()
	rec := f.do(t, pushViewer(), req{target: "/ui/push/applications"})
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	mustContain(t, body,
		"Push applications", `id="push-apps-table"`, `href="/ui/push/applications/alpha"`, "Alpha &lt;App&gt;",
		"1 override", "Defaults", "Paused", "Accepting", "1 min ago",
		`href="/ui/push/live"`, `href="/ui/push/recent"`, "Open an application by code")
	mustNotContain(t, body, "Alpha <App>", `href="/ui/push/test"`)

	// the manager also gets the test send tab
	mustContain(t, f.do(t, pushManager(), req{target: "/ui/push/applications"}).Body.String(), `href="/ui/push/test"`)

	// opening an application by code redirects; an unknown code says so
	open := f.do(t, pushViewer(), req{target: "/ui/push/applications?open=gamma"})
	assert.Equal(t, http.StatusSeeOther, open.Code)
	assert.Equal(t, "/ui/push/applications/gamma", open.Header().Get("Location"))
	unknown := f.do(t, pushViewer(), req{target: "/ui/push/applications?open=nope"})
	assert.Equal(t, http.StatusNotFound, unknown.Code)
	mustContain(t, unknown.Body.String(), "No application has the code nope.")

	f.apps = nil
	mustContain(t, f.do(t, pushViewer(), req{target: "/ui/push/applications"}).Body.String(), "No application uses the push gateway yet")
}

func TestPushPermissions(t *testing.T) {
	f := newPushFakes()
	clientScoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_one"}, Roles: []string{"r"}, Permissions: []string{pushViewPermTest, pushManagePermTest}, PrincipalType: auth.PrincipalTypeUser}
	for name, ac := range map[string]*auth.AuthContext{
		"no permission": connUser("platform:admin:cors:view"),
		"client scoped": clientScoped,
	} {
		for _, target := range []string{"/ui/push/applications", "/ui/push/applications/alpha", "/ui/push/live", "/ui/push/recent", "/ui/push/test"} {
			rec := f.do(t, ac, req{target: target})
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s: %s", name, target)
		}
	}
	// the view permission does not open the test send
	assert.Equal(t, http.StatusForbidden, f.do(t, pushViewer(), req{target: "/ui/push/test"}).Code)
	// manage alone reads (it implies view)
	assert.Equal(t, http.StatusOK, f.do(t, connUser(pushManagePermTest), req{target: "/ui/push/live"}).Code)
	// the super-admin wildcard
	assert.Equal(t, http.StatusOK, f.do(t, anchorAdmin(), req{target: "/ui/push/test"}).Code)
}

func TestPushApplicationDetail(t *testing.T) {
	f := newPushFakes()
	body := f.do(t, pushManager(), req{target: "/ui/push/applications/alpha"}).Body.String()
	mustContain(t, body,
		"Alpha &lt;App&gt;", "Push gateway configuration of alpha",
		// rules, read-only, with their sync time
		"alpha:orders:{principal}", "alpha:admin:*", "alpha:admin:view", "Public", "1 min ago",
		// keys, with the rotation note for two
		"k1", "k2", "Ed25519 OKP", "A rotation is under way",
		// limits: the override and the defaults, with the effective values
		`name="maxStreams"`, `value="500"`, "Effective 500", "(default 20,000)", "Effective 500", "Effective 2,000", "Effective 16,384",
		"Override", "Default", "Save limits", "Accepting pushes", "Pause")
	mustNotContain(t, body, `name="maxStreams" disabled`)

	// a viewer sees the same facts but no controls
	view := f.do(t, pushViewer(), req{target: "/ui/push/applications/alpha"}).Body.String()
	mustContain(t, view, "alpha:orders:{principal}", "Effective 500")
	mustNotContain(t, view, "Save limits", ">Pause<")
	mustContain(t, view, "disabled")

	assert.Equal(t, http.StatusNotFound, f.do(t, pushViewer(), req{target: "/ui/push/applications/missing"}).Code)
	// an application with no sync yet: empty states, defaults
	gamma := f.do(t, pushManager(), req{target: "/ui/push/applications/gamma"}).Body.String()
	mustContain(t, gamma, "No rules synced", "No ticket keys synced", "Effective 20,000")
}

func TestPushSaveLimits(t *testing.T) {
	f := newPushFakes()
	target := "/ui/push/applications/alpha/limits"

	// a round trip: save, read back, the controls patched and a flash
	rec := f.do(t, pushManager(), post(target, url.Values{"maxStreams": {"900"}, "publishRate": {"100"}, "publishBurst": {"300"}, "maxDataBytes": {""}}))
	require.Equal(t, http.StatusOK, rec.Code)
	mustContain(t, rec.Body.String(), `id="push-app-controls"`, `value="900"`, "Push limits saved")
	st := f.settings["app_alpha"]
	require.NotNil(t, st.MaxStreams)
	assert.Equal(t, 900, *st.MaxStreams)
	assert.Equal(t, 100, *st.PublishRate)
	assert.Equal(t, 300, *st.PublishBurst)
	assert.Nil(t, st.MaxDataBytes, "a blank field returns to the default")
	assert.Equal(t, []string{"PUSH_SETTINGS_CHANGED:alpha"}, f.audits)

	// a refused value is shown on its field and saves nothing
	bad := f.do(t, pushManager(), post(target, url.Values{"maxStreams": {"900"}, "publishRate": {"100"}, "publishBurst": {"50"}, "maxDataBytes": {""}}))
	mustContain(t, bad.Body.String(), "PublishBurst (50) must be at least publishRate (100)", "push-limit-publishBurst-err", `value="50"`)
	assert.Equal(t, 300, *f.settings["app_alpha"].PublishBurst, "unchanged")
	notNum := f.do(t, pushManager(), post(target, url.Values{"maxStreams": {"many"}}))
	mustContain(t, notNum.Body.String(), "Enter a whole number.", `value="many"`)
	tooBig := f.do(t, pushManager(), post(target, url.Values{"maxDataBytes": {"70000"}}))
	mustContain(t, tooBig.Body.String(), "MaxDataBytes is at most 65536")
	assert.Len(t, f.audits, 1, "refused edits are not audited")

	// a native post redirects with a flash
	native := f.do(t, pushManager(), req{method: http.MethodPost, target: target, form: url.Values{"maxStreams": {"700"}}})
	assert.Equal(t, http.StatusSeeOther, native.Code)
	assert.Equal(t, "/ui/push/applications/alpha", native.Header().Get("Location"))
	assert.Equal(t, 700, *f.settings["app_alpha"].MaxStreams)

	// a forged post by a viewer changes nothing
	forged := f.do(t, pushViewer(), post(target, url.Values{"maxStreams": {"1"}}))
	mustContain(t, forged.Body.String(), "fc-flash-error")
	assert.Equal(t, 700, *f.settings["app_alpha"].MaxStreams)
	assert.Equal(t, http.StatusNotFound, f.do(t, pushManager(), post("/ui/push/applications/missing/limits", url.Values{})).Code)
}

func TestPushPause(t *testing.T) {
	f := newPushFakes()
	target := "/ui/push/applications/alpha/pause"
	rec := f.do(t, pushManager(), post(target, url.Values{"paused": {"true"}}))
	mustContain(t, rec.Body.String(), "Resume", "Paused", "Ingest answers 429")
	st := f.settings["app_alpha"]
	assert.True(t, st.Paused)
	require.NotNil(t, st.MaxStreams, "pausing keeps the overrides")
	assert.Equal(t, 500, *st.MaxStreams)
	assert.Equal(t, []string{"PUSH_SETTINGS_CHANGED:alpha"}, f.audits, "audited through the settings operation")

	resume := f.do(t, pushManager(), post(target, url.Values{"paused": {"false"}}))
	mustContain(t, resume.Body.String(), "Accepting pushes", "Push ingest resumed")
	assert.False(t, f.settings["app_alpha"].Paused)

	forged := f.do(t, pushViewer(), post(target, url.Values{"paused": {"true"}}))
	mustContain(t, forged.Body.String(), "fc-flash-error")
	assert.False(t, f.settings["app_alpha"].Paused)
}

func TestPushLive(t *testing.T) {
	f := newPushFakes()
	// no task reporting: a clear state, not zeros
	empty := f.do(t, pushViewer(), req{target: "/ui/push/live"}).Body.String()
	mustContain(t, empty, "No gateway task is reporting", "FC_PUSH_ENABLED", `id="push-live-body"`, "data-on-interval")
	mustNotContain(t, empty, "Gateway tasks")

	f.live = pushgw.Live{
		Tasks: 2, Driver: "postgres", Newest: pushNow.Add(-4 * time.Second), Open: 1234,
		ByApplication: map[string]int{"alpha": 1200, "beta": 34},
		ByClient:      map[string]int{"clt_one": 1000, "_platform": 200, "clt_gone": 34},
		Lag:           pushgw.Histogram{Bounds: []float64{0.005, 0.01, 0.025}, Counts: []uint64{0, 90, 9, 1}},
		Published:     4321,
		Ingested:      map[string]map[string]float64{"alpha": {"accepted": 77, "refused": 3}},
	}
	body := f.do(t, pushViewer(), req{target: "/ui/push/live"}).Body.String()
	mustContain(t, body,
		"Gateway tasks", ">2<", "log driver: postgres", "last report 4 s ago",
		"1,234", "4,321", "Relay lag p50", "Relay lag p99", "100 relay runs",
		`href="/ui/push/applications/alpha"`, "1,200", "500", // alpha's limit from its override
		"77 accepted, 3 refused",
		"Client One", "Platform", "clt_gone")
	// the same body as a patch
	live := f.do(t, pushViewer(), req{target: "/ui/push/live", datastar: true}).Body.String()
	mustContain(t, live, `id="push-live-body"`, "1,234")
	mustNotContain(t, live, "<title>")
}

func TestPushRecentGatesData(t *testing.T) {
	f := newPushFakes()
	target := "/ui/push/recent?app=alpha"

	// no window: ids, times, clients and types show; the data does not
	body := f.do(t, pushViewer(), req{target: target}).Body.String()
	mustContain(t, body, `id="push-12"`, `id="push-11"`, "alpha:orders:7", "order.updated", "Client One", "Platform", "news.posted", "hidden", "Trace is")
	mustNotContain(t, body, "customer-data-12", "customer-data-11")

	// a viewer cannot open a window; a forged post is refused and nothing opens
	forged := f.do(t, pushViewer(), post("/ui/push/recent/trace/enable", url.Values{"app": {"alpha"}, "windowSecs": {"60"}}))
	mustContain(t, forged.Body.String(), "fc-flash-error")
	assert.Nil(t, f.windows["app_alpha"])
	assert.NotContains(t, f.do(t, pushViewer(), req{target: target}).Body.String(), "Switch on")

	// the manager switches a window on: audited, data shows, the timer runs
	on := f.do(t, pushManager(), post("/ui/push/recent/trace/enable", url.Values{"app": {"alpha"}, "windowSecs": {"300"}}))
	mustContain(t, on.Body.String(), "Trace window open", "customer-data-12", "5:00", "Live")
	require.NotNil(t, f.windows["app_alpha"])
	assert.Equal(t, pushNow.Add(5*time.Minute), f.windows["app_alpha"].HardStop)
	assert.Contains(t, f.audits, "PUSH_TRACE_ENABLED:app_alpha")
	live := f.do(t, pushViewer(), req{target: target}).Body.String()
	mustContain(t, live, "customer-data-12", "customer-data-11", "Refreshing every 5 s", `data-until="`)

	// an invalid length is refused
	badLen := f.do(t, pushManager(), post("/ui/push/recent/trace/enable", url.Values{"app": {"alpha"}, "windowSecs": {"7"}}))
	mustContain(t, badLen.Body.String(), "fc-flash-error")

	// the window is per application: beta's data stays hidden
	f.logRows = append(f.logRows, pushgw.LogRow{ID: 13, Topic: "beta:x:1", Type: "x.y", Data: `{"beta":"beta-secret"}`, LoggedAt: pushNow})
	beta := f.do(t, pushViewer(), req{target: "/ui/push/recent?app=beta"}).Body.String()
	mustContain(t, beta, `id="push-13"`)
	mustNotContain(t, beta, "beta-secret")

	// switching off hides the data again; it is audited once
	off := f.do(t, pushManager(), post("/ui/push/recent/trace/disable", url.Values{"app": {"alpha"}}))
	mustContain(t, off.Body.String(), "Trace window closed")
	mustNotContain(t, off.Body.String(), "customer-data-12")
	f.do(t, pushManager(), post("/ui/push/recent/trace/disable", url.Values{"app": {"alpha"}}))
	n := 0
	for _, a := range f.audits {
		if a == "PUSH_TRACE_DISABLED:app_alpha" {
			n++
		}
	}
	assert.Equal(t, 1, n, "switching off a window that is already off is not audited")

	// an expired window shows no data either
	f.windows["app_alpha"] = &pushgw.TraceWindow{ApplicationID: "app_alpha", HardStop: pushNow.Add(-time.Second), WindowSecs: 60}
	mustNotContain(t, f.do(t, pushViewer(), req{target: target}).Body.String(), "customer-data-12")
}

func TestPushRecentFiltersAndUnreadableLog(t *testing.T) {
	f := newPushFakes()
	// the first configured application is the default
	mustContain(t, f.do(t, pushViewer(), req{target: "/ui/push/recent"}).Body.String(), `id="push-12"`)
	// a datastar request patches the body and keeps the URL
	live := f.do(t, pushViewer(), req{target: "/ui/push/recent?app=alpha&type=order.updated", datastar: true}).Body.String()
	mustContain(t, live, `id="push-recent-body"`, "history.replaceState")
	// an unknown application is said so
	mustContain(t, f.do(t, pushViewer(), req{target: "/ui/push/recent?app=nope"}).Body.String(), "No application has the code nope.")
	// a log this process cannot read is a notice, not an empty list
	c := f.console()
	c.Log = nil
	rec := serveMounted(t, pushViewer(), req{target: "/ui/push/recent?app=alpha"}, c)
	mustContain(t, rec.Body.String(), "cannot be read from this process")
	mustNotContain(t, rec.Body.String(), "push-recent-table")
}

func TestPushTestSend(t *testing.T) {
	f := newPushFakes()
	page := f.do(t, pushManager(), req{target: "/ui/push/test"}).Body.String()
	mustContain(t, page, "Push test send", `name="topic"`, "Client One", "Platform (reaches every client)", "Send test push")
	mustNotContain(t, page, "does not run in this process")

	send := func(topic, clientID string) string {
		return f.do(t, pushManager(), post("/ui/push/test", url.Values{"topic": {topic}, "client": {clientID}})).Body.String()
	}
	ok := send("alpha:orders:7", "clt_one")
	mustContain(t, ok, "Test push sent", "fw-test.ping", "alpha:orders:7")
	assert.Equal(t, []string{"alpha:orders:7|clt_one"}, f.published)
	assert.Contains(t, f.audits, "PUSH_TEST_SENT:app_alpha", "audited against the topic's application")
	send("alpha:public:news", "_platform")
	assert.Equal(t, "alpha:public:news|_platform", f.published[1])

	// refusals publish nothing and are not audited
	audited := len(f.audits)
	mustContain(t, send("", "_platform"), "Enter a topic.")
	mustContain(t, send("Not A Topic", "_platform"), "push-test-topic-err")
	mustContain(t, send("zeta:a:b", "_platform"), "No application has the code zeta.")
	mustContain(t, send("alpha:a:b", "clt_missing"), "Choose one of the clients.")
	f.pubErr = errors.New("db down")
	mustContain(t, send("alpha:a:b", "_platform"), "could not be completed")
	assert.Len(t, f.published, 2)
	assert.Len(t, f.audits, audited)

	// a viewer's forged post publishes nothing
	f.pubErr = nil
	forged := f.do(t, pushViewer(), post("/ui/push/test", url.Values{"topic": {"alpha:a:b"}, "client": {"_platform"}}))
	mustContain(t, forged.Body.String(), "fc-flash-error")
	assert.Len(t, f.published, 2)

	// the gateway is elsewhere: a clear state and no send
	f.available = false
	mustContain(t, f.do(t, pushManager(), req{target: "/ui/push/test"}).Body.String(), "The push gateway does not run in this process")
	gone := f.do(t, pushManager(), post("/ui/push/test", url.Values{"topic": {"alpha:a:b"}, "client": {"_platform"}}))
	mustContain(t, gone.Body.String(), "does not run in this process")
	assert.Len(t, f.published, 2)
}

func TestPushHelpers(t *testing.T) {
	assert.Equal(t, "1,234,567", pushNumber(1234567))
	assert.Equal(t, "12", pushNumber(12))
	assert.Equal(t, "-", pushSeconds(0, false))
	assert.Equal(t, "12 ms", pushSeconds(0.0123, true))
	assert.Equal(t, "2.5 s", pushSeconds(2.5, true))
	assert.Equal(t, "never", pushAge(nil, pushNow))
	old := pushNow.Add(-3 * 24 * time.Hour)
	assert.Equal(t, "3 days ago", pushAge(&old, pushNow))
	assert.Equal(t, "Platform", pushClientLabel(nil, ""))
	assert.Equal(t, "Platform", pushClientLabel(nil, "_platform"))
}

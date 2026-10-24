package pages

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

const (
	permTraceView   = "platform:function:trace:view"
	permTraceManage = "platform:function:trace:manage"
	permTraceBodies = "platform:function:trace:view-bodies"

	traceSecretBody = "SECRET-BODY-hunter2"
)

// fakeTraceStore stands in for TraceReader: one function, its contexts and
// its records (newest first).
type fakeTraceStore struct {
	mu      sync.Mutex
	fn      *function.Function
	states  []operations.TraceContextState
	recs    []function.TraceLogRecord
	filters []function.TraceLogFilter
	reads   int
	// onRead runs before each Contexts call (a test moves the clock or the records).
	onRead func(call int, s *fakeTraceStore)
}

func (f *fakeTraceStore) FindByID(_ context.Context, id string) (*function.Function, error) {
	if f.fn != nil && f.fn.ID == id {
		return f.fn, nil
	}
	return nil, nil
}

func (f *fakeTraceStore) Contexts(_ context.Context, id string) ([]operations.TraceContextState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.onRead != nil {
		f.onRead(f.reads, f)
	}
	return append([]operations.TraceContextState(nil), f.states...), nil
}

func (f *fakeTraceStore) QueryTraceLog(_ context.Context, fl function.TraceLogFilter) ([]function.TraceLogRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.filters = append(f.filters, fl)
	var out []function.TraceLogRecord
	for _, r := range f.recs {
		if fl.Kind != "" && r.Kind != fl.Kind {
			continue
		}
		if fl.Context != "" && r.Context != fl.Context {
			continue
		}
		if fl.After != nil && (r.At.After(fl.After.At) || r.At.Equal(fl.After.At) && r.ID >= fl.After.ID) {
			continue
		}
		out = append(out, r)
	}
	if fl.Limit > 0 && len(out) > fl.Limit {
		out = out[:fl.Limit]
	}
	return out, nil
}

type fakeTraceOps struct {
	enabled  []operations.EnableTraceCommand
	disabled []operations.DisableTraceCommand
	err      error
}

func (o *fakeTraceOps) Enable(_ context.Context, cmd operations.EnableTraceCommand) error {
	o.enabled = append(o.enabled, cmd)
	return o.err
}

func (o *fakeTraceOps) Disable(_ context.Context, cmd operations.DisableTraceCommand) error {
	o.disabled = append(o.disabled, cmd)
	return nil
}

var traceNow = time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

func traceStates(liveHTTP bool) []operations.TraceContextState {
	http := operations.TraceContextState{Name: "http", Description: "Built-in capture.", BuiltIn: true, SampleRate: 1, MaxMinutes: 15}
	if liveHTTP {
		until := traceNow.Add(90 * time.Second)
		http.EnabledUntil, http.Live, http.SampleRate = &until, true, 5
	}
	return []operations.TraceContextState{
		http,
		{Name: "orders", Description: "What the mapping decided.", SampleRate: 1, MaxMinutes: 60},
	}
}

func traceHTTPData(body string) []byte {
	return []byte(`{"request":{"method":"POST","path":"/orders","rawQuery":"a=1","headers":{"content-type":["application/json"],"authorization":"[redacted]"},` +
		`"body":"{\"ref\":\"` + body + `\"}","bodyEncoding":"utf8","bodyBytes":40},` +
		`"response":{"status":200,"headers":{},"body":"ok","bodyEncoding":"utf8","bodyBytes":2},"durationMs":12,"outcome":"ok"}`)
}

func newTraceStore() *fakeTraceStore {
	ref, corr := "TRIP-1", "corr-123456789012345"
	return &fakeTraceStore{
		fn:     &function.Function{ID: "fn_one", Address: "billing.invoice-pdf", Name: "invoice-pdf"},
		states: traceStates(false),
		recs: []function.TraceLogRecord{
			{ID: "ftl_3", InvocationID: "inv_aaaaaaaaaaaa", Context: "http", Kind: "http", Level: "info", Message: "http", Version: 7, At: traceNow.Add(-time.Minute), Data: traceHTTPData(traceSecretBody), Reference: &ref, CorrelationID: &corr},
			{ID: "ftl_2", InvocationID: "inv_aaaaaaaaaaaa", Context: "orders", Kind: "trace", Level: "warn", Message: "mapped <b>3</b> lines", Version: 7, At: traceNow.Add(-2 * time.Minute), Data: []byte(`{"lines":3}`)},
			{ID: "ftl_1", InvocationID: "inv_bbbb", Context: "orders", Kind: "log", Level: "error", Message: "boom", Version: 6, At: traceNow.Add(-3 * time.Minute)},
		},
	}
}

type traceHarness struct {
	store *fakeTraceStore
	ops   *fakeTraceOps
	page  *FunctionTrace
}

func newTraceHarness() *traceHarness {
	h := &traceHarness{store: newTraceStore(), ops: &fakeTraceOps{}}
	h.page = &FunctionTrace{Store: h.store, Ops: h.ops, Shell: shellForTest(), now: func() time.Time { return traceNow }}
	return h
}

func (h *traceHarness) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page)
}

func (h *traceHarness) body(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.do(t, ac, q).Body.String()
}

func traceUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_tr", Email: "tr@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

func tracePost(path string, form url.Values) req {
	return req{method: http.MethodPost, target: path, datastar: true, multipart: true, form: form}
}

const traceLog = "/ui/functions/fn_one/traces"

func TestTraceLogPage(t *testing.T) {
	h := newTraceHarness()
	body := h.body(t, traceUser(permTraceView, "platform:function:function:view"), req{target: traceLog})
	mustContain(t, body,
		`<title>Trace logs · FlowCatalyst</title>`, `<h1 class="fc-page-title">Trace logs</h1>`, "billing.invoice-pdf",
		`href="/ui/functions/fn_one"`, "Back to function",
		`id="trace-filters"`, `<option value="orders">orders</option>`, `<option value="http">http</option>`,
		`id="trace-rec-ftl_3"`, `data-ignore-morph`, "mapped &lt;b&gt;3&lt;/b&gt; lines", "POST /orders?a=1 → 200 · 12 ms · ok",
		`data-field="reference" data-value="TRIP-1"`, "corr-123…345", `id="trace-more"`,
		"2 records in this view", "Tracing switches",
	)
	mustNotContain(t, body, "mapped <b>3</b>", "Auto refresh every 5 s", `data-init="@get(el.action`)
	// A viewer without the function view permission is not shown the address.
	mustNotContain(t, h.body(t, traceUser(permTraceView), req{target: traceLog}), "billing.invoice-pdf")
}

func TestTraceLogHidesBodiesWithoutPermission(t *testing.T) {
	h := newTraceHarness()
	for _, q := range []req{{target: traceLog}, {target: traceLog, datastar: true}} {
		body := h.body(t, traceUser(permTraceView), q)
		mustNotContain(t, body, traceSecretBody, "hunter2")
		mustContain(t, body, "Bodies hidden: you do not have permission to view request and response bodies")
	}
	body := h.body(t, traceUser(permTraceView, permTraceBodies), req{target: traceLog})
	mustContain(t, body, traceSecretBody, "JSON", "authorization", "[redacted]")
	mustNotContain(t, body, "Bodies hidden")
}

func TestTraceLogNeedsView(t *testing.T) {
	h := newTraceHarness()
	for _, path := range []string{traceLog, traceLog + "/more", "/ui/functions/fn_one/trace/switches"} {
		rec := h.do(t, traceUser(permTraceManage), req{target: path})
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d, want 403", path, rec.Code)
		}
		mustNotContain(t, rec.Body.String(), "ftl_3", traceSecretBody)
	}
	if h.store.reads != 0 || len(h.store.filters) != 0 {
		t.Fatal("something was read for a caller without the view permission")
	}
	// An unknown function, and an id that could not be an element id.
	for _, path := range []string{"/ui/functions/fn_nope/traces", "/ui/functions/a'b/traces"} {
		if rec := h.do(t, traceUser(permTraceView), req{target: path}); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}
}

func TestTraceLogFiltersAreTheURL(t *testing.T) {
	h := newTraceHarness()
	q := "?kind=http&level=bogus&since=60&reference=TRIP-1&context=orders&paused=1"
	rec := h.do(t, traceUser(permTraceView), req{target: traceLog + q, datastar: true})
	body := rec.Body.String()
	got := h.store.filters[0]
	if got.Kind != "http" || got.Level != "" || got.Reference != "TRIP-1" || got.Context != "orders" || got.Since == nil {
		t.Fatalf("filter = %+v", got)
	}
	if want := traceNow.Add(-time.Hour); !got.Since.Equal(want) {
		t.Fatalf("since = %v, want %v", got.Since, want)
	}
	mustContain(t, body, `id="trace-status"`, `id="trace-records"`, `id="trace-more"`, `id="trace-context-field"`, `id="trace-error"`,
		"history.replaceState", "context=orders", "kind=http", "since=60", "paused=1")
	mustContain(t, h.body(t, traceUser(permTraceView), req{target: traceLog + "?since=60"}), `<option value="60" selected>`)
	mustNotContain(t, body, "bogus")
	// Pause and resume are the submitter's.
	mustNotContain(t, h.body(t, traceUser(permTraceView), req{target: traceLog + "?paused=1&resume=1", datastar: true}), "paused=1")
	mustContain(t, h.body(t, traceUser(permTraceView), req{target: traceLog + "?pause=1", datastar: true}), "paused=1")
}

func TestTraceLogLoadMore(t *testing.T) {
	h := newTraceHarness()
	h.store.recs = nil
	for i := range 130 {
		h.store.recs = append(h.store.recs, function.TraceLogRecord{
			ID: fmt.Sprintf("ftl_%04d", 999-i), InvocationID: "inv", Context: "orders", Kind: "trace", Level: "info",
			Message: fmt.Sprintf("m%03d", i), At: traceNow.Add(-time.Duration(i) * time.Second),
		})
	}
	ac := traceUser(permTraceView)
	page := h.body(t, ac, req{target: traceLog})
	mustContain(t, page, "m000", "m099", `data-more="/ui/functions/fn_one/traces/more?cursor=`, "Load more")
	mustNotContain(t, page, "m100")
	// The link works without Datastar as well: the page of older records.
	cursor := page[strings.Index(page, "cursor=")+len("cursor="):]
	cursor = cursor[:strings.IndexAny(cursor, `"&`)]
	older := h.body(t, ac, req{target: traceLog + "?cursor=" + cursor})
	mustContain(t, older, "m100", "m129")
	mustNotContain(t, older, "m099", "Load more")
	more := h.body(t, ac, req{target: traceLog + "/more?cursor=" + cursor, datastar: true})
	mustContain(t, more, "m100", "m129", `id="trace-more"`, "selector #trace-records", "mode append")
	mustNotContain(t, more, "m099")
}

// While a window is live the request stays open and patches the log again; it
// ends by itself when the window closes, and a paused page does not stream.
func TestTraceLogStreamsWhileLive(t *testing.T) {
	h := newTraceHarness()
	h.page.refresh = 5 * time.Millisecond
	h.store.states = traceStates(true)
	h.store.onRead = func(call int, s *fakeTraceStore) {
		switch call {
		case 2: // the first tick finds a new record
			s.recs = append([]function.TraceLogRecord{{ID: "ftl_9", InvocationID: "inv_new", Context: "orders", Kind: "trace", Level: "info", Message: "brand-new-record", At: traceNow}}, s.recs...)
		case 4: // the window closes
			s.states = traceStates(false)
		}
	}
	rec := h.streamed(t, traceUser(permTraceView), traceLog+"?kind=", 5*time.Second)
	body := rec.Body.String()
	if strings.Count(body, `id="trace-records"`) < 3 {
		t.Fatalf("the log was patched %d times, want several", strings.Count(body, `id="trace-records"`))
	}
	mustContain(t, body, "brand-new-record", "Auto refresh every 5 s", "Pause")
	if h.store.reads < 4 {
		t.Fatalf("contexts read %d times", h.store.reads)
	}

	// Paused: one answer, and it ends.
	paused := newTraceHarness()
	paused.page.refresh = time.Millisecond
	paused.store.states = traceStates(true)
	body = paused.streamed(t, traceUser(permTraceView), traceLog+"?paused=1", time.Second).Body.String()
	mustContain(t, body, "Auto refresh paused", "Resume")
	if paused.store.reads != 1 {
		t.Fatalf("a paused page read %d times", paused.store.reads)
	}
}

func TestTraceStreamEndsAtItsLimit(t *testing.T) {
	h := newTraceHarness()
	h.page.refresh, h.page.maxStream = time.Millisecond, 20*time.Millisecond
	h.store.states = traceStates(true)
	body := h.streamed(t, traceUser(permTraceView), traceLog, 5*time.Second).Body.String()
	mustContain(t, body, "Auto refresh paused", "Resume")
}

// streamed runs a Datastar GET and waits for the handler to return by itself.
func (h *traceHarness) streamed(t *testing.T, ac *auth.AuthContext, target string, limit time.Duration) *httptest.ResponseRecorder {
	t.Helper()
	mux := chi.NewRouter()
	mux.Route("/ui", func(r chi.Router) { h.page.Mount(r) })
	ctx, cancel := context.WithTimeout(auth.WithContext(context.Background(), ac), limit)
	defer cancel()
	r := httptest.NewRequest(http.MethodGet, target, nil).WithContext(ctx)
	r.Header.Set(ds.RequestHeader, "true")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { mux.ServeHTTP(rec, r); close(done) }()
	select {
	case <-done:
	case <-time.After(limit):
		cancel()
		<-done
		t.Fatal("the stream did not end by itself")
	}
	return rec
}

func TestTraceSwitchesMatrix(t *testing.T) {
	t.Run("view only", func(t *testing.T) {
		h := newTraceHarness()
		h.store.states = traceStates(true)
		body := h.body(t, traceUser(permTraceView), req{target: traceLog})
		mustContain(t, body, `id="trace-context-http"`, `id="trace-context-orders"`, "Live", "1 in 5", "1:30", "You do not have permission to change tracing.")
		mustNotContain(t, body, ">Enable<", ">Change<", ">Disable<", "/enable", "/disable")
	})
	t.Run("manage without bodies", func(t *testing.T) {
		h := newTraceHarness()
		h.store.states = traceStates(true)
		body := h.body(t, traceUser(permTraceView, permTraceManage), req{target: traceLog})
		mustContain(t, body, "Capturing http records request and response bodies", `/ui/functions/fn_one/trace/orders/enable?page=traces`, ">Disable<")
		mustNotContain(t, body, `href="/ui/functions/fn_one/trace/http/enable`)
	})
	t.Run("manage with bodies", func(t *testing.T) {
		h := newTraceHarness()
		body := h.body(t, traceUser(permTraceView, permTraceManage, permTraceBodies), req{target: traceLog})
		mustContain(t, body, `href="/ui/functions/fn_one/trace/http/enable?page=traces"`, "Enable")
		mustNotContain(t, body, "Capturing http records")
	})
	t.Run("embedded in the drawer", func(t *testing.T) {
		h := newTraceHarness()
		ctx := auth.WithContext(context.Background(), traceUser(permTraceView, permTraceManage))
		c, err := h.page.Switches(ctx, "fn_one", true)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err := c.Render(ctx, &b); err != nil {
			t.Fatal(err)
		}
		mustContain(t, b.String(), `id="fn-trace-switches"`, `id="trace-dialog-slot"`, `href="/ui/functions/fn_one/traces"`, "View trace logs", `href="/ui/functions/fn_one/trace/orders/enable"`)
		mustNotContain(t, b.String(), "page=traces")
	})
	t.Run("no view permission reads nothing", func(t *testing.T) {
		h := newTraceHarness()
		ctx := auth.WithContext(context.Background(), traceUser(permTraceManage))
		c, err := h.page.Switches(ctx, "fn_one", true)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		_ = c.Render(ctx, &b)
		mustContain(t, b.String(), "You do not have permission to view trace logs (platform:function:trace:view).")
		mustNotContain(t, b.String(), "trace-context-http")
		if h.store.reads != 0 {
			t.Fatal("contexts were read")
		}
	})
}

func TestTraceCountdown(t *testing.T) {
	for d, want := range map[time.Duration]string{
		90 * time.Second: "1:30", 59*time.Second + time.Millisecond: "1:00", 5 * time.Second: "0:05",
		3661 * time.Second: "1:01:01", time.Hour: "1:00:00",
	} {
		if got := traceCountdown(d); got != want {
			t.Errorf("traceCountdown(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestTraceDurations(t *testing.T) {
	for max, want := range map[int][]int{60: {5, 15, 30, 60}, 15: {5, 15}, 3: {3}, 0: {1}} {
		if got := traceDurations(max); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("traceDurations(%d) = %v, want %v", max, got, want)
		}
	}
}

func TestTraceBodyView(t *testing.T) {
	s := func(v string) *string { return &v }
	n := int64(1234567)
	cases := []struct {
		name     string
		part     traceHTTPPart
		redacted bool
		state    string
		label    string
		note     string
	}{
		{"hidden", traceHTTPPart{Body: s("x")}, true, "hidden", "", ""},
		{"empty", traceHTTPPart{}, false, "empty", "", ""},
		{"json", traceHTTPPart{Body: s(`{"a":1}`)}, false, "shown", "JSON", ""},
		{"text", traceHTTPPart{Body: s("plain")}, false, "shown", "text", ""},
		{"base64", traceHTTPPart{Body: s("AAEC"), BodyEncoding: "base64"}, false, "shown", "base64", ""},
		{"truncated", traceHTTPPart{Body: s(`{"a":`), BodyTruncated: true, BodyBytes: &n}, false, "shown", "text", "truncated (1,234,567 bytes total)"},
	}
	for _, c := range cases {
		got := traceBodyOf(&c.part, c.redacted)
		if got.State != c.state || got.Label != c.label || got.Note != c.note {
			t.Errorf("%s: %+v", c.name, got)
		}
		if c.redacted && got.Text != "" {
			t.Errorf("%s: text kept", c.name)
		}
	}
	if got := traceBodyOf(&traceHTTPPart{Body: s(`{"a":1}`)}, false); got.Text != "{\n  \"a\": 1\n}" {
		t.Errorf("pretty = %q", got.Text)
	}
}

// A reference or id that looks like markup or script is text and attribute
// data only, never part of an expression.
func TestTraceUserDataIsNotCode(t *testing.T) {
	h := newTraceHarness()
	evil := `'"><script>alert(1)</script>`
	h.store.recs[0].Reference = &evil
	body := h.body(t, traceUser(permTraceView), req{target: traceLog + "?reference=" + url.QueryEscape(evil)})
	mustNotContain(t, body, "<script>alert(1)", "data-on:click__prevent__stop=\"const f = document.getElementById('trace-filters'); f.elements[el.dataset.field].value = '")
	mustContain(t, body, "&lt;script&gt;alert(1)")
}

func TestTraceEnableDialog(t *testing.T) {
	h := newTraceHarness()
	mgr := traceUser(permTraceView, permTraceManage, permTraceBodies)
	out := h.body(t, mgr, req{target: "/ui/functions/fn_one/trace/http/enable?page=traces", datastar: true})
	mustContain(t, out, `id="trace-enable"`, "Enable tracing: http", "This records request and response bodies",
		`<option value="5"`, `<option value="15"`, "At most 15 minutes", `name="page" value="traces"`, `name="sampleRate"`, "No time limit")
	mustNotContain(t, out, `<option value="30"`)

	h.store.states[1].Indefinite, h.store.states[1].EnabledUntil = true, &function.TraceIndefiniteUntil
	mustContain(t, h.body(t, mgr, req{target: "/ui/functions/fn_one/trace/orders/enable", datastar: true}), `<option value="60"`, "traceIndef: true")

	// Without the permissions the dialog is refused, and so is a context that does not exist.
	for _, ac := range []*auth.AuthContext{traceUser(permTraceView), traceUser(permTraceManage), traceUser(permTraceView, permTraceManage)} {
		path := "/ui/functions/fn_one/trace/http/enable"
		if ac.Permissions[len(ac.Permissions)-1] == permTraceManage && len(ac.Permissions) == 1 {
			path = "/ui/functions/fn_one/trace/orders/enable"
		}
		mustContain(t, h.body(t, ac, req{target: path, datastar: true}), "fc-flash-error")
	}
	mustContain(t, h.body(t, mgr, req{target: "/ui/functions/fn_one/trace/ghost/enable", datastar: true}), "fc-flash-error")
}

func TestTraceEnable(t *testing.T) {
	mgr := traceUser(permTraceView, permTraceManage)
	enable := func(h *traceHarness, ac *auth.AuthContext, name string, kv ...string) string {
		form := url.Values{"minutes": {"15"}, "sampleRate": {"1"}, "page": {"traces"}}
		for i := 0; i+1 < len(kv); i += 2 {
			form.Set(kv[i], kv[i+1])
		}
		return h.body(t, ac, tracePost("/ui/functions/fn_one/trace/"+name+"/enable", form))
	}

	t.Run("success", func(t *testing.T) {
		h := newTraceHarness()
		out := enable(h, mgr, "orders", "minutes", "30", "sampleRate", "5")
		if len(h.ops.enabled) != 1 {
			t.Fatalf("enabled = %+v", h.ops.enabled)
		}
		if c := h.ops.enabled[0]; c.FunctionID != "fn_one" || c.Context != "orders" || c.Minutes != 30 || c.SampleRate != 5 || c.Indefinite {
			t.Fatalf("command = %+v", c)
		}
		mustContain(t, out, "orders is live for 30 minutes", `id="fn-trace-switches"`, "requestSubmit", `.close()`)
	})
	t.Run("no time limit", func(t *testing.T) {
		h := newTraceHarness()
		out := enable(h, mgr, "orders", "indefinite", "1", "minutes", "")
		if c := h.ops.enabled[0]; !c.Indefinite || c.Minutes != 0 {
			t.Fatalf("command = %+v", c)
		}
		mustContain(t, out, "orders is on until you switch it off")
	})
	t.Run("drawer does not restart a stream", func(t *testing.T) {
		h := newTraceHarness()
		out := enable(h, mgr, "orders", "page", "")
		mustNotContain(t, out, "requestSubmit")
	})
	t.Run("field errors", func(t *testing.T) {
		for name, kv := range map[string][]string{
			"rate zero":   {"sampleRate", "0"},
			"rate large":  {"sampleRate", "1001"},
			"rate text":   {"sampleRate", "abc"},
			"rate blank":  {"sampleRate", ""},
			"no duration": {"minutes", ""},
		} {
			h := newTraceHarness()
			out := enable(h, mgr, "orders", kv...)
			mustContain(t, out, `role="alert"`)
			if len(h.ops.enabled) != 0 {
				t.Errorf("%s: the use case ran", name)
			}
		}
	})
	t.Run("use case refusal lands on the field", func(t *testing.T) {
		h := newTraceHarness()
		h.ops.err = usecase.Validation(operations.TraceWindowTooLong, "minutes is over the 15-minute limit")
		out := enable(h, mgr, "orders")
		mustContain(t, out, "Minutes is over the 15-minute limit", `id="trace-enable-minutes-field"`)
		mustNotContain(t, out, "fc-flash-success")
	})
	t.Run("forged posts are refused", func(t *testing.T) {
		h := newTraceHarness()
		for name, ac := range map[string]*auth.AuthContext{
			"view only":   traceUser(permTraceView),
			"nothing":     traceUser(),
			"bodies only": traceUser(permTraceBodies),
		} {
			mustContain(t, enable(h, ac, "orders"), `role="alert"`)
			if len(h.ops.enabled) != 0 {
				t.Fatalf("%s enabled a context", name)
			}
		}
		// http needs view-bodies as well as manage.
		out := enable(h, mgr, "http")
		mustContain(t, out, "Permission required")
		if len(h.ops.enabled) != 0 {
			t.Fatal("http was enabled without view-bodies")
		}
		enable(h, traceUser(permTraceManage, permTraceBodies), "http")
		if len(h.ops.enabled) != 1 {
			t.Fatal("manage with view-bodies may enable http")
		}
	})
	t.Run("native post redirects with a flash", func(t *testing.T) {
		h := newTraceHarness()
		rec := h.do(t, mgr, req{method: http.MethodPost, target: "/ui/functions/fn_one/trace/orders/enable", form: url.Values{"minutes": {"5"}, "sampleRate": {"1"}, "page": {"traces"}}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != traceLog {
			t.Fatalf("status %d, location %q", rec.Code, rec.Header().Get("Location"))
		}
		if !strings.Contains(strings.Join(rec.Header().Values("Set-Cookie"), ";"), "fc_ui_flash=trace-enabled") {
			t.Fatal("no flash cookie")
		}
	})
}

func TestTraceDisable(t *testing.T) {
	post := func(h *traceHarness, ac *auth.AuthContext, page string) string {
		return h.body(t, ac, tracePost("/ui/functions/fn_one/trace/orders/disable", url.Values{"page": {page}}))
	}
	h := newTraceHarness()
	out := post(h, traceUser(permTraceView, permTraceManage), "traces")
	if len(h.ops.disabled) != 1 || h.ops.disabled[0].Context != "orders" || h.ops.disabled[0].FunctionID != "fn_one" {
		t.Fatalf("disabled = %+v", h.ops.disabled)
	}
	mustContain(t, out, "orders is off", "requestSubmit")

	h = newTraceHarness()
	for _, ac := range []*auth.AuthContext{traceUser(permTraceView), traceUser(permTraceBodies), traceUser()} {
		mustContain(t, post(h, ac, ""), "fc-flash-error")
	}
	if len(h.ops.disabled) != 0 {
		t.Fatal("a caller without manage disabled a context")
	}
	rec := h.do(t, traceUser(permTraceView), req{method: http.MethodPost, target: "/ui/functions/fn_one/trace/orders/disable"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("native forged post: status %d", rec.Code)
	}
}

func TestTraceSwitchesRefresh(t *testing.T) {
	h := newTraceHarness()
	out := h.body(t, traceUser(permTraceView), req{target: "/ui/functions/fn_one/trace/switches?page=traces", datastar: true})
	mustContain(t, out, `id="fn-trace-switches"`)
	rec := h.do(t, traceUser(permTraceView), req{target: "/ui/functions/fn_one/trace/switches"})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/functions/fn_one" {
		t.Fatalf("status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}
}

// A client-scoped caller does not reach another client's function.
func TestTraceScope(t *testing.T) {
	h := newTraceHarness()
	other := "clt_other"
	h.store.fn.ClientID = &other
	ac := traceUser(permTraceView)
	ac.Scope, ac.Clients = auth.ScopeClient, []string{"clt_mine"}
	rec := h.do(t, ac, req{target: traceLog})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
	mustNotContain(t, rec.Body.String(), "ftl_3")
}

package pages

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

type fakeDJStore struct {
	jobs     []dispatchjob.DispatchJob
	attempts map[string][]dispatchjob.Attempt
	params   []dispatchjob.FilterParams
}

func (f *fakeDJStore) FindWithFilters(_ context.Context, p dispatchjob.FilterParams) ([]dispatchjob.DispatchJob, error) {
	f.params = append(f.params, p)
	var out []dispatchjob.DispatchJob
	for _, j := range f.jobs {
		if len(p.Statuses) > 0 && !slices.Contains(p.Statuses, string(j.Status)) {
			continue
		}
		if p.AccessibleClientIDs != nil && j.ClientID != nil && !slices.Contains(*p.AccessibleClientIDs, *j.ClientID) {
			continue
		}
		if p.MessageGroup != nil && deref(j.MessageGroup) != *p.MessageGroup {
			continue
		}
		if p.Cursor != nil && (j.CreatedAt.After(p.Cursor.CreatedAt) || (j.CreatedAt.Equal(p.Cursor.CreatedAt) && j.ID >= p.Cursor.ID)) {
			continue
		}
		out = append(out, j)
	}
	return out[:min(len(out), p.Limit)], nil
}

func (f *fakeDJStore) FindByID(_ context.Context, id string) (*dispatchjob.DispatchJob, error) {
	for i := range f.jobs {
		if f.jobs[i].ID == id {
			j := f.jobs[i]
			return &j, nil
		}
	}
	return nil, nil
}

func (f *fakeDJStore) AttemptsByJob(_ context.Context, id string) ([]dispatchjob.Attempt, error) {
	return f.attempts[id], nil
}

func (f *fakeDJStore) DistinctValues(_ context.Context, column string, _ int) ([]string, error) {
	switch column {
	case "status":
		return []string{"COMPLETED", "FAILED", "PENDING"}, nil
	case "code":
		return []string{"orders:billing:invoice:created", "orders:billing:invoice:paid", "orders:shipping:parcel:sent", "crm:contacts:person:updated"}, nil
	}
	return nil, nil
}

type fakeDJOps struct {
	requeued [][]string
	previews []dispatchjob.FilterParams
	byFilter []dispatchjob.FilterParams
	confirms []int64
	summary  djops.MatchSummary
	result   djops.ResendByFilterResult
	err      error
	planned  []string
}

func (o *fakeDJOps) Requeue(_ context.Context, ids []string) (int, error) {
	o.requeued = append(o.requeued, ids)
	return len(ids), o.err
}

func (o *fakeDJOps) Preview(_ context.Context, f dispatchjob.FilterParams) (djops.MatchSummary, error) {
	o.previews = append(o.previews, f)
	return o.summary, nil
}

func (o *fakeDJOps) RequeueByFilter(_ context.Context, f dispatchjob.FilterParams, confirm int64) (djops.ResendByFilterResult, error) {
	o.byFilter = append(o.byFilter, f)
	o.confirms = append(o.confirms, confirm)
	return o.result, o.err
}

func (o *fakeDJOps) Plan(_ context.Context, j *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error) {
	o.planned = append(o.planned, j.ID)
	return &dispatchprocessing.DeliveryPlan{
		Request: &dispatchjob.RequestSummary{SignedBy: "orders-sa", Signature: true, Target: "https://hooks.example.com/in"},
		Headers: map[string]string{"X-FlowCatalyst-Timestamp": "1700000000", "X-FlowCatalyst-Signature": "sig<1>", "Authorization": "Bearer ••••••"},
		Body:    `{"a":1}`,
	}, nil
}

var djBase = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func djJob(id, code string, status common.DispatchStatus, client string, minutesAgo int) dispatchjob.DispatchJob {
	j := dispatchjob.DispatchJob{
		ID: id, Code: code, Status: status, TargetURL: "https://hooks.example.com/in", Mode: common.DispatchImmediate,
		MaxRetries: 3, AttemptCount: 1, CreatedAt: djBase.Add(-time.Duration(minutesAgo) * time.Minute),
	}
	if client != "" {
		j.ClientID = &client
	}
	return j
}

type djHarness struct {
	store *fakeDJStore
	ops   *fakeDJOps
}

func newDJHarness() *djHarness {
	desc, group, payload, lastErr := "Notify <billing>", "order-42", `{"z":1,"order":42,"note":"<b>"}`, "HTTP 500"
	failed := djJob("djb_1", "orders:billing:invoice:created", common.DispatchFailed, "clt_1", 1)
	failed.Descriptor, failed.MessageGroup, failed.Payload, failed.LastError = &desc, &group, &payload, &lastErr
	failed.Metadata = []dispatchjob.Metadata{{Key: "tenant", Value: "acme-long-tenant-name"}, {Key: "region", Value: "eu"}}
	code500, code200, body := 500, 200, `{"error":"boom"}`
	errMsg := "subscriber said no"
	return &djHarness{
		store: &fakeDJStore{
			jobs: []dispatchjob.DispatchJob{
				failed,
				djJob("djb_2", "orders:shipping:parcel:sent", common.DispatchCompleted, "", 2),
				djJob("djb_3", "crm:contacts:person:updated", common.DispatchPending, "clt_2", 3),
			},
			attempts: map[string][]dispatchjob.Attempt{"djb_1": {
				{
					AttemptNumber: 1, AttemptedAt: djBase.Add(-50 * time.Second), ResponseCode: &code500, ResponseBody: &body, ErrorMessage: &errMsg,
					Request: &dispatchjob.RequestSummary{SignedBy: "orders-sa", Signature: true, Timestamp: "1700000000", Headers: []string{"Content-Type", "X-FlowCatalyst-Signature"}},
				},
				{AttemptNumber: 1, AttemptedAt: djBase.Add(-10 * time.Second), ResponseCode: &code200},
			}},
		},
		ops: &fakeDJOps{summary: djops.MatchSummary{Count: 12, Eligible: 9, ByStatus: map[string]int64{"FAILED": 10, "ERROR": 2}}},
	}
}

func (h *djHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &DispatchJobs{
		Jobs: h.store, Ops: h.ops, Clients: newCliHarness().store, Shell: shellForTest(),
		ClientIdentifier: func(_ context.Context, id string) (string, bool) {
			return map[string]string{"clt_1": "acme"}[id], id == "clt_1"
		},
	}
	return serveMounted(t, ac, q, page)
}

func (h *djHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func TestDispatchJobQueryRoundTrip(t *testing.T) {
	v := url.Values{
		"statuses": {"FAILED", "ERROR"}, "clients": {"clt_1"}, "codes": {"a:b:c:d"}, "q": {"orders"}, "group": {"g 1"},
		"from": {"2026-10-01"}, "to": {"2026-10-03"}, "size": {"500"}, "dir": {"asc"}, "after": {"1759665600000000.djb_9"},
	}
	q := parseDJQuery(v)
	if q.Size != 500 || q.Dir != "asc" || q.After == nil || q.After.ID != "djb_9" || !q.After.CreatedAt.Equal(time.Date(2025, 10, 5, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("parsed %+v", q)
	}
	again := parseDJQuery(mustQuery(t, q.URL()))
	if !reflect.DeepEqual(again.values(), q.values()) {
		t.Errorf("round trip: %s vs %s", again.URL(), q.URL())
	}
	p := q.filterParams()
	if *p.Since != time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC) || *p.Until != time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) ||
		*p.Source != "orders" || *p.MessageGroup != "g 1" || strings.Join(p.Statuses, ",") != "FAILED,ERROR" || p.Cursor != nil {
		t.Errorf("params %+v", p)
	}
	// The filter alone (what requeue-by-filter acts on) drops size, sort and position.
	if f := q.filterValues(); f.Has("size") || f.Has("after") || f.Has("dir") || f.Get("group") != "g 1" {
		t.Errorf("filter values %v", f)
	}
	junk := parseDJQuery(url.Values{"from": {"yesterday"}, "size": {"7"}, "after": {"nope"}, "dir": {"sideways"}})
	if junk.From != "" || junk.Size != djDefaultSize || junk.After != nil || junk.Dir != "desc" || junk.URL() != djPath {
		t.Errorf("junk %+v → %s", junk, junk.URL())
	}
}

func TestDispatchJobsList(t *testing.T) {
	h := newDJHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs"})
	mustContain(t, body, "<title>Dispatch Jobs · FlowCatalyst</title>", "Monitor webhook dispatch jobs and delivery status",
		"Notify &lt;billing&gt;", ">acme<", ">platform<", `title="Client id (identifier not resolved)">clt_2<`,
		`title="Filter by group order-42"`, "tenant=acme-long-ten…", "tenant: acme-long-tenant-name\nregion: eu",
		`datetime="2026-10-05T11:59:00Z"`, "Showing 3 dispatch jobs (newest first)", `name="size"`, `<option value="200" selected>`,
		`href="/ui/dispatch-jobs/requeue-by-filter"`, `href="/ui/dispatch-jobs/requeue-errors"`, `form="dj-selection"`,
		`data-action="/ui/dispatch-jobs/djb_1/requeue"`, `id="dj-filter-applications-field"`, `value="orders"`, `value="crm"`, `id="dj-created-from"`)
	// Group link: the first page narrowed to the group, re-rendering the toolbar.
	mustContain(t, body, `href="/ui/dispatch-jobs?group=order-42"`)

	// No view permission: refused.
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/dispatch-jobs"}); rec.Code != http.StatusForbidden {
		t.Errorf("no permission: %d", rec.Code)
	}

	// A client user: SQL scoping to its tenants (plus platform jobs).
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{djViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	own := h.do(t, scoped, req{target: "/ui/dispatch-jobs"})
	mustContain(t, own, "djb_1", "djb_2")
	mustNotContain(t, own, "djb_3", ">Globex<")
	if p := h.store.params[len(h.store.params)-1]; p.AccessibleClientIDs == nil || strings.Join(*p.AccessibleClientIDs, ",") != "clt_1" {
		t.Errorf("scoping %+v", p.AccessibleClientIDs)
	}

	// Live filter: table, pager, summaries and the dependent options; the URL.
	live := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?statuses=FAILED&applications=orders&size=50", datastar: true})
	mustContain(t, live, "djb_1", `id="dj-filter-statuses-summary" class="fc-multiselect-summary">FAILED<`,
		`id="dj-filter-subdomains-options"`, `value="billing"`, `value="shipping"`, "history.replaceState",
		"data: mode replace\ndata: elements <div id=\"dj-table\"")
	mustNotContain(t, live, "djb_3", `value="contacts"`, `id="dj-list"`)
	full := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?group=order-42", datastar: true, headers: map[string]string{djToolbarHeader: "full"}})
	mustContain(t, full, `id="dj-list"`, `id="dj-group"`, `value="order-42"`)
}

func TestDispatchJobsCursorPaging(t *testing.T) {
	h := newDJHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?size=50"})
	mustContain(t, body, `aria-disabled="true"`)
	// Fill a page: 50 jobs, one minute apart.
	for i := range 60 {
		h.store.jobs = append(h.store.jobs, djJob("djb_x"+strconv.Itoa(100+i), "a:b:c:d", common.DispatchPending, "", 10+i))
	}
	first := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?size=50"})
	last := h.store.jobs[49]
	older := "/ui/dispatch-jobs?after=" + strconv.FormatInt(last.CreatedAt.UnixMicro(), 10) + ".djb_x146&amp;size=50"
	mustContain(t, first, older, "All 50 jobs on this page are selected.")
	second := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?size=50&after=" + strconv.FormatInt(last.CreatedAt.UnixMicro(), 10) + ".djb_x146"})
	mustContain(t, second, "djb_x147", "Showing 13 dispatch jobs (newest first), an older page", `href="/ui/dispatch-jobs?size=50"`)
	mustNotContain(t, second, "djb_x146<", "All 13 jobs")
}

func TestDispatchJobDrawerAndSign(t *testing.T) {
	h := newDJHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, body, "Notify &lt;billing&gt;", "orders:billing:invoice:created", "1 / 3", "HTTP 500", "&#34;order&#34;: 42",
		"#2", "#1", "signed by orders-sa", "Content-Type, X-FlowCatalyst-Signature", "subscriber said no", "&#34;error&#34;: &#34;boom&#34;",
		`action="/ui/dispatch-jobs/djb_1/sign"`, `action="/ui/dispatch-jobs/djb_1/requeue"`, "tenant", "acme-long-tenant-name")
	// The payload as the SPA's formatJson shows it: key order kept, < not escaped as \u003c.
	page := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1"})
	mustContain(t, page, "&#34;z&#34;: 1,\n  &#34;order&#34;: 42,\n  &#34;note&#34;: &#34;&lt;b&gt;&#34;", `<aside id="dj-drawer"`)
	// Latest attempt first.
	if strings.Index(body, ">#2<") > strings.Index(body, ">#1<") {
		t.Error("attempts not latest-first")
	}

	viewer := connUser(djViewPerm)
	noSign := h.do(t, viewer, req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustNotContain(t, noSign, "/sign")
	if rec := h.rec(t, viewer, req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/sign", datastar: true, form: url.Values{}}); rec.Code == http.StatusOK && len(h.ops.planned) > 0 {
		t.Error("signed without the raw-view permission")
	}
	sign := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/sign", datastar: true, form: url.Values{}})
	mustContain(t, sign, "Delivery as it would go out now", "https://hooks.example.com/in", "--timestamp=&#39;1700000000&#39; --signature=&#39;sig&lt;1&gt;&#39;",
		`id="dj-plan-h-1"`, "Copy X-FlowCatalyst-Signature", "data: selector #dj-drawer-plan")
	mustNotContain(t, sign, "Copy Authorization")

	// Another tenant's job: the per-job scope check refuses it.
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{djViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	if rec := h.rec(t, scoped, req{target: "/ui/dispatch-jobs/djb_3"}); rec.Code != http.StatusForbidden {
		t.Errorf("other tenant's job: %d", rec.Code)
	}
}

func TestDispatchJobsRequeue(t *testing.T) {
	h := newDJHarness()
	sel := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", datastar: true, multipart: true,
		referer: "http://example.com/ui/dispatch-jobs?statuses=FAILED", form: url.Values{"ids": {"djb_1", "djb_3", "djb_1"}, "drawer": {"djb_3"}},
	})
	mustContain(t, sel, "2 dispatch jobs reset to PENDING", "data: selector #dj-drawer\n", "djSel")
	if got := h.ops.requeued[0]; strings.Join(got, ",") != "djb_1,djb_3" {
		t.Errorf("requeued %v", got)
	}
	one := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_2/requeue", datastar: true, form: url.Values{"drawer": {""}}})
	mustContain(t, one, "1 dispatch job reset to PENDING")
	mustNotContain(t, one, "selector #dj-drawer\n")
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_2/requeue", form: url.Values{}, referer: "http://example.com/ui/dispatch-jobs?size=50"})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/dispatch-jobs?size=50" {
		t.Errorf("native requeue: %d %s", native.Code, native.Header().Get("Location"))
	}
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_2/requeue", datastar: true, form: url.Values{}}); len(h.ops.requeued) != 3 {
		t.Errorf("requeued without permission (%d)", rec.Code)
	}
}

func TestDispatchJobsRequeueByFilter(t *testing.T) {
	h := newDJHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED&clients=clt_1&group=order-42&from=2026-10-01&size=50", datastar: true})
	mustContain(t, dlg, "Requeue all matching jobs", ">Client<", "Acme &lt;Corp&gt;", ">Status<", "FAILED", ">Message group<", "order-42", "2026-10-01",
		"12 jobs</strong> match", "FAILED · 10", "ERROR · 2", "9 jobs</strong> can be requeued", "3 platform-scoped or outside your client access",
		`name="confirmCount" value="9"`, "Requeue 9 jobs",
		`action="/ui/dispatch-jobs/requeue-by-filter?clients=clt_1&amp;from=2026-10-01&amp;group=order-42&amp;statuses=FAILED"`)
	if p := h.ops.previews[0]; strings.Join(p.Statuses, ",") != "FAILED" || *p.MessageGroup != "order-42" || p.Since == nil || p.Limit != 0 {
		t.Errorf("preview params %+v", p)
	}

	h.ops.result = djops.ResendByFilterResult{Requeued: 9}
	run := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED&group=order-42", datastar: true, multipart: true,
		referer: "http://example.com/ui/dispatch-jobs?statuses=FAILED&group=order-42", form: url.Values{"confirmCount": {"9"}},
	})
	mustContain(t, run, "Requeued 9 jobs", "9 dispatch jobs reset to PENDING.", "dj-rbf")
	if h.ops.confirms[0] != 9 || strings.Join(h.ops.byFilter[0].Statuses, ",") != "FAILED" || *h.ops.byFilter[0].MessageGroup != "order-42" {
		t.Errorf("run %+v %v", h.ops.byFilter[0], h.ops.confirms)
	}

	h.ops.result = djops.ResendByFilterResult{Requeued: 10000, Remaining: 250}
	capped := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter", datastar: true, form: url.Values{"confirmCount": {"10250"}}})
	mustContain(t, capped, "fc-flash-warn", "250 remain — run it again to continue.")

	h.ops.err = usecase.Conflict("FILTER_MATCH_CHANGED", "more match now")
	changed := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED", datastar: true, form: url.Values{"confirmCount": {"3"}}})
	mustContain(t, changed, "More jobs match than when you looked", `name="confirmCount" value="9"`)
	mustNotContain(t, changed, "fc-flash-error")

	bad := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue-by-filter", datastar: true, form: url.Values{"confirmCount": {"lots"}}})
	if !bytes.Contains(bad.Body.Bytes(), []byte("fc-flash-error")) || len(h.ops.byFilter) != 3 {
		t.Errorf("a bad confirm count ran: %s", bad.Body.String())
	}

	// The error preset switches the list to the error statuses and asks.
	h.ops.err = nil
	preset := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-errors?size=50", datastar: true})
	mustContain(t, preset, "statuses=FAILED&amp;statuses=ERROR", `id="dj-list"`, "Requeue all matching jobs", `value="ERROR" checked`)
	nofilter := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-by-filter", datastar: true})
	mustContain(t, nofilter, "No filters — this matches every dispatch job you can see.")
}

func BenchmarkDispatchJobsTable(b *testing.B) {
	for _, n := range []int{200, 1000} {
		v := djView{Query: parseDJQuery(url.Values{"size": {strconv.Itoa(n)}})}
		h := newDJHarness()
		tmpl := h.store.jobs[0]
		for i := range n {
			j := tmpl
			j.ID = "djb_" + strconv.Itoa(100000+i)
			v.Rows = append(v.Rows, (&DispatchJobs{}).toRow(context.Background(), v.Query, &j))
		}
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			var buf bytes.Buffer
			for b.Loop() {
				buf.Reset()
				if err := djTable(v).Render(context.Background(), &buf); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(buf.Len()), "bytes")
		})
	}
}

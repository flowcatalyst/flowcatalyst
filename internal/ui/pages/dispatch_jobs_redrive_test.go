package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	djops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// opsUser may requeue but not redrive.
func opsUser() *auth.AuthContext {
	return connUser(djViewPerm, djRequeuePerm)
}

func TestDispatchJobsRowActionsFollowStatusAndPermission(t *testing.T) {
	h := newDJHarness()
	// Requeue is offered on a status the API resets, Redrive on a COMPLETED job.
	full := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs"})
	mustContain(t, full, `data-action="/ui/dispatch-jobs/djb_1/requeue"`, `data-action="/ui/dispatch-jobs/djb_3/requeue"`, `data-action="/ui/dispatch-jobs/djb_2/redrive"`)
	mustNotContain(t, full, `data-action="/ui/dispatch-jobs/djb_2/requeue"`, `data-action="/ui/dispatch-jobs/djb_1/redrive"`)
	// Without redrive-completed the COMPLETED row has no action at all.
	plain := h.do(t, opsUser(), req{target: "/ui/dispatch-jobs"})
	mustContain(t, plain, `data-action="/ui/dispatch-jobs/djb_1/requeue"`)
	mustNotContain(t, plain, "/djb_2/redrive", "/djb_2/requeue")
	// Without the requeue permission, neither.
	viewer := h.do(t, connUser(djViewPerm), req{target: "/ui/dispatch-jobs"})
	mustNotContain(t, viewer, "/requeue\"", "/redrive\"")

	// The drawer follows the same rule.
	drawer := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_2", datastar: true})
	mustContain(t, drawer, `action="/ui/dispatch-jobs/djb_2/redrive"`, "Redrive")
	mustNotContain(t, drawer, `/djb_2/requeue"`)
	mustNotContain(t, h.do(t, opsUser(), req{target: "/ui/dispatch-jobs/djb_2", datastar: true}), "/redrive", "/requeue")
	mustContain(t, h.do(t, opsUser(), req{target: "/ui/dispatch-jobs/djb_1", datastar: true}), `action="/ui/dispatch-jobs/djb_1/requeue"`)
}

func TestDispatchJobsRequeueSelectedWithCompleted(t *testing.T) {
	h := newDJHarness()
	post := func(ac *auth.AuthContext, path string, form url.Values) string {
		return h.do(t, ac, req{method: http.MethodPost, target: path, datastar: true, multipart: true, referer: "http://example.com/ui/dispatch-jobs", form: form})
	}
	ids := []string{"djb_1", "djb_2", "djb_3"}

	// A holder of redrive-completed is asked first; nothing runs.
	review := post(anchorAdmin(), "/ui/dispatch-jobs/requeue", url.Values{"ids": ids})
	mustContain(t, review, `data: elements <div id="dj-dialog-slot">`, "Requeue selected jobs", "3 jobs</strong> selected, <strong>1</strong> of them COMPLETED",
		"Also redrive 1 COMPLETED job…", "Requeue 2 other jobs", `name="ids" value="djb_2"`)
	if len(h.ops.requeued) != 0 {
		t.Fatalf("ran before the dialog was answered: %v", h.ops.requeued)
	}

	// "Requeue 2 other jobs" runs a plain requeue.
	h.ops.requeue = RequeueResult{Requeued: 2, SkippedCompleted: 1}
	plain := post(anchorAdmin(), "/ui/dispatch-jobs/requeue", url.Values{"ids": ids, "step": {"plain"}})
	mustContain(t, plain, "Requeued 2 jobs", "2 dispatch jobs reset to PENDING — 1 COMPLETED job left alone", "dj-sel")
	if len(h.ops.requeued) != 1 || h.ops.redrives[0] {
		t.Fatalf("plain run: %v %v", h.ops.requeued, h.ops.redrives)
	}

	// "Also redrive" leads to the typed confirmation; nothing runs yet.
	confirm := post(anchorAdmin(), "/ui/dispatch-jobs/requeue", url.Values{"ids": ids, "step": {"confirm"}})
	mustContain(t, confirm, "Redrive COMPLETED jobs", "were already delivered", `name="word"`, `name="count"`, "Redrive 1 COMPLETED job")
	if len(h.ops.requeued) != 1 {
		t.Fatalf("ran at the confirm step: %v", h.ops.requeued)
	}

	// A wrong word or count re-asks, with the reason on the field.
	for name, form := range map[string]url.Values{
		"word":  {"ids": ids, "step": {"redrive"}, "word": {"redrive"}, "count": {"1"}},
		"count": {"ids": ids, "step": {"redrive"}, "word": {"REDRIVE"}, "count": {"2"}},
		"empty": {"ids": ids, "step": {"redrive"}},
	} {
		got := post(anchorAdmin(), "/ui/dispatch-jobs/requeue", form)
		mustContain(t, got, "fc-field-error", "Redrive COMPLETED jobs")
		if len(h.ops.requeued) != 1 {
			t.Fatalf("%s: ran with a wrong confirmation: %v", name, h.ops.requeued)
		}
	}

	// Typed correctly (a thousands separator is tolerated): every selected job goes, redriving the COMPLETED one.
	h.ops.requeue = RequeueResult{Requeued: 3, Redriven: 1}
	done := post(anchorAdmin(), "/ui/dispatch-jobs/requeue", url.Values{"ids": ids, "step": {"redrive"}, "word": {"REDRIVE"}, "count": {" 1 "}})
	mustContain(t, done, "Requeued 3 jobs (1 COMPLETED job redriven)")
	if len(h.ops.requeued) != 2 || !h.ops.redrives[1] || strings.Join(h.ops.requeued[1], ",") != "djb_1,djb_2,djb_3" {
		t.Fatalf("redrive run: %v %v", h.ops.requeued, h.ops.redrives)
	}

	// Without redrive-completed the same selection requeues at once and says what was left alone.
	h.ops.requeue = RequeueResult{Requeued: 2, SkippedCompleted: 1}
	direct := post(opsUser(), "/ui/dispatch-jobs/requeue", url.Values{"ids": ids})
	mustContain(t, direct, "2 dispatch jobs reset to PENDING — 1 COMPLETED job left alone")
	mustNotContain(t, direct, "Requeue selected jobs")
	if len(h.ops.requeued) != 3 || h.ops.redrives[2] {
		t.Fatalf("direct run: %v %v", h.ops.requeued, h.ops.redrives)
	}
	// ...and cannot ask for a redrive by posting the step.
	denied := h.rec(t, opsUser(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/requeue", form: url.Values{"ids": ids, "step": {"redrive"}, "word": {"REDRIVE"}, "count": {"1"}}})
	if denied.Code != http.StatusForbidden || len(h.ops.requeued) != 3 {
		t.Errorf("redrive without permission: %d, %d runs", denied.Code, len(h.ops.requeued))
	}

	// Nothing COMPLETED among them: no dialog, a plain requeue.
	h.ops.requeue = RequeueResult{}
	post(anchorAdmin(), "/ui/dispatch-jobs/requeue", url.Values{"ids": {"djb_1", "djb_3"}})
	if len(h.ops.requeued) != 4 {
		t.Errorf("no COMPLETED job, no run: %d", len(h.ops.requeued))
	}
}

func TestDispatchJobsRedriveOne(t *testing.T) {
	h := newDJHarness()
	dlg := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_2/redrive", datastar: true, form: url.Values{"drawer": {"djb_2"}}})
	mustContain(t, dlg, "1 job</strong> selected, <strong>1</strong> of them COMPLETED", `name="drawer" value="djb_2"`)
	// Nothing else is selected, so the plain requeue is disabled.
	mustContain(t, dlg, "Requeue 0 other jobs", "disabled")
	if len(h.ops.requeued) != 0 {
		t.Fatal("redrive ran before the confirmation")
	}
	if rec := h.rec(t, opsUser(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_2/redrive", datastar: true, form: url.Values{}}); len(h.ops.requeued) != 0 {
		t.Errorf("redrive without the permission (%d)", rec.Code)
	}
}

func TestDispatchJobsRequeueByFilterRedrive(t *testing.T) {
	h := newDJHarness()
	h.ops.summary = djops.MatchSummary{Count: 20, Eligible: 9, Completed: 4, InFlight: 1, ByStatus: map[string]int64{"FAILED": 9, "COMPLETED": 4, "QUEUED": 1}}

	// A holder is offered the redrive; the step carries the filter.
	review := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED&statuses=COMPLETED", datastar: true})
	mustContain(t, review, "Also redrive 4 COMPLETED jobs…", "statuses=FAILED&amp;statuses=COMPLETED&amp;step=redrive", `name="confirmCount" value="9"`)
	mustNotContain(t, review, `name="redrive"`)
	// Someone without it is told why, and offered nothing.
	noPerm := h.do(t, opsUser(), req{target: "/ui/dispatch-jobs/requeue-by-filter", datastar: true})
	mustContain(t, noPerm, "Redriving them needs the redrive-completed permission.")
	mustNotContain(t, noPerm, "Also redrive")

	// The redrive step confirms eligible + COMPLETED.
	step := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED&step=redrive", datastar: true})
	mustContain(t, step, "Redrive COMPLETED jobs", `name="confirmCount" value="13"`, `name="redrive" value="1"`, "number of jobs that will be reset: 13", "Redrive 4 COMPLETED jobs", ">Back<")
	// The step is not part of the filter the form posts to.
	mustContain(t, step, `action="/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED"`)

	runURL := "/ui/dispatch-jobs/requeue-by-filter?statuses=FAILED"
	bad := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: runURL, datastar: true, form: url.Values{"confirmCount": {"13"}, "redrive": {"1"}, "word": {"REDRIVE"}, "count": {"9"}}})
	mustContain(t, bad, "fc-field-error", "Type the number of jobs shown: 13.")
	if len(h.ops.byFilter) != 0 {
		t.Fatal("ran with a wrong count")
	}
	h.ops.result = djops.ResendByFilterResult{Requeued: 13, Redriven: 4, SkippedInFlight: 1}
	ok := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: runURL, datastar: true, form: url.Values{"confirmCount": {"13"}, "redrive": {"1"}, "word": {"REDRIVE"}, "count": {"13"}}})
	mustContain(t, ok, "Requeued 13 jobs (4 COMPLETED jobs redriven)", "1 job in flight (QUEUED or PROCESSING) left alone")
	if len(h.ops.byFilter) != 1 || !h.ops.byRedrive[0] || h.ops.confirms[0] != 13 {
		t.Fatalf("redrive run: %v %v", h.ops.byRedrive, h.ops.confirms)
	}

	// A plain run does not redrive, and reports the COMPLETED jobs it left alone.
	h.ops.result = djops.ResendByFilterResult{Requeued: 9, SkippedCompleted: 4}
	plain := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: runURL, datastar: true, form: url.Values{"confirmCount": {"9"}}})
	mustContain(t, plain, "Requeued 9 jobs", "4 COMPLETED jobs left alone")
	if h.ops.byRedrive[1] {
		t.Error("a plain run redrove")
	}

	// Posting the redrive without the permission runs nothing.
	denied := h.rec(t, opsUser(), req{method: http.MethodPost, target: runURL, form: url.Values{"confirmCount": {"13"}, "redrive": {"1"}, "word": {"REDRIVE"}, "count": {"13"}}})
	if denied.Code != http.StatusForbidden || len(h.ops.byFilter) != 2 {
		t.Errorf("redrive without permission: %d, %d runs", denied.Code, len(h.ops.byFilter))
	}

	// The filter changed under the dialog: back to the review with the new counts.
	h.ops.err = usecase.Conflict("FILTER_MATCH_CHANGED", "more match now")
	changed := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: runURL, datastar: true, form: url.Values{"confirmCount": {"12"}, "redrive": {"1"}, "word": {"REDRIVE"}, "count": {"12"}}})
	mustContain(t, changed, "More jobs match than when you looked", "Requeue all matching jobs")
	mustNotContain(t, changed, `name="redrive"`)
}

func TestDispatchJobsStuckLens(t *testing.T) {
	q := parseDJQuery(url.Values{"statuses": {"PENDING", "QUEUED"}, "lens": {"stuck"}})
	if !q.stuckLens() || q.failedLens() {
		t.Fatalf("parsed %+v", q)
	}
	if again := parseDJQuery(mustQuery(t, q.URL())); !again.stuckLens() || again.URL() != q.URL() {
		t.Errorf("round trip %s vs %s", again.URL(), q.URL())
	}
	// Picking other statuses by hand leaves the cap behind; a bare lens does not apply.
	if parseDJQuery(url.Values{"statuses": {"PENDING"}, "lens": {"stuck"}}).stuckLens() || parseDJQuery(url.Values{"lens": {"stuck"}}).stuckLens() {
		t.Error("the lens outlived its statuses")
	}
	if f := q.filterValues(); f.Get("lens") != "stuck" {
		t.Errorf("a requeue by filter must act on the lens: %v", f)
	}
	// Toggling: on sets the statuses and the lens; the Failed lens replaces it; again clears it.
	on := parseDJQuery(mustQuery(t, parseDJQuery(nil).stuckLensURL()))
	if !on.stuckLens() || strings.Join(on.facet(djFacetStatuses), ",") != "PENDING,QUEUED" {
		t.Errorf("toggle on: %+v", on)
	}
	if off := parseDJQuery(mustQuery(t, on.stuckLensURL())); off.stuckLens() || len(off.facet(djFacetStatuses)) != 0 {
		t.Errorf("toggle off: %+v", off)
	}
	if failed := parseDJQuery(mustQuery(t, on.failedLensURL())); failed.stuckLens() || !failed.failedLens() {
		t.Errorf("failed over stuck: %+v", failed)
	}

	// The list reads only jobs older than five minutes, inside the (defaulted) window.
	h := newDJHarness()
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	body := h.recAt(t, now, req{target: "/ui/dispatch-jobs?statuses=PENDING&statuses=QUEUED&lens=stuck"})
	mustContain(t, body, `aria-pressed="true"`, "Stuck", `name="lens" value="stuck"`)
	last := h.store.params[len(h.store.params)-1]
	if last.Until == nil || !last.Until.Equal(now.Add(-5*time.Minute)) || last.Since == nil || !last.Since.Equal(now.Add(-5*time.Minute-24*time.Hour)) {
		t.Errorf("window %v .. %v", last.Since, last.Until)
	}
	// Without the lens there is no cap.
	h.recAt(t, now, req{target: "/ui/dispatch-jobs?statuses=PENDING&statuses=QUEUED"})
	if last := h.store.params[len(h.store.params)-1]; last.Until != nil {
		t.Errorf("uncapped view has until %v", last.Until)
	}
	// A cap never raises an earlier end date chosen by the user.
	h.recAt(t, now, req{target: "/ui/dispatch-jobs?statuses=PENDING&statuses=QUEUED&lens=stuck&from=2026-10-10&to=2026-10-11"})
	if last := h.store.params[len(h.store.params)-1]; !last.Until.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("until %v", last.Until)
	}
}

func TestDispatchJobsDrawerNamesSubscriptionAndConnection(t *testing.T) {
	h := newDJHarness()
	subID := "sub_1"
	h.store.jobs[0].SubscriptionID = &subID
	s := sub("sub_1", "orders-hook", "Orders <hook>", subscription.StatusActive, "orders", "")
	con := "con_1"
	s.ConnectionID = &con
	h.subs = &fakeSubs{subs: []subscription.Subscription{s}}
	h.conns = &fakeConns{conns: []connection.Connection{conn("con_1", "billing", "Billing", connection.StatusActive, "")}}

	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, body, `<a href="/ui/subscriptions/sub_1">Orders &lt;hook&gt;</a>`, "(orders-hook)", `<a href="/ui/connections/con_1">billing</a>`, "Connection")

	// A caller who may not read subscriptions keeps the bare id (the Connection line still shows).
	bare := h.do(t, connUser(djViewPerm), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, bare, `<span class="font-mono">sub_1</span>`)
	mustNotContain(t, bare, "/ui/subscriptions/sub_1", "/ui/connections/con_1")

	// A job without a subscription has no Connection line.
	none := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_3", datastar: true})
	mustNotContain(t, none, ">Connection<")
}

func TestDispatchJobsAudit(t *testing.T) {
	h := newDJHarness()
	name, pid := "Ada <Ops>", "prn_9"
	h.audit = &fakeDJAudit{rows: []audit.Log{
		{ID: "aud_2", Operation: "EditPayloadCommand", PrincipalName: &name, PerformedAt: djBase},
		{ID: "aud_1", Operation: "CancelCommand", PrincipalID: &pid, PerformedAt: djBase.Add(-time.Hour)},
		{ID: "aud_0", Operation: "ResendDispatchJobs", PerformedAt: djBase.Add(-2 * time.Hour)},
	}}

	// The drawer carries the section (closed) for a holder of the audit permission; it is filled when opened.
	drawer := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, drawer, "<summary", ">Audit<", "/ui/dispatch-jobs/djb_1/audit")
	mustNotContain(t, h.do(t, connUser(djViewPerm), req{target: "/ui/dispatch-jobs/djb_1", datastar: true}), "/audit")

	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1/audit", datastar: true})
	mustContain(t, body, `id="dj-audit-body"`, "<strong>Edit payload</strong>", "Ada &lt;Ops&gt;", "<strong>Cancel</strong>", "prn_9", "<strong>Resend dispatch jobs</strong>", "system")
	if f := h.audit.filter[0]; *f.EntityType != "Dispatchjob" || *f.EntityID != "djb_1" || f.Limit != 500 {
		t.Errorf("filter %+v", f)
	}

	h.audit.rows = nil
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1/audit", datastar: true}), "No audit entries: nobody has changed this job by hand.")
	h.audit.err = http.ErrAbortHandler
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1/audit", datastar: true}), "The audit trail could not be loaded.")

	// The audit permission and the job's tenant are checked.
	if rec := h.rec(t, connUser(djViewPerm), req{target: "/ui/dispatch-jobs/djb_1/audit", datastar: true}); rec.Code != http.StatusForbidden && strings.Contains(rec.Body.String(), "Edit payload") {
		t.Errorf("audit without the permission: %d", rec.Code)
	}
}

func TestAuditOperationLabel(t *testing.T) {
	for in, want := range map[string]string{
		"CancelCommand": "Cancel", "EditPayloadCommand": "Edit payload", "ResendDispatchJobs": "Resend dispatch jobs", "": "",
	} {
		if got := auditOperationLabel(in); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

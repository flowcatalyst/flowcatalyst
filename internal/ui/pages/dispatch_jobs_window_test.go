package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// recAt is rec with the clock fixed, so the window presets are exact.
func (h *djHarness) recAt(t *testing.T, now time.Time, q req) string {
	t.Helper()
	page := &DispatchJobs{
		Jobs: h.store, Ops: h.ops, Clients: newCliHarness().store, Shell: shellForTest(),
		ClientIdentifier: func(_ context.Context, id string) (string, bool) { return id, false },
		Now:              func() time.Time { return now },
	}
	return serveMounted(t, anchorAdmin(), q, h.wire(page)).Body.String()
}

func TestDispatchJobQueryNewFilters(t *testing.T) {
	q := parseDJQuery(url.Values{
		"eventTypes": {"a:b:c:d, e:f:g:h", "a:b:c:d"}, "subscription": {" orders* "}, "connection": {"billing"}, "window": {"7d"},
	})
	if strings.Join(q.EventTypes, "|") != "a:b:c:d|e:f:g:h" || q.Subscription != "orders*" || q.Connection != "billing" || q.Window != "7d" {
		t.Fatalf("parsed %+v", q)
	}
	again := parseDJQuery(mustQuery(t, q.URL()))
	if again.URL() != q.URL() {
		t.Errorf("round trip %s vs %s", again.URL(), q.URL())
	}
	p := q.filterParams()
	if strings.Join(p.EventTypes, "|") != "a:b:c:d|e:f:g:h" || *p.SubscriptionName != "orders*" || *p.ConnectionCode != "billing" {
		t.Errorf("params %+v", p)
	}
	if q.activeFilters() != 3 || !q.filtered() {
		t.Errorf("badge %d filtered %v", q.activeFilters(), q.filtered())
	}
	// An unknown preset falls back to the default window.
	if parseDJQuery(url.Values{"window": {"5y"}}).Window != "" {
		t.Error("unknown preset kept")
	}
	// A requeue-by-filter acts on exactly this filter.
	if f := q.filterValues(); f.Get("eventTypes") != "a:b:c:d,e:f:g:h" || f.Get("window") != "7d" || f.Get("subscription") != "orders*" {
		t.Errorf("filter values %v", f)
	}
}

func TestDispatchJobsWindowBar(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	h := newDJHarness()

	// Nothing chosen: the server's default, shown as such.
	body := h.recAt(t, now, req{target: "/ui/dispatch-jobs"})
	mustContain(t, body, `data-testid="window-label">Showing the last 24 hours`, "default window",
		"No dates were chosen, so only the last 24 hours are shown", `href="/ui/dispatch-jobs?window=1h"`,
		`href="/ui/dispatch-jobs?window=7d"`, `href="/ui/dispatch-jobs?window=31d"`, `aria-current="true"`)
	p := h.store.params[len(h.store.params)-1]
	if p.Since == nil || !p.Since.Equal(now.Add(-24*time.Hour)) {
		t.Errorf("default since %v", p.Since)
	}

	// A preset: since = now minus the span, not defaulted.
	body = h.recAt(t, now, req{target: "/ui/dispatch-jobs?window=7d"})
	mustContain(t, body, `data-testid="window-label">Showing the last 7 days`)
	mustNotContain(t, body, "default window", "No dates were chosen")
	p = h.store.params[len(h.store.params)-1]
	if p.Since == nil || !p.Since.Equal(now.Add(-7*24*time.Hour)) {
		t.Errorf("7d since %v", p.Since)
	}

	// Chosen dates win over a preset and are described by their dates.
	body = h.recAt(t, now, req{target: "/ui/dispatch-jobs?window=7d&from=2026-10-01&to=2026-10-02"})
	mustContain(t, body, "Showing 2026-10-01 00:00 to 2026-10-03 00:00 UTC")
	p = h.store.params[len(h.store.params)-1]
	if p.Since == nil || !p.Since.Equal(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("custom since %v", p.Since)
	}

	// A window wider than the cap is a 400, not a silent clip.
	rec := serveMounted(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?from=2026-08-01&to=2026-10-01"},
		&DispatchJobs{Jobs: h.store, Ops: h.ops, Clients: newCliHarness().store, Shell: shellForTest(), Now: func() time.Time { return now }})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("too wide: %d", rec.Code)
	}
}

func TestDispatchJobsWindowText(t *testing.T) {
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		span time.Duration
		want string
	}{{time.Hour, "Showing the last hour"}, {24 * time.Hour, "Showing the last 24 hours"}, {72 * time.Hour, "Showing the last 3 days"}, {31 * 24 * time.Hour, "Showing the last 31 days"}} {
		if got := windowText(dispatchjob.Window{From: now.Add(-c.span), To: now}, now); got != c.want {
			t.Errorf("%v: %q", c.span, got)
		}
	}
}

func TestDispatchJobsNewFiltersReachTheQuery(t *testing.T) {
	h := newDJHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?eventTypes=orders:order:created&subscription=orders*&connection=billing"})
	mustContain(t, body, `id="dj-event-types"`, `value="orders:order:created"`, `value="orders*"`, `value="billing"`, `<datalist id="dj-event-type-options">`)
	p := h.store.params[len(h.store.params)-1]
	if strings.Join(p.EventTypes, ",") != "orders:order:created" || *p.SubscriptionName != "orders*" || *p.ConnectionCode != "billing" {
		t.Errorf("list params %+v", p)
	}
	// The by-name references are resolved after the tenant scope is applied (the API's order).
	r := h.store.resolved[len(h.store.resolved)-1]
	if r.AccessibleClientIDs != nil || r.SubscriptionName == nil {
		t.Errorf("resolved %+v", r)
	}
	// Requeue by filter acts on the same filter, resolved.
	before := len(h.store.resolved)
	h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-by-filter?eventTypes=a:b:c:d&connection=billing*", datastar: true})
	if len(h.store.resolved) != before+1 || h.ops.previews[0].ConnectionCode == nil || *h.ops.previews[0].ConnectionCode != "billing*" || h.ops.previews[0].Since == nil {
		t.Errorf("preview params %+v", h.ops.previews)
	}

	// More event types than the API allows: refused with its code.
	many := make([]string, 11)
	for i := range many {
		many[i] = "t:" + string(rune('a'+i))
	}
	rec := h.rec(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?eventTypes=" + url.QueryEscape(strings.Join(many, ","))})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("11 event types: %d", rec.Code)
	}
	// A reference that matches too many records is the repository's 400.
	h.store.resolveErr = usecase.Validation(dispatchjob.TooManyMatchesCode, "narrow it")
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?subscription=a*"}); rec.Code != http.StatusBadRequest {
		t.Errorf("too many matches: %d", rec.Code)
	}
}

func TestDispatchJobsFailedLens(t *testing.T) {
	h := newDJHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs"})
	mustContain(t, body, `href="/ui/dispatch-jobs?statuses=FAILED&amp;statuses=ERROR"`)
	mustNotContain(t, body, `title="FAILED and ERROR jobs" aria-pressed`)
	// On: the link turns it off, keeping the other filters.
	on := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?statuses=FAILED&statuses=ERROR&window=3d"})
	mustContain(t, on, `title="FAILED and ERROR jobs" aria-pressed="true"`, `href="/ui/dispatch-jobs?window=3d"`)
	// Off to on keeps the window and the group.
	other := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs?group=order-42&window=3d"})
	mustContain(t, other, "statuses=FAILED&amp;statuses=ERROR&amp;window=3d")
	// The error preset keeps the window.
	preset := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/requeue-errors?window=7d", datastar: true})
	mustContain(t, preset, "window=7d", "Last 7 days")
}

func TestDispatchJobsCancelComplete(t *testing.T) {
	h := newDJHarness()
	// The drawer of a FAILED job offers both to a holder...
	full := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, full, `data-action="/ui/dispatch-jobs/djb_1/cancel"`, `data-action="/ui/dispatch-jobs/djb_1/complete"`, "Cancel job", "Mark completed", ">Event type<")
	// ...each with its own permission...
	cancelOnly := h.do(t, connUser(djViewPerm, djCancelPerm), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustContain(t, cancelOnly, `/djb_1/cancel"`)
	mustNotContain(t, cancelOnly, `/djb_1/complete"`, `/djb_1/requeue"`)
	// ...view alone gets no write action...
	viewer := h.do(t, connUser(djViewPerm), req{target: "/ui/dispatch-jobs/djb_1", datastar: true})
	mustNotContain(t, viewer, `/djb_1/cancel"`, `/djb_1/complete"`)
	// ...and only a FAILED job has them.
	pending := h.do(t, anchorAdmin(), req{target: "/ui/dispatch-jobs/djb_3", datastar: true})
	mustNotContain(t, pending, `/djb_3/cancel"`, `/djb_3/complete"`)

	run := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/cancel", datastar: true, form: url.Values{}})
	mustContain(t, run, "Cancelled", "The job will not be retried")
	done := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/complete", datastar: true, form: url.Values{}})
	mustContain(t, done, "Marked completed")
	if strings.Join(h.ops.cancels, ",") != "djb_1" || strings.Join(h.ops.completes, ",") != "djb_1" {
		t.Errorf("ran cancel %v complete %v", h.ops.cancels, h.ops.completes)
	}
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/cancel", form: url.Values{}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/dispatch-jobs/djb_1" {
		t.Errorf("native: %d %s", native.Code, native.Header().Get("Location"))
	}

	// Without the permission the use case never runs.
	before := len(h.ops.cancels) + len(h.ops.completes)
	for _, act := range []string{"cancel", "complete"} {
		if rec := h.rec(t, connUser(djViewPerm, djRequeuePerm), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/" + act, form: url.Values{}}); rec.Code != http.StatusForbidden {
			t.Errorf("%s without permission: %d", act, rec.Code)
		}
	}
	if len(h.ops.cancels)+len(h.ops.completes) != before {
		t.Error("ran without permission")
	}

	// The use case's 409 (no longer FAILED) reaches the user, not a success flash.
	h.ops.err = usecase.Conflict("INVALID_STATE", "only a FAILED job can be cancelled")
	failed := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dispatch-jobs/djb_1/cancel", datastar: true, form: url.Values{}})
	mustContain(t, failed, "fc-flash-error")
	mustNotContain(t, failed, "The job will not be retried")
}

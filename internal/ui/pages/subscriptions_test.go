package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeSubs struct{ subs []subscription.Subscription }

func (f *fakeSubs) FindWithFilters(_ context.Context, status, _ *string) ([]subscription.Subscription, error) {
	var out []subscription.Subscription
	for _, s := range f.subs {
		if status == nil || string(s.Status) == *status {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSubs) FindByID(_ context.Context, id string) (*subscription.Subscription, error) {
	for i := range f.subs {
		if f.subs[i].ID == id {
			s := f.subs[i]
			return &s, nil
		}
	}
	return nil, nil
}

type fakeSubOps struct {
	created []subops.CreateCommand
	updated []subops.UpdateCommand
	actions []string
	err     error
	subs    *fakeSubs
}

func (o *fakeSubOps) Create(_ context.Context, cmd subops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.subs.subs = append(o.subs.subs, sub("sub_9", cmd.Code, cmd.Name, subscription.StatusActive, "", ""))
	return "sub_9", nil
}

func (o *fakeSubOps) Update(_ context.Context, cmd subops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeSubOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeSubOps) Pause(_ context.Context, id string) error  { return o.act("pause", id) }
func (o *fakeSubOps) Resume(_ context.Context, id string) error { return o.act("resume", id) }
func (o *fakeSubOps) Delete(_ context.Context, id string) error { return o.act("delete", id) }

func sub(id, code, name string, status subscription.Status, app, client string) subscription.Subscription {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	pool := "orders-pool"
	et, v := "orders:sales:order:placed", "1.0"
	s := subscription.Subscription{
		ID: id, Code: code, Name: name, Status: status, Endpoint: "https://hooks.example.com/" + code,
		Source: subscription.SourceUI, Mode: common.DispatchNextOnError, MaxAgeSeconds: 86400, TimeoutSeconds: 30, Sequence: 99,
		DispatchPoolCode: &pool, EventTypes: []subscription.EventTypeBinding{{EventTypeCode: et, SpecVersion: &v}}, CreatedAt: day, UpdatedAt: day,
	}
	if app != "" {
		s.ApplicationCode = &app
	}
	if client != "" {
		cid, ident := ids.ClientID("clt_"+client), client
		s.ClientID, s.ClientIdentifier = &cid, &ident
	}
	return s
}

type subHarness struct {
	subs *fakeSubs
	ops  *fakeSubOps
	ets  *fakeETs
}

func newSubHarness() *subHarness {
	f := &fakeSubs{subs: []subscription.Subscription{
		sub("sub_1", "orders-hook", "Orders <hook>", subscription.StatusActive, "orders", ""),
		sub("sub_2", "billing-hook", "Billing hook", subscription.StatusPaused, "billing", "acme"),
		sub("sub_3", "misc-hook", "Misc hook", subscription.StatusActive, "", "globex"),
	}}
	ets := newETHarness().store
	scoped := et("evt_c", "orders:sales:order:scoped", "Scoped order", eventtype.StatusCurrent, spec("2.0", eventtype.SpecCurrent, orderSchema))
	scoped.ClientScoped = true
	ets.ets = append(ets.ets, scoped)
	return &subHarness{subs: f, ops: &fakeSubOps{subs: f}, ets: ets}
}

func (h *subHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &Subscriptions{Subs: h.subs, Ops: h.ops, Pools: fixturePools(), Connections: newConnHarness().conns, EventTypes: h.ets, Apps: fakeApps{}, Clients: someClients(), Shell: shellForTest()}
	return serveMounted(t, ac, q, page)
}

func (h *subHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const subViewPerm = "platform:messaging:subscription:view"

func TestSubscriptionsList(t *testing.T) {
	h := newSubHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions"})
	mustContain(t, body, `<title>Subscriptions · FlowCatalyst</title>`, "Manage event subscriptions and webhook routing",
		"Create Subscription", "Orders &lt;hook&gt;", "1 event type<", "Next on Error", `<code class="fc-mono-chip">orders-pool</code>`,
		"Anchor-level", ">acme<", `name="app" value="billing"`, "Showing 1 to 3 of 3 subscriptions",
		`<dialog id="sub-pause"`, `<dialog id="sub-delete"`, `aria-label="Edit Orders &lt;hook&gt;"`)
	filtered := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions?app=billing&status=PAUSED&q=hook", datastar: true})
	mustContain(t, filtered, "Billing hook", `id="subs-filter-app-summary" class="fc-multiselect-summary">billing<`)
	mustNotContain(t, filtered, "Orders &lt;hook&gt;")
	// A subscription created in the UI stores the pool id only; its code is looked up.
	byID := sub("sub_4", "ui-made", "UI made", subscription.StatusActive, "", "")
	pid := "dpl_1"
	byID.DispatchPoolCode, byID.DispatchPoolID = nil, &pid
	h.subs.subs = append(h.subs.subs, byID)
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/subscriptions/sub_4", datastar: true}), "<code>billing</code>")
	sorted := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions?sort=name&dir=desc"})
	if strings.Index(sorted, "Orders &lt;hook&gt;") > strings.Index(sorted, "Billing hook") {
		t.Error("name desc: Orders should come first")
	}
}

func TestSubscriptionsPermissions(t *testing.T) {
	h := newSubHarness()
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/subscriptions"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view: %d", rec.Code)
	}
	viewer := h.do(t, connUser(subViewPerm), req{target: "/ui/subscriptions"})
	mustNotContain(t, viewer, "Create Subscription", `aria-label="Edit `, `<dialog id="sub-pause"`, `<dialog id="sub-delete"`)
	deleter := connUser(subViewPerm, "platform:messaging:subscription:delete")
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/subscriptions/sub_1/pause", datastar: true, form: url.Values{}})
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: subCreateForm()})
	if len(h.ops.actions) != 0 || len(h.ops.created) != 0 {
		t.Fatalf("a delete-only user ran %v / created %d", h.ops.actions, len(h.ops.created))
	}
	drawer := h.do(t, deleter, req{target: "/ui/subscriptions/sub_1", datastar: true})
	mustContain(t, drawer, "Delete Subscription")
	mustNotContain(t, drawer, "Pause Subscription", ">Edit<")
	scoped := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"}, Permissions: []string{subViewPerm}, PrincipalType: auth.PrincipalTypeUser}
	mustNotContain(t, h.do(t, scoped, req{target: "/ui/subscriptions"}), "Misc hook")
	if rec := h.rec(t, scoped, req{target: "/ui/subscriptions/sub_3"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's subscription: %d", rec.Code)
	}
}

func TestSubscriptionDrawerAndActions(t *testing.T) {
	h := newSubHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions/sub_1", datastar: true})
	mustContain(t, body, "Subscription Details", "Event Types (1)", "orders</span>", "https://hooks.example.com/orders-hook",
		"86400 seconds", "Pause Subscription", `"selSub":"sub_1"`)
	mustNotContain(t, body, "Resume Subscription")
	for _, a := range []string{"sub_1/pause", "sub_2/resume", "sub_1/delete"} {
		out := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/" + a, datastar: true, form: url.Values{"drawer": {"sub_1"}}})
		mustContain(t, out, "fc-flash-success", `id="subs-table"`)
	}
	if got := strings.Join(h.ops.actions, ","); got != "pause:sub_1,resume:sub_2,delete:sub_1" {
		t.Errorf("actions = %s", got)
	}
}

func subCreateForm(kv ...string) url.Values {
	v := url.Values{
		"code": {"new-hook"}, "name": {"New hook"}, "description": {""}, "eventTypes": {"evt_1"}, "endpoint": {"https://x.example.com/h"},
		"connectionId": {""}, "queue": {"HIGH_PRIORITY"}, "dispatchPoolId": {"dpl_1"}, "mode": {"BLOCK_ON_ERROR"},
		"maxAgeSeconds": {"600"}, "timeoutSeconds": {"20"}, "delaySeconds": {"0"}, "client": {""},
	}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestSubscriptionCreate(t *testing.T) {
	h := newSubHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions/new", datastar: true})
	mustContain(t, dlg, "data: mode replace\n", `data: elements <div id="sub-dialog-slot">`, "Create Subscription", `name="eventTypes" value="evt_1"`, "Order placed — orders:sales:order:placed (v1.0)",
		`<option value="HIGH_PRIORITY">High priority</option>`, `<option value="NEXT_ON_ERROR" selected>`, `value="86400"`,
		"Only non-client-scoped event types are shown.", `data-show="$subClientScoped"`)
	mustNotContain(t, dlg, `value="evt_c"`, `value="evt_3"`, `value="evt_5"`)

	opts := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions/event-type-options?clientScoped=true", datastar: true})
	mustContain(t, opts, `value="evt_c"`, "Only client-scoped event types are shown.")
	mustNotContain(t, opts, `value="evt_1"`)

	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, multipart: true, form: subCreateForm()})
	if len(h.ops.created) != 1 {
		t.Fatalf("creates %d: %s", len(h.ops.created), body)
	}
	c := h.ops.created[0]
	if c.Code != "new-hook" || len(c.EventTypes) != 1 || c.EventTypes[0].EventTypeCode != "orders:sales:order:placed" || deref(c.EventTypes[0].SpecVersion) != "1.0" ||
		deref(c.Queue) != "HIGH_PRIORITY" || deref(c.DispatchPoolID) != "dpl_1" || c.Mode != "BLOCK_ON_ERROR" || *c.MaxAgeSeconds != 600 || c.ClientID != nil || c.ConnectionID != nil {
		t.Errorf("command = %+v", c)
	}
	mustContain(t, body, `"selSub":"sub_9"`, "Subscription created")

	if c.ClientScoped == nil || *c.ClientScoped || c.Sequence != nil {
		t.Errorf("clientScoped/sequence = %v/%v", c.ClientScoped, c.Sequence)
	}
	mustContain(t, dlg, `name="sequence"`, `value="99"`)
	scoped := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: subCreateForm("eventTypes", "evt_c", "clientScoped", "true", "client", "acme", "sequence", "5")})
	if c := h.ops.created[1]; deref(c.ClientID) != "clt_acme" || c.EventTypes[0].EventTypeCode != "orders:sales:order:scoped" ||
		c.ClientScoped == nil || !*c.ClientScoped || c.Sequence == nil || *c.Sequence != 5 {
		t.Errorf("scoped command = %+v %s", c, scoped)
	}
	wrongScope := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: subCreateForm("eventTypes", "evt_c")})
	mustContain(t, wrongScope, "Choose event types from the list.")

	h.ops.err = usecase.Validation("INVALID_ENDPOINT", "endpoint must be http or https")
	bad := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: subCreateForm()})
	mustContain(t, bad, `id="sub-form-endpoint-err"`, "Endpoint must be http or https")
	nonNumber := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: subCreateForm("maxAgeSeconds", "1.5")})
	mustContain(t, nonNumber, `id="sub-form-max-age-seconds-err"`, notWholeNumber)
}

func TestSubscriptionEdit(t *testing.T) {
	h := newSubHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/subscriptions/sub_1/edit", datastar: true})
	mustContain(t, dlg, "Edit Subscription", `value="https://hooks.example.com/orders-hook"`, `name="connectionId"`, `<option value="con_1">Webhooks (hooks)</option>`)
	mustNotContain(t, dlg, `name="code"`, `name="eventTypes"`)
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/subscriptions/sub_1/edit", datastar: true, form: url.Values{
		"name": {"Renamed"}, "description": {""}, "endpoint": {"https://y.example.com"}, "connectionId": {""}, "queue": {""},
		"maxAgeSeconds": {"100"}, "timeoutSeconds": {"30"}, "delaySeconds": {"0"}, "mode": {"IMMEDIATE"},
	}})
	u := h.ops.updated[0]
	if *u.Name != "Renamed" || u.Description != nil || u.ConnectionID != nil || *u.Queue != "" || *u.Mode != "IMMEDIATE" || *u.MaxAgeSeconds != 100 || u.DelaySeconds != nil {
		t.Errorf("update = %+v", u)
	}
}

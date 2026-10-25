package pages

import (
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Undismissed notices (rejected failures) show on the list row and in the
// drawer, with their count and the newest message (C3).
func TestIngestionNoticesShowOnTheListAndTheDrawer(t *testing.T) {
	h := newIngHarness()
	h.store.notices = map[string]ingestion.Notices{
		"ing_1": {Count: 2, LatestMessage: "target pool <orders> suspended", LatestAt: time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)},
	}
	list := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustContain(t, list, `data-testid="ings-notices"`, `2 notices: target pool &lt;orders&gt; suspended`)
	drawer := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1", datastar: true}))
	mustContain(t, drawer, `id="ings-rejected"`, `2 notices: the platform refused this ingestion&#39;s jobs`, `target pool &lt;orders&gt; suspended`)
	other := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_2", datastar: true}))
	mustNotContain(t, other, `id="ings-rejected"`)
}

// Dismissing a failure runs the use case with the update permission, and
// refuses without it; the confirmation is rendered for those who may.
func TestIngestionDismissFailure(t *testing.T) {
	h := newIngHarness()
	form := url.Values{"drawer": {"ing_1"}, "tab": {"failures"}}
	rec := ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_1/failures/ingf_0000000001/dismiss", form)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if !slices.Contains(h.ops.calls, "dismiss:ing_1:ingf_0000000001") {
		t.Fatalf("calls = %v", h.ops.calls)
	}
	mustContain(t, rec.Body.String(), "Failure dismissed")

	h = newIngHarness()
	rec = ingPost(h, t, ingWithout(ingPermUpdate), "/ui/ingestions/ing_1/failures/ingf_0000000001/dismiss", form)
	if len(h.ops.calls) != 0 {
		t.Fatalf("without update the use case ran: %v", h.ops.calls)
	}
	mustContain(t, rec.Body.String(), "platform:ingestion:ingestion:update")
	page := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	mustContain(t, page, `id="ings-dismiss"`)
	page = ingBody(t, h.do(t, ingUser(ingPermView), req{target: "/ui/ingestions"}))
	mustNotContain(t, page, `id="ings-dismiss"`)
}

// The failure rows: a notice is tagged, a dismissed one says by whom, and
// Dismiss is offered only on undismissed ones to those who may.
func TestIngestionFailureRowsOfferDismiss(t *testing.T) {
	i := &ingestion.Ingestion{ID: "ing_1"}
	at := time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC)
	by := "prn_ops"
	fs := []ingestion.Failure{
		{ID: "ingf_1", Kind: "NOT_DISPATCHED", Category: ingestion.FailureRejected, Message: "refused", OccurredAt: at},
		{ID: "ingf_2", Kind: "SOURCE", Category: ingestion.FailurePayload, Message: "timeout", OccurredAt: at, DismissedAt: &at, DismissedBy: &by},
	}
	rows := ingFailureRows(i, fs, true)
	if !rows[0].Notice || rows[0].DismissAction != "/ui/ingestions/ing_1/failures/ingf_1/dismiss" {
		t.Errorf("row 0 = %+v", rows[0])
	}
	if rows[1].Notice || rows[1].DismissAction != "" || rows[1].Dismissed != "prn_ops" {
		t.Errorf("row 1 = %+v", rows[1])
	}
	if ingFailureRows(i, fs, false)[0].DismissAction != "" {
		t.Error("Dismiss offered without the update permission")
	}
}

// An sftp source that checks no host key warns in the drawer: the runtime
// refuses to start it. The form offers the switch that accepts any key.
func TestIngestionSftpHostKeyWarning(t *testing.T) {
	h := newIngHarness()
	drawer := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_2", datastar: true}))
	mustContain(t, drawer, `id="ings-warning"`, `the runtime refuses to start an sftp source`)
	form := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/new", datastar: true}))
	mustContain(t, form, `Accept any host key`)
}

// Without promote the create form offers no live switch and says the
// ingestion is saved as a draft (R3); with it, the switch is offered.
func TestIngestionCreateFormLiveNeedsPromote(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingWithout(ingPermPromote), req{target: "/ui/ingestions/new", datastar: true}))
	mustNotContain(t, body, `Make live when saved`)
	mustContain(t, body, `It is saved as a draft`)
	body = ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/new", datastar: true}))
	mustContain(t, body, `Make live when saved`)
}

// The route permission counts only at anchor tier (R2): a client-tier user
// holding it is not offered camel-route, and is told why it cannot change one.
func TestIngestionRouteKindNeedsAnchorTier(t *testing.T) {
	h := newIngHarness()
	clientTier := &auth.AuthContext{
		PrincipalID: "prn_8", Email: "ct@flowcatalyst.local", Scope: auth.ScopeClient, Clients: []string{"clt_acme"},
		Roles: []string{"r"}, Permissions: ingAllPerms, PrincipalType: auth.PrincipalTypeUser, AllApplications: true,
	}
	body := ingBody(t, h.do(t, clientTier, req{target: "/ui/ingestions/new", datastar: true}))
	mustNotContain(t, body, `<option value="camel-route"`)
	drawer := ingBody(t, h.do(t, clientTier, req{target: "/ui/ingestions/ing_4", datastar: true}))
	mustContain(t, drawer, `needs an anchor-tier account as well as platform:ingestion:ingestion:route`)
}

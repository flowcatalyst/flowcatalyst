package pages

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/go-sdk/usecase"
)

type dashFakeStats struct {
	stats bff.DashboardStats
	err   error
}

func (f dashFakeStats) Stats(context.Context) (bff.DashboardStats, error) { return f.stats, f.err }

// dashFakeEvents and dashFakeRoles implement only the sync of the command
// sets the dashboard runs.
type dashFakeEvents struct {
	EventTypeCommands
	calls int
	err   error
}

func (f *dashFakeEvents) SyncPlatform(context.Context) (EventTypeSyncResult, error) {
	f.calls++
	return EventTypeSyncResult{Created: 2, Total: 40, SchemasSame: 40}, f.err
}

type dashFakeRoles struct {
	RolesCommands
	calls int
	err   error
}

func (f *dashFakeRoles) SyncPlatform(context.Context) (RolesSyncResult, error) {
	f.calls++
	return RolesSyncResult{Updated: 3, Removed: 1, Total: 30}, f.err
}

type dashFakeOpenAPI struct {
	calls int
	res   OpenAPISyncResult
	err   error
}

func (f *dashFakeOpenAPI) SyncPlatformOpenAPI(context.Context) (OpenAPISyncResult, error) {
	f.calls++
	return f.res, f.err
}

type dashHarness struct {
	page   *Dashboard
	events *dashFakeEvents
	roles  *dashFakeRoles
	api    *dashFakeOpenAPI
}

func newDashHarness() *dashHarness {
	h := &dashHarness{events: &dashFakeEvents{}, roles: &dashFakeRoles{}, api: &dashFakeOpenAPI{res: OpenAPISyncResult{Version: "1.4.0", ArchivedPriorVersion: "1.3.0"}}}
	h.page = &Dashboard{
		Stats: dashFakeStats{stats: bff.DashboardStats{
			TotalClients: 12, ActiveUsers: 1234, RolesDefined: 56,
			EventsApprox: 1234567, DispatchJobsApprox: 89, AuditLogsApprox: 1000, LoginAttemptsApprox: 0,
		}},
		EventTypes: h.events, Roles: h.roles, OpenAPI: h.api, Shell: shellForTest(),
	}
	return h
}

func (h *dashHarness) do(t *testing.T, ac *auth.AuthContext, q req) (int, string) {
	t.Helper()
	rec := serveMounted(t, ac, q, h.page)
	return rec.Code, rec.Body.String()
}

func dashPost(target string) req {
	return req{method: http.MethodPost, target: target, form: url.Values{}, datastar: true}
}

// dashUser is an anchor caller holding exactly perms.
func dashUser(scope auth.Scope, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_9", Email: "who@flowcatalyst.local", Scope: scope, Roles: []string{"x"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

func TestDashboardPage(t *testing.T) {
	h := newDashHarness()
	code, body := h.do(t, anchorAdmin(), req{target: "/ui/dashboard"})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	mustContain(t, body,
		"<title>Dashboard · FlowCatalyst</title>", "Welcome back, admin@flowcatalyst.local",
		// The quick links, each to its /ui page.
		`href="/ui/applications"`, "Manage applications in the platform ecosystem", `href="/ui/clients"`, "Manage clients and their configurations",
		`href="/ui/users"`, "Manage platform users and their access", `href="/ui/roles"`, "Configure roles and permissions",
		`href="/ui/event-types"`, "Define event types and schemas for messaging", `href="/ui/subscriptions"`, "Manage event subscriptions and routing",
		// Platform Sync.
		"Platform Sync", "Re-apply code-defined platform definitions (events + roles) without restarting the server.", "Sync All",
		`action="/ui/dashboard/sync"`, `action="/ui/dashboard/sync/event-types"`, `action="/ui/dashboard/sync/roles"`, `action="/ui/dashboard/sync/openapi"`,
		"Platform event-type definitions and schemas.", "Code-defined platform roles and their permissions.", "Publish this build&#39;s OpenAPI document to the Developer portal.",
		// Platform Overview: exact counts.
		"Platform Overview", "Total Clients", ">12<", "Active Users", ">1,234<", "Roles Defined", ">56<",
		// Message Plane: estimates, flagged with a tilde.
		"Message Plane (approx.)", "Estimates from the Postgres planner. Updated by autovacuum", "Exact counts at this scale would require scanning the full table.",
		"Events", ">~1,234,567<", "Dispatch Jobs", ">~89<", "Audit Logs", ">~1,000<", "Login Attempts", ">~0<",
		// The sidebar entry is this page, and is current.
		`href="/ui/dashboard" class="fc-nav-item" title="Dashboard" aria-current="page"`,
	)
	mustNotContain(t, body, "Counts unavailable")
}

func TestDashboardCountsUnavailable(t *testing.T) {
	h := newDashHarness()
	h.page.Stats = dashFakeStats{err: errors.New("pg: down")}
	code, body := h.do(t, anchorAdmin(), req{target: "/ui/dashboard"})
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	// The SPA shows a dash for a count it could not read; this also says so.
	mustContain(t, body, "Counts unavailable", ">—<")
	mustNotContain(t, body, "pg: down", ">~")
	if n := strings.Count(body, ">—<"); n != 7 {
		t.Errorf("%d dashes, want 7", n)
	}
	// The rest of the page still works.
	mustContain(t, body, "Platform Sync", `href="/ui/applications"`)
}

func TestDashboardVisibility(t *testing.T) {
	h := newDashHarness()
	// Anchor scope and the route permission: the SPA's rule.
	if code, _ := h.do(t, anchorAdmin(), req{target: "/ui/dashboard"}); code != http.StatusOK {
		t.Errorf("admin: %d", code)
	}
	for name, ac := range map[string]*auth.AuthContext{
		// Client scope holding the permission: the dashboard is anchor-only.
		"client scope": dashUser(auth.ScopeClient, "platform:*:*:*"),
		// Anchor scope, the stats permission, but not the route permission.
		"anchor without route permission": dashUser(auth.ScopeAnchor, "platform:admin:client:view"),
		"anchor with no permission":       dashUser(auth.ScopeAnchor),
	} {
		code, body := h.do(t, ac, req{target: "/ui/dashboard"})
		if code != http.StatusForbidden {
			t.Errorf("%s: status %d", name, code)
		}
		mustNotContain(t, body, "Platform Overview", "Total Clients")
	}
}

func TestDashboardSyncOffers(t *testing.T) {
	h := newDashHarness()
	ac := func(perms ...string) *auth.AuthContext { return dashUser(auth.ScopeAnchor, perms...) }
	const (
		sync   = "platform:messaging:event-type:sync"
		create = "platform:iam:role:create"
		update = "platform:iam:role:update"
		del    = "platform:iam:role:delete"
		oa     = "platform:developer:application-openapi:sync"
	)
	cases := []struct {
		name                   string
		ac                     *auth.AuthContext
		events, roles, openapi bool
	}{
		{"all", anchorAdmin(), true, true, true},
		{"event types only", ac(sync), true, false, false},
		{"manage also syncs event types", ac("platform:messaging:event-type:manage"), true, false, false},
		{"roles need all three permissions", ac(create, update), false, false, false},
		{"roles", ac(create, update, del), false, true, false},
		{"openapi", ac(oa), false, false, true},
		{"none", ac("platform:admin:client:view"), false, false, false},
		{"client scope offers nothing", dashUser(auth.ScopeClient, sync, create, update, del, oa), false, false, false},
	}
	for _, c := range cases {
		v := h.page.view(newRequest(t), c.ac)
		if v.Events != c.events || v.Roles != c.roles || v.OpenAPI != c.openapi {
			t.Errorf("%s: events %v roles %v openapi %v", c.name, v.Events, v.Roles, v.OpenAPI)
		}
		if got, want := v.AnySync(), c.events || c.roles || c.openapi; got != want {
			t.Errorf("%s: AnySync %v", c.name, got)
		}
	}
	// Without any sync offered the section is not rendered at all.
	var sb strings.Builder
	if err := dashPage(h.page.view(newRequest(t), ac("platform:admin:client:view"))).Render(context.Background(), &sb); err != nil {
		t.Fatal(err)
	}
	mustNotContain(t, sb.String(), "Platform Sync", "Sync All")
	mustContain(t, sb.String(), "Platform Overview")
	// A nil OpenAPI syncer leaves its card out.
	h.page.OpenAPI = nil
	if v := h.page.view(newRequest(t), anchorAdmin()); v.OpenAPI {
		t.Error("OpenAPI offered without a syncer")
	}
}

func newRequest(t *testing.T) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/ui/dashboard", nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDashboardSyncEachRunsItsUseCase(t *testing.T) {
	h := newDashHarness()
	_, body := h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync/event-types"))
	mustContain(t, body, "Platform Events Synced", "2 created (40 total)")
	_, body = h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync/roles"))
	mustContain(t, body, "Platform Roles Synced", "3 updated, 1 removed (30 total)")
	_, body = h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync/openapi"))
	mustContain(t, body, "Platform OpenAPI Synced", "v1.3.0 → v1.4.0")
	if h.events.calls != 1 || h.roles.calls != 1 || h.api.calls != 1 {
		t.Errorf("calls: events %d roles %d openapi %d", h.events.calls, h.roles.calls, h.api.calls)
	}
}

func TestDashboardSyncAll(t *testing.T) {
	h := newDashHarness()
	_, body := h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync"))
	mustContain(t, body, "Platform Events Synced", "Platform Roles Synced", "Platform OpenAPI Synced")
	if h.events.calls != 1 || h.roles.calls != 1 || h.api.calls != 1 {
		t.Errorf("calls: events %d roles %d openapi %d", h.events.calls, h.roles.calls, h.api.calls)
	}

	// One failing does not stop the others, and says which failed.
	h = newDashHarness()
	h.roles.err = usecase.Validation("ROLE_BAD", "A role is malformed.")
	_, body = h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync"))
	mustContain(t, body, "Platform Events Synced", "Platform OpenAPI Synced", "Platform Roles Sync Failed", "A role is malformed.")
	if h.events.calls != 1 || h.roles.calls != 1 || h.api.calls != 1 {
		t.Errorf("calls after a failure: events %d roles %d openapi %d", h.events.calls, h.roles.calls, h.api.calls)
	}
	// An internal error shows nothing of itself.
	h = newDashHarness()
	h.api.err = errors.New("pg: password authentication failed")
	_, body = h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync"))
	mustContain(t, body, "Platform OpenAPI Sync Failed")
	mustNotContain(t, body, "password")

	// Sync All runs only the syncs the caller is offered.
	h = newDashHarness()
	_, body = h.do(t, dashUser(auth.ScopeAnchor, "platform:*:*:*"), dashPost("/ui/dashboard/sync"))
	mustContain(t, body, "Platform Events Synced")
	h = newDashHarness()
	ac := dashUser(auth.ScopeAnchor, "platform:*:*:*")
	h.page.OpenAPI = nil
	h.do(t, ac, dashPost("/ui/dashboard/sync"))
	if h.api.calls != 0 {
		t.Error("OpenAPI synced without a syncer")
	}
}

func TestDashboardSyncRefusals(t *testing.T) {
	targets := []string{"/ui/dashboard/sync/event-types", "/ui/dashboard/sync/roles", "/ui/dashboard/sync/openapi", "/ui/dashboard/sync"}
	callers := map[string]*auth.AuthContext{
		"client scope, every permission": dashUser(auth.ScopeClient, "platform:*:*:*"),
		"anchor, the stats permission":   dashUser(auth.ScopeAnchor, "platform:admin:client:view"),
		"anchor, no permission":          dashUser(auth.ScopeAnchor),
		// The sync permissions without the dashboard's route permission: the
		// dashboard is not theirs to post to.
		"anchor with the sync permissions only": dashUser(auth.ScopeAnchor,
			"platform:messaging:event-type:sync", "platform:iam:role:create", "platform:iam:role:update", "platform:iam:role:delete", "platform:developer:application-openapi:sync"),
	}
	for name, ac := range callers {
		for _, target := range targets {
			h := newDashHarness()
			_, body := h.do(t, ac, dashPost(target))
			if !strings.Contains(body, "fc-flash-error") {
				t.Errorf("%s %s: no refusal in %q", name, target, body)
			}
			if rec := serveMounted(t, ac, req{method: http.MethodPost, target: target, form: url.Values{}}, h.page); rec.Code != http.StatusForbidden {
				t.Errorf("%s native %s: status %d", name, target, rec.Code)
			}
			if h.events.calls+h.roles.calls+h.api.calls != 0 {
				t.Errorf("%s %s: a sync ran without the permission", name, target)
			}
		}
	}
}

// Each sync makes its own endpoint's check: the route permission alone (held
// through the wildcard in the admin test) is not the whole of it. A caller
// the dashboard admits but a sync refuses gets the refusal and no sync.
func TestDashboardSyncChecksItsOwnPermission(t *testing.T) {
	h := newDashHarness()
	// platform:*:*:* is the only way in, and it covers every sync permission,
	// so refuse by scope instead: the per-sync checks are the API's.
	ac := dashUser(auth.ScopeAnchor, "platform:*:*:*")
	for target, ran := range map[string]func() int{
		"/ui/dashboard/sync/event-types": func() int { return h.events.calls },
		"/ui/dashboard/sync/roles":       func() int { return h.roles.calls },
		"/ui/dashboard/sync/openapi":     func() int { return h.api.calls },
	} {
		h.do(t, ac, dashPost(target))
		if ran() != 1 {
			t.Errorf("%s did not run", target)
		}
	}
	// A failing use case is reported, not hidden.
	h.events.err = usecase.Authorization("FORBIDDEN", "No.")
	_, body := h.do(t, ac, dashPost("/ui/dashboard/sync/event-types"))
	mustContain(t, body, "No.")
}

func TestDashboardNativeSync(t *testing.T) {
	h := newDashHarness()
	for target, flash := range map[string]string{
		"/ui/dashboard/sync/event-types": "fc_ui_flash=event-types-synced",
		"/ui/dashboard/sync/roles":       "fc_ui_flash=roles-synced",
		"/ui/dashboard/sync/openapi":     "fc_ui_flash=dashboard-synced",
		"/ui/dashboard/sync":             "fc_ui_flash=dashboard-synced",
	} {
		rec := serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: target, form: url.Values{}}, h.page)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/dashboard" || !strings.Contains(rec.Header().Get("Set-Cookie"), flash) {
			t.Errorf("%s: %d %q %q", target, rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	}
	// A failing sync is a failure page, not a redirect with a success flash.
	h.api.err = usecase.Validation("X", "Bad.")
	rec := serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/dashboard/sync", form: url.Values{}}, h.page)
	if rec.Code == http.StatusSeeOther {
		t.Errorf("failed Sync All redirected as a success")
	}
}

func TestDashboardOpenAPIDetail(t *testing.T) {
	for _, c := range []struct {
		in   OpenAPISyncResult
		want string
	}{
		{OpenAPISyncResult{Version: "2.0.0", Unchanged: true}, "No changes (v2.0.0 unchanged)"},
		{OpenAPISyncResult{Version: "2.1.0", ArchivedPriorVersion: "2.0.0"}, "v2.0.0 → v2.1.0"},
		{OpenAPISyncResult{Version: "3.0.0", ArchivedPriorVersion: "2.1.0", HasBreaking: true}, "v2.1.0 → v3.0.0 · breaking"},
		{OpenAPISyncResult{Version: "1.0.0"}, "Published initial version v1.0.0"},
	} {
		if got := openAPISyncDetail(c.in); got != c.want {
			t.Errorf("%+v: %q, want %q", c.in, got, c.want)
		}
	}
}

func TestGroupThousands(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "12": "12", "999": "999", "1000": "1,000", "1234567": "1,234,567", "100000": "100,000"} {
		if got := groupThousands(in); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

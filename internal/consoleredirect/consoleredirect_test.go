package consoleredirect

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The SPA's own static check, stubbed.
func isStatic(p string) bool {
	return strings.HasPrefix(p, "/assets/") || p == "/favicon.svg" || strings.HasPrefix(p, "/api/")
}

var spaHit bool

var spa = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	spaHit = true
	w.WriteHeader(http.StatusTeapot)
})

func get(t *testing.T, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	spaHit = false
	r := httptest.NewRequest(method, target, nil)
	rec := httptest.NewRecorder()
	New(spa, isStatic).ServeHTTP(rec, r)
	return rec
}

func TestMapping(t *testing.T) {
	cases := []struct{ from, to string }{
		{"/", "/ui/"},
		{"/dashboard", "/ui/"},
		{"/nonsense/page", "/ui/"},
		{"/auth", "/ui/auth/login"},
		{"/auth/login", "/ui/auth/login"},
		{"/auth/forgot-password", "/ui/auth/forgot-password"},
		{"/auth/reset-password", "/ui/auth/reset-password"},
		{"/auth/set-password", "/ui/auth/set-password"},
		{"/logout", "/ui/auth/logout"},
		{"/portal/login", "/ui/auth/portal-login"},
		{"/applications", "/ui/applications"},
		{"/applications/new", "/ui/applications/new"},
		{"/applications/app_1", "/ui/applications/app_1"},
		{"/clients", "/ui/clients"},
		{"/clients/new", "/ui/clients/new"},
		{"/clients/clt_1", "/ui/clients/clt_1"},
		{"/clients/clt_1/theme", "/ui/clients/clt_1/theme"},
		{"/users", "/ui/users"},
		{"/users/new", "/ui/users/new"},
		{"/users/prn_1", "/ui/users/prn_1"},
		{"/client-administration/users", "/ui/users"},
		{"/client-administration/users/new", "/ui/users/new"},
		{"/client-administration/users/prn_1", "/ui/users/prn_1"},
		{"/identity/service-accounts", "/ui/service-accounts"},
		{"/identity/service-accounts/new", "/ui/service-accounts/new"},
		{"/identity/service-accounts/sa_1", "/ui/service-accounts/sa_1"},
		{"/identity/portal-users", "/ui/portal-users"},
		{"/identity/portal-apps", "/ui/portal-apps"},
		{"/identity/developer-users", "/ui/developer-users"},
		{"/authorization/roles", "/ui/roles"},
		{"/authorization/roles/acme:admin", "/ui/roles/acme:admin"},
		{"/authorization/roles/acme:admin/edit", "/ui/roles/acme:admin/edit"},
		{"/authorization/permissions", "/ui/permissions"},
		{"/roles", "/ui/roles"},
		{"/authentication/identity-providers", "/ui/identity-providers"},
		{"/authentication/identity-providers/new", "/ui/identity-providers/new"},
		{"/authentication/identity-providers/idp_1", "/ui/identity-providers/idp_1"},
		{"/authentication/domain-idps", "/ui/identity-providers"},
		{"/authentication/email-domain-mappings", "/ui/email-domains"},
		{"/authentication/email-domain-mappings/new", "/ui/email-domains/new"},
		{"/authentication/email-domain-mappings/edm_1", "/ui/email-domains/edm_1"},
		{"/authentication/anchor-domains", "/ui/email-domains"},
		{"/authentication/reset-approvals", "/ui/reset-approvals"},
		{"/authentication/reset-approvals/rst_1", "/ui/reset-approvals"},
		{"/authentication/oauth-clients", "/ui/oauth-clients"},
		{"/authentication/oauth-clients/new", "/ui/oauth-clients/new"},
		{"/authentication/oauth-clients/oac_1", "/ui/oauth-clients/oac_1"},
		{"/event-types", "/ui/event-types"},
		{"/event-types/create", "/ui/event-types/new"},
		{"/event-types/evt_1", "/ui/event-types/evt_1"},
		{"/event-types/evt_1/add-schema", "/ui/event-types/evt_1/add-schema"},
		{"/scheduled-jobs", "/ui/scheduled-jobs"},
		{"/scheduled-jobs/create", "/ui/scheduled-jobs/new"},
		{"/scheduled-jobs/sjb_1", "/ui/scheduled-jobs/sjb_1"},
		{"/scheduled-jobs/sjb_1/instances", "/ui/scheduled-jobs/sjb_1/instances"},
		{"/scheduled-jobs/instances/sji_1", "/ui/scheduled-jobs/instances/sji_1"},
		{"/subscriptions", "/ui/subscriptions"},
		{"/subscriptions/new", "/ui/subscriptions/new"},
		{"/subscriptions/sub_1", "/ui/subscriptions/sub_1"},
		{"/connections", "/ui/connections"},
		{"/connections/new", "/ui/connections/new"},
		{"/connections/con_1", "/ui/connections/con_1"},
		{"/dispatch-pools", "/ui/dispatch-pools"},
		{"/dispatch-pools/new", "/ui/dispatch-pools/new"},
		{"/dispatch-pools/dpl_1", "/ui/dispatch-pools/dpl_1"},
		{"/functions", "/ui/functions"},
		{"/functions/new", "/ui/functions/new"},
		{"/functions/fn_1", "/ui/functions/fn_1"},
		{"/functions/fn_1/traces", "/ui/functions/fn_1/traces"},
		{"/dispatch-jobs", "/ui/dispatch-jobs"},
		{"/dispatch-jobs/djb_1", "/ui/dispatch-jobs/djb_1"},
		{"/events", "/ui/events"},
		{"/events/evn_1", "/ui/events/evn_1"},
		{"/processes", "/ui/processes"},
		{"/processes/create", "/ui/processes/new"},
		{"/processes/prc_1", "/ui/processes/prc_1"},
		{"/processes/prc_1/edit", "/ui/processes/prc_1/edit"},
		{"/platform/cors", "/ui/cors-origins"},
		{"/platform/audit-log", "/ui/audit-log"},
		{"/platform/docs", "/ui/platform/docs"},
		{"/platform/docs/app", "/ui/platform/docs"},
		{"/platform/docs/app/getting-started", "/ui/platform/docs/app/getting-started"},
		{"/platform/login-attempts", "/ui/login-attempts"},
		{"/platform/settings/theme", "/ui/platform/settings/theme"},
		{"/platform/settings/names", "/ui/platform/settings/names"},
		{"/platform/settings/config", "/ui/platform/config"},
		{"/platform/debug/events", "/ui/debug/events"},
		{"/platform/debug/dispatch-jobs", "/ui/debug/dispatch-jobs"},
		{"/developer", "/ui/developer"},
		{"/developer/applications/app_1", "/ui/developer/applications/app_1"},
		{"/developer/applications/app_1/versions", "/ui/developer/applications/app_1/versions"},
		{"/profile", "/ui/profile"},
		// A trailing slash is the same page.
		{"/dispatch-jobs/", "/ui/dispatch-jobs"},
	}
	for _, c := range cases {
		for _, m := range []string{http.MethodGet, http.MethodHead} {
			rec := get(t, m, c.from)
			if rec.Code != http.StatusFound || rec.Header().Get("Location") != c.to || rec.Header().Get("Cache-Control") != "no-store" || spaHit {
				t.Errorf("%s %s: %d Location %q cc %q spa=%v, want %s", m, c.from, rec.Code, rec.Header().Get("Location"), rec.Header().Get("Cache-Control"), spaHit, c.to)
			}
		}
	}
	// Every table entry is reachable (not shadowed by an earlier one).
	for _, pr := range Pairs() {
		sample := strings.NewReplacer("{id}", "x_1", "{name}", "x_1", "{instanceId}", "x_1", "{source}", "x_1", "{slug}", "x_1").Replace(pr[0])
		want := strings.NewReplacer("{id}", "x_1", "{name}", "x_1", "{instanceId}", "x_1", "{source}", "x_1", "{slug}", "x_1").Replace(pr[1])
		if pr[0] == "/authentication/reset-approvals/{id}" || pr[0] == "/platform/docs/{source}" {
			want = pr[1] // list targets carry no parameter
		}
		if rec := get(t, http.MethodGet, sample); rec.Header().Get("Location") != want {
			t.Errorf("table entry %s -> %s: %s gave %q", pr[0], pr[1], sample, rec.Header().Get("Location"))
		}
	}
}

func TestQueryAndRedirect(t *testing.T) {
	cases := []struct{ from, to string }{
		// Query kept byte for byte.
		{"/dispatch-jobs?status=ERROR&page=2&q=a%20b", "/ui/dispatch-jobs?status=ERROR&page=2&q=a%20b"},
		// The OAuth login the authorize endpoint sends (authorize.go).
		{
			"/auth/login?oauth=true&response_type=code&client_id=c&redirect_uri=https%3A%2F%2Fapp.example%2Fcb&state=s",
			"/ui/auth/login?oauth=true&response_type=code&client_id=c&redirect_uri=https%3A%2F%2Fapp.example%2Fcb&state=s",
		},
		// Emailed links keep their token / client / email / flow names.
		{"/auth/reset-password?token=abc&client=acme", "/ui/auth/reset-password?token=abc&client=acme"},
		{"/auth/set-password?token=abc", "/ui/auth/set-password?token=abc"},
		{"/auth/forgot-password?email=a%40b.c&client=x&reset=success", "/ui/auth/forgot-password?email=a%40b.c&client=x&reset=success"},
		{"/portal/login?flow=abc123", "/ui/auth/portal-login?flow=abc123"},
		// The SPA return path becomes its /ui page.
		{"/auth/login?redirect=/dispatch-jobs", "/ui/auth/login?redirect=%2Fui%2Fdispatch-jobs"},
		{"/auth/login?redirect=%2Fdispatch-jobs%3Fstatus%3DERROR&client=acme", "/ui/auth/login?redirect=%2Fui%2Fdispatch-jobs%3Fstatus%3DERROR&client=acme"},
		{"/auth/login?oauth=true&redirect=%2Fauthentication%2Foauth-clients%2Foac_1", "/ui/auth/login?oauth=true&redirect=%2Fui%2Foauth-clients%2Foac_1"},
		{"/auth/login?redirect=%2Fui%2Fusers", "/ui/auth/login?redirect=%2Fui%2Fusers"},
		{"/auth/login?redirect=%2Fdashboard", "/ui/auth/login?redirect=%2Fui%2F"},
		// Unmapped, absolute and protocol-relative returns are dropped.
		{"/auth/login?redirect=%2Fnope%2Fnothing", "/ui/auth/login"},
		{"/auth/login?redirect=https%3A%2F%2Fevil.example", "/ui/auth/login"},
		{"/auth/login?redirect=%2F%2Fevil.example", "/ui/auth/login"},
		{"/auth/login?redirect=%2F%5Cevil.example&client=a", "/ui/auth/login?client=a"},
		{"/auth/login?a=1&redirect=&b=2", "/ui/auth/login?a=1&b=2"},
		// Only the auth pages translate "redirect"; elsewhere it is just a query.
		{"/events?redirect=/dispatch-jobs", "/ui/events?redirect=/dispatch-jobs"},
	}
	for _, c := range cases {
		rec := get(t, http.MethodGet, c.from)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != c.to {
			t.Errorf("%s: %d Location %q, want %q", c.from, rec.Code, rec.Header().Get("Location"), c.to)
		}
	}
}

func TestPassThrough(t *testing.T) {
	for _, c := range []struct{ method, target string }{
		{http.MethodPost, "/dispatch-jobs"},
		{http.MethodPut, "/auth/login"},
		{http.MethodDelete, "/clients/x"},
		{http.MethodGet, "/assets/index-abc.js"},
		{http.MethodHead, "/assets/index-abc.css"},
		{http.MethodGet, "/favicon.svg"},
		{http.MethodGet, "/api/nope"},
		{http.MethodGet, "/ui/anything"},
		{http.MethodGet, "/ui"},
	} {
		rec := get(t, c.method, c.target)
		if !spaHit || rec.Code != http.StatusTeapot || rec.Header().Get("Location") != "" {
			t.Errorf("%s %s should pass through: %d spa=%v", c.method, c.target, rec.Code, spaHit)
		}
	}
}

func TestEnv(t *testing.T) {
	for v, want := range map[string]bool{"": true, "ui": true, "UI": true, "spa": false, "SPA": false, "weird": true} {
		t.Setenv(EnvVar, v)
		if got := Enabled(); got != want {
			t.Errorf("%s=%q: Enabled() = %v, want %v", EnvVar, v, got, want)
		}
	}
	t.Setenv(EnvVar, "spa")
	spaHit = false
	rec := httptest.NewRecorder()
	Wrap(spa, isStatic).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dispatch-jobs", nil))
	if !spaHit || rec.Code != http.StatusTeapot {
		t.Errorf("FC_CONSOLE=spa must serve the SPA: %d", rec.Code)
	}
	t.Setenv(EnvVar, "ui")
	spaHit = false
	rec = httptest.NewRecorder()
	Wrap(spa, isStatic).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dispatch-jobs", nil))
	if spaHit || rec.Code != http.StatusFound {
		t.Errorf("FC_CONSOLE=ui must redirect: %d", rec.Code)
	}
}

func TestHostileNeverLeavesUI(t *testing.T) {
	long := strings.Repeat("a", 5000)
	targets := []string{
		"//evil.example", "//evil.example/path", "/%2F%2Fevil.example", "/%2f%2fevil.example",
		"/users/..", "/users/%2e%2e", "/users/%2E%2E/x", "/users/..%2f..%2fetc", "/users/a%2Fb", "/users/a%5Cb",
		"/clients/../../x", "/developer/applications/../versions", "/\\evil.example", "/users/%00",
		"/users/" + long, "/" + long, "/users//x", "/users/a b", "/users/%0d%0aSet-Cookie:x=1",
		"/authorization/roles/%2f", "/platform/docs/a/..", "/auth/login?redirect=//evil.example",
		"/auth/login?redirect=/\\evil.example", "/auth/login?redirect=javascript:alert(1)",
		"/auth/login?redirect=%2Fusers%2F..%2F..%2Fevil", "/auth/login?next=//evil.example",
	}
	for _, p := range targets {
		r, err := http.NewRequest(http.MethodGet, "http://x"+p, nil)
		if err != nil {
			// Not even a valid request target; the server would 400 it.
			continue
		}
		rec := httptest.NewRecorder()
		New(spa, isStatic).ServeHTTP(rec, r)
		if rec.Code == http.StatusTeapot {
			continue // passed to the SPA, not redirected
		}
		loc := rec.Header().Get("Location")
		if rec.Code != http.StatusFound || !strings.HasPrefix(loc, "/ui/") || strings.HasPrefix(loc, "//") || strings.ContainsAny(loc, "\\\r\n ") {
			t.Errorf("%.60q: %d Location %.80q", p, rec.Code, loc)
		}
		pathOnly, _, _ := strings.Cut(loc, "?")
		if strings.Contains(pathOnly, "..") || strings.Contains(strings.ToLower(pathOnly), "%2f") {
			t.Errorf("%.60q: Location path %.80q has dot segments or escaped slash", p, loc)
		}
		if len(loc) > 3000 {
			t.Errorf("%.60q: Location is %d bytes", p, len(loc))
		}
	}
}

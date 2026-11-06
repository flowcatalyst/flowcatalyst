package consoleredirect

import "strings"

// route is one SPA page pattern and its /ui page. Both are written with
// {name} segments; a name in the target is filled from the same name in the
// pattern (the segment copied, still escaped, from the request path).
type route struct {
	spa, ui string
	pat     []string
}

func isParam(s string) bool { return len(s) > 2 && s[0] == '{' && s[len(s)-1] == '}' }

func (rt route) match(segs []string) (map[string]string, bool) {
	if len(rt.pat) != len(segs) {
		return nil, false
	}
	var params map[string]string
	for i, ps := range rt.pat {
		if isParam(ps) {
			if !safeSegment(segs[i]) {
				return nil, false
			}
			if params == nil {
				params = map[string]string{}
			}
			params[ps[1:len(ps)-1]] = segs[i]
			continue
		}
		if ps != segs[i] {
			return nil, false
		}
	}
	return params, true
}

func (rt route) build(params map[string]string) string {
	if rt.ui == root {
		return root
	}
	segs := strings.Split(rt.ui[1:], "/")
	for i, s := range segs {
		if isParam(s) {
			segs[i] = params[s[1:len(s)-1]]
		}
	}
	return "/" + strings.Join(segs, "/")
}

// safeSegment accepts a copied path segment: bounded, no dot segments (raw or
// escaped), no backslash, control character or escaped slash, backslash or NUL.
func safeSegment(s string) bool {
	if s == "" || len(s) > maxSegment {
		return false
	}
	l := strings.ToLower(s)
	switch l {
	case ".", "..", "%2e", "%2e%2e", ".%2e", "%2e.":
		return false
	}
	if strings.Contains(l, "%2f") || strings.Contains(l, "%5c") || strings.Contains(l, "%00") {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] == 0x7f || s[i] == '\\' {
			return false
		}
	}
	return true
}

// pairs is the map, SPA page -> /ui page. Specific routes come before
// {param} ones. Where the SPA page has no /ui twin the target is the list
// page (marked "list").
var pairs = [][2]string{
	{"/", "/ui/"},
	{"/dashboard", "/ui/"},

	{"/auth", "/ui/auth/login"},
	{"/auth/login", "/ui/auth/login"},
	{"/auth/forgot-password", "/ui/auth/forgot-password"},
	{"/auth/reset-password", "/ui/auth/reset-password"},
	{"/auth/set-password", "/ui/auth/set-password"},
	{"/logout", "/ui/auth/logout"},
	{"/portal/login", "/ui/auth/portal-login"},

	{"/applications", "/ui/applications"},
	{"/applications/new", "/ui/applications/new"},
	{"/applications/{id}", "/ui/applications/{id}"},

	{"/clients", "/ui/clients"},
	{"/clients/new", "/ui/clients/new"},
	{"/clients/{id}", "/ui/clients/{id}"},
	{"/clients/{id}/theme", "/ui/clients/{id}/theme"},

	{"/users", "/ui/users"},
	{"/users/new", "/ui/users/new"},
	{"/users/{id}", "/ui/users/{id}"},
	{"/client-administration/users", "/ui/users"},
	{"/client-administration/users/new", "/ui/users/new"},
	{"/client-administration/users/{id}", "/ui/users/{id}"},

	{"/identity/service-accounts", "/ui/service-accounts"},
	{"/identity/service-accounts/new", "/ui/service-accounts/new"},
	{"/identity/service-accounts/{id}", "/ui/service-accounts/{id}"},
	{"/identity/portal-users", "/ui/portal-users"},
	{"/identity/portal-apps", "/ui/portal-apps"},
	{"/identity/developer-users", "/ui/developer-users"},

	{"/authorization/roles", "/ui/roles"},
	{"/authorization/roles/{name}", "/ui/roles/{name}"},
	{"/authorization/roles/{name}/edit", "/ui/roles/{name}/edit"},
	{"/authorization/permissions", "/ui/permissions"},
	{"/roles", "/ui/roles"},

	{"/authentication/identity-providers", "/ui/identity-providers"},
	{"/authentication/identity-providers/new", "/ui/identity-providers/new"},
	{"/authentication/identity-providers/{id}", "/ui/identity-providers/{id}"},
	{"/authentication/domain-idps", "/ui/identity-providers"},
	{"/authentication/email-domain-mappings", "/ui/email-domains"},
	{"/authentication/email-domain-mappings/new", "/ui/email-domains/new"},
	{"/authentication/email-domain-mappings/{id}", "/ui/email-domains/{id}"},
	{"/authentication/anchor-domains", "/ui/email-domains"},
	{"/authentication/reset-approvals", "/ui/reset-approvals"},
	{"/authentication/reset-approvals/{id}", "/ui/reset-approvals"}, // list: no detail page
	{"/authentication/oauth-clients", "/ui/oauth-clients"},
	{"/authentication/oauth-clients/new", "/ui/oauth-clients/new"},
	{"/authentication/oauth-clients/{id}", "/ui/oauth-clients/{id}"},

	{"/event-types", "/ui/event-types"},
	{"/event-types/create", "/ui/event-types/new"},
	{"/event-types/{id}", "/ui/event-types/{id}"},
	{"/event-types/{id}/add-schema", "/ui/event-types/{id}/add-schema"},

	{"/scheduled-jobs", "/ui/scheduled-jobs"},
	{"/scheduled-jobs/create", "/ui/scheduled-jobs/new"},
	{"/scheduled-jobs/instances/{instanceId}", "/ui/scheduled-jobs/instances/{instanceId}"},
	{"/scheduled-jobs/{id}", "/ui/scheduled-jobs/{id}"},
	{"/scheduled-jobs/{id}/instances", "/ui/scheduled-jobs/{id}/instances"},

	{"/subscriptions", "/ui/subscriptions"},
	{"/subscriptions/new", "/ui/subscriptions/new"},
	{"/subscriptions/{id}", "/ui/subscriptions/{id}"},
	{"/connections", "/ui/connections"},
	{"/connections/new", "/ui/connections/new"},
	{"/connections/{id}", "/ui/connections/{id}"},
	{"/dispatch-pools", "/ui/dispatch-pools"},
	{"/dispatch-pools/new", "/ui/dispatch-pools/new"},
	{"/dispatch-pools/{id}", "/ui/dispatch-pools/{id}"},
	{"/functions", "/ui/functions"},
	{"/functions/new", "/ui/functions/new"},
	{"/functions/{id}", "/ui/functions/{id}"},
	{"/functions/{id}/traces", "/ui/functions/{id}/traces"},
	{"/dispatch-jobs", "/ui/dispatch-jobs"},
	{"/dispatch-jobs/{id}", "/ui/dispatch-jobs/{id}"},
	{"/events", "/ui/events"},
	{"/events/{id}", "/ui/events/{id}"},

	{"/processes", "/ui/processes"},
	{"/processes/create", "/ui/processes/new"},
	{"/processes/{id}", "/ui/processes/{id}"},
	{"/processes/{id}/edit", "/ui/processes/{id}/edit"},

	{"/platform/cors", "/ui/cors-origins"},
	{"/platform/audit-log", "/ui/audit-log"},
	{"/platform/docs", "/ui/platform/docs"},
	{"/platform/docs/{source}", "/ui/platform/docs"}, // list: no per-source page
	{"/platform/docs/{source}/{slug}", "/ui/platform/docs/{source}/{slug}"},
	{"/platform/login-attempts", "/ui/login-attempts"},
	{"/platform/settings/theme", "/ui/platform/settings/theme"},
	{"/platform/settings/names", "/ui/platform/settings/names"},
	{"/platform/settings/config", "/ui/platform/config"},
	{"/platform/debug/events", "/ui/debug/events"},
	{"/platform/debug/dispatch-jobs", "/ui/debug/dispatch-jobs"},

	{"/developer", "/ui/developer"},
	{"/developer/applications/{id}", "/ui/developer/applications/{id}"},
	{"/developer/applications/{id}/versions", "/ui/developer/applications/{id}/versions"},
	{"/profile", "/ui/profile"},
}

var routes = func() []route {
	out := make([]route, len(pairs))
	for i, p := range pairs {
		pat := strings.Split(strings.TrimPrefix(p[0], "/"), "/")
		if p[0] == "/" {
			pat = nil
		}
		out[i] = route{spa: p[0], ui: p[1], pat: pat}
	}
	return out
}()

// UITargets lists the /ui route patterns the map can send a browser to (with
// {name} segments), for a test that each exists in the /ui router.
func UITargets() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range pairs {
		if !seen[p[1]] {
			seen[p[1]] = true
			out = append(out, p[1])
		}
	}
	return out
}

// Pairs returns the map (SPA pattern, /ui pattern) for tests.
func Pairs() [][2]string { return append([][2]string(nil), pairs...) }

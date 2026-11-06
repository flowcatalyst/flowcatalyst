package shell

import (
	"slices"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/framework/go/ui/kit"
)

func labels(groups []kit.NavGroup) []string {
	var out []string
	for _, g := range groups {
		for _, it := range g.Items {
			out = append(out, g.Label+"/"+it.Label)
			for _, c := range it.Children {
				out = append(out, g.Label+"/"+it.Label+"/"+c.Label)
			}
		}
	}
	return out
}

func TestNavigationVisibility(t *testing.T) {
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}}
	adminNav := labels(Navigation(admin, "/ui/dispatch-pools"))
	for _, want := range []string{"Overview/Dashboard", "Identity & Access/Roles", "Platform/Settings/Theme", "Messaging/Dispatch Pools"} {
		if !slices.Contains(adminNav, want) {
			t.Errorf("admin nav lacks %s", want)
		}
	}
	if slices.Contains(adminNav, "Client Administration/User Management") {
		t.Error("client-scoped entry shown to an anchor user")
	}

	clientAdmin := &auth.AuthContext{
		Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"platform:client-admin"},
		Permissions: []string{"platform:iam:user:view", "platform:iam:user:update", "platform:messaging:dispatch-pool:view"},
	}
	clientNav := labels(Navigation(clientAdmin, ""))
	want := []string{"Client Administration/User Management", "Client Administration/Reset Approvals", "Messaging/Dispatch Pools"}
	if !slices.Equal(clientNav, want) {
		t.Errorf("client admin nav = %v, want %v", clientNav, want)
	}
}

func TestNavigationLinksAndCurrent(t *testing.T) {
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}, Roles: []string{"r"}}
	var pools, conns, subs kit.NavItem
	for _, g := range Navigation(admin, "/ui/dispatch-pools") {
		for _, it := range g.Items {
			switch it.Label {
			case "Dispatch Pools":
				pools = it
			case "Connections":
				conns = it
			case "Dashboard":
				subs = it
			}
			if it.Current && it.Label != "Dispatch Pools" {
				t.Errorf("%s marked current", it.Label)
			}
		}
	}
	if pools.Href != "/ui/dispatch-pools" || !pools.Current {
		t.Errorf("ported entry: %+v", pools)
	}
	if conns.Href != "/ui/connections" || conns.Current {
		t.Errorf("ported entry, not current: %+v", conns)
	}
	if subs.Href != "/ui/dashboard" || subs.Current {
		t.Errorf("ported entry, not current: %+v", subs)
	}
}

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{"Ada Lovelace": "AL", "admin@flowcatalyst.local": "A", "grace.hopper@example.com": "GH", "": "", "x": "X"} {
		if got := initials(in); got != want {
			t.Errorf("initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSectionKeys(t *testing.T) {
	for in, want := range map[string]string{"Identity & Access": "identity-access", "Client Administration": "client-administration", "Overview": "overview"} {
		if got := sectionKey(in); got != want {
			t.Errorf("sectionKey(%q) = %q, want %q", in, got, want)
		}
	}
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}, Roles: []string{"r"}}
	seen := map[string]bool{}
	for _, g := range Navigation(admin, "") {
		if g.Key == "" || seen[g.Key] {
			t.Errorf("section %q has empty or duplicate key %q", g.Label, g.Key)
		}
		seen[g.Key] = true
	}
}

func TestLanding(t *testing.T) {
	sh := &Shell{}
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}}
	if got := sh.Landing(admin); got != "/ui/dashboard" {
		t.Errorf("admin lands on %s, want /ui/dashboard", got)
	}
	// An anchor caller the dashboard is not offered to (no platform:*:*:*)
	// lands on Dispatch Pools, as before there was a dashboard.
	anchorOps := &auth.AuthContext{
		Scope: auth.ScopeAnchor, Roles: []string{"platform:ops"},
		Permissions: []string{"platform:messaging:dispatch-pool:view", "platform:iam:user:view"},
	}
	if got := sh.Landing(anchorOps); got != "/ui/dispatch-pools" {
		t.Errorf("anchor without the dashboard lands on %s, want /ui/dispatch-pools", got)
	}
	// A client-scoped caller never lands on the dashboard, even holding the
	// permission.
	clientAll := &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"x"}, Permissions: []string{"platform:*:*:*"}}
	if got := sh.Landing(clientAll); got == "/ui/dashboard" {
		t.Errorf("a client-scoped caller lands on the dashboard")
	}
	clientAdmin := &auth.AuthContext{
		Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"platform:client-admin"},
		Permissions: []string{"platform:iam:user:view", "platform:iam:user:update"},
	}
	if got := sh.Landing(clientAdmin); got != "/ui/users" {
		t.Errorf("client admin lands on %s, want /ui/users", got)
	}
	withPools := &auth.AuthContext{
		Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"platform:client-admin"},
		Permissions: []string{"platform:iam:user:view", "platform:messaging:dispatch-pool:view"},
	}
	if got := sh.Landing(withPools); got != "/ui/dispatch-pools" {
		t.Errorf("a caller who may see Dispatch Pools lands on %s", got)
	}
	nobody := &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"app:viewer"}}
	if got := sh.Landing(nobody); got != "/ui/profile" {
		t.Errorf("a caller with no console page lands on %s, want /ui/profile", got)
	}
	// An SPA-linked entry is never a landing.
	if got := landing([]kit.NavGroup{{Items: []kit.NavItem{{Href: "/dashboard"}, {Href: "/ui/roles"}}}}); got != "/ui/roles" {
		t.Errorf("landing = %s", got)
	}
	// The dashboard outranks Dispatch Pools wherever each sits in the sidebar.
	both := []kit.NavGroup{{Items: []kit.NavItem{{Href: "/ui/dispatch-pools"}}}, {Items: []kit.NavItem{{Href: "/ui/dashboard"}}}}
	if got := landing(both); got != "/ui/dashboard" {
		t.Errorf("landing = %s", got)
	}
}

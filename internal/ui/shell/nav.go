package shell

import (
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The sidebar mirrors the SPA's navigation (frontend/src/config/navigation.ts)
// and its visibility rules (frontend/src/stores/permissions.ts:
// ROUTE_PERMISSIONS + canSeeScope). Entries not yet ported link to the SPA
// route; a ported entry links to its /ui page (Ported). Keep this list in
// step with navigation.ts until the SPA is retired.
//
// Icons: the kit sprite has 38 Lucide icons; PrimeIcons the SPA uses that
// have no counterpart are approximated (recorded as a kit gap in
// docs/ui/README.md).

type navEntry struct {
	label string
	icon  string
	// spa is the SPA route; ui the ported /ui route ("" = not ported).
	spa, ui string
	// permission required to see the entry ("" = none), as ROUTE_PERMISSIONS.
	permission string
	// scope restricts the entry to anchor or client users ("" = everyone).
	scope    string
	children []navEntry
}

type navSection struct {
	label   string
	entries []navEntry
}

const (
	scopeAnchor = "anchor"
	scopeClient = "client"
)

var navigation = []navSection{
	{"Overview", []navEntry{
		{label: "Dashboard", icon: "layout-dashboard", spa: "/dashboard", permission: "platform:*:*:*", scope: scopeAnchor},
	}},
	{"Identity & Access", []navEntry{
		{label: "User Management", icon: "users", spa: "/users", permission: "platform:iam:user:view", scope: scopeAnchor},
		{label: "Service Accounts", icon: "terminal", spa: "/identity/service-accounts", permission: "platform:iam:service-account:view"},
		{label: "Developer Users", icon: "terminal", spa: "/identity/developer-users", scope: scopeAnchor},
		{label: "Identity Providers", icon: "user", spa: "/authentication/identity-providers", permission: "platform:iam:identity-provider:view"},
		{label: "Email Domains", icon: "list", spa: "/authentication/email-domain-mappings", permission: "platform:iam:email-domain-mapping:view"},
		{label: "OAuth Clients", icon: "key-round", spa: "/authentication/oauth-clients", permission: "platform:iam:oauth-client:view"},
		{label: "Roles", icon: "shield-check", spa: "/authorization/roles", permission: "platform:iam:role:view", scope: scopeAnchor},
		{label: "Permissions", icon: "key-round", spa: "/authorization/permissions", permission: "platform:iam:permission:view", scope: scopeAnchor},
	}},
	{"Client Administration", []navEntry{
		{label: "User Management", icon: "users", spa: "/client-administration/users", permission: "platform:iam:user:view", scope: scopeClient},
		{label: "Reset Approvals", icon: "shield-check", spa: "/authentication/reset-approvals", permission: "platform:iam:user:update"},
	}},
	{"Portal", []navEntry{
		{label: "Portal Apps", icon: "panel-left", spa: "/identity/portal-apps", permission: "platform:iam:portal-user:view"},
		{label: "Portal Users", icon: "users", spa: "/identity/portal-users", permission: "platform:iam:portal-user:view"},
	}},
	{"Platform", []navEntry{
		{label: "Applications", icon: "layout-dashboard", spa: "/applications", permission: "platform:admin:application:view"},
		{label: "Clients", icon: "list", spa: "/clients", permission: "platform:admin:client:view"},
		{label: "CORS Origins", icon: "workflow", spa: "/platform/cors", permission: "platform:admin:cors:view"},
		{label: "Audit Log", icon: "list", spa: "/platform/audit-log", permission: "platform:admin:audit:view"},
		{label: "Documentation", icon: "list", spa: "/platform/docs", scope: scopeAnchor},
		{label: "Login Attempts", icon: "user", spa: "/platform/login-attempts", permission: "platform:admin:login-attempt:view"},
		{label: "Settings", icon: "settings", children: []navEntry{
			{label: "Theme", icon: "settings", spa: "/platform/settings/theme", permission: "platform:admin:settings:view"},
			{label: "Names", icon: "list", spa: "/platform/settings/names", permission: "platform:admin:settings:view"},
		}},
		{label: "Debug", icon: "terminal", children: []navEntry{
			{label: "Raw Events", icon: "list", spa: "/platform/debug/events", permission: "platform:messaging:event:view-raw"},
			{label: "Raw Dispatch Jobs", icon: "list", spa: "/platform/debug/dispatch-jobs", permission: "platform:messaging:dispatch-job:view"},
		}},
	}},
	{"Messaging", []navEntry{
		{label: "Events", icon: "inbox", spa: "/events", permission: "platform:messaging:event:view"},
		{label: "Event Types", icon: "workflow", spa: "/event-types", permission: "platform:messaging:event-type:view"},
		{label: "Subscriptions", icon: "info", spa: "/subscriptions", permission: "platform:messaging:subscription:view"},
		{label: "Connections", icon: "workflow", spa: "/connections", permission: "platform:messaging:connection:view"},
		{label: "Dispatch Pools", icon: "list", spa: "/dispatch-pools", ui: "/ui/dispatch-pools", permission: "platform:messaging:dispatch-pool:view"},
		{label: "Functions", icon: "terminal", spa: "/functions", permission: "platform:function:function:view"},
		{label: "Dispatch Jobs", icon: "refresh-cw", spa: "/dispatch-jobs", permission: "platform:messaging:dispatch-job:view"},
		{label: "Scheduled Jobs", icon: "refresh-cw", spa: "/scheduled-jobs", permission: "platform:messaging:scheduled-job:view"},
	}},
	{"Developer", []navEntry{
		{label: "Applications", icon: "list", spa: "/developer", permission: "platform:developer:application-openapi:view"},
		{label: "Processes", icon: "workflow", spa: "/processes", permission: "platform:messaging:process:view"},
	}},
}

// visible applies the SPA's rules: a role-less user sees nothing (the guard
// refuses them before this runs); the scope must match (anchor users have no
// home client: the AuthContext's anchor scope); the permission, if any, must
// be held (wildcards as the backend matches them).
func (e navEntry) visible(ac *auth.AuthContext) bool {
	switch e.scope {
	case scopeAnchor:
		if !ac.IsAnchor() {
			return false
		}
	case scopeClient:
		if ac.IsAnchor() {
			return false
		}
	}
	return e.permission == "" || ac.HasPermission(e.permission)
}

func (e navEntry) href() string {
	if e.ui != "" {
		return e.ui
	}
	return e.spa
}

// isCurrent marks the entry for the page at path (the /ui path being rendered).
func (e navEntry) isCurrent(path string) bool {
	return e.ui != "" && (path == e.ui || strings.HasPrefix(path, e.ui+"/"))
}

// Navigation is the sidebar for ac, with the entry for currentPath marked
// current. Empty groups are dropped, as the SPA does.
func Navigation(ac *auth.AuthContext, currentPath string) []kit.NavGroup {
	var out []kit.NavGroup
	for _, sec := range navigation {
		var items []kit.NavItem
		for _, e := range sec.entries {
			if len(e.children) > 0 {
				var kids []kit.NavItem
				open := false
				for _, c := range e.children {
					if !c.visible(ac) {
						continue
					}
					cur := c.isCurrent(currentPath)
					open = open || cur
					kids = append(kids, kit.NavItem{Label: c.label, Href: c.href(), Icon: c.icon, Current: cur})
				}
				if len(kids) > 0 && e.visible(ac) {
					items = append(items, kit.NavItem{Label: e.label, Icon: e.icon, Open: open, Children: kids})
				}
				continue
			}
			if e.visible(ac) {
				items = append(items, kit.NavItem{Label: e.label, Href: e.href(), Icon: e.icon, Current: e.isCurrent(currentPath)})
			}
		}
		if len(items) > 0 {
			out = append(out, kit.NavGroup{Label: sec.label, Items: items})
		}
	}
	return out
}

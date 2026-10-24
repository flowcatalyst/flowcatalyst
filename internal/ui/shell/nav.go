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
// Icons: the Lucide counterpart of each PrimeIcon in navigation.ts (pi-home →
// house, pi-th-large → layout-grid, pi-sitemap → network, pi-bolt → zap, ...).
// Every name must be in the kit sprite (assets_test.go checks every icon
// the UI names).

type navEntry struct {
	label string
	icon  string
	// spa is the SPA route; ui the ported /ui route ("" = not ported).
	spa, ui string
	// permission required to see the entry ("" = none), as ROUTE_PERMISSIONS.
	permission string
	// scope restricts the entry to anchor or client users ("" = everyone).
	scope string
	// feature is the platform feature the entry needs ("" = none).
	feature  string
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

const featureIngestion = "ingestion"

// Features are the platform features that switch navigation entries on (the
// public platform config's features: GET /api/config/platform).
type Features struct {
	// Ingestion shows the ingestion pages (FC_INGESTION_ENABLED).
	Ingestion bool
}

func (f Features) has(feature string) bool {
	switch feature {
	case "":
		return true
	case featureIngestion:
		return f.Ingestion
	}
	return false
}

var navigation = []navSection{
	{"Overview", []navEntry{
		{label: "Dashboard", icon: "house", spa: "/dashboard", permission: "platform:*:*:*", scope: scopeAnchor},
	}},
	{"Identity & Access", []navEntry{
		{label: "User Management", icon: "users", spa: "/users", ui: "/ui/users", permission: "platform:iam:user:view", scope: scopeAnchor},
		{label: "Service Accounts", icon: "server", spa: "/identity/service-accounts", ui: "/ui/service-accounts", permission: "platform:iam:service-account:view"},
		{label: "Developer Users", icon: "code", spa: "/identity/developer-users", ui: "/ui/developer-users", scope: scopeAnchor},
		{label: "Identity Providers", icon: "id-card", spa: "/authentication/identity-providers", permission: "platform:iam:identity-provider:view"},
		{label: "Email Domains", icon: "mail", spa: "/authentication/email-domain-mappings", permission: "platform:iam:email-domain-mapping:view"},
		{label: "OAuth Clients", icon: "key-round", spa: "/authentication/oauth-clients", permission: "platform:iam:oauth-client:view"},
		{label: "Roles", icon: "shield", spa: "/authorization/roles", ui: "/ui/roles", permission: "platform:iam:role:view", scope: scopeAnchor},
		{label: "Permissions", icon: "lock", spa: "/authorization/permissions", ui: "/ui/permissions", permission: "platform:iam:permission:view", scope: scopeAnchor},
	}},
	{"Client Administration", []navEntry{
		{label: "User Management", icon: "users", spa: "/client-administration/users", ui: "/ui/users", permission: "platform:iam:user:view", scope: scopeClient},
		{label: "Reset Approvals", icon: "shield", spa: "/authentication/reset-approvals", permission: "platform:iam:user:update"},
	}},
	{"Portal", []navEntry{
		{label: "Portal Apps", icon: "app-window", spa: "/identity/portal-apps", ui: "/ui/portal-apps", permission: "platform:iam:portal-user:view"},
		{label: "Portal Users", icon: "globe", spa: "/identity/portal-users", ui: "/ui/portal-users", permission: "platform:iam:portal-user:view"},
	}},
	{"Platform", []navEntry{
		{label: "Applications", icon: "layout-grid", spa: "/applications", ui: "/ui/applications", permission: "platform:admin:application:view"},
		{label: "Clients", icon: "building-2", spa: "/clients", ui: "/ui/clients", permission: "platform:admin:client:view"},
		{label: "Ingestions", icon: "download", ui: "/ui/ingestions", permission: "platform:ingestion:ingestion:view", feature: featureIngestion},
		{label: "Ingestion sets", icon: "archive", ui: "/ui/ingestion-sets", permission: "platform:ingestion:ingestion:view", scope: scopeAnchor, feature: featureIngestion},
		{label: "CORS Origins", icon: "link", spa: "/platform/cors", permission: "platform:admin:cors:view"},
		{label: "Audit Log", icon: "history", spa: "/platform/audit-log", permission: "platform:admin:audit:view"},
		{label: "Documentation", icon: "book-open", spa: "/platform/docs", scope: scopeAnchor},
		{label: "Login Attempts", icon: "log-in", spa: "/platform/login-attempts", permission: "platform:admin:login-attempt:view"},
		{label: "Settings", icon: "settings", children: []navEntry{
			{label: "Theme", icon: "palette", spa: "/platform/settings/theme", permission: "platform:admin:settings:view"},
			{label: "Names", icon: "tag", spa: "/platform/settings/names", permission: "platform:admin:settings:view"},
			{label: "Configuration", icon: "list", spa: "/platform/settings/config", ui: "/ui/platform/config", permission: "platform:admin:config:view"},
		}},
		{label: "Debug", icon: "wrench", children: []navEntry{
			{label: "Raw Events", icon: "database", spa: "/platform/debug/events", permission: "platform:messaging:event:view-raw"},
			{label: "Raw Dispatch Jobs", icon: "database", spa: "/platform/debug/dispatch-jobs", permission: "platform:messaging:dispatch-job:view"},
		}},
	}},
	{"Messaging", []navEntry{
		{label: "Events", icon: "inbox", spa: "/events", ui: "/ui/events", permission: "platform:messaging:event:view"},
		{label: "Event Types", icon: "zap", spa: "/event-types", ui: "/ui/event-types", permission: "platform:messaging:event-type:view"},
		{label: "Subscriptions", icon: "bell", spa: "/subscriptions", ui: "/ui/subscriptions", permission: "platform:messaging:subscription:view"},
		{label: "Connections", icon: "link", spa: "/connections", ui: "/ui/connections", permission: "platform:messaging:connection:view"},
		{label: "Dispatch Pools", icon: "database", spa: "/dispatch-pools", ui: "/ui/dispatch-pools", permission: "platform:messaging:dispatch-pool:view"},
		{label: "Functions", icon: "box", spa: "/functions", ui: "/ui/functions", permission: "platform:function:function:view"},
		{label: "Dispatch Jobs", icon: "send", spa: "/dispatch-jobs", ui: "/ui/dispatch-jobs", permission: "platform:messaging:dispatch-job:view"},
		{label: "Scheduled Jobs", icon: "clock", spa: "/scheduled-jobs", ui: "/ui/scheduled-jobs", permission: "platform:messaging:scheduled-job:view"},
	}},
	{"Developer", []navEntry{
		{label: "Applications", icon: "book-open", spa: "/developer", ui: "/ui/developer", permission: "platform:developer:application-openapi:view"},
		{label: "Processes", icon: "network", spa: "/processes", ui: "/ui/processes", permission: "platform:messaging:process:view"},
	}},
}

// visible applies the SPA's rules: a role-less user sees nothing (the guard
// refuses them before this runs); the scope must match (anchor users have no
// home client: the AuthContext's anchor scope); the permission, if any, must
// be held (wildcards as the backend matches them).
func (e navEntry) visible(ac *auth.AuthContext, features Features) bool {
	if !features.has(e.feature) {
		return false
	}
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
	return NavigationWith(ac, currentPath, Features{})
}

// NavigationWith is Navigation with the platform features that are on: an
// entry that needs a feature is absent without it.
func NavigationWith(ac *auth.AuthContext, currentPath string, features Features) []kit.NavGroup {
	var out []kit.NavGroup
	for _, sec := range navigation {
		var items []kit.NavItem
		for _, e := range sec.entries {
			if len(e.children) > 0 {
				var kids []kit.NavItem
				open := false
				for _, c := range e.children {
					if !c.visible(ac, features) {
						continue
					}
					cur := c.isCurrent(currentPath)
					open = open || cur
					kids = append(kids, kit.NavItem{Label: c.label, Href: c.href(), Icon: c.icon, Current: cur})
				}
				if len(kids) > 0 && e.visible(ac, features) {
					items = append(items, kit.NavItem{Label: e.label, Icon: e.icon, Open: open, Children: kids})
				}
				continue
			}
			if e.visible(ac, features) {
				items = append(items, kit.NavItem{Label: e.label, Href: e.href(), Icon: e.icon, Current: e.isCurrent(currentPath)})
			}
		}
		if len(items) > 0 {
			out = append(out, kit.NavGroup{Label: sec.label, Key: sectionKey(sec.label), Items: items})
		}
	}
	return out
}

// sectionKey is the stable slug that identifies a section in the viewer's
// stored collapsed-section list ("Identity & Access" -> "identity-access").
func sectionKey(label string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(label) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	return b.String()
}

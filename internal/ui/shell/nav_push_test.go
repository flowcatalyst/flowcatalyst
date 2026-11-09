package shell

import (
	"slices"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The Push pages are anchor-only and need the view permission (the manage permission implies it);
// the test send needs manage. They show whether or not the gateway runs in this process: the
// pages read the database.
func TestPushNavigationNeedsAnchorAndThePermissions(t *testing.T) {
	anchor := func(perms ...string) *auth.AuthContext {
		return &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: perms}
	}
	pushLabels := func(ac *auth.AuthContext) []string {
		var out []string
		for _, l := range labels(Navigation(ac, "")) {
			if len(l) > 0 && slices.Contains([]string{"Platform/Push/Applications", "Platform/Push/Live", "Platform/Push/Recent pushes", "Platform/Push/Test send"}, l) {
				out = append(out, l)
			}
		}
		return out
	}
	all := []string{"Platform/Push/Applications", "Platform/Push/Live", "Platform/Push/Recent pushes", "Platform/Push/Test send"}
	read := all[:3]

	if got := pushLabels(anchor("platform:*:*:*")); !slices.Equal(got, all) {
		t.Errorf("admin: %v", got)
	}
	if got := pushLabels(anchor("platform:admin:push:view")); !slices.Equal(got, read) {
		t.Errorf("viewer: %v", got)
	}
	if got := pushLabels(anchor("platform:admin:push:manage")); !slices.Equal(got, all) {
		t.Errorf("manager (manage implies view): %v", got)
	}
	if got := pushLabels(anchor("platform:messaging:dispatch-pool:view")); len(got) != 0 {
		t.Errorf("unrelated permission: %v", got)
	}
	client := &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"c"}, Roles: []string{"r"}, Permissions: []string{"platform:admin:push:manage"}}
	if got := pushLabels(client); len(got) != 0 {
		t.Errorf("client scope: %v", got)
	}
}

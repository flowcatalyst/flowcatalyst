package shell

import (
	"slices"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The ingestion entries need the feature flag and the view permission.
func TestIngestionNavigationNeedsTheFlagAndThePermission(t *testing.T) {
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}}
	viewer := &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{"platform:ingestion:ingestion:view"}}
	other := &auth.AuthContext{Scope: auth.ScopeAnchor, Roles: []string{"r"}, Permissions: []string{"platform:messaging:dispatch-pool:view"}}
	const a, b = "Platform/Ingestions", "Platform/Ingestion sets"

	if got := labels(Navigation(admin, "")); slices.Contains(got, a) || slices.Contains(got, b) {
		t.Errorf("the entries show with the flag off: %v", got)
	}
	if got := labels(NavigationWith(admin, "", Features{})); slices.Contains(got, a) {
		t.Errorf("zero features show the entry: %v", got)
	}
	on := Features{Ingestion: true}
	for name, ac := range map[string]*auth.AuthContext{"admin": admin, "viewer": viewer} {
		if got := labels(NavigationWith(ac, "", on)); !slices.Contains(got, a) || !slices.Contains(got, b) {
			t.Errorf("%s: entries missing with the flag on: %v", name, got)
		}
	}
	if got := labels(NavigationWith(other, "", on)); slices.Contains(got, a) || slices.Contains(got, b) {
		t.Errorf("entries show without the view permission: %v", got)
	}
}

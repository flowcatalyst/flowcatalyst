package auth

import "testing"

func TestRequireConfigGate(t *testing.T) {
	cases := []struct {
		name         string
		ac           *AuthContext
		view, update bool
	}{
		{"nil", nil, false, false},
		{
			"anchor with no permission is refused (scope is reach, not authority)",
			&AuthContext{Scope: ScopeAnchor, AllApplications: true}, false, false,
		},
		{
			"anchor with config view only",
			&AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view"}}, true, false,
		},
		{
			"anchor with config view and update",
			&AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view", "platform:admin:config:update"}}, true, true,
		},
		{
			"super-admin wildcard",
			&AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}, true, true,
		},
		{
			"application service account with the application-service permissions",
			&AuthContext{Scope: ScopeAnchor, Applications: []string{"app_x"}, Permissions: []string{"platform:application-service:config:view", "platform:application-service:config:update"}}, true, true,
		},
		{
			"application-service permission needs an application-bound account",
			&AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{"platform:application-service:config:view", "platform:application-service:config:update"}}, false, false,
		},
		{
			"application service account with an unrelated role",
			&AuthContext{Scope: ScopeAnchor, Applications: []string{"app_x"}, Permissions: []string{"platform:application-service:event-type:view"}}, false, false,
		},
		{
			"below anchor tier is left to the access grants",
			&AuthContext{Scope: ScopeClient, Clients: []string{"clt_a"}}, true, true,
		},
	}
	for _, c := range cases {
		if got := RequireConfigView(c.ac) == nil; got != c.view {
			t.Errorf("%s: view allowed=%v, want %v", c.name, got, c.view)
		}
		if got := RequireConfigUpdate(c.ac) == nil; got != c.update {
			t.Errorf("%s: update allowed=%v, want %v", c.name, got, c.update)
		}
	}
}

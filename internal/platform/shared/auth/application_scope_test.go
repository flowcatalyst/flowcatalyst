package auth

import "testing"

func TestCanAccessApplication(t *testing.T) {
	cases := []struct {
		name string
		ac   *AuthContext
		app  string
		want bool
	}{
		{"nil context", nil, "app_1", false},
		{"all-applications grants any", &AuthContext{AllApplications: true}, "app_1", true},
		{"all-applications grants even with empty list", &AuthContext{AllApplications: true, Applications: nil}, "app_x", true},
		{"scoped: in list", &AuthContext{Applications: []string{"app_1", "app_2"}}, "app_2", true},
		{"scoped: not in list", &AuthContext{Applications: []string{"app_1"}}, "app_2", false},
		{"scoped: empty list denies", &AuthContext{Applications: nil}, "app_1", false},
		// An anchor-tier service account pinned to one app must NOT reach others.
		{"anchor but app-scoped is confined", &AuthContext{Scope: ScopeAnchor, Applications: []string{"app_own"}}, "app_other", false},
		{"anchor but app-scoped reaches own", &AuthContext{Scope: ScopeAnchor, Applications: []string{"app_own"}}, "app_own", true},
	}
	for _, c := range cases {
		if got := c.ac.CanAccessApplication(c.app); got != c.want {
			t.Errorf("%s: CanAccessApplication(%q) = %v, want %v", c.name, c.app, got, c.want)
		}
	}
}

func TestIsApplicationScoped(t *testing.T) {
	if (&AuthContext{AllApplications: true}).IsApplicationScoped() {
		t.Error("all-applications principal should not be application-scoped")
	}
	if !(&AuthContext{Applications: []string{"app_1"}}).IsApplicationScoped() {
		t.Error("a principal without all-applications is application-scoped")
	}
	if (*AuthContext)(nil).IsApplicationScoped() {
		t.Error("nil context should not report application-scoped")
	}
}

func TestParseApplicationsClaim(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
		wantIDs []string
		wantAll bool
	}{
		{"empty", nil, nil, false},
		{"wildcard means all", []string{"*"}, []string{}, true},
		{"pairs split to ids", []string{"app_1:alpha", "app_2:beta"}, []string{"app_1", "app_2"}, false},
		// Tokens minted before the pair shape existed must keep working for
		// their remaining TTL.
		{"legacy bare ids", []string{"app_1", "app_2"}, []string{"app_1", "app_2"}, false},
		{"mixed forms", []string{"app_1:alpha", "app_2"}, []string{"app_1", "app_2"}, false},
		// Only the FIRST colon delimits, so a code containing one is discarded
		// whole rather than corrupting the id.
		{"code with a colon", []string{"app_1:a:b"}, []string{"app_1"}, false},
		{"wildcard alongside ids", []string{"*", "app_1:alpha"}, []string{"app_1"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ids, all := ParseApplicationsClaim(c.entries)
			if all != c.wantAll {
				t.Errorf("allApplications = %v, want %v", all, c.wantAll)
			}
			if len(ids) != len(c.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, c.wantIDs)
			}
			for i := range c.wantIDs {
				if ids[i] != c.wantIDs[i] {
					t.Errorf("ids[%d] = %q, want %q", i, ids[i], c.wantIDs[i])
				}
			}
		})
	}
}

// The application read rule: the admin view permission reads any application;
// the application-service view reads only the applications the principal is
// bound to.
func TestCanReadApplication(t *testing.T) {
	const adminView = "platform:admin:application:view"
	const appSvcView = "platform:application-service:application:view"
	cases := []struct {
		name string
		ac   *AuthContext
		app  string
		ok   bool
	}{
		{"nil context", nil, "app_1", false},
		{"no permission", &AuthContext{AllApplications: true}, "app_1", false},
		{"admin view reads any, even when application-scoped",
			&AuthContext{Permissions: []string{adminView}, Applications: []string{"app_1"}}, "app_2", true},
		{"super-admin wildcard reads any",
			&AuthContext{Permissions: []string{"platform:*:*:*"}}, "app_2", true},
		{"app-service view reads its own application",
			&AuthContext{Permissions: []string{appSvcView}, Applications: []string{"app_1"}}, "app_1", true},
		{"app-service view is refused another application",
			&AuthContext{Permissions: []string{appSvcView}, Applications: []string{"app_1"}}, "app_2", false},
		{"app-service view with no binding reads nothing",
			&AuthContext{Permissions: []string{appSvcView}}, "app_1", false},
		{"app-service view at anchor tier is still confined",
			&AuthContext{Scope: ScopeAnchor, Permissions: []string{appSvcView}, Applications: []string{"app_1"}}, "app_2", false},
		{"app-service view with all-applications reads any",
			&AuthContext{Permissions: []string{appSvcView}, AllApplications: true}, "app_9", true},
	}
	for _, c := range cases {
		err := CanReadApplication(c.ac, c.app)
		if (err == nil) != c.ok {
			t.Errorf("%s: CanReadApplication(%q) error = %v, want ok=%v", c.name, c.app, err, c.ok)
		}
	}
}

func TestCanReadApplicationsCoarseGuard(t *testing.T) {
	if err := CanReadApplications(&AuthContext{Permissions: []string{"platform:application-service:application:view"}}); err != nil {
		t.Errorf("application-service view should pass the coarse guard: %v", err)
	}
	if err := CanReadApplications(&AuthContext{Permissions: []string{"platform:application-service:role:view"}}); err == nil {
		t.Error("an unrelated application-service permission must not pass the coarse guard")
	}
	if CanReadAllApplications(&AuthContext{Permissions: []string{"platform:application-service:application:view"}}) {
		t.Error("the application-service view must not count as reading all applications")
	}
}

// The SDK definitions sync publishes an application's OpenAPI document as the
// application's own service account.
func TestCanSyncApplicationOpenAPIAdmitsApplicationService(t *testing.T) {
	if err := CanSyncApplicationOpenAPI(&AuthContext{Permissions: []string{"platform:application-service:application-openapi:sync"}}); err != nil {
		t.Errorf("application-service openapi sync should pass the guard: %v", err)
	}
	if err := CanSyncApplicationOpenAPI(&AuthContext{Permissions: []string{"platform:developer:application-openapi:sync"}}); err != nil {
		t.Errorf("developer openapi sync should still pass the guard: %v", err)
	}
	if err := CanSyncApplicationOpenAPI(&AuthContext{Permissions: []string{"platform:application-service:application:view"}}); err == nil {
		t.Error("the application view permission must not admit an openapi sync")
	}
}

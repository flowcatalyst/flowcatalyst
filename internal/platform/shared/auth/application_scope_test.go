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

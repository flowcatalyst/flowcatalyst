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

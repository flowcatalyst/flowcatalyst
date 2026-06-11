package sdksync

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// TestRequireAppAccess pins the per-app sync confinement: authority is the
// principal's application scope (all-applications, or the explicit binding),
// not the client tier. An anchor-tier service account bound to one application
// may sync that application and is rejected for any other.
func TestRequireAppAccess(t *testing.T) {
	s := &State{}
	app := &application.Application{ID: "app_own", Code: "own"}

	cases := []struct {
		name      string
		ac        *auth.AuthContext
		wantAllow bool
	}{
		{"all-applications admin", &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}, true},
		{"app SA scoped to this app", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_own"}}, true},
		{"app SA scoped to another app", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_other"}}, false},
		{"no application access", &auth.AuthContext{Scope: auth.ScopeClient, Applications: nil}, false},
	}
	for _, c := range cases {
		err := s.requireAppAccess(c.ac, app)
		if c.wantAllow && err != nil {
			t.Errorf("%s: want allow, got %v", c.name, err)
		}
		if !c.wantAllow && err == nil {
			t.Errorf("%s: want forbidden, got allow", c.name)
		}
	}
}

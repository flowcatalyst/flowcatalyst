package api

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type fakeApps map[string]*application.Application

func (f fakeApps) FindByCode(_ context.Context, code string) (*application.Application, error) {
	return f[code], nil
}

func TestConfineApp(t *testing.T) {
	s := &State{Apps: fakeApps{
		"orders":  {ID: "app_orders", Code: "orders"},
		"billing": {ID: "app_billing", Code: "billing"},
	}}
	cases := []struct {
		name    string
		ac      *auth.AuthContext
		app     string
		allowed bool
	}{
		{"all-applications anchor reads any", &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}, "billing", true},
		{"scoped anchor reads its own", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "orders", true},
		{"scoped anchor refused another app", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "billing", false},
		{"scoped anchor refused unknown code", &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "nope", false},
		{"scoped anchor with no applications refused", &auth.AuthContext{Scope: auth.ScopeAnchor}, "orders", false},
		{"non-anchor is left to the role grants", &auth.AuthContext{Scope: auth.ScopeClient}, "billing", true},
	}
	for _, c := range cases {
		err := s.confineApp(context.Background(), c.ac, c.app)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err=%v, want allowed=%v", c.name, err, c.allowed)
		}
	}
}

func TestAuthorizeReveal(t *testing.T) {
	s := &State{Apps: fakeApps{
		"orders":  {ID: "app_orders", Code: "orders"},
		"billing": {ID: "app_billing", Code: "billing"},
	}}
	revealer := []string{"platform:admin:config:reveal"}
	cases := []struct {
		name     string
		ac       *auth.AuthContext
		app      string
		clientID string
		allowed  bool
	}{
		{"own application's service account, no permission needed",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "orders", "", true},
		{"service account refused another application",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "billing", "", false},
		{"all-applications anchor without the permission is refused",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}, "orders", "", false},
		{"all-applications anchor with config:view+update only is refused",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view", "platform:admin:config:update"}}, "orders", "", false},
		{"all-applications anchor with the permission",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: revealer}, "orders", "", true},
		{"super-admin wildcard",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}, "orders", "", true},
		{"non-anchor with the permission is still refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_a"}, Permissions: revealer}, "orders", "", false},
		{"client-scoped reveal needs that client (anchor reaches every client)",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: revealer}, "orders", "clt_a", true},
	}
	for _, c := range cases {
		err := s.authorizeReveal(context.Background(), c.ac, c.app, c.clientID)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err=%v, want allowed=%v", c.name, err, c.allowed)
		}
	}
}

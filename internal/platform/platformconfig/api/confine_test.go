package api

import (
	"context"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
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
	s := &State{
		Apps: fakeApps{
			"orders":  {ID: "app_orders", Code: "orders"},
			"billing": {ID: "app_billing", Code: "billing"},
		},
		Clients: newFakeClients(),
	}
	const (
		admin  = "platform:admin:config:reveal"
		appSvc = "platform:application-service:config:reveal"
		acme   = "clt_acme00000000001"
	)
	cases := []struct {
		name      string
		ac        *auth.AuthContext
		app       string
		clientRef string
		allowed   bool
	}{
		// platform administrators
		{"all-applications anchor with the admin permission",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{admin}}, "orders", "", true},
		{"super-admin wildcard",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}, "orders", "acme", true},
		{"admin without the permission is refused, even with config view and update",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view", "platform:admin:config:update"}}, "orders", "", false},
		{"the admin permission needs anchor tier",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, AllApplications: true, Permissions: []string{admin}}, "orders", "acme", false},

		// an application's service account (application-service permission)
		{"own application's service account WITH the permission",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "", true},
		{"own application's service account WITHOUT the permission is refused (no implicit grant)",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "orders", "", false},
		{"service account refused another application",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "billing", "", false},
		{"the application-service permission does not reach every application",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{appSvc}}, "orders", "", false},

		// below anchor tier: bound application AND the clients it may act for
		{"client-tier service account: its client's value in its application",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "acme", true},
		{"client-tier service account: the application-wide value is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "", false},
		{"client-tier service account: another client is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_other0000000001"}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "acme", false},
		{"client-tier service account: another application is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "billing", "acme", false},
		{"client-tier account bound to no application is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Permissions: []string{appSvc}}, "orders", "acme", false},
	}
	for _, c := range cases {
		_, err := s.authorizeReveal(context.Background(), c.ac, c.app, c.clientRef)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err=%v, want allowed=%v", c.name, err, c.allowed)
		}
	}
}

type fakeClients struct {
	byID         map[string]*client.Client
	byIdentifier map[string]*client.Client
}

func (f fakeClients) FindByID(_ context.Context, id string) (*client.Client, error) {
	return f.byID[id], nil
}
func (f fakeClients) FindByIdentifier(_ context.Context, ident string) (*client.Client, error) {
	return f.byIdentifier[ident], nil
}

func newFakeClients() fakeClients {
	acme := &client.Client{ID: "clt_acme00000000001", Identifier: "acme"}
	return fakeClients{
		byID:         map[string]*client.Client{string(acme.ID): acme},
		byIdentifier: map[string]*client.Client{"acme": acme},
	}
}

// A client has one scope however it is named: the id, the identifier, or the
// identifier in another case all resolve to the id; an unknown client is
// refused rather than becoming a row keyed by a string that is no client.
func TestResolveClientID(t *testing.T) {
	s := &State{Clients: newFakeClients()}
	for ref, want := range map[string]string{
		"":                    "",
		"  ":                  "",
		"clt_acme00000000001": "clt_acme00000000001",
		"acme":                "clt_acme00000000001",
		"ACME":                "clt_acme00000000001",
	} {
		got, err := s.resolveClientID(context.Background(), ref)
		if err != nil || got != want {
			t.Errorf("resolve(%q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	if _, err := s.resolveClientID(context.Background(), "nobody"); err == nil {
		t.Error("an unknown client must be refused")
	}
	// No client repository (handler tests): the reference passes through.
	if got, err := (&State{}).resolveClientID(context.Background(), "x"); err != nil || got != "x" {
		t.Errorf("no lookup: got %q, %v", got, err)
	}
}

// Reveal canonicalises the client, and does so only after the caller has
// passed the permission check: an unauthorised caller learns nothing about
// which clients exist (403 for a real and for an unknown client alike).
func TestAuthorizeReveal_ClientResolution(t *testing.T) {
	s := &State{
		Apps:    fakeApps{"orders": {ID: "app_orders", Code: "orders"}},
		Clients: newFakeClients(),
	}
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:reveal"}}
	got, err := s.authorizeReveal(context.Background(), admin, "orders", "acme")
	if err != nil || got != "clt_acme00000000001" {
		t.Fatalf("code resolves to the id: got %q, %v", got, err)
	}
	if _, err := s.authorizeReveal(context.Background(), admin, "orders", "nobody"); err == nil {
		t.Error("an unknown client is refused for an authorised caller")
	}
	nobody := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	for _, ref := range []string{"acme", "nobody"} {
		_, err := s.authorizeReveal(context.Background(), nobody, "orders", ref)
		if err == nil || strings.Contains(err.Error(), "Client") {
			t.Errorf("unauthorised caller, client %q: want a plain 403, got %v", ref, err)
		}
	}
}

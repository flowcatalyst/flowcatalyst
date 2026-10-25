package platformconfig

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
	s := &Service{Apps: fakeApps{
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
		err := s.ConfineApp(context.Background(), c.ac, c.app)
		if (err == nil) != c.allowed {
			t.Errorf("%s: err=%v, want allowed=%v", c.name, err, c.allowed)
		}
	}
}

// fakeLinks: principal "prn_sa_explicit" is a service account linked to
// clt_acme explicitly; "prn_sa_app" (and the unnamed principal of the table
// tests) a service account with no explicit clients, so it reaches the clients
// its application is enabled for; any other principal is not a service
// account.
type fakeLinks struct {
	enabled map[string][]string // application id -> enabled client ids
}

func (fakeLinks) ServiceAccountClients(_ context.Context, principalID string) ([]string, bool, error) {
	switch principalID {
	case "prn_sa_explicit":
		return []string{"clt_acme00000000001"}, true, nil
	case "prn_sa_app", "":
		return nil, true, nil
	}
	return nil, false, nil
}

func (f fakeLinks) ApplicationEnabledClients(_ context.Context, applicationID string) ([]string, error) {
	return f.enabled[applicationID], nil
}

func newConfinedService() *Service {
	return &Service{
		Apps: fakeApps{
			"orders":  {ID: "app_orders", Code: "orders"},
			"billing": {ID: "app_billing", Code: "billing"},
		},
		Clients: newFakeClients(),
		Links:   fakeLinks{enabled: map[string][]string{"app_orders": {"clt_acme00000000001"}, "app_billing": {"clt_other0000000001"}}},
	}
}

func TestAuthorizeReveal(t *testing.T) {
	s := newConfinedService()
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
		{
			"all-applications anchor with the admin permission",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{admin}}, "orders", "", true,
		},
		{
			"super-admin wildcard",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"}}, "orders", "acme", true,
		},
		{
			"admin without the permission is refused, even with config view and update",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:view", "platform:admin:config:update"}}, "orders", "", false,
		},
		{
			"the admin permission needs anchor tier",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, AllApplications: true, Permissions: []string{admin}}, "orders", "acme", false,
		},

		// an application's service account (application-service permission)
		{
			"own application's service account WITH the permission",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "", true,
		},
		{
			"own application's service account WITHOUT the permission is refused (no implicit grant)",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}}, "orders", "", false,
		},
		{
			"service account refused another application",
			&auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "billing", "", false,
		},
		{
			"the application-service permission does not reach every application",
			&auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{appSvc}}, "orders", "", false,
		},

		// below anchor tier: bound application AND the clients it may act for
		{
			"client-tier service account: its client's value in its application",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "acme", true,
		},
		{
			"client-tier service account: the application-wide value is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "", false,
		},
		{
			"client-tier service account: another client is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"clt_other0000000001"}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "orders", "acme", false,
		},
		{
			"client-tier service account: another application is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Applications: []string{"app_orders"}, Permissions: []string{appSvc}}, "billing", "acme", false,
		},
		{
			"client-tier account bound to no application is refused",
			&auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme}, Permissions: []string{appSvc}}, "orders", "acme", false,
		},
	}
	for _, c := range cases {
		_, err := s.AuthorizeReveal(context.Background(), c.ac, c.app, c.clientRef)
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
	s := &Service{Clients: newFakeClients()}
	for ref, want := range map[string]string{
		"":                    "",
		"  ":                  "",
		"clt_acme00000000001": "clt_acme00000000001",
		"acme":                "clt_acme00000000001",
		"ACME":                "clt_acme00000000001",
	} {
		got, err := s.ResolveClientID(context.Background(), ref)
		if err != nil || got != want {
			t.Errorf("resolve(%q) = %q, %v; want %q", ref, got, err, want)
		}
	}
	if _, err := s.ResolveClientID(context.Background(), "nobody"); err == nil {
		t.Error("an unknown client must be refused")
	}
	// No client repository (handler tests): the reference passes through.
	if got, err := (&Service{}).ResolveClientID(context.Background(), "x"); err != nil || got != "x" {
		t.Errorf("no lookup: got %q, %v", got, err)
	}
}

// Reveal canonicalises the client, and does so only after the caller has
// passed the permission check: an unauthorised caller learns nothing about
// which clients exist (403 for a real and for an unknown client alike).
func TestAuthorizeReveal_ClientResolution(t *testing.T) {
	s := &Service{
		Apps:    fakeApps{"orders": {ID: "app_orders", Code: "orders"}},
		Clients: newFakeClients(),
	}
	admin := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:reveal"}}
	got, err := s.AuthorizeReveal(context.Background(), admin, "orders", "acme")
	if err != nil || got != "clt_acme00000000001" {
		t.Fatalf("code resolves to the id: got %q, %v", got, err)
	}
	if _, err := s.AuthorizeReveal(context.Background(), admin, "orders", "nobody"); err == nil {
		t.Error("an unknown client is refused for an authorised caller")
	}
	nobody := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	for _, ref := range []string{"acme", "nobody"} {
		_, err := s.AuthorizeReveal(context.Background(), nobody, "orders", ref)
		if err == nil || strings.Contains(err.Error(), "Client") {
			t.Errorf("unauthorised caller, client %q: want a plain 403, got %v", ref, err)
		}
	}
}

// An application's service account reaches only the clients it is linked to:
// its explicit client ids, else the clients its application is enabled for;
// the application-wide value stays reachable. A person confined to some
// applications is not confined to clients. Below anchor tier the caller's own
// clients, and the global value is readable but not writable.
func TestReachFor(t *testing.T) {
	s := newConfinedService()
	s.Clients = nil
	const acme, other = "clt_acme00000000001", "clt_other0000000001"
	appSA := func(principal string) *auth.AuthContext {
		return &auth.AuthContext{PrincipalID: principal, Scope: auth.ScopeAnchor, Applications: []string{"app_orders", "app_billing"}}
	}
	cases := []struct {
		name                 string
		ac                   *auth.AuthContext
		app                  string
		readAcme, readOther  bool
		writeAcme, writeGlob bool
	}{
		{"all-applications administrator", &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}, "orders", true, true, true, true},
		{"person confined to applications", appSA("prn_person"), "orders", true, true, true, true},
		{"service account with explicit clients", appSA("prn_sa_explicit"), "billing", true, false, true, true},
		{"service account, application enabled for acme", appSA("prn_sa_app"), "orders", true, false, true, true},
		{"service account, application enabled for another client", appSA("prn_sa_app"), "billing", false, true, false, true},
		{"below anchor tier, own client (id:identifier claim)", &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{acme + ":acme"}}, "orders", true, false, true, false},
	}
	for _, c := range cases {
		r, err := s.ReachFor(context.Background(), c.ac, c.app)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if r.CanRead(acme) != c.readAcme || r.CanRead(other) != c.readOther || r.CanWrite(acme) != c.writeAcme || r.CanWrite("") != c.writeGlob || !r.CanRead("") {
			t.Errorf("%s: reach %+v: read acme %v other %v, write acme %v global %v", c.name, r, r.CanRead(acme), r.CanRead(other), r.CanWrite(acme), r.CanWrite(""))
		}
	}
}

// The reveal of an application's service account is confined to its linked
// clients like every other call.
func TestAuthorizeReveal_ServiceAccountLinkedClients(t *testing.T) {
	s := newConfinedService()
	sa := &auth.AuthContext{
		PrincipalID: "prn_sa_app", Scope: auth.ScopeAnchor, Applications: []string{"app_orders", "app_billing"},
		Permissions: []string{"platform:application-service:config:reveal"},
	}
	if _, err := s.AuthorizeReveal(context.Background(), sa, "orders", "acme"); err != nil {
		t.Errorf("a client its application is enabled for: %v", err)
	}
	if _, err := s.AuthorizeReveal(context.Background(), sa, "billing", "acme"); err == nil {
		t.Error("a client its application is not enabled for must be refused")
	}
	if _, err := s.AuthorizeReveal(context.Background(), sa, "billing", ""); err != nil {
		t.Errorf("the application-wide value: %v", err)
	}
}

func TestOnBehalfOf(t *testing.T) {
	sa := &auth.AuthContext{PrincipalType: auth.PrincipalTypeService}
	if got, err := OnBehalfOf(sa, "  alice@example.com "); err != nil || got != "alice@example.com" {
		t.Errorf("service account: %q, %v", got, err)
	}
	if got, err := OnBehalfOf(&auth.AuthContext{PrincipalType: auth.PrincipalTypeUser}, "mallory"); err != nil || got != "" {
		t.Errorf("a person's header is ignored: %q, %v", got, err)
	}
	if _, err := OnBehalfOf(sa, "a\nb"); err == nil {
		t.Error("a control character is refused")
	}
	if _, err := OnBehalfOf(sa, strings.Repeat("x", MaxOnBehalfOf+1)); err == nil {
		t.Error("too long is refused")
	}
}

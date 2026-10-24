package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeEdmStore struct {
	mappings []emaildomainmapping.EmailDomainMapping
}

func (f *fakeEdmStore) FindAll(context.Context) ([]emaildomainmapping.EmailDomainMapping, error) {
	return append([]emaildomainmapping.EmailDomainMapping(nil), f.mappings...), nil
}

func (f *fakeEdmStore) FindByID(_ context.Context, id string) (*emaildomainmapping.EmailDomainMapping, error) {
	for i := range f.mappings {
		if f.mappings[i].ID == id {
			m := f.mappings[i]
			return &m, nil
		}
	}
	return nil, nil
}

type fakeEdmOps struct {
	store   *fakeEdmStore
	created []edmops.CreateCommand
	updated []edmops.UpdateCommand
	moved   []edmops.MoveProviderCommand
	deleted []string
	// usersReset is what a move reports; moveErr and err fail the move or
	// the other calls.
	usersReset int
	moveErr    error
	err        error
}

func (o *fakeEdmOps) Create(_ context.Context, cmd edmops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.mappings = append(o.store.mappings, edmFixture("edm_9", cmd.EmailDomain, cmd.IdentityProviderID, emaildomainmapping.ScopeType(cmd.ScopeType)))
	return "edm_9", nil
}

func (o *fakeEdmOps) Update(_ context.Context, cmd edmops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeEdmOps) Move(_ context.Context, cmd edmops.MoveProviderCommand) (edmops.MoveProviderResult, error) {
	o.moved = append(o.moved, cmd)
	return edmops.MoveProviderResult{MappingID: cmd.ID, UsersReset: o.usersReset}, o.moveErr
}

func (o *fakeEdmOps) Delete(_ context.Context, id string) error {
	o.deleted = append(o.deleted, id)
	return o.err
}

func (o *fakeEdmOps) calls() int {
	return len(o.created) + len(o.updated) + len(o.moved) + len(o.deleted)
}

func edmFixture(id, domain, idp string, scope emaildomainmapping.ScopeType) emaildomainmapping.EmailDomainMapping {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	m := *emaildomainmapping.New(domain, idp, scope)
	m.ID, m.CreatedAt, m.UpdatedAt = id, day, day
	return m
}

type edmHarness struct {
	store *fakeEdmStore
	ops   *fakeEdmOps
	idps  *fakeIdpStore
}

func newEdmHarness() *edmHarness {
	acme := ids.ClientID("clt_acme")
	internal := edmFixture("edm_1", "acme.com", "idp_1", emaildomainmapping.ScopeClient)
	internal.PrimaryClientID = &acme
	internal.Require2FA, internal.Allowed2FAMethods, internal.RememberDeviceEnabled, internal.RememberDeviceDays = true, []string{"TOTP"}, true, 14
	tenant := "tenant-guid-1234"
	sso := edmFixture("edm_2", "globex.com", "idp_2", emaildomainmapping.ScopeAnchor)
	sso.RequiredOIDCTenantID = &tenant
	partner := edmFixture("edm_3", "partner.org", "idp_3", emaildomainmapping.ScopePartner)
	partner.PrimaryClientID = &acme
	orphan := edmFixture("edm_4", "orphan.net", "idp_gone", emaildomainmapping.ScopeAnchor)
	store := &fakeEdmStore{mappings: []emaildomainmapping.EmailDomainMapping{internal, sso, partner, orphan}}
	return &edmHarness{store: store, ops: &fakeEdmOps{store: store}, idps: newIdpHarness().store}
}

func (h *edmHarness) page() *EmailDomains {
	return &EmailDomains{
		Store: h.store, Ops: h.ops, IdPs: h.idps, Clients: someClients(),
		ClientNames: &fakeClientStore{clients: []client.Client{cli("clt_acme", "acme", "Acme <Corp>", client.StatusActive)}},
		Shell:       shellForTest(),
	}
}

func (h *edmHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

func (h *edmHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func (h *edmHarness) post(t *testing.T, ac *auth.AuthContext, target string, f url.Values) string {
	t.Helper()
	return h.do(t, ac, req{method: http.MethodPost, target: target, datastar: true, form: f})
}

const (
	edmPermView   = "platform:iam:email-domain-mapping:view"
	edmPermCreate = "platform:iam:email-domain-mapping:create"
	edmPermUpdate = "platform:iam:email-domain-mapping:update"
	edmPermDelete = "platform:iam:email-domain-mapping:delete"
)

func TestEmailDomainsList(t *testing.T) {
	h := newEdmHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/email-domains"})
	mustContain(t, body, `<title>Email Domains · FlowCatalyst</title>`, "Map email domains to identity providers and define user scope.", "Add Domain Mapping",
		`href="/ui/email-domains/new"`, "acme.com", "Entra &lt;ID&gt;", ">ANCHOR<", ">PARTNER<", ">CLIENT<", "Acme &lt;Corp&gt;", "Unknown",
		"Showing 1 to 4 of 4 email domain mappings", `<dialog id="edm-delete"`, `<aside id="edm-drawer"`, `Filter by provider name`, `<option value="PARTNER">Partner</option>`)
	mustContain(t, body, `href="/ui/email-domains" class="fc-nav-item" title="Email Domains" aria-current="page"`)

	byScope := h.do(t, anchorAdmin(), req{target: "/ui/email-domains?status=ANCHOR", datastar: true})
	mustContain(t, byScope, "globex.com", "orphan.net", "status=ANCHOR")
	mustNotContain(t, byScope, "acme.com", "partner.org")
	byIdp := h.do(t, anchorAdmin(), req{target: "/ui/email-domains?idp=ENTRA", datastar: true})
	mustContain(t, byIdp, "globex.com", "idp=ENTRA")
	mustNotContain(t, byIdp, "partner.org")
	search := h.do(t, anchorAdmin(), req{target: "/ui/email-domains?q=partner", datastar: true})
	mustContain(t, search, "partner.org")
	mustNotContain(t, search, "globex.com")
	bogus := h.do(t, anchorAdmin(), req{target: "/ui/email-domains?status=NOPE"})
	mustContain(t, bogus, "Showing 1 to 4 of 4")
	sorted := h.do(t, anchorAdmin(), req{target: "/ui/email-domains?sort=domain&dir=desc", datastar: true})
	if strings.Index(sorted, "partner.org") > strings.Index(sorted, "acme.com") {
		t.Error("sort=domain&dir=desc: partner.org must come before acme.com")
	}
}

func TestEmailDomainsListWithoutClientNames(t *testing.T) {
	h := newEdmHarness()
	// Naming the clients needs the client list: without it the id is shown.
	body := h.do(t, connUser(edmPermView), req{target: "/ui/email-domains"})
	mustContain(t, body, "clt_acme")
	mustNotContain(t, body, "Acme &lt;Corp&gt;", "Add Domain Mapping", `aria-label="Edit `, `<dialog id="edm-delete"`)
}

func TestEmailDomainsDrawer(t *testing.T) {
	h := newEdmHarness()
	internal := h.do(t, anchorAdmin(), req{target: "/ui/email-domains/edm_1", datastar: true})
	mustContain(t, internal, "acme.com", "Identity Provider: Internal", "Acme &lt;Corp&gt;", "Two-Factor Authentication", ">Required<", "Authenticator app",
		"Allowed (14 days)", `data-action="/ui/email-domains/edm_1/delete"`, `href="/ui/email-domains/edm_1/edit"`)
	mustNotContain(t, internal, "Required OIDC Tenant ID")
	// Two-factor applies to internal-auth domains only: an OIDC-linked mapping
	// shows none, and shows the tenant of a multi-tenant provider.
	sso := h.do(t, anchorAdmin(), req{target: "/ui/email-domains/edm_2", datastar: true})
	mustContain(t, sso, "Required OIDC Tenant ID", "tenant-guid-1234", "Entra &lt;ID&gt;")
	mustNotContain(t, sso, "Two-Factor Authentication", "Primary Client")
	// A partner mapping shows no primary client (the SPA shows it for CLIENT).
	mustNotContain(t, h.do(t, anchorAdmin(), req{target: "/ui/email-domains/edm_3", datastar: true}), "Primary Client")
	// A viewer sees the drawer with no control.
	viewer := h.do(t, connUser(edmPermView), req{target: "/ui/email-domains/edm_1", datastar: true})
	mustNotContain(t, viewer, "/edit", "/delete", "Delete Mapping")
	assert.Equal(t, http.StatusForbidden, h.rec(t, connUser(), req{target: "/ui/email-domains/edm_1"}).Code)
	assert.Equal(t, http.StatusNotFound, h.rec(t, anchorAdmin(), req{target: "/ui/email-domains/edm_404"}).Code)
}

func TestEmailDomainsPermissionMatrix(t *testing.T) {
	forged := []struct {
		name, target string
		form         url.Values
	}{
		{"create", "/ui/email-domains/new", url.Values{"emailDomain": {"x.com"}, "identityProviderId": {"idp_1"}, "scopeType": {"ANCHOR"}}},
		{"edit", "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_3"}, "confirmMove": {"yes"}, "client": {"acme"}}},
		{"delete", "/ui/email-domains/edm_1/delete", url.Values{}},
	}
	callers := []struct {
		name string
		ac   *auth.AuthContext
	}{
		{"no permission", connUser()},
		{"viewer", connUser(edmPermView)},
		{"non-anchor with every permission", &auth.AuthContext{
			PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"},
			Permissions: []string{edmPermView, edmPermCreate, edmPermUpdate, edmPermDelete}, PrincipalType: auth.PrincipalTypeUser,
		}},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			h := newEdmHarness()
			for _, f := range forged {
				h.post(t, c.ac, f.target, f.form)
				if h.ops.calls() != 0 {
					t.Fatalf("%s: a forged post by the %s reached the use case: %+v", f.name, c.name, h.ops)
				}
			}
			for _, target := range []string{"/ui/email-domains/new", "/ui/email-domains/edm_1/edit"} {
				mustNotContain(t, h.do(t, c.ac, req{target: target, datastar: true}), `id="edm-form"`)
			}
			rec := h.rec(t, c.ac, req{method: http.MethodPost, target: "/ui/email-domains/edm_1/delete", form: url.Values{}})
			if rec.Code < 400 || len(h.ops.deleted) != 0 {
				t.Errorf("native forged post: status %d, deleted %v", rec.Code, h.ops.deleted)
			}
		})
	}

	h := newEdmHarness()
	viewer := h.do(t, connUser(edmPermView), req{target: "/ui/email-domains"})
	mustNotContain(t, viewer, "Add Domain Mapping", `aria-label="Edit `, `<dialog id="edm-delete"`, "Delete…")
	updater := h.do(t, connUser(edmPermView, edmPermUpdate), req{target: "/ui/email-domains"})
	mustContain(t, updater, `aria-label="Edit `)
	mustNotContain(t, updater, "Delete…", `<dialog id="edm-delete"`)
	deleter := h.do(t, connUser(edmPermView, edmPermDelete), req{target: "/ui/email-domains"})
	mustContain(t, deleter, "Delete…", `<dialog id="edm-delete"`)
	mustNotContain(t, deleter, `aria-label="Edit `)
	// Editing needs the read as well as the update; create alone opens the form.
	assert.Equal(t, http.StatusForbidden, h.rec(t, connUser(edmPermUpdate), req{target: "/ui/email-domains/edm_1/edit"}).Code)
	mustContain(t, h.do(t, connUser(edmPermCreate), req{target: "/ui/email-domains/new", datastar: true}), `id="edm-form"`)
}

func TestEmailDomainsCreateForm(t *testing.T) {
	h := newEdmHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/email-domains/new", datastar: true})
	mustContain(t, body, `id="edm-form"`, "Create Mapping", `name="emailDomain"`, `name="identityProviderId"`, `name="scopeType"`, `name="client"`, `name="tenant"`,
		`name="require2fa"`, `name="methods"`, "Authenticator app", "Email code", `name="days"`, `action="/ui/email-domains/new"`,
		"Applies to password sign-in for this domain. Passkey sign-in is unaffected; federated (SSO) users are never prompted.",
		"Internal (internal)", "Entra &lt;ID&gt; (entra)", `edmFormScope: &#39;CLIENT&#39;`, `edmFormOidc: [&#34;idp_2&#34;,&#34;idp_3&#34;]`, `edmFormMt: [&#34;idp_2&#34;]`)
}

func TestEmailDomainsCreate(t *testing.T) {
	h := newEdmHarness()
	// An internal-auth domain with a client scope and a two-factor policy.
	body := h.post(t, anchorAdmin(), "/ui/email-domains/new", url.Values{
		"emailDomain": {" NewCo.COM "}, "identityProviderId": {"idp_1"}, "scopeType": {"CLIENT"}, "client": {"acme"},
		"tenant": {"ignored"}, "require2fa": {"true"}, "methods": {"TOTP"}, "remember": {"true"}, "days": {"21"},
	})
	require.Len(t, h.ops.created, 1)
	c := h.ops.created[0]
	assert.Equal(t, "newco.com", c.EmailDomain)
	assert.Equal(t, "idp_1", c.IdentityProviderID)
	assert.Equal(t, "CLIENT", c.ScopeType)
	assert.Equal(t, "clt_acme", deref(c.PrimaryClientID))
	assert.Nil(t, c.RequiredOIDCTenantID, "a provider that is not multi-tenant takes no tenant")
	assert.True(t, c.Require2FA)
	assert.Equal(t, []string{"TOTP"}, c.Allowed2FAMethods)
	assert.True(t, c.RememberDeviceEnabled)
	assert.Equal(t, 21, c.RememberDeviceDays)
	mustContain(t, body, "Email domain mapping created successfully", "selEdm", "newco.com")

	// A multi-tenant OIDC provider takes the tenant and no two-factor policy.
	h = newEdmHarness()
	h.post(t, anchorAdmin(), "/ui/email-domains/new", url.Values{
		"emailDomain": {"sso.example.com"}, "identityProviderId": {"idp_2"}, "scopeType": {"ANCHOR"}, "client": {"acme"},
		"tenant": {" 2e789bd9-a313 "}, "require2fa": {"true"}, "methods": {"TOTP"}, "remember": {"true"},
	})
	require.Len(t, h.ops.created, 1)
	c = h.ops.created[0]
	assert.Equal(t, "2e789bd9-a313", deref(c.RequiredOIDCTenantID))
	assert.Nil(t, c.PrimaryClientID, "an anchor mapping links no client")
	assert.False(t, c.Require2FA, "two-factor does not apply to an OIDC domain")
	assert.Nil(t, c.Allowed2FAMethods)
	assert.False(t, c.RememberDeviceEnabled)
	assert.Zero(t, c.RememberDeviceDays)

	// A partner mapping carries a client too: the platform requires one, which
	// the SPA never sent.
	h = newEdmHarness()
	h.post(t, anchorAdmin(), "/ui/email-domains/new", url.Values{"emailDomain": {"p.example.org"}, "identityProviderId": {"idp_3"}, "scopeType": {"PARTNER"}, "client": {"acme"}})
	require.Len(t, h.ops.created, 1)
	assert.Equal(t, "clt_acme", deref(h.ops.created[0].PrimaryClientID))
}

func TestEmailDomainsCreateRefusals(t *testing.T) {
	base := func() url.Values {
		return url.Values{"emailDomain": {"newco.com"}, "identityProviderId": {"idp_1"}, "scopeType": {"ANCHOR"}}
	}
	check := func(t *testing.T, mutate func(url.Values), wants ...string) *edmHarness {
		t.Helper()
		h := newEdmHarness()
		f := base()
		mutate(f)
		mustContain(t, h.post(t, anchorAdmin(), "/ui/email-domains/new", f), wants...)
		return h
	}
	// The SPA's own checks refuse before the use case.
	for name, c := range map[string]struct {
		mutate func(url.Values)
		want   []string
	}{
		"domain shape":      {func(f url.Values) { f.Set("emailDomain", "not a domain") }, []string{`id="edm-form-email-domain-err"`, edmDomainHelp}},
		"no provider":       {func(f url.Values) { f.Set("identityProviderId", "") }, []string{`id="edm-form-identity-provider-id-err"`, "Choose an identity provider."}},
		"unknown provider":  {func(f url.Values) { f.Set("identityProviderId", "idp_nope") }, []string{"Choose an identity provider from the list."}},
		"unknown scope":     {func(f url.Values) { f.Set("scopeType", "ROOT") }, []string{`id="edm-form-scope-type-err"`}},
		"unknown client":    {func(f url.Values) { f.Set("scopeType", "CLIENT"); f.Set("client", "nobody") }, []string{noSuchClient}},
		"tenant required":   {func(f url.Values) { f.Set("identityProviderId", "idp_2") }, []string{`id="edm-form-tenant-err"`}},
		"days out of range": {func(f url.Values) { f.Set("days", "400") }, []string{`id="edm-form-days-err"`, edmDaysHelp}},
		"days not a number": {func(f url.Values) { f.Set("days", "many") }, []string{edmDaysHelp}},
	} {
		t.Run(name, func(t *testing.T) {
			h := check(t, c.mutate, c.want...)
			assert.Empty(t, h.ops.created, "refused before the use case")
		})
	}
	// The use case's own codes land on their fields.
	for name, c := range map[string]struct {
		err  error
		want string
	}{
		"mapped":       {usecase.Conflict("DOMAIN_ALREADY_MAPPED", "Email domain 'newco.com' is already mapped"), `id="edm-form-email-domain-err"`},
		"client":       {usecase.Validation("PRIMARY_CLIENT_REQUIRED", "primaryClientId is required for PARTNER and CLIENT scope"), `id="edm-form-client-err"`},
		"method":       {usecase.Validation("2FA_METHOD_REQUIRED", "at least one 2FA method must be allowed when require2fa is set"), `id="edm-form-methods-err"`},
		"unclassified": {usecase.Internal("REPO", "boom", nil), `id="edm-form-error"`},
	} {
		t.Run(name, func(t *testing.T) {
			h := newEdmHarness()
			h.ops.err = c.err
			f := base()
			f.Set("require2fa", "true")
			f.Set("scopeType", "CLIENT")
			f.Set("client", "acme")
			mustContain(t, h.post(t, anchorAdmin(), "/ui/email-domains/new", f), c.want)
		})
	}
	// A native post that fails renders the page with the form and the status.
	h := newEdmHarness()
	f := base()
	f.Set("emailDomain", "bad")
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/email-domains/new", form: f})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	mustContain(t, rec.Body.String(), `id="edm-form"`, `id="edm-form-email-domain-err"`)
}

func TestEmailDomainsEditForm(t *testing.T) {
	h := newEdmHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/email-domains/edm_1/edit"})
	mustContain(t, body, "Edit Email Domain Mapping", "Email domain cannot be changed", `action="/ui/email-domains/edm_1/edit"`, `name="drawer"`,
		`value="acme"`, "The scope of an existing mapping cannot be changed here", "Changing the provider moves this domain&#39;s users")
	// The scope is shown, not offered: the API cannot change it.
	mustNotContain(t, body, `name="scopeType"`, `name="emailDomain"`)
	sso := h.do(t, anchorAdmin(), req{target: "/ui/email-domains/edm_2/edit"})
	mustContain(t, sso, `value="tenant-guid-1234"`)
}

func TestEmailDomainsUpdate(t *testing.T) {
	// Internal-auth domain: the client, the two-factor policy.
	h := newEdmHarness()
	body := h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{
		"identityProviderId": {"idp_1"}, "client": {"acme"}, "require2fa": {"true"}, "methods": {"EMAIL_PIN"}, "remember": {""}, "days": {"7"}, "drawer": {"edm_1"},
	})
	assert.Empty(t, h.ops.moved)
	require.Len(t, h.ops.updated, 1)
	u := h.ops.updated[0]
	assert.Equal(t, "edm_1", u.ID)
	assert.Equal(t, "clt_acme", deref(u.PrimaryClientID))
	assert.Nil(t, u.RequiredOIDCTenantID)
	assert.True(t, *u.Require2FA)
	assert.Equal(t, []string{"EMAIL_PIN"}, u.Allowed2FAMethods)
	assert.False(t, *u.RememberDeviceEnabled)
	assert.Equal(t, 7, *u.RememberDeviceDays)
	mustContain(t, body, "Email domain mapping updated successfully")

	// Two-factor off sends an empty method set, as the SPA does.
	h = newEdmHarness()
	h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_1"}, "client": {"acme"}, "methods": {"TOTP"}})
	require.Len(t, h.ops.updated, 1)
	assert.False(t, *h.ops.updated[0].Require2FA)
	assert.Equal(t, []string{}, h.ops.updated[0].Allowed2FAMethods)

	// An anchor mapping clears its client; a multi-tenant provider's tenant is
	// required and sent (blank would clear it, but it is required).
	h = newEdmHarness()
	h.post(t, anchorAdmin(), "/ui/email-domains/edm_2/edit", url.Values{"identityProviderId": {"idp_2"}, "tenant": {"  new-tenant "}, "client": {"acme"}})
	require.Len(t, h.ops.updated, 1)
	u = h.ops.updated[0]
	assert.Equal(t, "", deref(u.PrimaryClientID))
	require.NotNil(t, u.PrimaryClientID, "an anchor mapping's client is cleared, not left")
	assert.Equal(t, "new-tenant", deref(u.RequiredOIDCTenantID))
	assert.Nil(t, u.Require2FA, "two-factor does not apply to an OIDC domain")
	missing := h.post(t, anchorAdmin(), "/ui/email-domains/edm_2/edit", url.Values{"identityProviderId": {"idp_2"}})
	mustContain(t, missing, `id="edm-form-tenant-err"`)
	assert.Len(t, h.ops.updated, 1)

	// A client or partner mapping needs its client.
	h = newEdmHarness()
	mustContain(t, h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_1"}}), `id="edm-form-client-err"`, "A primary client is required.")
	mustContain(t, h.post(t, anchorAdmin(), "/ui/email-domains/edm_3/edit", url.Values{"identityProviderId": {"idp_3"}, "client": {"nobody"}}), noSuchClient)
	assert.Empty(t, h.ops.updated)

	// The scope and the domain are not editable: a post that names others
	// changes neither (the command has no field for them).
	h = newEdmHarness()
	h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_1"}, "client": {"acme"}, "scopeType": {"ANCHOR"}, "emailDomain": {"evil.com"}})
	require.Len(t, h.ops.updated, 1)
	assert.Equal(t, "clt_acme", deref(h.ops.updated[0].PrimaryClientID), "the mapping stays a client mapping")
}

// TestEmailDomainsMoveNeedsConfirmation: changing the provider changes how a
// domain's users sign in, so the save waits for the SPA's confirmation; a post
// that names no confirmation never reaches the use case.
func TestEmailDomainsMoveNeedsConfirmation(t *testing.T) {
	form := func() url.Values {
		return url.Values{"identityProviderId": {"idp_3"}, "client": {"acme"}}
	}
	h := newEdmHarness()
	body := h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", form())
	assert.Zero(t, h.ops.calls(), "nothing is done before the move is confirmed")
	mustContain(t, body, "Change Identity Provider", `name="confirmMove"`, "Move acme.com to Keycloak", "will sign in through Keycloak", "Password login for this domain will be disabled")

	// To an internal provider: the SSO users are converted.
	h = newEdmHarness()
	f := url.Values{"identityProviderId": {"idp_1"}, "confirmMove": {"yes"}}
	body = h.post(t, anchorAdmin(), "/ui/email-domains/edm_3/edit", url.Values{"identityProviderId": {"idp_1"}, "client": {"acme"}})
	mustContain(t, body, "converted to password sign-in", "must reset their password", "Moving from Keycloak to Internal")
	assert.Zero(t, h.ops.calls())
	f.Set("client", "acme")
	h.ops.usersReset = 3
	body = h.post(t, anchorAdmin(), "/ui/email-domains/edm_3/edit", f)
	require.Len(t, h.ops.moved, 1)
	assert.Equal(t, "edm_3", h.ops.moved[0].ID)
	assert.Equal(t, "idp_1", h.ops.moved[0].IdentityProviderID)
	require.Len(t, h.ops.updated, 1, "the move is followed by the update")
	mustContain(t, body, "3 SSO-provisioned users converted to password sign-in (password reset required)", "Email domain mapping updated successfully")

	h.ops.usersReset = 1
	one := h.post(t, anchorAdmin(), "/ui/email-domains/edm_3/edit", f)
	mustContain(t, one, "1 SSO-provisioned user converted")

	// A provider that is not on the list is refused; a failed move does not
	// go on to the update.
	h = newEdmHarness()
	mustContain(t, h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_nope"}, "client": {"acme"}, "confirmMove": {"yes"}}), "Choose an identity provider from the list.")
	assert.Zero(t, h.ops.calls())
	h.ops.moveErr = usecase.Conflict("ALREADY_ON_PROVIDER", "already mapped to that identity provider")
	h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/edit", url.Values{"identityProviderId": {"idp_3"}, "client": {"acme"}, "confirmMove": {"yes"}})
	assert.Len(t, h.ops.moved, 1)
	assert.Empty(t, h.ops.updated)
}

func TestEmailDomainsDelete(t *testing.T) {
	h := newEdmHarness()
	body := h.post(t, anchorAdmin(), "/ui/email-domains/edm_1/delete", url.Values{"drawer": {"edm_1"}})
	assert.Equal(t, []string{"edm_1"}, h.ops.deleted)
	mustContain(t, body, "Email domain mapping deleted", "selEdm")
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/email-domains/edm_2/delete", form: url.Values{}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/email-domains", rec.Header().Get("Location"))
	h.ops.err = usecase.Internal("REPO", "boom", nil)
	failed := h.post(t, anchorAdmin(), "/ui/email-domains/edm_3/delete", url.Values{})
	mustContain(t, failed, "Something went wrong.")
	mustNotContain(t, failed, "Email domain mapping deleted")
}

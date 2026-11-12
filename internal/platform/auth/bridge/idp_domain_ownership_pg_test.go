//go:build integration

package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalauth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// An IdP authenticates the email domains it owns, nothing else. Provider-direct
// login (an explicit provider id, no email-domain mapping in the state) must
// therefore refuse an IdP with no mapped domain, bind the token's email domain
// to the IdP's own domains, and apply the owning mapping's tenant pin.

// newBareIdP creates a single-tenant OIDC IdP on the fixture's fake IdP with NO
// email domains. Its mapping scope is explicit, as domain creation requires.
func (f *oidcTestFixture) newBareIdP(t *testing.T) string {
	t.Helper()
	pool := testpg.Pool(t)
	idps := identityprovider.NewRepository(pool)
	suffix := strings.NewReplacer("-", "a", "_", "b").Replace(strings.ToLower(randString(6)))
	issuer, clientID := f.idp.issuerURL, f.idp.clientID
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), testpg.NewUoW(t),
		idpops.CreateIdentityProvider(idpops.Deps{
			Repo: idps,
			MoveDeps: edmops.MoveDeps{
				Mappings: emaildomainmapping.NewRepository(pool), IDPs: idps, Principals: principal.NewRepository(pool),
			},
		}),
		idpops.CreateCommand{
			Code: "bare-" + suffix, Name: "Bare IdP " + suffix, Type: "OIDC",
			OIDCIssuerURL: &issuer, OIDCClientID: &clientID, MappingScope: new("ANCHOR"),
		}, testpg.TestEC())
	require.NoError(t, err)
	return res.IdentityProviderID
}

// mapDomain points an email domain at an IdP, optionally pinning a tenant.
func (f *oidcTestFixture) mapDomain(t *testing.T, domain, idpID string, tenant *string) {
	t.Helper()
	_, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t),
		edmops.CreateMapping(emaildomainmapping.NewRepository(testpg.Pool(t)), identityprovider.NewRepository(testpg.Pool(t))),
		edmops.CreateCommand{
			EmailDomain: domain, IdentityProviderID: idpID, ScopeType: "ANCHOR", RequiredOIDCTenantID: tenant,
		}, testpg.TestEC())
	require.NoError(t, err)
}

// directCallback drives the callback for a provider-direct login state (empty
// mapping id) against idpID, with the fake IdP asserting claims.
func (f *oidcTestFixture) directCallback(t *testing.T, idpID string, claims map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	nonce := "nonce-" + randString(10)
	claims["nonce"] = nonce
	state := randString(16)
	require.NoError(t, f.states.Insert(context.Background(), NewLoginState(state, "", idpID, "", nonce, "")))
	f.idp.idTokenClaims = claims
	f.idp.accessToken = "opaque"
	f.lastSessionCalled = false
	f.lastSessionPrincipalID = ""
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+state+"&code=fake-code-"+state, nil)
	req.AddCookie(f.endpoint.loginBindingCookie(state))
	setSSOTestHeaders(req)
	rec := httptest.NewRecorder()
	f.endpoint.handleCallback(rec, req)
	return rec
}

func uniqueDomain(t *testing.T, label string) string {
	t.Helper()
	return label + "-" + strings.ToLower(strings.NewReplacer("-", "a", "_", "b").Replace(randString(8))) + ".example"
}

func (f *oidcTestFixture) newPortalUser(t *testing.T, email string) *principal.Principal {
	t.Helper()
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(f.pool)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreatePortalUser(repo),
		principalops.CreatePortalUserCommand{Email: email, Provider: new("OIDC")}, testpg.TestEC())
	require.NoError(t, err)
	p, err := repo.FindByID(context.Background(), ev.UserID)
	require.NoError(t, err)
	require.NotNil(t, p)
	return p
}

// T1
func TestResolveByProviderID_RequiresAnOwnedEmailDomain(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	b := f.endpoint.bridge

	_, _, err := b.ResolveByProviderID(context.Background(), idpID)
	require.ErrorIs(t, err, ErrProviderHasNoEmailDomains)
	assert.Contains(t, err.Error(), idpID, "the provider id is in the message")

	f.mapDomain(t, uniqueDomain(t, "t1"), idpID, nil)
	res, idp, err := b.ResolveByProviderID(context.Background(), idpID)
	require.NoError(t, err)
	assert.NotNil(t, res)
	assert.Len(t, idp.AllowedEmailDomains, 1)
}

// T2
func TestProviderDirectStart_NoEmailDomainsRefused(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)

	rec := httptest.NewRecorder()
	f.endpoint.handleLogin(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?provider_id="+url.QueryEscape(idpID), nil))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "IDP_NO_EMAIL_DOMAINS", envelopeCode(t, rec), rec.Body.String())
	assert.Empty(t, rec.Header().Get("Location"))
	assert.Empty(t, rec.Result().Cookies(), "no binding cookie")
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_oidc_login_states WHERE identity_provider_id = $1`, idpID).Scan(&n))
	assert.Zero(t, n, "no login state inserted")
}

// T3
func TestPortalStart_NoEmailDomainsRefused(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	flows := portalauth.NewFlowRepo(f.pool)
	f.endpoint.Portal = &PortalBridge{Flows: flows}
	flow, err := portalauth.NewLoginFlow("oc-"+randString(8), "clt_"+randString(8), "https://app.example.org/cb", "rp-state")
	require.NoError(t, err)
	require.NoError(t, flows.Insert(context.Background(), flow))

	rec := httptest.NewRecorder()
	f.endpoint.handlePortalOIDCLogin(rec, httptest.NewRequest(http.MethodGet,
		"/portal/auth/oidc/login?flow="+url.QueryEscape(flow.ID)+"&provider_id="+url.QueryEscape(idpID), nil))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "IDP_NO_EMAIL_DOMAINS", envelopeCode(t, rec), rec.Body.String())
	assert.Empty(t, rec.Header().Get("Location"))
}

// T4: the account takeover. An IdP that owns no domain asserts an existing
// administrator's email.
func TestProviderDirect_IdPWithNoDomainCannotSignInAnExistingAccount(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	email := "victim-" + strings.ToLower(strings.NewReplacer("-", "a", "_", "b").Replace(randString(8))) + "@owned-by-nobody.example"
	victim := f.newPortalUser(t, email)
	_, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t),
		principalops.AssignRoles(principal.NewRepository(f.pool), role.NewRepository(f.pool)),
		principalops.AssignRolesCommand{UserID: victim.ID, Roles: []string{f.platformRoleName}}, testpg.TestEC())
	require.NoError(t, err)
	repo := principal.NewRepository(f.pool)
	before, err := repo.FindByEmail(context.Background(), email)
	require.NoError(t, err)
	require.NotEmpty(t, before.Roles, "the victim holds a role")

	// The start refuses outright.
	rec := httptest.NewRecorder()
	f.endpoint.handleLogin(rec, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?provider_id="+url.QueryEscape(idpID), nil))
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "IDP_NO_EMAIL_DOMAINS", envelopeCode(t, rec))
	assert.Empty(t, rec.Result().Cookies())

	// The IdP lost its domains mid-flow: a provider-direct state already exists.
	rec = f.directCallback(t, idpID, map[string]any{"email": email})
	assert.False(t, f.lastSessionCalled, "no session writer call")
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, "IDP_NO_EMAIL_DOMAINS", envelopeCode(t, rec), rec.Body.String())
	for _, c := range rec.Result().Cookies() {
		assert.NotEqual(t, "fc_session", c.Name)
	}

	// And an IdP that owns an UNRELATED domain cannot assert it either.
	f.mapDomain(t, uniqueDomain(t, "t4"), idpID, nil)
	rec = f.directCallback(t, idpID, map[string]any{"email": email})
	assert.Equal(t, "EMAIL_DOMAIN_MISMATCH", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled)

	after, err := repo.FindByEmail(context.Background(), email)
	require.NoError(t, err)
	assert.Equal(t, before.ID, after.ID)
	assert.Equal(t, before.Roles, after.Roles)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "the principal is untouched")
}

// T5
func TestProviderDirect_ForeignDomainRefused(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	f.mapDomain(t, uniqueDomain(t, "x"), idpID, nil)
	other := "someone@" + uniqueDomain(t, "other")
	f.newPortalUser(t, other)

	rec := f.directCallback(t, idpID, map[string]any{"email": other})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, "EMAIL_DOMAIN_MISMATCH", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled)
}

// T6
func TestProviderDirect_TenantPinApplied(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	domain := uniqueDomain(t, "x")
	tenant := "tenant-" + strings.ToLower(strings.NewReplacer("-", "a", "_", "b").Replace(randString(6)))
	f.mapDomain(t, domain, idpID, &tenant)
	email := "a@" + domain

	rec := f.directCallback(t, idpID, map[string]any{"email": email, "tid": "another-tenant"})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Equal(t, "TENANT_MISMATCH", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled)

	rec = f.directCallback(t, idpID, map[string]any{"email": email})
	assert.Equal(t, "TENANT_MISMATCH", envelopeCode(t, rec), rec.Body.String())
	assert.False(t, f.lastSessionCalled)

	rec = f.directCallback(t, idpID, map[string]any{"email": email, "tid": tenant})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.lastSessionCalled, "the right tenant signs in")
}

// T7: ordinary provider-direct login for a domain the IdP owns.
func TestProviderDirect_OwnedDomainSignsInExistingAndProvisionsNew(t *testing.T) {
	f := newOidcTestFixture(t)
	idpID := f.newBareIdP(t)
	domain := uniqueDomain(t, "x")
	f.mapDomain(t, domain, idpID, nil)
	repo := principal.NewRepository(f.pool)

	existing := f.newPortalUser(t, "a@"+domain)
	rec := f.directCallback(t, idpID, map[string]any{"email": "a@" + domain})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.lastSessionCalled)
	assert.Equal(t, string(existing.ID), f.lastSessionPrincipalID, "the existing principal is signed in")

	rec = f.directCallback(t, idpID, map[string]any{"email": "b@" + domain})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, f.lastSessionCalled)
	created, err := repo.FindByEmail(context.Background(), "b@"+domain)
	require.NoError(t, err)
	require.NotNil(t, created, "a new email is JIT-provisioned")
	assert.Equal(t, string(created.ID), f.lastSessionPrincipalID)
	assert.Empty(t, created.Roles, "the provisioned principal is inert")
}

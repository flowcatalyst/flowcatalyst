//go:build integration

package portalauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestPortalPasswordJourney pins the portal plane's password flow end to
// end: /portal/authorize parks a flow and bounces to the SPA page; a wrong
// password is refused WITHOUT consuming the flow; the right password
// consumes it, mints a code bound to the portal-identity subject, and
// answers with the app redirect; a replayed login finds no flow.
func TestPortalPasswordJourney(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)

	// Tenant client + its portal-flagged PUBLIC OAuth client.
	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal Flow Co", Identifier: "portal-flow-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	oauthEv, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName:     "Portal Flow App",
			ClientType:     "PUBLIC",
			RedirectURIs:   []string{"https://portal.flow.test/callback"},
			GrantTypes:     []string{"authorization_code"},
			PortalClientID: &tenantID,
		}, testpg.TestEC())
	require.NoError(t, err)
	oauthClientID := oauthEv.ClientID

	// Portal identity with a working password.
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "pat@portal-flow.test", Source: "INVITE"}, testpg.TestEC())
	require.NoError(t, err)
	hash, err := passwordhash.Hash("Portal-pass-123456")
	require.NoError(t, err)
	require.NoError(t, identities.SetPasswordHash(ctx, identEv.IdentityID, hash))

	s := &State{
		OAuthClients: authRepo.OAuthClients,
		Identities:   identities,
		IdPs:         identityprovider.NewRepository(pool),
		Flows:        NewFlowRepo(pool),
		AuthCodes:    grantstore.NewAuthorizationCodeRepository(pool),
	}

	// 1. /portal/authorize parks the flow and bounces to the SPA page.
	authorizeURL := "/portal/authorize?response_type=code&client_id=" + url.QueryEscape(oauthClientID) +
		"&redirect_uri=" + url.QueryEscape("https://portal.flow.test/callback") +
		"&state=xyz&code_challenge=aGVsbG8&code_challenge_method=S256"
	rec := httptest.NewRecorder()
	s.Authorize(rec, httptest.NewRequest(http.MethodGet, authorizeURL, nil))
	require.Equal(t, http.StatusTemporaryRedirect, rec.Code, rec.Body.String())
	loc := rec.Header().Get("Location")
	require.True(t, strings.HasPrefix(loc, "/portal/login?flow="), loc)
	flowID, _ := url.QueryUnescape(strings.TrimPrefix(loc, "/portal/login?flow="))

	// 2. check-domain: no portal-bound IdP owns the domain → password.
	rec = postJSON(t, s.CheckDomain, "/portal/auth/check-domain",
		map[string]string{"flowId": flowID, "email": "pat@portal-flow.test"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"PASSWORD"`)

	// 3. Wrong password → uniform 401; the flow survives for a retry.
	rec = postJSON(t, s.PasswordLogin, "/portal/auth/login",
		map[string]string{"flowId": flowID, "email": "pat@portal-flow.test", "password": "wrong"})
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())

	// 4. Right password → code redirect; the code's subject is the identity.
	rec = postJSON(t, s.PasswordLogin, "/portal/auth/login",
		map[string]string{"flowId": flowID, "email": "pat@portal-flow.test", "password": "Portal-pass-123456"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		RedirectURL string `json:"redirectUrl"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.True(t, strings.HasPrefix(out.RedirectURL, "https://portal.flow.test/callback?"), out.RedirectURL)
	u, err := url.Parse(out.RedirectURL)
	require.NoError(t, err)
	codeParam := u.Query().Get("code")
	require.NotEmpty(t, codeParam)
	assert.Equal(t, "xyz", u.Query().Get("state"))

	code, err := s.AuthCodes.FindAndConsume(ctx, codeParam)
	require.NoError(t, err)
	require.NotNil(t, code)
	assert.Equal(t, identEv.IdentityID, code.PrincipalID, "code subject is the portal identity (ptu_)")
	assert.True(t, strings.HasPrefix(code.PrincipalID, "ptu_"))
	require.NotNil(t, code.CodeChallenge)

	// 5. The flow was consumed — a replayed login is refused.
	rec = postJSON(t, s.PasswordLogin, "/portal/auth/login",
		map[string]string{"flowId": flowID, "email": "pat@portal-flow.test", "password": "Portal-pass-123456"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestPortalAuthorizeRefusesNonPortalClient pins the plane boundary: an
// ordinary OAuth client cannot enter through /portal/authorize.
func TestPortalAuthorizeRefusesNonPortalClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	uow := testpg.NewUoW(t)

	ev, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName:   "Ordinary App",
			ClientType:   "PUBLIC",
			RedirectURIs: []string{"https://app.flow.test/cb"},
			GrantTypes:   []string{"authorization_code"},
		}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{OAuthClients: authRepo.OAuthClients, Flows: NewFlowRepo(pool)}
	rec := httptest.NewRecorder()
	s.Authorize(rec, httptest.NewRequest(http.MethodGet,
		"/portal/authorize?response_type=code&client_id="+url.QueryEscape(ev.ClientID)+
			"&redirect_uri="+url.QueryEscape("https://app.flow.test/cb")+"&state=s&code_challenge=c", nil))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "not a portal client")
}

// TestPortalLoginRefusesSuspendedIdentity: DISABLED identities are refused
// with the same uniform 401 as unknown ones.
func TestPortalLoginRefusesSuspendedIdentity(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal Susp Co", Identifier: "portal-susp-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	oauthEv, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "Portal Susp App", ClientType: "PUBLIC",
			RedirectURIs: []string{"https://susp.flow.test/cb"},
			GrantTypes:   []string{"authorization_code"}, PortalClientID: &tenantID,
		}, testpg.TestEC())
	require.NoError(t, err)

	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "sus@portal-susp.test", Source: "INVITE"}, testpg.TestEC())
	require.NoError(t, err)
	hash, err := passwordhash.Hash("Portal-pass-123456")
	require.NoError(t, err)
	require.NoError(t, identities.SetPasswordHash(ctx, identEv.IdentityID, hash))
	_, err = usecaseop.Run(ctx, uow, portalidentity.SetStatus(identities),
		portalidentity.SetStatusCommand{ID: identEv.IdentityID, ClientID: tenantID, Status: "DISABLED"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{
		OAuthClients: authRepo.OAuthClients, Identities: identities,
		IdPs: identityprovider.NewRepository(pool), Flows: NewFlowRepo(pool),
		AuthCodes: grantstore.NewAuthorizationCodeRepository(pool),
	}
	rec := httptest.NewRecorder()
	s.Authorize(rec, httptest.NewRequest(http.MethodGet,
		"/portal/authorize?response_type=code&client_id="+url.QueryEscape(oauthEv.ClientID)+
			"&redirect_uri="+url.QueryEscape("https://susp.flow.test/cb")+"&state=s&code_challenge=c", nil))
	require.Equal(t, http.StatusTemporaryRedirect, rec.Code)
	flowID, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "/portal/login?flow="))

	rec = postJSON(t, s.PasswordLogin, "/portal/auth/login",
		map[string]string{"flowId": flowID, "email": "sus@portal-susp.test", "password": "Portal-pass-123456"})
	require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
}

func postJSON(t *testing.T, h http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// fakeResetSender records portal reset sends.
type fakeResetSender struct {
	sent []string
	uris []*string
}

func (f *fakeResetSender) SendPortalReset(_ context.Context, identityID, _ string, redirectURI *string) error {
	f.sent = append(f.sent, identityID)
	f.uris = append(f.uris, redirectURI)
	return nil
}

// TestPortalForgotPassword pins the portal reset-request flow: an existing
// ACTIVE identity is mailed a reset whose redirect defaults to the portal's
// origin; unknown emails get the identical silent-success response with no
// send (anti-enumeration).
func TestPortalForgotPassword(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal Reset Co", Identifier: "portal-reset-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	oauthEv, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "Portal Reset App", ClientType: "PUBLIC",
			RedirectURIs: []string{"https://reset.flow.test/auth/callback"},
			GrantTypes:   []string{"authorization_code"}, PortalClientID: &tenantID,
		}, testpg.TestEC())
	require.NoError(t, err)
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "resetme@portal-reset.test", Source: "INVITE"}, testpg.TestEC())
	require.NoError(t, err)

	sender := &fakeResetSender{}
	s := &State{
		OAuthClients: authRepo.OAuthClients, Identities: identities,
		IdPs: identityprovider.NewRepository(pool), Flows: NewFlowRepo(pool),
		AuthCodes:     grantstore.NewAuthorizationCodeRepository(pool),
		PasswordReset: sender,
	}
	rec := httptest.NewRecorder()
	s.Authorize(rec, httptest.NewRequest(http.MethodGet,
		"/portal/authorize?response_type=code&client_id="+url.QueryEscape(oauthEv.ClientID)+
			"&redirect_uri="+url.QueryEscape("https://reset.flow.test/auth/callback")+"&state=s&code_challenge=c", nil))
	require.Equal(t, http.StatusTemporaryRedirect, rec.Code)
	flowID, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "/portal/login?flow="))

	// Existing identity → mailed, redirect = portal origin.
	rec = postJSON(t, s.RequestPasswordReset, "/portal/auth/password-reset",
		map[string]string{"flowId": flowID, "email": "resetme@portal-reset.test"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, sender.sent, 1)
	assert.Equal(t, identEv.IdentityID, sender.sent[0])
	require.NotNil(t, sender.uris[0])
	assert.Equal(t, "https://reset.flow.test/", *sender.uris[0])

	// Unknown email → identical response, nothing sent.
	rec = postJSON(t, s.RequestPasswordReset, "/portal/auth/password-reset",
		map[string]string{"flowId": flowID, "email": "nobody@portal-reset.test"})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "If an account exists")
	assert.Len(t, sender.sent, 1, "unknown email must not send")
}

//go:build integration

package portalauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestPortalPasswordLoginAppGate pins the per-app gate: a client runs two
// portal apps; an identity granted only the customer portal signs in there
// but is refused (403 NO_PORTAL_ACCESS, after the password check) at the
// supplier portal, and admitted once granted it. An inactive app refuses
// everyone.
func TestPortalPasswordLoginAppGate(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal Gate Co", Identifier: "portal-gate-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID

	mkApp := func(code string) string {
		ev, err := usecaseop.Run(ctx, uow, portalidentity.CreateApp(apps, clients),
			portalidentity.CreateAppCommand{ClientID: tenantID, Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.AppID
	}
	customerApp, supplierApp := mkApp("customer-portal"), mkApp("supplier-portal")

	mkOAuth := func(name, redirect, appID string) string {
		ev, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
			authops.CreateOAuthClientCommand{
				ClientName: name, ClientType: "PUBLIC",
				RedirectURIs: []string{redirect}, GrantTypes: []string{"authorization_code"},
				PortalClientID: &tenantID, PortalAppID: &appID,
			}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	customerOAuth := mkOAuth("Gate Customer", "https://customer.gate.test/cb", customerApp)
	supplierOAuth := mkOAuth("Gate Supplier", "https://supplier.gate.test/cb", supplierApp)

	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, apps),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "pat@gate.test", Source: "INVITE", PortalAppID: customerApp},
		testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, "customer-portal", identEv.AppCode)
	hash, err := passwordhash.Hash("Portal-pass-123456")
	require.NoError(t, err)
	require.NoError(t, identities.SetPasswordHash(ctx, identEv.IdentityID, hash))

	s := &State{
		OAuthClients: authRepo.OAuthClients,
		Identities:   identities,
		Apps:         apps,
		IdPs:         identityprovider.NewRepository(pool),
		Flows:        NewFlowRepo(pool),
		AuthCodes:    grantstore.NewAuthorizationCodeRepository(pool),
	}
	login := func(oauthClientID, redirect, password string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.Authorize(rec, httptest.NewRequest(http.MethodGet,
			"/portal/authorize?response_type=code&client_id="+url.QueryEscape(oauthClientID)+
				"&redirect_uri="+url.QueryEscape(redirect)+"&state=s&code_challenge=aGVsbG8&code_challenge_method=S256", nil))
		require.Equal(t, http.StatusTemporaryRedirect, rec.Code, rec.Body.String())
		flowID, _ := url.QueryUnescape(strings.TrimPrefix(rec.Header().Get("Location"), "/portal/login?flow="))
		return postJSON(t, s.PasswordLogin, "/portal/auth/login",
			map[string]string{"flowId": flowID, "email": "pat@gate.test", "password": password})
	}

	rec := login(customerOAuth, "https://customer.gate.test/cb", "Portal-pass-123456")
	require.Equal(t, http.StatusOK, rec.Code, "granted app admits: %s", rec.Body.String())

	rec = login(supplierOAuth, "https://supplier.gate.test/cb", "wrong-password")
	require.Equal(t, http.StatusUnauthorized, rec.Code, "the gate never answers before the password check")

	rec = login(supplierOAuth, "https://supplier.gate.test/cb", "Portal-pass-123456")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), "NO_PORTAL_ACCESS")

	_, err = usecaseop.Run(ctx, uow, portalidentity.GrantApp(identities, apps),
		portalidentity.AppGrantCommand{ClientID: tenantID, IdentityID: identEv.IdentityID, PortalAppID: supplierApp}, testpg.TestEC())
	require.NoError(t, err)
	rec = login(supplierOAuth, "https://supplier.gate.test/cb", "Portal-pass-123456")
	require.Equal(t, http.StatusOK, rec.Code, "granted after GrantApp: %s", rec.Body.String())

	inactive := false
	_, err = usecaseop.Run(ctx, uow, portalidentity.UpdateApp(apps),
		portalidentity.UpdateAppCommand{ClientID: tenantID, ID: customerApp, Active: &inactive}, testpg.TestEC())
	require.NoError(t, err)
	rec = login(customerOAuth, "https://customer.gate.test/cb", "Portal-pass-123456")
	require.Equal(t, http.StatusForbidden, rec.Code, "inactive app refuses: %s", rec.Body.String())

	// Revoking one app leaves the identity and its other grant intact.
	_, err = usecaseop.Run(ctx, uow, portalidentity.RevokeApp(identities, apps),
		portalidentity.AppGrantCommand{ClientID: tenantID, IdentityID: identEv.IdentityID, PortalAppID: customerApp}, testpg.TestEC())
	require.NoError(t, err)
	ident, err := identities.FindByID(ctx, identEv.IdentityID)
	require.NoError(t, err)
	assert.False(t, ident.HasApp(customerApp))
	assert.True(t, ident.HasApp(supplierApp))

	// Deleting an app takes its OAuth client with it (unlinking would turn
	// the client into an ungated client-wide portal) and leaves the other
	// app's client alone.
	res, err := usecaseop.RunTx(ctx, uow, portalidentity.DeleteApp(apps, authRepo.OAuthClients),
		portalidentity.DeleteAppCommand{ClientID: tenantID, ID: supplierApp}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, []string{supplierOAuth}, res.DeletedOAuthClientIDs)
	gone, err := authRepo.OAuthClients.FindByClientID(ctx, supplierOAuth)
	require.NoError(t, err)
	assert.Nil(t, gone)
	kept, err := authRepo.OAuthClients.FindByClientID(ctx, customerOAuth)
	require.NoError(t, err)
	assert.NotNil(t, kept)
	ident, err = identities.FindByID(ctx, identEv.IdentityID)
	require.NoError(t, err)
	assert.False(t, ident.HasApp(supplierApp), "grants cascade with the app")
}

//go:build integration

package oauthapi

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestPortalCodeRedemptionReportsApp pins the app half of redemption: a
// code minted through an app-linked portal OAuth client yields an id_token
// carrying portal_app_code / portal_app_id / portal_client_id, and a grant
// revoked between issuance and redemption kills the code.
func TestPortalCodeRedemptionReportsApp(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := auth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	apps := portalidentity.NewAppRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal App Token Co", Identifier: "portal-app-token-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	appEv, err := usecaseop.Run(ctx, uow, portalidentity.CreateApp(apps, clients),
		portalidentity.CreateAppCommand{ClientID: tenantID, Code: "Customer-Portal", Name: "Customer Portal"}, testpg.TestEC())
	require.NoError(t, err)
	oauthEv, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "App Token Portal", ClientType: "PUBLIC",
			RedirectURIs: []string{"https://portal.app-token.test/cb"}, GrantTypes: []string{"authorization_code"},
			PortalClientID: &tenantID, PortalAppID: &appEv.AppID,
		}, testpg.TestEC())
	require.NoError(t, err)
	portalClient, err := authRepo.OAuthClients.FindByClientID(ctx, oauthEv.ClientID)
	require.NoError(t, err)

	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients, apps),
		portalidentity.EnsureCommand{ClientID: tenantID, Email: "app@portal-app-token.test", Source: "INVITE", PortalAppID: appEv.AppID},
		testpg.TestEC())
	require.NoError(t, err)

	authCodes := grantstore.NewAuthorizationCodeRepository(pool)
	s := &State{
		OAuthClients:     fakeClientFinder{client: portalClient},
		PortalIdentities: identities,
		PortalApps:       apps,
		AuthCodes:        authCodes,
		Auth:             testAuthService(t),
	}
	redeem := func(raw string) (int, string) {
		scope := "openid profile"
		code := grantstore.NewAuthorizationCode(raw, oauthEv.ClientID, identEv.IdentityID, "https://portal.app-token.test/cb")
		code.Scope = &scope
		require.NoError(t, authCodes.Insert(ctx, code))
		rr := doTokenRequest(t, s, url.Values{
			"grant_type":   {"authorization_code"},
			"code":         {raw},
			"redirect_uri": {"https://portal.app-token.test/cb"},
			"client_id":    {oauthEv.ClientID},
		})
		return rr.Code, rr.Body.String()
	}

	status, body := redeem("pac_granted_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	require.Equal(t, 200, status, body)
	var resp struct {
		IDToken string `json:"id_token"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &resp))
	claims := decodeClaims(t, resp.IDToken)
	assert.Equal(t, "customer-portal", claims["portal_app_code"], "code is normalised and reported")
	assert.Equal(t, appEv.AppID, claims["portal_app_id"])
	assert.Equal(t, tenantID, claims["portal_client_id"])

	// The access token is client-bound (azp) like every other interactive
	// identity token, and the id_token's updated_at is the identity's own —
	// not the mint time.
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &tok))
	assert.Equal(t, oauthEv.ClientID, decodeClaims(t, tok.AccessToken)["azp"])
	ident, err := identities.FindByID(ctx, identEv.IdentityID)
	require.NoError(t, err)
	assert.EqualValues(t, ident.UpdatedAt.Unix(), claims["updated_at"])

	_, err = usecaseop.Run(ctx, uow, portalidentity.RevokeApp(identities, apps),
		portalidentity.AppGrantCommand{ClientID: tenantID, IdentityID: identEv.IdentityID, PortalAppID: appEv.AppID}, testpg.TestEC())
	require.NoError(t, err)
	status, body = redeem("pac_revoked_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	require.Equal(t, 400, status, body)
	assert.Contains(t, body, "invalid_grant")
}

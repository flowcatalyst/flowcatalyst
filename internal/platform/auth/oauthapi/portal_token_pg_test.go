//go:build integration

package oauthapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestPortalCodeRedemption pins the portal-subject branch of the
// authorization_code grant: the id_token subject is the ptu_ identity, its
// roles claim is EMPTY, no refresh token is ever issued (even with
// offline_access), and a suspended identity's code is refused.
func TestPortalCodeRedemption(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Portal Token Co", Identifier: "portal-token-co"}, testpg.TestEC())
	require.NoError(t, err)
	identEv, err := usecaseop.Run(ctx, uow, portalidentity.Ensure(identities, clients),
		portalidentity.EnsureCommand{ClientID: clientEv.ClientID, Email: "tok@portal-token.test", Source: "INVITE"}, testpg.TestEC())
	require.NoError(t, err)

	// A PUBLIC portal OAuth client, injected via the fake finder (client
	// authentication is what's under test upstream; here we test redemption).
	portalClient := &auth.OAuthClient{
		ClientID: "portal-app-1", Active: true,
		RedirectURIs: []string{"https://portal.token.test/cb"},
		GrantTypes:   []string{"authorization_code"},
	}
	authCodes := grantstore.NewAuthorizationCodeRepository(pool)
	s := &State{
		OAuthClients:     fakeClientFinder{client: portalClient},
		PortalIdentities: identities,
		AuthCodes:        authCodes,
		Auth:             testAuthService(t),
	}

	mintCode := func(scope string) string {
		raw := "ptc_" + strings.ReplaceAll(identEv.IdentityID, "_", "") + scope[:2] + "xxxxxxxxxxxxxxxxxxxxxxxxxxx"
		code := grantstore.NewAuthorizationCode(raw, "portal-app-1", identEv.IdentityID, "https://portal.token.test/cb")
		code.Scope = &scope
		require.NoError(t, authCodes.Insert(ctx, code))
		return raw
	}

	// Redeem: openid + offline_access — id_token yes, refresh token NO.
	form := url.Values{
		"grant_type":   {"authorization_code"},
		"code":         {mintCode("openid profile offline_access")},
		"redirect_uri": {"https://portal.token.test/cb"},
		"client_id":    {"portal-app-1"},
	}
	rr := doTokenRequest(t, s, form)
	require.Equal(t, 200, rr.Code, rr.Body.String())
	var resp struct {
		AccessToken  string  `json:"access_token"`
		IDToken      *string `json:"id_token"`
		RefreshToken *string `json:"refresh_token"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	require.NotNil(t, resp.IDToken)
	assert.Nil(t, resp.RefreshToken, "portal subjects never get refresh tokens")

	claims := decodeClaims(t, *resp.IDToken)
	assert.Equal(t, identEv.IdentityID, claims["sub"])
	assert.Equal(t, "tok@portal-token.test", claims["email"])
	if roles, ok := claims["roles"].([]any); ok {
		assert.Empty(t, roles, "portal id_tokens carry no platform roles")
	}
	atClaims := decodeClaims(t, resp.AccessToken)
	assert.Equal(t, "identity", atClaims["token_use"], "portal access tokens are authority-free")

	// A suspended identity's outstanding code is dead on redemption.
	_, err = usecaseop.Run(ctx, uow, portalidentity.SetStatus(identities),
		portalidentity.SetStatusCommand{ID: identEv.IdentityID, ClientID: clientEv.ClientID, Status: "DISABLED"}, testpg.TestEC())
	require.NoError(t, err)
	form.Set("code", mintCode("openid suspended-check"))
	rr = doTokenRequest(t, s, form)
	require.Equal(t, 400, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "invalid_grant")
}

// decodeClaims parses a JWT's payload without verifying (test-side peek).
func decodeClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	require.Len(t, parts, 3)
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(payload, &m))
	return m
}

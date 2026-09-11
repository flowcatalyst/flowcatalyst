//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestCreatePortalAppProvisionsOAuthClient pins POST /api/portal-apps:
// creating an app provisions its portal OAuth client in the same
// transaction — portal-flagged for the app's client, linked to the app,
// authorization_code only, PKCE on, with the callback registered — and a
// CONFIDENTIAL client's secret comes back exactly once. PUBLIC gets none.
// A duplicate code writes nothing (no orphan OAuth client).
func TestCreatePortalAppProvisionsOAuthClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	s := &State{
		Identities:   portalidentity.NewRepository(pool),
		Apps:         portalidentity.NewAppRepository(pool),
		Clients:      clients,
		OAuthClients: oauth,
		UoW:          testpg.NewUoW(t),
	}
	clientEv, err := usecaseop.Run(ctx, s.UoW, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Create App Co", Identifier: "portal-create-app-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID
	actx := testpg.AnchorCtx()

	create := func(code, clientType string, redirects ...string) (*CreatePortalAppResponse, error) {
		req := CreatePortalAppRequest{ClientID: tenantID, Code: code, Name: "Portal " + code, RedirectURIs: redirects}
		if clientType != "" {
			req.ClientType = &clientType
		}
		out, err := s.createApp(actx, &apicommon.In[CreatePortalAppRequest]{Body: req})
		if err != nil {
			return nil, err
		}
		return &out.Body, nil
	}

	res, err := create("customer-portal", "", "https://customer.create.test/cb")
	require.NoError(t, err)
	assert.Equal(t, "CONFIDENTIAL", res.ClientType, "server-side default")
	require.NotNil(t, res.ClientSecret, "confidential secret returned once")
	assert.NotEmpty(t, *res.ClientSecret)
	require.Len(t, res.PortalApp.OAuthClients, 1, "the app lists its provisioned OAuth client")
	assert.Equal(t, res.OAuthClientID, res.PortalApp.OAuthClients[0].ClientID)

	oc, err := oauth.FindByClientID(ctx, res.OAuthClientID)
	require.NoError(t, err)
	require.NotNil(t, oc)
	require.NotNil(t, oc.PortalClientID)
	assert.Equal(t, tenantID, *oc.PortalClientID)
	require.NotNil(t, oc.PortalAppID)
	assert.Equal(t, res.PortalApp.ID, *oc.PortalAppID)
	assert.Equal(t, []string{"https://customer.create.test/cb"}, oc.RedirectURIs)
	assert.Equal(t, []string{"authorization_code"}, oc.GrantTypes, "portal logins never get refresh tokens")
	assert.True(t, oc.PKCERequired)
	assert.False(t, oc.APIAccess)
	assert.NotEqual(t, *res.ClientSecret, derefOr(oc.SecretRef), "only the hash is stored")

	pub, err := create("supplier-portal", "PUBLIC")
	require.NoError(t, err)
	assert.Equal(t, "PUBLIC", pub.ClientType)
	assert.Nil(t, pub.ClientSecret)

	// Duplicate code: refused, and no second OAuth client is left behind.
	before, err := oauth.FindByPortalClient(ctx, tenantID)
	require.NoError(t, err)
	_, err = create("Customer-Portal", "", "https://dup.create.test/cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CODE_EXISTS")
	after, err := oauth.FindByPortalClient(ctx, tenantID)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "the transaction wrote nothing")

	_, err = create("bad-redirect", "", "https://*.wild.test/cb")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "REDIRECT_URI_INVALID")
}

func derefOr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

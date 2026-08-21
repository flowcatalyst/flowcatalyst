//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// fakeInviteMinter records what kind of invite ensure decided to send.
type fakeInviteMinter struct {
	setPasswordInvites []string // emails that got a set-password invite
	ssoInvites         []string // emails that got the "open the portal" invite
}

func (f *fakeInviteMinter) PortalInviteLink(_ context.Context, identityID string, _ *string) (string, error) {
	return "https://platform.test/auth/set-password?token=for-" + identityID, nil
}

func (f *fakeInviteMinter) SendPortalInvite(_ context.Context, _, email string, _ *string) error {
	f.setPasswordInvites = append(f.setPasswordInvites, email)
	return nil
}

func (f *fakeInviteMinter) SendPortalSSOInvite(_ context.Context, email, _ string) error {
	f.ssoInvites = append(f.ssoInvites, email)
	return nil
}

// TestEnsurePortalUser_SSODomainGetsNoPassword pins the invite routing: an
// email whose domain is owned by an OIDC IdP gets the SSO "open the portal"
// invite (never set-password) — domain ownership alone decides; the IdP
// carries no per-client binding. An unowned domain gets the set-password
// invite.
func TestEnsurePortalUser_SSODomainGetsNoPassword(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	authRepo := platformauth.NewRepository(pool)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	idps := identityprovider.NewRepository(pool)
	uow := testpg.NewUoW(t)

	clientEv, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Ensure SSO Co", Identifier: "ensure-sso-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenantID := clientEv.ClientID

	// The client's portal OAuth client — the invite's derived portal origin.
	_, err = usecaseop.Run(ctx, uow, authops.CreateOAuthClient(authRepo.OAuthClients),
		authops.CreateOAuthClientCommand{
			ClientName: "Ensure SSO Portal", ClientType: "PUBLIC",
			RedirectURIs: []string{"https://portal.ensure-sso.test/cb"},
			GrantTypes:   []string{"authorization_code"}, PortalClientID: &tenantID,
		}, testpg.TestEC())
	require.NoError(t, err)

	// An OIDC IdP owning ssoorg.test — domain ownership alone routes it.
	issuer := "https://login.ssoorg.test"
	oidcClient := "sso-org"
	_, err = usecaseop.RunTx(testpg.AnchorCtx(), uow,
		idpops.CreateIdentityProvider(idpops.Deps{
			Repo: idps,
			MoveDeps: edmops.MoveDeps{
				Mappings:   emaildomainmapping.NewRepository(pool),
				IDPs:       idps,
				Principals: principal.NewRepository(pool),
			},
		}),
		idpops.CreateCommand{
			Code: "ensure-sso-idp", Name: "Ensure SSO Org", Type: "OIDC",
			OIDCIssuerURL: &issuer, OIDCClientID: &oidcClient,
			AllowedEmailDomains: []string{"ssoorg.test"},
		}, testpg.TestEC())
	require.NoError(t, err)

	minter := &fakeInviteMinter{}
	s := &State{
		Identities:   identities,
		Clients:      clients,
		OAuthClients: authRepo.OAuthClients,
		UoW:          uow,
		Invites:      minter,
		IdPs:         idps,
	}
	anchorCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_ensuresso_admin", Scope: auth.ScopeAnchor,
	})

	// SSO-owned domain → ssoManaged, "open the portal" invite, no password.
	out, err := s.ensure(anchorCtx, &apicommon.In[PortalUserRequest]{Body: PortalUserRequest{
		ClientID: tenantID, Email: "pat@ssoorg.test",
	}})
	require.NoError(t, err)
	assert.True(t, out.Body.SSOManaged, "OIDC-owned domain must be SSO-managed")
	assert.True(t, out.Body.Invited)
	assert.Equal(t, []string{"pat@ssoorg.test"}, minter.ssoInvites)
	assert.Empty(t, minter.setPasswordInvites, "SSO-owned domain must never get a set-password invite")

	// returnInviteLink for an SSO domain hands back the portal entry, not a
	// set-password link.
	out, err = s.ensure(anchorCtx, &apicommon.In[PortalUserRequest]{Body: PortalUserRequest{
		ClientID: tenantID, Email: "sam@ssoorg.test", ReturnInviteLink: true,
	}})
	require.NoError(t, err)
	assert.True(t, out.Body.SSOManaged)
	require.NotNil(t, out.Body.InviteURL)
	assert.Equal(t, "https://portal.ensure-sso.test/", *out.Body.InviteURL)

	// Unowned domain → ordinary set-password invite.
	out, err = s.ensure(anchorCtx, &apicommon.In[PortalUserRequest]{Body: PortalUserRequest{
		ClientID: tenantID, Email: "lee@passwordorg.test",
	}})
	require.NoError(t, err)
	assert.False(t, out.Body.SSOManaged)
	assert.True(t, out.Body.Invited)
	assert.Equal(t, []string{"lee@passwordorg.test"}, minter.setPasswordInvites)
}

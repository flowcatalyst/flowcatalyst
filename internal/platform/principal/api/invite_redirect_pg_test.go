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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestCreateUser_InviteRedirectURI pins inviteRedirectUri: it is validated
// against the login OAuth clients of the caller's applications (same matcher
// as /oauth/authorize, wildcards included) BEFORE the user is written, and a
// valid one rides on the minted invite.
func TestCreateUser_InviteRedirectURI(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	authRepo := platformauth.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	cev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Invite Redirect Co", Identifier: "INVITE-REDIRECT"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := cev.ClientID

	const appA, appB = "app_ivr_a", "app_ivr_b"
	seed := func(name string, apps, grants, uris []string, portalOwner *string) {
		t.Helper()
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, authops.CreateOAuthClient(authRepo.OAuthClients),
			authops.CreateOAuthClientCommand{
				ClientName: name, ClientType: "PUBLIC", RedirectURIs: uris,
				GrantTypes: grants, ApplicationIDs: apps, PortalClientID: portalOwner,
			}, testpg.TestEC())
		require.NoError(t, err)
	}
	loginGrants := []string{"authorization_code", "refresh_token"}
	seed("App A login", []string{appA}, loginGrants, []string{"https://*.app-a.invredir.test/callback"}, nil)
	seed("App B login", []string{appB}, loginGrants, []string{"https://app-b.invredir.test/callback"}, nil)
	seed("App A machine", []string{appA}, []string{"client_credentials"}, []string{"https://machine.invredir.test/cb"}, nil)
	seed("Unlinked login", nil, loginGrants, []string{"https://unlinked.invredir.test/cb"}, nil)
	owner := "clt_ivr_portal"
	seed("Portal login", []string{appA}, loginGrants, []string{"https://portal.invredir.test/cb"}, &owner)

	// An application service account pinned to app A (anchor tier, so the
	// client axis never interferes).
	appScoped := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_invredir_sa", Scope: auth.ScopeAnchor,
		Permissions:  []string{"platform:*:*:*"},
		Applications: []string{appA},
	})
	allApps := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_invredir_admin", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:*:*:*"}, AllApplications: true,
	})

	invite := &fakeInviteEmailer{linkToReturn: "https://fc.invredir.test/auth/set-password?token=t"}
	s := &State{Repo: repo, UoW: uow, Clients: clients, OAuthClients: authRepo.OAuthClients, InviteEmailer: invite}
	yes := true
	create := func(ctx context.Context, email, redirect string) (*apicommon.Out[PrincipalResponse], error) {
		return s.createUser(ctx, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
			Email: email, Name: "Invitee", ClientID: &clientID, ReturnInviteLink: &yes, InviteRedirectURI: &redirect,
		}})
	}

	// Wildcard tenant subdomain of the caller's own app: accepted and carried
	// on the minted invite.
	out, err := create(appScoped, "ok@invredir.test", "https://acme.app-a.invredir.test/callback")
	require.NoError(t, err)
	require.NotNil(t, out.Body.InviteLink)
	require.NotNil(t, invite.lastRedirect)
	assert.Equal(t, "https://acme.app-a.invredir.test/callback", *invite.lastRedirect)

	rejected := []struct{ name, redirect string }{
		{"another application's client", "https://app-b.invredir.test/callback"},
		{"a client_credentials client", "https://machine.invredir.test/cb"},
		{"a client linked to no application", "https://unlinked.invredir.test/cb"},
		{"a portal client", "https://portal.invredir.test/cb"},
		{"an unregistered URL", "https://evil.test/callback"},
		{"a relative path", "/dashboard"},
	}
	for i, tc := range rejected {
		email := "rejected" + string(rune('a'+i)) + "@invredir.test"
		_, err := create(appScoped, email, tc.redirect)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVITE_REDIRECT_URI_INVALID")
		// Rejected before any write.
		p, ferr := repo.FindByEmail(ctx, email)
		require.NoError(t, ferr)
		assert.Nil(t, p, "%s: user must not be created", tc.name)
	}

	// All-applications callers reach every login client, including unlinked
	// ones — but still never a portal client.
	_, err = create(allApps, "admin-b@invredir.test", "https://app-b.invredir.test/callback")
	require.NoError(t, err)
	_, err = create(allApps, "admin-unlinked@invredir.test", "https://unlinked.invredir.test/cb")
	require.NoError(t, err)
	_, err = create(allApps, "admin-portal@invredir.test", "https://portal.invredir.test/cb")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVITE_REDIRECT_URI_INVALID")

	// Blank is simply no redirect.
	_, err = create(appScoped, "blank@invredir.test", "  ")
	require.NoError(t, err)
	assert.Nil(t, invite.lastRedirect)
}

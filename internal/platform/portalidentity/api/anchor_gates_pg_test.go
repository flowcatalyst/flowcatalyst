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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// An anchor-tier caller without the portal-user permission (a read-only staff
// role, an application's service account) used to pass the portal gates and,
// with returnInviteLink, receive a live set-password link for another
// tenant's portal user. It is refused, and nothing is created.
func TestEnsurePortalUser_AnchorNeedsThePermission(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	identities := portalidentity.NewRepository(pool)
	uow := testpg.NewUoW(t)

	ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Anchor Gate Co", Identifier: "anchor-gate-co"}, testpg.TestEC())
	require.NoError(t, err)
	tenant := ev.ClientID

	minter := &fakeInviteMinter{}
	s := &State{
		Identities: identities, Clients: clients,
		OAuthClients: platformauth.NewRepository(pool).OAuthClients, UoW: uow, Invites: minter,
	}
	caller := func(perms ...string) context.Context {
		return auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_anchorgate003", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: perms,
		})
	}
	body := func(email string) *apicommon.In[PortalUserRequest] {
		return &apicommon.In[PortalUserRequest]{Body: PortalUserRequest{ClientID: tenant, Email: email, ReturnInviteLink: true}}
	}

	for _, perms := range [][]string{nil, {"platform:iam:portal-user:view"}, {"platform:admin:client:view"}} {
		_, err = s.ensure(caller(perms...), body("victim@anchor-gate.test"))
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "perms %v", perms)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
		assert.Equal(t, "PERMISSION_REQUIRED", ue.Code)
	}
	got, err := identities.FindByClientAndEmail(ctx, tenant, "victim@anchor-gate.test")
	require.NoError(t, err)
	assert.Nil(t, got, "no identity was created")
	assert.Empty(t, minter.setPasswordInvites)

	out, err := s.ensure(caller("platform:iam:portal-user:manage"), body("victim@anchor-gate.test"))
	require.NoError(t, err)
	require.NotNil(t, out.Body.InviteURL, "the manage permission still gets the link")
	got, err = identities.FindByClientAndEmail(ctx, tenant, "victim@anchor-gate.test")
	require.NoError(t, err)
	assert.NotNil(t, got)
}

//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestCreateUser_InviteRedirectURI pins inviteRedirectUri end to end: the
// application's own URL rides on the minted invite, and a malformed one is
// rejected before the user is written.
func TestCreateUser_InviteRedirectURI(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)

	cev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Invite Redirect Co", Identifier: "INVITE-REDIRECT"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := cev.ClientID

	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_invredir_sa", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:*:*:*"},
	})
	invite := &fakeInviteEmailer{linkToReturn: "https://fc.invredir.test/auth/set-password?token=t"}
	s := &State{Repo: repo, UoW: uow, Clients: clients, InviteEmailer: invite}
	yes := true
	create := func(email, redirect string) (*apicommon.Out[PrincipalResponse], error) {
		return s.createUser(caller, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
			Email: email, Name: "Invitee", ClientID: &clientID, ReturnInviteLink: &yes, InviteRedirectURI: &redirect,
		}})
	}

	// The application's home page — no OAuth registration involved.
	out, err := create("ok@invredir.test", "https://acme.app.invredir.test/")
	require.NoError(t, err)
	require.NotNil(t, out.Body.InviteLink)
	require.NotNil(t, invite.lastRedirect)
	assert.Equal(t, "https://acme.app.invredir.test/", *invite.lastRedirect)

	// Malformed: rejected before any write.
	_, err = create("bad@invredir.test", "javascript:alert(1)")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVITE_REDIRECT_URI_INVALID")
	p, err := repo.FindByEmail(ctx, "bad@invredir.test")
	require.NoError(t, err)
	assert.Nil(t, p, "user must not be created when inviteRedirectUri is rejected")
}

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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestCreateUser_ClientIdentifierResolution pins the createUser handler's
// client-reference handling: `clientId` accepts the client's identifier slug
// (resolved to the clt_ id), the clt_ id itself, and rejects unknown
// references instead of creating a mis-scoped user. Scope derivation itself
// is unit-tested in create_user_test.go.
func TestCreateUser_ClientIdentifierResolution(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	uow := testpg.NewUoW(t)

	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Create User Ref Co", Identifier: "CREATE-USER-REF"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := ev.ClientID

	s := &State{Repo: repo, UoW: uow, Clients: clients, InviteEmailer: noopInviteEmailer{}}
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_createuser_admin", Scope: auth.ScopeAnchor,
	})

	// Identifier slug resolves to the canonical client id; absent scope
	// defaults to CLIENT.
	slug := "CREATE-USER-REF"
	out, err := s.createUser(authCtx, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
		Email: "byslug@createuserref.test", Name: "By Slug", ClientID: &slug,
	}})
	require.NoError(t, err)
	assert.Equal(t, "CLIENT", out.Body.Scope)
	require.NotNil(t, out.Body.ClientID)
	assert.Equal(t, clientID, *out.Body.ClientID)

	// The clt_ id itself still works.
	out, err = s.createUser(authCtx, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
		Email: "byid@createuserref.test", Name: "By ID", ClientID: &clientID,
	}})
	require.NoError(t, err)
	require.NotNil(t, out.Body.ClientID)
	assert.Equal(t, clientID, *out.Body.ClientID)

	// Unknown reference fails closed.
	bogus := "NO-SUCH-CLIENT-REF"
	_, err = s.createUser(authCtx, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
		Email: "unknown@createuserref.test", Name: "Unknown", ClientID: &bogus,
	}})
	require.Error(t, err)

	// Explicit ANCHOR without a registered anchor domain is rejected — the
	// caller must not be able to mint anchors by just asking.
	anchor := "ANCHOR"
	_, err = s.createUser(authCtx, &apicommon.In[CreateUserRequest]{Body: CreateUserRequest{
		Email: "anchor@createuserref.test", Name: "Anchor Wannabe", Scope: &anchor,
	}})
	require.Error(t, err)
}

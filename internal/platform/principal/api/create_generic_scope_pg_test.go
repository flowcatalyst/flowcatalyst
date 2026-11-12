//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// POST /api/principals applies the rules POST /api/principals/users applies
// when the requested scope is not CLIENT: user:manage-access, and the email
// domain must back the scope.
func TestGenericCreate_NonClientScopeFollowsUserRoute(t *testing.T) {
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	authRepo := auth.NewRepository(pool)
	uow := testpg.NewUoW(t)

	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, authops.CreateAnchorDomain(authRepo.AnchorDomains),
		authops.CreateAnchorDomainCommand{Domain: "gencreate-anchor.test"}, testpg.TestEC())
	require.NoError(t, err)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
		clientops.CreateCommand{Name: "Gen Create Co", Identifier: "gen-create-co"}, testpg.TestEC())
	require.NoError(t, err)
	clientID := ev.ClientID

	s := &State{Repo: repo, UoW: uow, Clients: clients, AnchorDomains: authRepo.AnchorDomains, InviteEmailer: noopInviteEmailer{}}
	withoutAccess := anchorHolding("USER", "platform:iam:user:create")
	withAccess := anchorHolding("USER", "platform:iam:user:create", permManageAccess)

	exists := func(email string) bool {
		p, ferr := repo.FindByEmail(context.Background(), email)
		require.NoError(t, ferr)
		return p != nil
	}
	body := func(email, scope string) *CreatePrincipalRequest {
		return &CreatePrincipalRequest{Email: email, Scope: scope, Password: new("Str0ng-Passphrase-gen-create!")}
	}

	// Anchor without manage-access: refused, nothing written.
	_, err = s.create(withoutAccess, inOf(*body("nomanage@gencreate-anchor.test", "ANCHOR")))
	requirePermissionRequired(t, err, "anchor without manage-access")
	assert.False(t, exists("nomanage@gencreate-anchor.test"))

	// With manage-access but a domain that does not back ANCHOR: 400 code of the user route.
	_, err = s.create(withAccess, inOf(*body("wrongdomain@gencreate-other.test", "ANCHOR")))
	require.Error(t, err)
	assert.Equal(t, "ANCHOR_DOMAIN_REQUIRED", usecaseCode(err))
	assert.False(t, exists("wrongdomain@gencreate-other.test"))

	// Caller-chosen idp type cannot stand in for the domain's setup.
	req := body("idp@gencreate-other.test", "ANCHOR")
	req.IDPType = new("OIDC")
	_, err = s.create(withAccess, inOf(*req))
	require.Error(t, err)
	assert.False(t, exists("idp@gencreate-other.test"))

	// PARTNER without a PARTNER mapping: refused.
	_, err = s.create(withAccess, inOf(*body("partner@gencreate-other.test", "PARTNER")))
	require.Error(t, err)
	assert.Equal(t, "PARTNER_DOMAIN_REQUIRED", usecaseCode(err))

	// With manage-access and a registered anchor domain: created.
	out, err := s.create(withAccess, inOf(*body("ok@gencreate-anchor.test", "ANCHOR")))
	require.NoError(t, err)
	assert.NotEmpty(t, out.Body.ID)
	assert.True(t, exists("ok@gencreate-anchor.test"))

	// CLIENT scope is unchanged: user-create alone suffices, any domain.
	c := body("client@gencreate-other.test", "CLIENT")
	c.ClientID = &clientID
	_, err = s.create(withoutAccess, inOf(*c))
	require.NoError(t, err)
	assert.True(t, exists("client@gencreate-other.test"))
}

func inOf(b CreatePrincipalRequest) *apicommon.In[CreatePrincipalRequest] {
	return &apicommon.In[CreatePrincipalRequest]{Body: b}
}

func usecaseCode(err error) string {
	if ue := usecase.AsError(err); ue != nil {
		return ue.Code
	}
	return ""
}

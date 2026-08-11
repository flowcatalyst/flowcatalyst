//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// recordingInviteEmailer counts invites so tests can assert the resend rules.
type recordingInviteEmailer struct{ sent []string }

func (r *recordingInviteEmailer) SendInvite(_ context.Context, p *principal.Principal) error {
	r.sent = append(r.sent, p.UserIdentity.Email)
	return nil
}

// TestEnsurePortalUser pins the Phase-2 ensure/invite contract
// (docs/portal-identity-plan.md): first call creates the inert null-client
// identity and invites; a repeat call is idempotent (created=false) but
// re-sends the invite while the user still can't sign in; an existing regular
// user is returned untouched and uninvited; non-anchor callers are refused.
func TestEnsurePortalUser(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)

	invites := &recordingInviteEmailer{}
	s := &State{Repo: repo, UoW: uow, InviteEmailer: invites}
	anchorCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_portal_admin", Scope: auth.ScopeAnchor,
	})

	// First ensure: creates the inert identity and invites.
	name := "Piet Pompies"
	out, err := s.ensurePortalUser(anchorCtx, &apicommon.In[PortalUserRequest]{
		Body: PortalUserRequest{Email: "Piet@Portal-Ensure.test", Name: &name},
	})
	require.NoError(t, err)
	assert.True(t, out.Body.Created)
	assert.True(t, out.Body.Invited)
	require.NotEmpty(t, out.Body.PrincipalID)

	p, err := repo.FindByEmail(ctx, "piet@portal-ensure.test")
	require.NoError(t, err)
	require.NotNil(t, p, "email must be normalised to lower-case")
	assert.Equal(t, out.Body.PrincipalID, p.ID)
	assert.Equal(t, principal.ScopeClient, p.Scope)
	assert.Nil(t, p.ClientID, "portal identities are null-client")
	assert.False(t, p.AllApplications)
	assert.Empty(t, p.Roles)
	require.NotNil(t, p.UserIdentity)
	assert.Nil(t, p.UserIdentity.PasswordHash)

	// Second ensure: idempotent, and re-sends the invite (user still has no
	// way to sign in — a lost invite is recovered by just calling again).
	out2, err := s.ensurePortalUser(anchorCtx, &apicommon.In[PortalUserRequest]{
		Body: PortalUserRequest{Email: "piet@portal-ensure.test"},
	})
	require.NoError(t, err)
	assert.False(t, out2.Body.Created)
	assert.True(t, out2.Body.Invited)
	assert.Equal(t, out.Body.PrincipalID, out2.Body.PrincipalID)
	assert.Len(t, invites.sent, 2)

	// An existing platform user with a password resolves to the same
	// principal (one human = one principal) but is not re-invited.
	exName := "Existing"
	pw := "correct-horse-battery-staple-9"
	_, err = usecaseop.Run(ctx, uow, operations.CreateUser(repo), operations.CreateCommand{
		Email: "existing@portal-ensure.test", Name: &exName, Scope: "ANCHOR", Password: &pw,
	}, testpg.TestEC())
	require.NoError(t, err)

	out3, err := s.ensurePortalUser(anchorCtx, &apicommon.In[PortalUserRequest]{
		Body: PortalUserRequest{Email: "existing@portal-ensure.test"},
	})
	require.NoError(t, err)
	assert.False(t, out3.Body.Created)
	assert.False(t, out3.Body.Invited, "an account that can sign in is left alone")

	// A client-scoped caller is refused: portal identities belong to no tenant,
	// so the nil-target anchor rule applies.
	clientCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_portal_clientadmin", Scope: auth.ScopeClient,
		Clients: []string{"cli_portal_ens1"},
	})
	_, err = s.ensurePortalUser(clientCtx, &apicommon.In[PortalUserRequest]{
		Body: PortalUserRequest{Email: "someone@portal-ensure.test"},
	})
	require.Error(t, err)
	assert.Len(t, invites.sent, 2, "refused calls send nothing")
}

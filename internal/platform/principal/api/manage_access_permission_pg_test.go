//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Owner ruling 2026-10-25: changing a user's scope or client access needs
// platform:iam:user:manage-access in addition to anchor tier. Pinned end to
// end: the controllers, and the use cases on their own.

const permManageAccess = "platform:iam:user:manage-access"

type accessFixture struct {
	s                *State
	repo             *principal.Repository
	grants           *principal.ClientAccessGrantRepo
	clientA, clientB string
	target           ids.PrincipalID
}

func newAccessFixture(t *testing.T, suffix string) accessFixture {
	t.Helper()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	grants := principal.NewClientAccessGrantRepo(pool)
	uow := testpg.NewUoW(t)
	mk := func(name string) string {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients),
			clientops.CreateCommand{Name: "Access " + name + " " + suffix, Identifier: "access-" + name + "-" + suffix}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	a, b := mk("a"), mk("b")
	u, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(repo),
		principalops.CreateCommand{Email: "access-" + suffix + "@example.com", Scope: "CLIENT", ClientID: &a}, testpg.TestEC())
	require.NoError(t, err)
	return accessFixture{
		s:    &State{Repo: repo, UoW: uow, Clients: clients, GrantRepo: grants},
		repo: repo, grants: grants, clientA: a, clientB: b, target: u.UserID,
	}
}

func (f accessFixture) scopeOf(t *testing.T) (string, string) {
	t.Helper()
	p, err := f.repo.FindByID(context.Background(), f.target)
	require.NoError(t, err)
	require.NotNil(t, p)
	home := ""
	if p.ClientID != nil {
		home = string(*p.ClientID)
	}
	return string(p.Scope), home
}

func (f accessFixture) grantCount(t *testing.T) int {
	t.Helper()
	gs, err := f.grants.FindByPrincipal(context.Background(), f.target)
	require.NoError(t, err)
	return len(gs)
}

// Everything an IAM administrator holds except manage-access: still refused.
func TestAccessChanges_RefusedWithoutManageAccess(t *testing.T) {
	f := newAccessFixture(t, "nomanage")
	caller := anchorHolding("USER",
		"platform:iam:user:view", "platform:iam:user:create", "platform:iam:user:update",
		"platform:iam:user:assign-roles", "platform:iam:client-access:grant", "platform:iam:client-access:revoke")

	_, err := f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: f.clientB, Mode: new("TO_PARTNER")}})
	requirePermissionRequired(t, err, "to partner")
	_, err = f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: "*"}})
	requirePermissionRequired(t, err, "promote to anchor")
	_, err = f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: f.clientB, Mode: new("CHANGE_CLIENT")}})
	requirePermissionRequired(t, err, "change client")
	_, err = f.s.grantClientAccess(caller, &grantClientAccessInput{ID: f.target, Body: GrantClientAccessRequest{ClientID: f.clientB}})
	requirePermissionRequired(t, err, "grant")
	_, err = f.s.revokeClientAccess(caller, &revokeClientAccessInput{ID: f.target, ClientID: ids.ClientID(f.clientB)})
	requirePermissionRequired(t, err, "revoke")

	scope, home := f.scopeOf(t)
	assert.Equal(t, "CLIENT", scope)
	assert.Equal(t, f.clientA, home)
	assert.Zero(t, f.grantCount(t))
}

// A client administrator never passes, whatever it holds: anchor tier is
// required as well.
func TestAccessChanges_ClientAdminNeverPasses(t *testing.T) {
	f := newAccessFixture(t, "clientadm")
	caller := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_access_clientadm", Scope: auth.ScopeClient, Clients: []string{f.clientA, f.clientB},
		PrincipalType: "USER", Permissions: []string{"platform:iam:user:view", "platform:iam:user:update", permManageAccess},
	})
	for name, call := range map[string]func() error{
		"to partner": func() error {
			_, err := f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: f.clientB, Mode: new("TO_PARTNER")}})
			return err
		},
		"promote to anchor": func() error {
			_, err := f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: "*"}})
			return err
		},
		"grant": func() error {
			_, err := f.s.grantClientAccess(caller, &grantClientAccessInput{ID: f.target, Body: GrantClientAccessRequest{ClientID: f.clientB}})
			return err
		},
		"revoke": func() error {
			_, err := f.s.revokeClientAccess(caller, &revokeClientAccessInput{ID: f.target, ClientID: ids.ClientID(f.clientB)})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			testpg.RequireUsecaseError(t, call(), usecase.KindAuthorization, "ANCHOR_REQUIRED")
		})
	}
	scope, _ := f.scopeOf(t)
	assert.Equal(t, "CLIENT", scope)
	assert.Zero(t, f.grantCount(t))
}

// With manage-access (and anchor tier) each change goes through.
func TestAccessChanges_AllowedWithManageAccess(t *testing.T) {
	f := newAccessFixture(t, "manage")
	caller := anchorHolding("USER", "platform:iam:user:view", permManageAccess)

	_, err := f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: f.clientB, Mode: new("TO_PARTNER")}})
	require.NoError(t, err)
	scope, _ := f.scopeOf(t)
	assert.Equal(t, "PARTNER", scope)
	require.Positive(t, f.grantCount(t))

	_, err = f.s.revokeClientAccess(caller, &revokeClientAccessInput{ID: f.target, ClientID: ids.ClientID(f.clientB)})
	require.NoError(t, err)
	_, err = f.s.grantClientAccess(caller, &grantClientAccessInput{ID: f.target, Body: GrantClientAccessRequest{ClientID: f.clientB}})
	require.NoError(t, err)

	_, err = f.s.setClientAssociation(caller, &setClientAssociationInput{ID: f.target, Body: ClientAssociationRequest{ClientID: "*"}})
	require.NoError(t, err)
	scope, _ = f.scopeOf(t)
	assert.Equal(t, "ANCHOR", scope)
}

// The use cases check it themselves (their Authorize is no longer Public), so
// a caller that skips the controller is refused too.
func TestAccessChangeUseCases_AuthorizeManageAccess(t *testing.T) {
	f := newAccessFixture(t, "usecase")
	without := anchorHolding("USER", "platform:iam:user:update", "platform:iam:client-access:grant")
	ec := testpg.TestEC()

	_, err := usecaseop.Run(without, f.s.UoW, principalops.SetClientAssociation(f.repo, f.s.Clients, f.grants),
		principalops.SetClientAssociationCommand{UserID: f.target, ClientID: "*"}, ec)
	requirePermissionRequired(t, err, "set association")
	_, err = usecaseop.Run(without, f.s.UoW, principalops.GrantClientAccess(f.repo, f.s.Clients, f.grants),
		principalops.GrantClientAccessCommand{UserID: f.target, ClientID: f.clientB}, ec)
	requirePermissionRequired(t, err, "grant")
	_, err = usecaseop.Run(without, f.s.UoW, principalops.RevokeClientAccess(f.repo, f.grants),
		principalops.RevokeClientAccessCommand{UserID: f.target, ClientID: f.clientB}, ec)
	requirePermissionRequired(t, err, "revoke")

	scope, _ := f.scopeOf(t)
	assert.Equal(t, "CLIENT", scope)
}

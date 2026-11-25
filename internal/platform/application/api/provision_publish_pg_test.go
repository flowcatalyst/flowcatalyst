//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// provision-service-account takes an optional body: publishCredentials and
// publishBasePath. Without a body it keeps working and follows the default.
func TestProvisionServiceAccount_PublishCredentials(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	roles := role.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ensureApplicationServiceRole(t, roles, uow)
	cfg := credentialpublish.Config{
		Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/app/",
		ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath,
	}
	s := &State{
		Repo: repo, Roles: roles, Principals: principal.NewRepository(pool),
		ServiceAccounts: serviceaccount.NewRepository(pool), OAuthClients: platformauth.NewRepository(pool).OAuthClients,
		UoW: uow, CredentialPublish: cfg,
	}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_provpub01", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	mkApp := func(code string) string {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
			operations.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	pubs := credentialpublish.NewRepository(pool)
	yes := true

	app := mkApp("provpub-yes")
	_, err := s.provisionServiceAccount(caller, &provisionServiceAccountInput{
		ID: app, Body: &ProvisionServiceAccountRequest{PublishCredentials: &yes},
	})
	require.NoError(t, err)
	sa, err := s.ServiceAccounts.FindFirstByApplicationID(ctx, app)
	require.NoError(t, err)
	pub, items, err := pubs.Status(ctx, credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: sa.ID})
	require.NoError(t, err)
	require.NotNil(t, pub)
	assert.Equal(t, "/fc/app/service-accounts/app.provpub-yes/", pub.BasePath)
	assert.Len(t, items, 4)

	app = mkApp("provpub-none")
	_, err = s.provisionServiceAccount(caller, &provisionServiceAccountInput{ID: app})
	require.NoError(t, err)
	sa, err = s.ServiceAccounts.FindFirstByApplicationID(ctx, app)
	require.NoError(t, err)
	pub, items, err = pubs.Status(ctx, credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: sa.ID})
	require.NoError(t, err)
	assert.Nil(t, pub, "a body-less call follows the default, which is off")
	assert.Empty(t, items)

	s.CredentialPublish = credentialpublish.Config{}
	app = mkApp("provpub-unavail")
	_, err = s.provisionServiceAccount(caller, &provisionServiceAccountInput{
		ID: app, Body: &ProvisionServiceAccountRequest{PublishCredentials: &yes},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)
	none, err := s.ServiceAccounts.FindFirstByApplicationID(ctx, app)
	require.NoError(t, err)
	assert.Nil(t, none, "nothing is provisioned")
}

//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	cpapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func publishingState(t *testing.T, cfg credentialpublish.Config) *State {
	t.Helper()
	pool := testpg.Pool(t)
	return &State{
		Repo: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool),
		OAuthClients: platformauth.NewRepository(pool).OAuthClients,
		Clients:      client.NewRepository(pool), Grants: principal.NewClientAccessGrantRepo(pool),
		UoW: testpg.NewUoW(t), CredentialPublish: cfg,
		Publications: credentialpublish.NewRepository(pool), Apps: application.NewRepository(pool),
	}
}

func callerWith(perms ...string) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_cpapi", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true,
	})
}

// The credential-publication endpoints: create with publishCredentials, the
// status read, switch off, switch on later, retry; 404 for an unknown account;
// the account's own view / update permissions and anchor reach gate them.
func TestCredentialPublicationEndpoints(t *testing.T) {
	cfg := credentialpublish.Config{
		Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/api/",
		ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath,
	}
	s := publishingState(t, cfg)
	admin := callerWith("platform:iam:service-account:view", "platform:iam:service-account:create", "platform:iam:service-account:update")
	yes := true

	created, err := s.create(admin, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{
		Code: "cp-api-one", Name: "One", PublishCredentials: &yes,
	}})
	require.NoError(t, err)
	id := created.Body.ServiceAccount.ID

	t.Run("the status shape", func(t *testing.T) {
		got, err := s.getPublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		b := got.Body
		assert.True(t, b.Available)
		assert.True(t, b.Enabled)
		require.NotNil(t, b.Store)
		assert.Equal(t, "aws-ps", *b.Store)
		require.NotNil(t, b.BasePath)
		assert.Equal(t, "/fc/api/service-accounts/cp-api-one/", *b.BasePath)
		require.NotNil(t, b.EnabledBy)
		assert.NotNil(t, b.EnabledAt)
		assert.Nil(t, b.LinkedServiceAccountID)
		require.Len(t, b.Items, 4)
		byCred := map[string]cpapi.ItemResponse{}
		for _, it := range b.Items {
			byCred[it.Credential] = it
		}
		secret := byCred["client-secret"]
		assert.Equal(t, "PENDING_PUT", secret.State)
		assert.Equal(t, "/fc/api/service-accounts/cp-api-one/client-secret", secret.Path)
		assert.Equal(t, "aws-ps:///fc/api/service-accounts/cp-api-one/client-secret", secret.Reference)
		assert.NotNil(t, secret.NextAttemptAt)
		assert.Nil(t, secret.PublishedAt)
	})

	t.Run("unknown account is 404", func(t *testing.T) {
		_, err := s.getPublication(admin, &apicommon.IDInput{ID: "sac_nope"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
		_, err = s.enablePublication(admin, &enablePublicationInput{ID: "sac_nope"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
		_, err = s.retryPublication(admin, &apicommon.IDInput{ID: "sac_nope"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
	})

	t.Run("permissions", func(t *testing.T) {
		viewOnly := callerWith("platform:iam:service-account:view")
		_, err := s.getPublication(viewOnly, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		_, err = s.enablePublication(viewOnly, &enablePublicationInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = s.disablePublication(viewOnly, &apicommon.IDInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = s.retryPublication(viewOnly, &apicommon.IDInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		noPerms := callerWith()
		_, err = s.getPublication(noPerms, &apicommon.IDInput{ID: id})
		require.Error(t, err)
	})

	t.Run("disable, enable again later, retry", func(t *testing.T) {
		_, err := s.disablePublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		got, err := s.getPublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		assert.False(t, got.Body.Enabled)
		assert.True(t, got.Body.Available)
		for _, it := range got.Body.Items {
			assert.Equal(t, "PENDING_DELETE", it.State, "the deletes are still waiting")
		}

		// Enabling is refused while the deletes are waiting.
		_, err = s.enablePublication(admin, &enablePublicationInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, credentialpublish.CodeRemovalPending)

		// A fresh account switched on later, with a custom path.
		plain, err := s.create(admin, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "cp-api-two", Name: "Two"}})
		require.NoError(t, err)
		pid := plain.Body.ServiceAccount.ID
		base := "/fc/api/custom/two/"
		on, err := s.enablePublication(admin, &enablePublicationInput{ID: pid, Body: &cpapi.EnableRequest{BasePath: &base}})
		require.NoError(t, err)
		assert.True(t, on.Body.Enabled)
		assert.Equal(t, base, *on.Body.BasePath)
		states := map[string]string{}
		for _, it := range on.Body.Items {
			states[it.Credential] = it.State
		}
		assert.Equal(t, "NOT_PUBLISHABLE", states["client-secret"])
		assert.Equal(t, "PENDING_PUT", states["auth-token"])

		_, err = s.retryPublication(admin, &apicommon.IDInput{ID: pid})
		require.NoError(t, err)
	})
}

func TestCredentialPublicationNotConfigured(t *testing.T) {
	s := publishingState(t, credentialpublish.Config{})
	admin := callerWith("platform:iam:service-account:view", "platform:iam:service-account:create", "platform:iam:service-account:update")
	yes := true
	_, err := s.create(admin, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{
		Code: "cp-api-unavail", Name: "U", PublishCredentials: &yes,
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)

	made, err := s.create(admin, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "cp-api-none", Name: "N"}})
	require.NoError(t, err)
	got, err := s.getPublication(admin, &apicommon.IDInput{ID: made.Body.ServiceAccount.ID})
	require.NoError(t, err)
	assert.False(t, got.Body.Available)
	assert.False(t, got.Body.Enabled)
	assert.Empty(t, got.Body.Items)
	_, err = s.enablePublication(admin, &enablePublicationInput{ID: made.Body.ServiceAccount.ID})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeUnavailable)
}

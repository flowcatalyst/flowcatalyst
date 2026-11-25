//go:build integration

package api

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	cpapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

func caller(perms ...string) context.Context {
	return testpg.WithAuth(context.Background(), &platformauth.AuthContext{
		PrincipalID: "prn_ocapi", Scope: platformauth.ScopeAnchor, Permissions: perms, AllApplications: true,
	})
}

func TestOAuthClientCredentialPublicationEndpoints(t *testing.T) {
	cfg := credentialpublish.Config{
		Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/api/",
		ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath,
	}
	pool := testpg.Pool(t)
	s := &State{
		Repo: auth.NewRepository(pool), UoW: testpg.NewUoW(t),
		CredentialPublish: cfg, Publications: credentialpublish.NewRepository(pool),
	}
	admin := caller("platform:auth:oauth-client:view", "platform:auth:oauth-client:create", "platform:auth:oauth-client:update")
	yes := true

	made, err := s.createOAuthClient(admin, &apicommon.In[CreateOAuthClientRequest]{Body: CreateOAuthClientRequest{
		ClientName: "cp api", ClientType: "CONFIDENTIAL", PublishCredentials: &yes,
	}})
	require.NoError(t, err)
	require.NotEmpty(t, made.Body.ClientSecret)
	id := made.Body.Client.ID

	t.Run("status shape", func(t *testing.T) {
		got, err := s.getPublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		b := got.Body
		assert.True(t, b.Available)
		assert.True(t, b.Enabled)
		assert.Equal(t, "/fc/api/oauth-clients/"+made.Body.Client.ClientID+"/", *b.BasePath)
		assert.Nil(t, b.LinkedServiceAccountID)
		require.Len(t, b.Items, 2)
		assert.Equal(t, "client-id", b.Items[0].Credential)
		assert.Equal(t, "PENDING_PUT", b.Items[0].State)
		assert.Contains(t, b.Items[0].Reference, "aws-ps://")
	})

	t.Run("404 and permissions", func(t *testing.T) {
		_, err := s.getPublication(admin, &apicommon.IDInput{ID: "oac_nope"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
		_, err = s.retryPublication(admin, &apicommon.IDInput{ID: "oac_nope"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "OAuthClient_NOT_FOUND")
		viewOnly := caller("platform:auth:oauth-client:view")
		_, err = s.getPublication(viewOnly, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		_, err = s.enablePublication(viewOnly, &enablePublicationInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = s.disablePublication(viewOnly, &apicommon.IDInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_, err = s.retryPublication(viewOnly, &apicommon.IDInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		nonAnchor := testpg.WithAuth(context.Background(), &platformauth.AuthContext{
			PrincipalID: "prn_x", Scope: platformauth.ScopeClient, Permissions: []string{"platform:auth:oauth-client:view"},
		})
		_, err = s.getPublication(nonAnchor, &apicommon.IDInput{ID: id})
		require.Error(t, err)
	})

	t.Run("disable then retry then refuse enable while removing", func(t *testing.T) {
		_, err := s.disablePublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		got, err := s.getPublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		assert.False(t, got.Body.Enabled)
		_, err = s.retryPublication(admin, &apicommon.IDInput{ID: id})
		require.NoError(t, err)
		_, err = s.enablePublication(admin, &enablePublicationInput{ID: id})
		testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, credentialpublish.CodeRemovalPending)
	})

	t.Run("enable later on a client that did not publish", func(t *testing.T) {
		plain, err := s.createOAuthClient(admin, &apicommon.In[CreateOAuthClientRequest]{Body: CreateOAuthClientRequest{
			ClientName: "cp api plain", ClientType: "CONFIDENTIAL",
		}})
		require.NoError(t, err)
		base := "/fc/api/mine/"
		got, err := s.enablePublication(admin, &enablePublicationInput{ID: plain.Body.Client.ID, Body: &cpapi.EnableRequest{BasePath: &base}})
		require.NoError(t, err)
		assert.True(t, got.Body.Enabled)
		assert.Equal(t, base, *got.Body.BasePath)
	})

	t.Run("a public client is not offered it", func(t *testing.T) {
		pub, err := s.createOAuthClient(admin, &apicommon.In[CreateOAuthClientRequest]{Body: CreateOAuthClientRequest{
			ClientName: "cp api public", ClientType: "PUBLIC",
		}})
		require.NoError(t, err)
		_, err = s.enablePublication(admin, &enablePublicationInput{ID: pub.Body.Client.ID})
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeNotConfidential)
		_, err = s.createOAuthClient(admin, &apicommon.In[CreateOAuthClientRequest]{Body: CreateOAuthClientRequest{
			ClientName: "cp api public2", ClientType: "PUBLIC", PublishCredentials: &yes,
		}})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, credentialpublish.CodeNotConfidential)
	})
}

func TestOAuthClientLinkedToAccountReportsTheAccount(t *testing.T) {
	pool := testpg.Pool(t)
	cfg := credentialpublish.Config{Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/api/"}
	s := &State{Repo: auth.NewRepository(pool), UoW: testpg.NewUoW(t), CredentialPublish: cfg, Publications: credentialpublish.NewRepository(pool)}
	admin := caller("platform:auth:oauth-client:view", "platform:auth:oauth-client:update")

	// A client wired to a service account's principal, as account creation makes one.
	_, err := pool.Exec(context.Background(),
		`INSERT INTO iam_service_accounts (id, code, name, active, created_at, updated_at) VALUES ('sac_cplink','cp-link','x',true,now(),now())`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, service_account_id, active, created_at, updated_at)
		 VALUES ('prn_cplink','SERVICE','ANCHOR','x','sac_cplink',true,now(),now())`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO oauth_clients (id, client_id, client_name, client_type, service_account_principal_id, active, created_at, updated_at)
		 VALUES ('oac_cplink','cplink','x','CONFIDENTIAL','prn_cplink',true,now(),now())`)
	require.NoError(t, err)

	got, err := s.getPublication(admin, &apicommon.IDInput{ID: "oac_cplink"})
	require.NoError(t, err)
	require.NotNil(t, got.Body.LinkedServiceAccountID)
	assert.Equal(t, "sac_cplink", *got.Body.LinkedServiceAccountID)
	assert.False(t, got.Body.Enabled)

	_, err = s.enablePublication(admin, &enablePublicationInput{ID: "oac_cplink"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, credentialpublish.CodeLinked)
}

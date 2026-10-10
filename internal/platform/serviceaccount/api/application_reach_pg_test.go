//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// An application-scoped caller (no all-applications access, as an
// application's own service account) manages only service accounts bound to
// an application it can access: never a platform-wide one or another
// application's, even at anchor tier with the permissions. A caller with
// all-applications access is not confined.
func TestServiceAccountWrites_ApplicationScopedCallerConfined(t *testing.T) {
	pool := testpg.Pool(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauthRepo := platformauth.NewRepository(pool).OAuthClients
	clients := client.NewRepository(pool)
	grants := principal.NewClientAccessGrantRepo(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: saRepo, Principals: principals, OAuthClients: oauthRepo, Clients: clients, Grants: grants, UoW: uow}

	const ownApp, otherApp = "app_sareachown01", "app_sareachoth01"
	mk := func(code string, applicationID *string) string {
		t.Helper()
		res, err := usecaseop.RunTx(testpg.AnchorCtx(), uow,
			operations.CreateServiceAccountWithCredentials(saRepo, principals, oauthRepo, clients, grants),
			operations.CreateCommand{Code: code, Name: code, ApplicationID: applicationID}, testpg.TestEC())
		require.NoError(t, err)
		return res.ServiceAccount.ID
	}
	own, other := ownApp, otherApp
	ownSA := mk("sa-reach-own", &own)
	otherSA := mk("sa-reach-other", &other)
	platformSA := mk("sa-reach-platform", nil)

	confined := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_sareachapp01", Scope: auth.ScopeAnchor,
		Applications: []string{ownApp},
		Permissions: []string{
			"platform:iam:service-account:create",
			"platform:iam:service-account:update",
		},
	})
	refused := func(t *testing.T, err error) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "expected a refusal, got %v", err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
		assert.Equal(t, "APPLICATION_ACCESS_REQUIRED", ue.Code)
	}

	for _, id := range []string{otherSA, platformSA} {
		in := &apicommon.IDInput{ID: id}
		_, err := s.deactivate(confined, in)
		refused(t, err)
		_, err = s.regenerateSigningSecret(confined, in)
		refused(t, err)
		_, err = s.regenerateAuthToken(confined, in)
		refused(t, err)
		_, err = s.mintToken(confined, in)
		refused(t, err)
		_, err = s.assignRoles(confined, &assignRolesInput{ID: id})
		refused(t, err)
		_, err = s.update(confined, &updateInput{ID: id})
		refused(t, err)
	}
	// Creating a platform-wide account or another application's is refused.
	_, err := s.create(confined, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "sa-reach-new", Name: "N"}})
	refused(t, err)
	_, err = s.create(confined, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "sa-reach-new", Name: "N", ApplicationID: &other}})
	refused(t, err)

	// Its own application's account: managed.
	_, err = s.regenerateSigningSecret(confined, &apicommon.IDInput{ID: ownSA})
	require.NoError(t, err)
	_, err = s.deactivate(confined, &apicommon.IDInput{ID: ownSA})
	require.NoError(t, err)
	_, err = s.create(confined, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "sa-reach-new", Name: "N", ApplicationID: &own}})
	require.NoError(t, err)

	// All-applications access is not confined.
	admin := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_sareachadm01", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:iam:service-account:update"},
	})
	_, err = s.deactivate(admin, &apicommon.IDInput{ID: otherSA})
	require.NoError(t, err)
	_, err = s.deactivate(admin, &apicommon.IDInput{ID: platformSA})
	require.NoError(t, err)
}

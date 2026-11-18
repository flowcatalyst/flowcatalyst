//go:build integration

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// GET /api/applications/{id}/service-account answers the account attached to
// the application as GET /api/service-accounts/{id} does: the account's own
// id (not the SERVICE principal id the application stores), its code and
// name, the principal id and the OAuth client_id, and no secret. It needs read
// access to the application and service-account:view; an application without
// an account, and an account the caller may not view, answer 404.
func TestGetApplicationServiceAccount(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := application.NewRepository(pool)
	roles := role.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ensureApplicationServiceRole(t, roles, uow)
	s := &State{
		Repo: repo, Roles: roles, Principals: principal.NewRepository(pool),
		ServiceAccounts: serviceaccount.NewRepository(pool),
		OAuthClients:    platformauth.NewRepository(pool).OAuthClients, UoW: uow,
	}
	sfx := strings.ToLower(tsid.GenerateUntyped())[6:]
	mkApp := func(code string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.CreateApplication(repo),
			operations.CreateCommand{Code: code + sfx, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	withSA, without := mkApp("getsa-with"), mkApp("getsa-none")
	prov, err := usecaseop.RunTx(ctx, uow,
		operations.ProvisionServiceAccount(repo, s.ServiceAccounts, s.Principals, s.OAuthClients),
		operations.ProvisionServiceAccountCommand{ApplicationID: withSA}, usecase.NewExecutionContext("prn_bootstrap001"))
	require.NoError(t, err)

	caller := func(ac auth.AuthContext) context.Context {
		if ac.PrincipalID == "" {
			ac.PrincipalID = "prn_getsa0000001"
		}
		return auth.WithContext(ctx, &ac)
	}
	admin := caller(auth.AuthContext{
		Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:admin:application:view", "platform:iam:service-account:view"},
	})
	failsWith := func(t *testing.T, err error, kind usecase.Kind, code string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "want %s, got %v", code, err)
		assert.Equal(t, kind, ue.Kind)
		assert.Equal(t, code, ue.Code)
	}

	t.Run("answers the attached account", func(t *testing.T) {
		out, err := s.getServiceAccount(admin, &apicommon.IDInput{ID: withSA})
		require.NoError(t, err)
		sa := out.Body
		assert.Equal(t, prov.ServiceAccountID, sa.ID, "the account's own sac_ id, not the principal id")
		assert.Equal(t, prov.ServiceAccountCode, sa.Code)
		assert.Equal(t, prov.ServiceAccountName, sa.Name)
		require.NotNil(t, sa.PrincipalID)
		assert.Equal(t, prov.ServicePrincipalID, *sa.PrincipalID)
		require.NotNil(t, sa.OAuthClientID)
		assert.Equal(t, prov.OAuthClientID, *sa.OAuthClientID)

		app, err := repo.FindByID(ctx, withSA)
		require.NoError(t, err)
		assert.Equal(t, prov.ServicePrincipalID, *app.ServiceAccountID, "the application stores the principal id")

		raw, err := json.Marshal(sa)
		require.NoError(t, err)
		for _, secret := range []string{prov.OAuthClientSecret, prov.AuthToken, prov.SigningSecret} {
			require.NotEmpty(t, secret)
			assert.NotContains(t, string(raw), secret, "no secret in the answer")
		}
	})

	t.Run("application without an account is 404", func(t *testing.T) {
		_, err := s.getServiceAccount(admin, &apicommon.IDInput{ID: without})
		failsWith(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
	})

	t.Run("unknown application is 404", func(t *testing.T) {
		_, err := s.getServiceAccount(admin, &apicommon.IDInput{ID: tsid.Generate(tsid.Application)})
		failsWith(t, err, usecase.KindNotFound, "Application_NOT_FOUND")
	})

	t.Run("needs service-account:view", func(t *testing.T) {
		_, err := s.getServiceAccount(caller(auth.AuthContext{
			Scope: auth.ScopeAnchor, AllApplications: true,
			Permissions: []string{"platform:admin:application:view"},
		}), &apicommon.IDInput{ID: withSA})
		failsWith(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	})

	t.Run("needs read access to the application", func(t *testing.T) {
		_, err := s.getServiceAccount(caller(auth.AuthContext{
			Scope: auth.ScopeAnchor, AllApplications: true,
			Permissions: []string{"platform:iam:service-account:view"},
		}), &apicommon.IDInput{ID: withSA})
		failsWith(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

		// An application service account confined to another application.
		_, err = s.getServiceAccount(caller(auth.AuthContext{
			Scope:        auth.ScopeAnchor,
			Permissions:  []string{appSvcApplicationView, "platform:iam:service-account:view"},
			Applications: []string{without},
		}), &apicommon.IDInput{ID: withSA})
		failsWith(t, err, usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	})

	t.Run("an account outside the caller's reach is 404", func(t *testing.T) {
		// A client-scoped administrator reads the application but cannot view
		// an anchor account (no client links).
		_, err := s.getServiceAccount(caller(auth.AuthContext{
			Scope: auth.ScopeClient, Clients: []string{"clt_getsa000001"},
			AllApplications: true,
			Permissions:     []string{"platform:admin:application:view", "platform:iam:service-account:view"},
		}),
			&apicommon.IDInput{ID: withSA})
		failsWith(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
	})
}

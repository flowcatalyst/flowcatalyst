//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const appSvcApplicationView = "platform:application-service:application:view"

// F21: the application reads an application service account is admitted to
// (application-service application:view) stay confined to its own application:
// the application itself, its roles and its client configs.
func TestApplicationReads_ConfinedToBoundApplication(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	ownID, otherID := tsid.Generate(tsid.Application), tsid.Generate(tsid.Application)
	own, other := "own"+sfx, "oth"+sfx
	for _, a := range [][2]string{{ownID, own}, {otherID, other}} {
		_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, application_id, name, display_name, application_code, source)
			VALUES ($1, $2, $3, 'R', $4, 'SDK')`, "rol_f21a"+a[0][len(a[0])-6:], a[0], a[1]+":clerk", a[1])
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO app_client_configs (id, application_id, client_id) VALUES ($1, $2, $3)`,
			tsid.Generate(tsid.AppClientConfig), a[0], "clt_f21"+sfx[:7])
		require.NoError(t, err)
	}
	s := &State{
		Repo: application.NewRepository(pool), ClientConfigRepo: application.NewClientConfigRepo(pool),
		Roles: role.NewRepository(pool), OAuthClients: platformauth.NewRepository(pool).OAuthClients,
	}
	clientID := "clt_f21" + sfx[:7]
	with := func(ac *auth.AuthContext) context.Context { return auth.WithContext(ctx, ac) }
	svc := with(&auth.AuthContext{
		PrincipalID: "prn_svc", Scope: auth.ScopeAnchor,
		Permissions: []string{appSvcApplicationView}, Applications: []string{ownID},
	})
	forbidden := func(label string, err error) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, label)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
	}

	t.Run("own application", func(t *testing.T) {
		l, err := s.list(svc, &listInput{})
		require.NoError(t, err)
		var codes []string
		for _, a := range l.Body.Applications {
			codes = append(codes, a.Code)
		}
		assert.Equal(t, []string{own}, codes, "the list is only the bound application")
		_, err = s.getByID(svc, &apicommon.IDInput{ID: ownID})
		require.NoError(t, err)
		_, err = s.getByCode(svc, &getByCodeInput{Code: own})
		require.NoError(t, err)
		r, err := s.listApplicationRoles(svc, &apicommon.IDInput{ID: ownID})
		require.NoError(t, err)
		assert.Equal(t, []string{own + ":clerk"}, r.Body.Roles)
		c, err := s.listClientConfigs(svc, &apicommon.IDInput{ID: ownID})
		require.NoError(t, err)
		assert.Len(t, c.Body.Items, 1)
		_, err = s.getClientConfig(svc, &singleConfigInput{ID: ids.ApplicationID(ownID), ClientID: clientID})
		require.NoError(t, err)
	})

	t.Run("another application is refused on every read", func(t *testing.T) {
		_, err := s.getByID(svc, &apicommon.IDInput{ID: otherID})
		forbidden("get", err)
		_, err = s.getByCode(svc, &getByCodeInput{Code: other})
		forbidden("by-code", err)
		_, err = s.listApplicationRoles(svc, &apicommon.IDInput{ID: otherID})
		forbidden("roles", err)
		_, err = s.listClientConfigs(svc, &apicommon.IDInput{ID: otherID})
		forbidden("clients", err)
		_, err = s.getClientConfig(svc, &singleConfigInput{ID: ids.ApplicationID(otherID), ClientID: clientID})
		forbidden("client config", err)
	})

	t.Run("unbound and dangling credentials are refused, not answered empty", func(t *testing.T) {
		for label, apps := range map[string][]string{"unbound": nil, "dangling": {"app_doesnotexist"}} {
			c := with(&auth.AuthContext{
				PrincipalID: "prn_x", Scope: auth.ScopeAnchor,
				Permissions: []string{appSvcApplicationView}, Applications: apps,
			})
			l, err := s.list(c, &listInput{})
			require.NoError(t, err)
			assert.Empty(t, l.Body.Applications, label+": lists nothing")
			_, err = s.getByID(c, &apicommon.IDInput{ID: ownID})
			forbidden(label+" get", err)
			_, err = s.listApplicationRoles(c, &apicommon.IDInput{ID: ownID})
			forbidden(label+" roles", err)
			_, err = s.listClientConfigs(c, &apicommon.IDInput{ID: ownID})
			forbidden(label+" clients", err)
		}
	})

	t.Run("the admin permission and all-applications callers are not narrowed", func(t *testing.T) {
		admin := with(&auth.AuthContext{
			PrincipalID: "prn_adm", Scope: auth.ScopeAnchor, AllApplications: true,
			Permissions: []string{"platform:admin:application:view"},
		})
		r, err := s.listApplicationRoles(admin, &apicommon.IDInput{ID: otherID})
		require.NoError(t, err)
		assert.Equal(t, []string{other + ":clerk"}, r.Body.Roles)
		_, err = s.listClientConfigs(admin, &apicommon.IDInput{ID: otherID})
		require.NoError(t, err)
		// Application-bound but holding the admin permission: unchanged (console).
		bound := with(&auth.AuthContext{
			PrincipalID: "prn_adm2", Scope: auth.ScopeAnchor, Applications: []string{ownID},
			Permissions: []string{"platform:admin:application:view"},
		})
		_, err = s.getByID(bound, &apicommon.IDInput{ID: otherID})
		require.NoError(t, err)
	})

	t.Run("no permission, no read", func(t *testing.T) {
		c := with(&auth.AuthContext{
			PrincipalID: "prn_n", Scope: auth.ScopeAnchor, Applications: []string{ownID},
			Permissions: []string{"platform:application-service:role:view"},
		})
		_, err := s.listApplicationRoles(c, &apicommon.IDInput{ID: ownID})
		forbidden("roles", err)
	})
}

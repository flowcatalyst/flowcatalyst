//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// The role ceiling (F8) on POST /api/applications/{appCode}/principals/sync:
// the application's own roles sync as before, but the route cannot hand out a
// role holding platform permissions the caller lacks. The route checked only
// the caller's application access, never which roles it named, so a caller
// with a user permission and the application could sync anyone a platform
// role.
func TestSyncPrincipals_AppRouteBoundedByCallersPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	principals := principal.NewRepository(pool)

	const appCode = "ceilsdk"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Ceiling SDK"}, testpg.TestEC())
	require.NoError(t, err)
	mkRole := func(name string, perms ...string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: appCode, RoleName: name, DisplayName: name, Permissions: perms},
			testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	clerk := mkRole("clerk", appCode+":order:view", appCode+":order:create")
	platformish := mkRole("platformish", "platform:iam:user:create")

	s := &State{Apps: application.NewRepository(pool), Principals: principals, Roles: roles, UoW: uow}
	syncer := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_ceilsdk_sync1", Scope: auth.ScopeAnchor,
		Applications: []string{app.ApplicationID},
		Permissions:  []string{"platform:iam:user:update"},
	})
	sync := func(email string, rs ...string) error {
		_, err := s.syncPrincipals(syncer, &syncPrincipalsInput{AppCode: appCode, Body: syncPrincipalsRequest{
			Principals: []syncPrincipalInputRequest{{Email: email, Name: email, Roles: rs}},
		}})
		return err
	}

	// The application's own roles: synced.
	require.NoError(t, sync("clerk@ceilsdk.test", clerk))
	p, err := principals.FindByEmail(ctx, "clerk@ceilsdk.test")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, []string{clerk}, roleNamesOfAssignments(p))

	// A role holding a platform permission the caller lacks: refused, nothing written.
	err = sync("escalate@ceilsdk.test", platformish)
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a refusal, got %v", err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, "ROLE_ABOVE_CALLER", ue.Code)
	assert.Contains(t, ue.Message, platformish)
	none, err := principals.FindByEmail(ctx, "escalate@ceilsdk.test")
	require.NoError(t, err)
	assert.Nil(t, none)
}

func roleNamesOfAssignments(p *principal.Principal) []string {
	out := []string{}
	for _, ra := range p.Roles {
		out = append(out, ra.Role)
	}
	return out
}

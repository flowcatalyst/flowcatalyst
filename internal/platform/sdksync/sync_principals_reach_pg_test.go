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
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// seedSyncUser inserts a USER principal with the given scope and home client
// ("" for none), named "before", holding roleName (if any) as SDK_SYNC.
func seedSyncUser(t *testing.T, id, email, scope, clientID, roleName string) {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	var client *string
	if clientID != "" {
		client = &clientID
	}
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', $2, $3, 'before', TRUE, $4)`, id, scope, client, email)
	require.NoError(t, err)
	if roleName != "" {
		_, err = pool.Exec(ctx,
			`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
			 VALUES ($1, $2, 'SDK_SYNC')`, id, roleName)
		require.NoError(t, err)
	}
}

// Target reach (F12) on POST /api/applications/{appCode}/principals/sync: a
// listed user the caller could not administer refuses the whole sync, 403
// SYNC_TARGET_FORBIDDEN; the removeUnlisted sweep skips such users instead (it
// ranges over every user, so refusing would make any non-anchor sweep
// impossible). Java and Rust do the same. Before the fix a caller with the
// application could re-role, rename or deactivate any user by email.
func TestSyncPrincipals_AppRouteTargetsWithinCallersReach(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	principals := principal.NewRepository(pool)

	const appCode = "reachsdk"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Reach SDK"}, testpg.TestEC())
	require.NoError(t, err)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{
			ApplicationCode: appCode, RoleName: "clerk", DisplayName: "Clerk",
			Permissions: []string{appCode + ":order:view"},
		}, testpg.TestEC())
	require.NoError(t, err)
	clerk := ev.Name

	const mine, theirs = "cli_reachsdk0001", "cli_reachsdk0002"
	seedSyncUser(t, "prn_reachsdk_own1", "own@reachsdk.test", "CLIENT", mine, "")
	seedSyncUser(t, "prn_reachsdk_oth1", "other@reachsdk.test", "CLIENT", theirs, "")
	seedSyncUser(t, "prn_reachsdk_anc1", "anchor@reachsdk.test", "ANCHOR", "", "")
	seedSyncUser(t, "prn_reachsdk_nul1", "clientless@reachsdk.test", "CLIENT", "", "")
	// Unlisted users holding the application's synced role, for the sweep.
	seedSyncUser(t, "prn_reachsdk_own2", "own-unlisted@reachsdk.test", "CLIENT", mine, clerk)
	seedSyncUser(t, "prn_reachsdk_oth2", "other-unlisted@reachsdk.test", "CLIENT", theirs, clerk)

	s := &State{Apps: application.NewRepository(pool), Principals: principals, Roles: roles, UoW: uow}
	clientCaller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_reachsdk_cal1", Scope: auth.ScopeClient, Clients: []string{mine},
		Applications: []string{app.ApplicationID},
		Permissions:  []string{"platform:iam:user:update"},
	})
	sync := func(c context.Context, removeUnlisted bool, emails ...string) error {
		in := make([]syncPrincipalInputRequest, 0, len(emails))
		for _, e := range emails {
			in = append(in, syncPrincipalInputRequest{Email: e, Name: "after", Roles: []string{clerk}})
		}
		_, err := s.syncPrincipals(c, &syncPrincipalsInput{
			AppCode: appCode, RemoveUnlisted: removeUnlisted,
			Body: syncPrincipalsRequest{Principals: in},
		})
		return err
	}
	user := func(email string) *principal.Principal {
		t.Helper()
		p, err := principals.FindByEmail(ctx, email)
		require.NoError(t, err)
		require.NotNil(t, p, email)
		return p
	}
	forbidden := func(label string, err error, email string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
		assert.Equal(t, "SYNC_TARGET_FORBIDDEN", ue.Code, label)
		assert.Equal(t, "Not authorised to manage principal '"+email+"'", ue.Message, label)
	}

	for _, email := range []string{"other@reachsdk.test", "anchor@reachsdk.test", "clientless@reachsdk.test"} {
		forbidden(email, sync(clientCaller, false, "own@reachsdk.test", email), email)
		assert.Equal(t, "before", user(email).Name)
		assert.Empty(t, roleNamesOfAssignments(user(email)))
	}
	assert.Equal(t, "before", user("own@reachsdk.test").Name, "a refused sync writes nothing")

	// In reach: synced; the sweep strips the unlisted user in reach and skips
	// the one out of reach.
	require.NoError(t, sync(clientCaller, true, "own@reachsdk.test"))
	assert.Equal(t, "after", user("own@reachsdk.test").Name)
	assert.Equal(t, []string{clerk}, roleNamesOfAssignments(user("own@reachsdk.test")))
	assert.Empty(t, roleNamesOfAssignments(user("own-unlisted@reachsdk.test")), "in reach: stripped")
	assert.Equal(t, []string{clerk}, roleNamesOfAssignments(user("other-unlisted@reachsdk.test")),
		"out of reach: skipped")

	// An anchor-tier caller (an application's service account given a user
	// permission) reaches everyone, as before.
	sa := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_reachsdk_sa01", Scope: auth.ScopeAnchor,
		Applications: []string{app.ApplicationID},
		Permissions:  []string{"platform:iam:user:update"},
	})
	require.NoError(t, sync(sa, false, "other@reachsdk.test", "anchor@reachsdk.test", "clientless@reachsdk.test"))
	assert.Equal(t, "after", user("anchor@reachsdk.test").Name)
}

package role

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func fixedRoles(defs map[string][]string) PermissionsOf {
	return func(_ context.Context, name string) ([]string, error) {
		return defs[name], nil
	}
}

var ceilingRoles = fixedRoles(map[string][]string{
	"platform:super-admin": {"platform:*:*:*"},
	"platform:iam-admin":   {"platform:iam:user:view", "platform:iam:user:assign-roles", "platform:iam:role:view"},
	"platform:viewer":      {"platform:iam:user:view"},
	"platform:messaging":   {"platform:messaging:*:*"},
	"orders:clerk":         {"orders:order:create", "orders:order:view"},
	"orders:mixed":         {"orders:order:view", "platform:iam:user:view"},
})

func caller(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_ceiling", Scope: auth.ScopeAnchor, Permissions: perms}
}

func TestRoleCeiling_CallerLackingAPermissionCannotGrantARoleHoldingIt(t *testing.T) {
	ac := caller("platform:iam:user:view", "platform:iam:user:assign-roles")
	err := RequireRolesWithinCaller(context.Background(), ac, []string{"platform:super-admin"}, ceilingRoles)
	require.Error(t, err)
	var ue *usecase.Error
	require.True(t, errors.As(err, &ue))
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, CeilingCode, ue.Code)
	assert.Contains(t, ue.Message, "platform:super-admin")

	// One permission short of the role is enough to refuse it.
	err = RequireRolesWithinCaller(context.Background(), ac, []string{"platform:iam-admin"}, ceilingRoles)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "platform:iam-admin")
}

func TestRoleCeiling_SubsetCallerCanGrant(t *testing.T) {
	ac := caller("platform:iam:user:view", "platform:iam:user:assign-roles", "platform:iam:role:view")
	require.NoError(t, RequireRolesWithinCaller(context.Background(), ac,
		[]string{"platform:iam-admin", "platform:viewer"}, ceilingRoles))
}

func TestRoleCeiling_WildcardCallerCanGrant(t *testing.T) {
	// The super-admin wildcard may assign anything, super-admin included.
	require.NoError(t, RequireRolesWithinCaller(context.Background(), caller("platform:*:*:*"),
		[]string{"platform:super-admin", "platform:iam-admin", "platform:messaging"}, ceilingRoles))

	// A narrower held wildcard covers what it names, and a role's own
	// wildcard only when the held one is at least as wide.
	ac := caller("platform:iam:*:*")
	require.NoError(t, RequireRolesWithinCaller(context.Background(), ac,
		[]string{"platform:iam-admin", "platform:viewer"}, ceilingRoles))
	require.Error(t, RequireRolesWithinCaller(context.Background(), ac,
		[]string{"platform:messaging"}, ceilingRoles))
	require.NoError(t, RequireRolesWithinCaller(context.Background(), caller("platform:messaging:*:*"),
		[]string{"platform:messaging"}, ceilingRoles))
}

func TestRoleCeiling_ApplicationPermissionsAreOutsideTheCeiling(t *testing.T) {
	// No caller outside the application holds orders:*, so only the role's
	// platform permissions count.
	ac := caller("platform:iam:user:assign-roles")
	require.NoError(t, RequireRolesWithinCaller(context.Background(), ac, []string{"orders:clerk"}, ceilingRoles))
	require.Error(t, RequireRolesWithinCaller(context.Background(), ac, []string{"orders:mixed"}, ceilingRoles))
}

func TestRoleCeiling_UnknownRoleAndNilCaller(t *testing.T) {
	require.NoError(t, RequireRolesWithinCaller(context.Background(), caller(), []string{"no:such-role"}, ceilingRoles))
	require.Error(t, RequireRolesWithinCaller(context.Background(), nil, []string{"platform:viewer"}, ceilingRoles))
	require.NoError(t, RequireRolesWithinCaller(context.Background(), nil, nil, ceilingRoles))
}

func TestRoleCeiling_LookupFailureIsInternal(t *testing.T) {
	failing := func(context.Context, string) ([]string, error) { return nil, errors.New("db down") }
	err := RequireRolesWithinCaller(context.Background(), caller(), []string{"x"}, failing)
	var ue *usecase.Error
	require.True(t, errors.As(err, &ue))
	assert.Equal(t, usecase.KindInternal, ue.Kind)
}

func TestChangedRoles(t *testing.T) {
	assert.Equal(t, []string{"c", "a"}, ChangedRoles([]string{"a", "b"}, []string{"b", "c", "c"}))
	assert.Empty(t, ChangedRoles([]string{"a"}, []string{"a"}))
}

// The permission ceiling (F10): the wildcard covers everything, a caller
// lacking a platform permission cannot add or remove it, held wildcards cover
// what they name, application permissions are outside it, and a nil caller
// holds nothing.
func TestPermissionCeiling(t *testing.T) {
	require.NoError(t, RequirePermissionsWithinCaller(caller("platform:*:*:*"),
		[]string{"platform:*:*:*", "platform:iam:user:create"}))

	err := RequirePermissionsWithinCaller(caller("platform:iam:role:update"),
		[]string{"platform:iam:role:update", "platform:*:*:*", "platform:iam:user:view", "platform:*:*:*"})
	var ue *usecase.Error
	require.True(t, errors.As(err, &ue))
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, PermissionCeilingCode, ue.Code)
	assert.Equal(t, "You may only add or remove permissions you hold yourself: platform:*:*:*, platform:iam:user:view", ue.Message)

	require.NoError(t, RequirePermissionsWithinCaller(caller("platform:iam:*:*"), []string{"platform:iam:user:create"}))
	require.Error(t, RequirePermissionsWithinCaller(caller("platform:iam:user:*"), []string{"platform:iam:*:*"}))
	require.NoError(t, RequirePermissionsWithinCaller(caller(), []string{"orders:order:create"}))
	require.Error(t, RequirePermissionsWithinCaller(nil, []string{"platform:iam:user:view"}))
	require.NoError(t, RequirePermissionsWithinCaller(nil, nil))
}

func TestChangedPermissions(t *testing.T) {
	assert.Equal(t, []string{"p:c", "p:a"}, ChangedPermissions([]string{"p:a", "p:b"}, []string{"p:b", "p:c"}))
	assert.Empty(t, ChangedPermissions([]string{"p:a", "p:b"}, []string{"p:b", "p:a"}))
}

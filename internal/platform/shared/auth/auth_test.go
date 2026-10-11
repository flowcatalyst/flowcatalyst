package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermissionMatches(t *testing.T) {
	const required = "platform:messaging:event-type:view"
	cases := []struct {
		held string
		want bool
		why  string
	}{
		{"platform:messaging:event-type:view", true, "exact"},
		{"platform:*:*:*", true, "super-admin wildcard"},
		{"platform:messaging:*:*", true, "domain wildcard"},
		{"platform:messaging:event-type:*", true, "action wildcard"},
		{"platform:iam:*:*", false, "different domain"},
		{"platform:messaging:subscription:view", false, "different resource"},
		{"platform:messaging:event-type", false, "segment-count mismatch"},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, permissionMatches(c.held, required), "held=%q (%s)", c.held, c.why)
	}
}

// TestNonAnchorWithSeededPermissionIsAllowed is the regression test for the
// V2 bug: a non-anchor principal holding the actual stored 4-segment
// permission must pass the typed check. (The old logical names never matched
// any stored permission, so every non-anchor principal was denied.)
func TestNonAnchorWithSeededPermissionIsAllowed(t *testing.T) {
	a := &AuthContext{
		Scope:       ScopeClient,
		Permissions: []string{"platform:messaging:event-type:view"},
	}
	assert.NoError(t, CanReadEventTypes(a), "exact stored permission must be allowed")
	assert.Error(t, CanCreateEventTypes(a), "a view permission must not grant create")
}

func TestDomainWildcardGrantsWholeDomain(t *testing.T) {
	a := &AuthContext{
		Scope:       ScopeClient,
		Permissions: []string{"platform:messaging:*:*"},
	}
	assert.NoError(t, CanReadEventTypes(a))
	assert.NoError(t, CanWriteSubscriptions(a))
	assert.NoError(t, CanUpdateDispatchPools(a))
	assert.NoError(t, CanFireScheduledJobs(a))
	// The wildcard is scoped to the messaging domain — admin/iam are excluded.
	assert.Error(t, CanReadApplications(a), "messaging wildcard must not grant admin domain")
	assert.Error(t, CanReadRoles(a), "messaging wildcard must not grant iam domain")
}

func TestSuperAdminWildcardGrantsEverything(t *testing.T) {
	a := &AuthContext{Scope: ScopeClient, Permissions: []string{"platform:*:*:*"}}
	assert.NoError(t, CanReadEventTypes(a))
	assert.NoError(t, CanWriteApplications(a))
	assert.NoError(t, CanDeleteRoles(a))
	assert.NoError(t, CanViewDashboardStats(a), "the super-admin wildcard matches every permission")
}

// Scope is reach; authority comes from roles. An anchor principal passes
// RequireAnchor (it may act for every tenant) and nothing else: without a
// permission behind it, every gate refuses. This is the rule that makes a
// provisioned service account's role mean something — every one of them is
// anchor-scoped, so while scope implied authority, any application's
// credentials could call every admin route.
func TestAnchorScopeIsReachNotAuthority(t *testing.T) {
	a := &AuthContext{Scope: ScopeAnchor} // no explicit permissions
	assert.NoError(t, RequireAnchor(a), "anchor reach is unchanged")
	assert.Error(t, CanReadEventTypes(a), "anchor scope alone must not grant a read")
	assert.Error(t, CanWriteConnections(a), "anchor scope alone must not grant a write")
	assert.Error(t, CanViewDashboardStats(a))

	// The same principal, once its roles actually grant the permission.
	withPerm := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:messaging:event-type:view"}}
	assert.NoError(t, CanReadEventTypes(withPerm))
	assert.Error(t, CanWriteConnections(withPerm), "an unrelated permission grants nothing else")
}

// The user-admin check: an anchor's reach covers any target, but it must still
// hold the permission for the operation.
func TestRequireUserAdmin_AnchorStillNeedsAUserWritePermission(t *testing.T) {
	bare := &AuthContext{Scope: ScopeAnchor}
	assert.Error(t, RequireUserAdmin(bare, nil, CanUpdatePrincipals))

	writer := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:iam:user:update"}}
	assert.NoError(t, RequireUserAdmin(writer, nil, CanUpdatePrincipals), "a platform user is an anchor-only target")
	other := "clt_other"
	assert.NoError(t, RequireUserAdmin(writer, &other, CanUpdatePrincipals), "an anchor reaches every client")
}

// The user-admin check is the permission for the operation, not any user write
// (docs/platform-bug-fixes.md): an update-only administrator cannot create.
func TestRequireUserAdmin_ChecksTheOperationsPermission(t *testing.T) {
	client := "clt_mine"
	updater := &AuthContext{Scope: ScopeClient, Clients: []string{client}, Permissions: []string{"platform:iam:user:update"}}
	assert.Error(t, RequireUserAdmin(updater, &client, CanCreatePrincipals))
	assert.NoError(t, RequireUserAdmin(updater, &client, CanUpdatePrincipals))
	deleter := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:iam:user:delete"}}
	assert.Error(t, RequireUserAdmin(deleter, nil, CanCreatePrincipals))
	assert.Error(t, RequireUserAdmin(deleter, nil, CanUpdatePrincipals))
}

// Changing a user's roles needs user:assign-roles itself (owner ruling
// 2026-10-11); user:update no longer grants it, and assign-roles grants no
// other user write.
func TestCanAssignPrincipalRoles_NeedsAssignRoles(t *testing.T) {
	updater := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:iam:user:update"}}
	assert.Error(t, CanAssignPrincipalRoles(updater))
	assigner := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:iam:user:assign-roles"}}
	assert.NoError(t, CanAssignPrincipalRoles(assigner))
	assert.Error(t, CanUpdatePrincipals(assigner))
	wildcard := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:*:*:*"}}
	assert.NoError(t, CanAssignPrincipalRoles(wildcard))
}

func TestNonAnchorWithoutPermissionDenied(t *testing.T) {
	a := &AuthContext{Scope: ScopeClient, Permissions: []string{"platform:messaging:event:view"}}
	assert.Error(t, CanReadEventTypes(a), "an unrelated permission must not grant event-type view")
	assert.Error(t, RequireAnchor(a), "non-anchor must fail RequireAnchor")
	assert.Error(t, CanViewDashboardStats(a))
}

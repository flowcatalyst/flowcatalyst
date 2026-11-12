package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Anchor reach is not authority: the portal-user gates and the application
// enablement gates used to pass any anchor-tier caller (every application's
// service account, a read-only staff role) with no permission check.

const (
	testPortalView   = "platform:iam:portal-user:view"
	testPortalManage = "platform:iam:portal-user:manage"
	testEnable       = "platform:admin:application:enable-client"
	testDisable      = "platform:admin:application:disable-client"
	testAppUpdate    = "platform:admin:application:update"
)

func codeOf(t *testing.T, err error) string {
	t.Helper()
	require.Error(t, err)
	return err.Error()
}

func TestPortalUserGates(t *testing.T) {
	anchor := func(perms ...string) *AuthContext {
		return &AuthContext{Scope: ScopeAnchor, Permissions: perms}
	}
	clientTier := func(clients []string, perms ...string) *AuthContext {
		return &AuthContext{Scope: ScopeClient, Clients: clients, Permissions: perms}
	}

	t.Run("anchor with no portal permission is refused for read and manage", func(t *testing.T) {
		assert.Contains(t, codeOf(t, CanReadPortalUsers(anchor(), "clt_a")), "PERMISSION_REQUIRED")
		assert.Contains(t, codeOf(t, CanManagePortalUsers(anchor(), "clt_a")), "PERMISSION_REQUIRED")
		other := anchor("platform:admin:client:view")
		assert.Error(t, CanReadPortalUsers(other, "clt_a"))
		assert.Error(t, CanManagePortalUsers(other, "clt_a"))
	})
	t.Run("anchor with view may read only", func(t *testing.T) {
		a := anchor(testPortalView)
		assert.NoError(t, CanReadPortalUsers(a, "clt_a"))
		assert.Contains(t, codeOf(t, CanManagePortalUsers(a, "clt_a")), "PERMISSION_REQUIRED")
	})
	t.Run("anchor with manage may read and manage", func(t *testing.T) {
		a := anchor(testPortalManage)
		assert.NoError(t, CanReadPortalUsers(a, "clt_a"))
		assert.NoError(t, CanManagePortalUsers(a, "clt_a"))
	})
	t.Run("super-admin wildcard passes", func(t *testing.T) {
		a := anchor("platform:*:*:*")
		assert.NoError(t, CanReadPortalUsers(a, "clt_a"))
		assert.NoError(t, CanManagePortalUsers(a, "clt_a"))
	})
	t.Run("client tier with manage and access passes", func(t *testing.T) {
		a := clientTier([]string{"clt_a"}, testPortalManage)
		assert.NoError(t, CanReadPortalUsers(a, "clt_a"))
		assert.NoError(t, CanManagePortalUsers(a, "clt_a"))
	})
	t.Run("client tier with manage but no access keeps SCOPE_FORBIDDEN", func(t *testing.T) {
		a := clientTier([]string{"clt_a"}, testPortalManage)
		assert.Contains(t, codeOf(t, CanReadPortalUsers(a, "clt_b")), "SCOPE_FORBIDDEN")
		assert.Contains(t, codeOf(t, CanManagePortalUsers(a, "clt_b")), "SCOPE_FORBIDDEN")
	})
	t.Run("nil is unauthenticated", func(t *testing.T) {
		assert.Contains(t, codeOf(t, CanReadPortalUsers(nil, "clt_a")), "UNAUTHENTICATED")
		assert.Contains(t, codeOf(t, CanManagePortalUsers(nil, "clt_a")), "UNAUTHENTICATED")
	})
}

func TestApplicationEnablementGates(t *testing.T) {
	anchor := func(perms ...string) *AuthContext {
		return &AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: perms}
	}
	client := &AuthContext{
		Scope: ScopeClient, Clients: []string{"clt_a"}, AllApplications: true,
		Permissions: []string{testEnable, testDisable, testAppUpdate},
	}

	t.Run("enable and disable each need their own permission", func(t *testing.T) {
		assert.Contains(t, codeOf(t, CanEnableApplicationForClient(anchor())), "PERMISSION_REQUIRED")
		assert.Contains(t, codeOf(t, CanDisableApplicationForClient(anchor())), "PERMISSION_REQUIRED")
		assert.NoError(t, CanEnableApplicationForClient(anchor(testEnable)))
		assert.Error(t, CanDisableApplicationForClient(anchor(testEnable)))
		assert.NoError(t, CanDisableApplicationForClient(anchor(testDisable)))
		assert.Error(t, CanEnableApplicationForClient(anchor(testDisable)))
		assert.NoError(t, CanEnableApplicationForClient(anchor("platform:*:*:*")))
	})
	t.Run("a non-anchor holder is still refused", func(t *testing.T) {
		assert.Error(t, CanEnableApplicationForClient(client))
		assert.Error(t, CanDisableApplicationForClient(client))
		assert.Error(t, CanReplaceClientApplications(client))
		assert.Error(t, CanProvisionApplicationLoginClient(client))
	})
	t.Run("bulk replace needs both", func(t *testing.T) {
		assert.Error(t, CanReplaceClientApplications(anchor()))
		assert.Error(t, CanReplaceClientApplications(anchor(testEnable)))
		assert.Error(t, CanReplaceClientApplications(anchor(testDisable)))
		assert.NoError(t, CanReplaceClientApplications(anchor(testEnable, testDisable)))
	})
	t.Run("login client needs application:update", func(t *testing.T) {
		assert.Contains(t, codeOf(t, CanProvisionApplicationLoginClient(anchor())), "PERMISSION_REQUIRED")
		assert.NoError(t, CanProvisionApplicationLoginClient(anchor(testAppUpdate)))
	})
	t.Run("application confinement", func(t *testing.T) {
		scoped := &AuthContext{Scope: ScopeAnchor, Applications: []string{"app_a"}}
		assert.NoError(t, RequireApplicationConfinement(scoped, "app_a"))
		assert.Contains(t, codeOf(t, RequireApplicationConfinement(scoped, "app_b")), "APPLICATION_ACCESS_REQUIRED")
		assert.Contains(t, codeOf(t, RequireAllApplications(scoped)), "APPLICATION_ACCESS_REQUIRED")
		assert.NoError(t, RequireApplicationConfinement(anchor(), "app_b"))
		assert.NoError(t, RequireAllApplications(anchor()))
	})
}

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These families were anchor-only with NO permission gate. That was invisible
// while anchor scope satisfied every gate; with the bypass withdrawn, "anchor
// alone" would have meant a read-only staff role — or any provisioned service
// account, all of which are anchor-scoped — could still write here.
//
// Each case pins both halves of the rule: reach (a non-anchor holding the
// permission is still refused) and authority (an anchor without it is refused,
// with it allowed).
func TestReachOnlyRoutesNowRequireTheirPermission(t *testing.T) {
	cases := []struct {
		name  string
		perm  string
		check func(*AuthContext) error
	}{
		{"client view", "platform:admin:client:view", CanReadClients},
		{"client create", "platform:admin:client:create", CanCreateClients},
		{"client update", "platform:admin:client:update", CanUpdateClients},
		{"client delete", "platform:admin:client:delete", CanDeleteClients},
		{"client activate", "platform:admin:client:activate", CanActivateClients},
		{"client suspend", "platform:admin:client:suspend", CanSuspendClients},
		{"client deactivate", "platform:admin:client:deactivate", CanDeactivateClients},

		{"oauth client view", "platform:auth:oauth-client:view", CanReadOAuthClients},
		{"oauth client create", "platform:auth:oauth-client:create", CanCreateOAuthClients},
		{"oauth client update", "platform:auth:oauth-client:update", CanUpdateOAuthClients},
		{"oauth client delete", "platform:auth:oauth-client:delete", CanDeleteOAuthClients},
		{"oauth secret rotation", "platform:auth:oauth-client:regenerate-secret", CanRotateOAuthClientSecrets},

		{"idp view", "platform:iam:idp:view", CanReadIdentityProviders},
		{"idp create", "platform:iam:idp:create", CanCreateIdentityProviders},
		{"idp update", "platform:iam:idp:update", CanUpdateIdentityProviders},
		{"idp delete", "platform:iam:idp:delete", CanDeleteIdentityProviders},

		{"anchor domain view", "platform:admin:anchor-domain:view", CanReadAnchorDomains},
		{"anchor domain create", "platform:admin:anchor-domain:create", CanCreateAnchorDomains},
		{"anchor domain update", "platform:admin:anchor-domain:update", CanUpdateAnchorDomains},
		{"anchor domain delete", "platform:admin:anchor-domain:delete", CanDeleteAnchorDomains},

		{"auth config view", "platform:auth:client-auth-config:view", CanReadAuthConfigs},
		{"auth config create", "platform:auth:client-auth-config:create", CanCreateAuthConfigs},
		{"auth config update", "platform:auth:client-auth-config:update", CanUpdateAuthConfigs},
		{"auth config delete", "platform:auth:client-auth-config:delete", CanDeleteAuthConfigs},

		{"email domain mapping view", "platform:iam:email-domain-mapping:view", CanReadEmailDomainMappings},
		{"email domain mapping create", "platform:iam:email-domain-mapping:create", CanCreateEmailDomainMappings},
		{"email domain mapping update", "platform:iam:email-domain-mapping:update", CanUpdateEmailDomainMappings},
		{"email domain mapping delete", "platform:iam:email-domain-mapping:delete", CanDeleteEmailDomainMappings},

		{"platform config view", "platform:admin:config:view", CanReadPlatformConfig},
		{"platform config update", "platform:admin:config:update", CanUpdatePlatformConfig},

		{"cors origin view", "platform:admin:cors-origin:view", CanReadCorsOrigins},
		{"cors origin create", "platform:admin:cors-origin:create", CanCreateCorsOrigins},
		{"cors origin delete", "platform:admin:cors-origin:delete", CanDeleteCorsOrigins},

		{"login attempt view", "platform:admin:login-attempt:view", CanReadLoginAttempts},

		{"developer portal view", "platform:developer:application-openapi:view", CanReadDeveloperPortal},
		{"platform openapi sync", "platform:developer:application-openapi:sync", CanSyncPlatformOpenAPI},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bare := &AuthContext{Scope: ScopeAnchor}
			err := c.check(bare)
			require.Error(t, err, "anchor scope alone must not grant this")
			assert.Contains(t, err.Error(), "PERMISSION_REQUIRED")

			granted := &AuthContext{Scope: ScopeAnchor, Permissions: []string{c.perm}}
			assert.NoError(t, c.check(granted), "the family's own code must grant it")

			// Reach is still enforced: holding the permission from a
			// client-scoped principal does not reach platform-owner routes.
			clientScoped := &AuthContext{Scope: ScopeClient, Permissions: []string{c.perm}}
			err = c.check(clientScoped)
			require.Error(t, err, "a client-scoped principal must not reach this")
			assert.Contains(t, err.Error(), "ANCHOR_REQUIRED")

			// And the super-admin wildcard passes everywhere.
			assert.NoError(t, c.check(&AuthContext{Scope: ScopeAnchor, Permissions: []string{permSuperAdmin}}))
		})
	}
}

// One family's code grants nothing in another: the gates are per-family, not a
// single "admin" bit wearing different names.
func TestReachOnlyPermissionsDoNotLeakAcrossFamilies(t *testing.T) {
	corsOnly := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:admin:cors-origin:view"}}
	assert.NoError(t, CanReadCorsOrigins(corsOnly))
	assert.Error(t, CanReadClients(corsOnly))
	assert.Error(t, CanReadIdentityProviders(corsOnly))
	assert.Error(t, CanReadLoginAttempts(corsOnly))
}

//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A provider that serves a client or partner domain may not sync roles without
// an allow-list of the roles it may grant (docs/oidc-security-audit.md). A
// provider serving only ANCHOR domains, with sync off, or with an allow-list,
// saves as before; a provider stored in the refused state keeps saving for
// other edits.

const allowlistRefusal = "ROLE_ALLOWLIST_REQUIRED"

func assertAllowlistRefused(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "a use-case error, got %v", err)
	assert.Equal(t, allowlistRefusal, ue.Code)
	assert.Equal(t, usecase.KindValidation, ue.Kind)
}

// createCmd builds an OIDC create command for one hand-unique domain.
func createCmd(code string, scope string, sync bool, allow []string) operations.CreateCommand {
	issuer := "https://login." + code + ".example.com/v2.0"
	clientID := code + "-client-id"
	cmd := operations.CreateCommand{
		Code: code, Name: code, Type: "OIDC",
		OIDCIssuerURL: &issuer, OIDCClientID: &clientID,
		AllowedEmailDomains: []string{code + ".example.com"},
		MappingScope:        &scope,
		SyncRolesFromIDP:    sync,
		AllowedRoleIDs:      allow,
	}
	if scope == "CLIENT" {
		cmd.PrimaryClientID = new("cli_" + code)
	}
	return cmd
}

// TA1 (create)
func TestCreateIdentityProvider_RoleSyncNeedsAllowListForNonAnchorDomain(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	ctx := context.Background()

	cases := []struct {
		name   string
		code   string
		scope  string
		sync   bool
		allow  []string
		refuse bool
	}{
		{"client domain, sync, no allow-list", "ta1c-refused", "CLIENT", true, nil, true},
		{"client domain, sync, empty allow-list", "ta1c-empty", "CLIENT", true, []string{}, true},
		{"client domain, sync, allow-list", "ta1c-allow", "CLIENT", true, []string{"rol_ta1callow1"}, false},
		{"anchor domain, sync, no allow-list", "ta1c-anchor", "ANCHOR", true, nil, false},
		{"client domain, sync off", "ta1c-syncoff", "CLIENT", false, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), createCmd(tc.code, tc.scope, tc.sync, tc.allow))
			got, ferr := d.Repo.FindByCode(ctx, tc.code)
			require.NoError(t, ferr)
			if tc.refuse {
				assertAllowlistRefused(t, err)
				assert.Nil(t, got, "a refused create saves nothing")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

// TA1 (create): an existing CLIENT mapping claimed by a new provider counts
// with the scope it already has, whatever mappingScope the command says.
func TestCreateIdentityProvider_ClaimedClientDomainCounts(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)

	mustCreateCmd(t, d, createCmd("ta1cl-src", "CLIENT", false, nil))
	cmd := createCmd("ta1cl-claimer", "ANCHOR", true, nil)
	cmd.AllowedEmailDomains = []string{"ta1cl-src.example.com"} // already a CLIENT mapping
	_, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), cmd)
	assertAllowlistRefused(t, err)
}

func mustCreateCmd(t *testing.T, d operations.Deps, cmd operations.CreateCommand) operations.CreateResult {
	t.Helper()
	res, err := runAuthorizedTx(testpg.NewUoW(t), operations.CreateIdentityProvider(d), cmd)
	require.NoError(t, err)
	return res
}

// TA1 (update)
func TestUpdateIdentityProvider_RoleSyncNeedsAllowListForNonAnchorDomain(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	ctx := context.Background()
	yes := true

	t.Run("turning sync on for a client domain without an allow-list is refused", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-on", "CLIENT", false, nil))
		_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{ID: res.IdentityProviderID, SyncRolesFromIDP: &yes})
		assertAllowlistRefused(t, err)
		got, ferr := d.Repo.FindByID(ctx, res.IdentityProviderID)
		require.NoError(t, ferr)
		assert.False(t, got.SyncRolesFromIDP, "the refused update saved nothing")
	})

	t.Run("with an allow-list it saves", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-allow", "CLIENT", false, nil))
		_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{ID: res.IdentityProviderID, SyncRolesFromIDP: &yes, AllowedRoleIDs: []string{"rol_ta1uallow1"}})
		require.NoError(t, err)
	})

	t.Run("an anchor-only provider saves without one", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-anchor", "ANCHOR", false, nil))
		_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{ID: res.IdentityProviderID, SyncRolesFromIDP: &yes})
		require.NoError(t, err)
	})

	t.Run("emptying the allow-list of a syncing client provider is refused", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-empty", "CLIENT", true, []string{"rol_ta1uempty1"}))
		_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{ID: res.IdentityProviderID, AllowedRoleIDs: []string{}})
		assertAllowlistRefused(t, err)
	})

	t.Run("adding a client domain to a syncing anchor provider without an allow-list is refused", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-adddom", "ANCHOR", true, nil))
		_, err := runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{
				ID:                  res.IdentityProviderID,
				AllowedEmailDomains: []string{"ta1u-adddom.example.com", "ta1u-adddom-new.example.com"},
				MappingScope:        new("CLIENT"), PrimaryClientID: new("cli_ta1uadddom"),
			})
		assertAllowlistRefused(t, err)
	})

	t.Run("a stored provider already in the state keeps saving other edits", func(t *testing.T) {
		res := mustCreateCmd(t, d, createCmd("ta1u-legacy", "CLIENT", true, []string{"rol_ta1ulegacy1"}))
		// Predates the rule: strip the allow-list behind the use case's back.
		_, err := testpg.Pool(t).Exec(ctx,
			`DELETE FROM oauth_identity_provider_allowed_roles WHERE identity_provider_id = $1`, res.IdentityProviderID)
		require.NoError(t, err)
		_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d),
			operations.UpdateCommand{ID: res.IdentityProviderID, Name: new("Renamed legacy")})
		require.NoError(t, err)
		got, ferr := d.Repo.FindByID(ctx, res.IdentityProviderID)
		require.NoError(t, ferr)
		assert.True(t, got.RoleSyncNeedsAllowList(), "still flagged, still saving")
	})
}

// TA1 (mapping side): a client or partner domain cannot be mapped to, or moved
// onto, a provider that syncs roles without an allow-list.
func TestMappingOps_RoleSyncNeedsAllowListForNonAnchorDomain(t *testing.T) {
	t.Parallel()
	d := deps(t)
	uow := testpg.NewUoW(t)
	ctx := context.Background()

	syncing := mustCreateCmd(t, d, createCmd("ta1m-syncing", "ANCHOR", true, nil)) // anchor-only: allowed
	other := mustCreateCmd(t, d, createCmd("ta1m-other", "CLIENT", false, nil))

	t.Run("create a client mapping onto it is refused", func(t *testing.T) {
		_, err := runAuthorized(uow, edmops.CreateMapping(d.MoveDeps.Mappings, d.Repo), edmops.CreateCommand{
			EmailDomain: "ta1m-new-client.example.com", IdentityProviderID: syncing.IdentityProviderID,
			ScopeType: "CLIENT", PrimaryClientID: new("cli_ta1mnew"),
		})
		assertAllowlistRefused(t, err)
		m, ferr := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "ta1m-new-client.example.com")
		require.NoError(t, ferr)
		assert.Nil(t, m)
	})

	t.Run("create an anchor mapping onto it saves", func(t *testing.T) {
		_, err := runAuthorized(uow, edmops.CreateMapping(d.MoveDeps.Mappings, d.Repo), edmops.CreateCommand{
			EmailDomain: "ta1m-new-anchor.example.com", IdentityProviderID: syncing.IdentityProviderID, ScopeType: "ANCHOR",
		})
		require.NoError(t, err)
	})

	t.Run("move a client mapping onto it is refused", func(t *testing.T) {
		m, ferr := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "ta1m-other.example.com")
		require.NoError(t, ferr)
		require.NotNil(t, m)
		_, err := runAuthorizedTx(uow, edmops.MoveMappingToProvider(d.MoveDeps),
			edmops.MoveProviderCommand{ID: m.ID, IdentityProviderID: syncing.IdentityProviderID})
		assertAllowlistRefused(t, err)
		again, ferr := d.MoveDeps.Mappings.FindByEmailDomain(ctx, "ta1m-other.example.com")
		require.NoError(t, ferr)
		assert.Equal(t, other.IdentityProviderID, again.IdentityProviderID, "the domain did not move")
	})
}

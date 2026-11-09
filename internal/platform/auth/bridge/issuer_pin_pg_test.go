//go:build integration

package bridge

import (
	"context"
	"net/http"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The issuer pin end to end (docs/oidc-security-audit.md §1): a multi-tenant
// provider pinned to one tenant signs in a token of that tenant and refuses a
// genuinely-signed token of another tenant at the issuer check; a broad
// pattern is refused at save; a legacy broad pattern keeps signing in until
// FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED is turned on.
func TestOidcLogin_MultiTenantIssuerPin(t *testing.T) {
	t.Cleanup(func() { SetIssuerPatternRequirePinned(false) })
	f := newOidcTestFixture(t)
	ctx := context.Background()
	pool := testpg.Pool(t)
	idps := identityprovider.NewRepository(pool)
	update := func(cmd idpops.UpdateCommand) error {
		_, err := usecaseop.RunTx(testpg.AnchorCtx(), testpg.NewUoW(t), idpops.UpdateIdentityProvider(idpops.Deps{
			Repo: idps,
			MoveDeps: edmops.MoveDeps{
				Mappings: emaildomainmapping.NewRepository(pool), IDPs: idps, Principals: principal.NewRepository(pool),
			},
		}), cmd, testpg.TestEC())
		return err
	}
	tenantA := f.idp.issuerURL + "/tenant-a"
	tenantB := f.idp.issuerURL + "/tenant-b"
	multi := true

	// Save refuses a pattern that admits any tenant.
	broad := "^" + regexp.QuoteMeta(f.idp.issuerURL) + "/[^/]+$"
	err := update(idpops.UpdateCommand{ID: f.identityProviderID, OIDCMultiTenant: &multi, OIDCIssuerPattern: &broad})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ISSUER_PATTERN_NOT_PINNED")

	pinned := "^" + regexp.QuoteMeta(tenantA) + "$"
	require.NoError(t, update(idpops.UpdateCommand{ID: f.identityProviderID, OIDCMultiTenant: &multi, OIDCIssuerPattern: &pinned}))

	rec := f.login(t, map[string]any{"nonce": "nonce-pin-a", "email": "pin-a@" + f.domain, "iss": tenantA}, "opaque")
	require.Equal(t, http.StatusOK, rec.Code, "our tenant signs in: %s", rec.Body.String())

	rec = f.login(t, map[string]any{"nonce": "nonce-pin-b", "email": "pin-b@" + f.domain, "iss": tenantB}, "opaque")
	assert.NotEqual(t, http.StatusOK, rec.Code, "another tenant's token is refused")
	assert.False(t, f.lastSessionCalled)

	// A legacy broad pattern (stored before save refused it).
	_, err = pool.Exec(ctx, `UPDATE oauth_identity_providers SET oidc_issuer_pattern = $2 WHERE id = $1`, f.identityProviderID, broad)
	require.NoError(t, err)
	rec = f.login(t, map[string]any{"nonce": "nonce-pin-legacy", "email": "pin-legacy@" + f.domain, "iss": tenantB}, "opaque")
	assert.Equal(t, http.StatusOK, rec.Code, "a legacy pattern keeps working by default (warned, not locked out): %s", rec.Body.String())

	SetIssuerPatternRequirePinned(true)
	rec = f.login(t, map[string]any{"nonce": "nonce-pin-strict", "email": "pin-strict@" + f.domain, "iss": tenantA}, "opaque")
	assert.NotEqual(t, http.StatusOK, rec.Code, "REQUIRE_PINNED refuses every sign-in through a broad pattern")
	assert.False(t, f.lastSessionCalled)
}

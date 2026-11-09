//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The issuer-pin rule at save (docs/oidc-security-audit.md §1): a pattern
// that does not pin the issuer — any tenant ([^/]+), unescaped dots, no
// anchors — is refused on create and when an update sets it or puts it in
// force (turning multi-tenant on). A broad pattern already stored keeps
// working while the provider is edited for other reasons, so existing tenants
// are not locked out by an unrelated save.
func TestIdentityProviderIssuerPatternMustPinTheTenant(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d := deps(t)
	uow := testpg.NewUoW(t)

	issuer := "https://login.microsoftonline.com/organizations/v2.0"
	clientID := "issuerpin-client"
	multi := true
	broad := `^https://login\.microsoftonline\.com/[^/]+/v2\.0$`
	pinned := `^https://login\.microsoftonline\.com/11111111-2222-3333-4444-555555555555/v2\.0$`

	create := func(code, pattern string) (operations.CreateResult, error) {
		p := pattern
		return runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
			Code: code, Name: code, Type: "OIDC", OIDCIssuerURL: &issuer, OIDCClientID: &clientID,
			OIDCMultiTenant: &multi, OIDCIssuerPattern: &p,
		})
	}

	// Create: broad refused, pinned accepted.
	for _, p := range []string{broad, `https://login.microsoftonline.com/{tenantId}/v2.0`, `login\.microsoftonline\.com/abc`} {
		_, err := create("issuerpin-broad", p)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ISSUER_PATTERN_NOT_PINNED")
	}
	res, err := create("issuerpin-ok", pinned)
	require.NoError(t, err)

	// Update: setting a broad pattern is refused, and nothing is written.
	b := broad
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{ID: res.IdentityProviderID, OIDCIssuerPattern: &b})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ISSUER_PATTERN_NOT_PINNED")
	got, err := d.Repo.FindByID(ctx, res.IdentityProviderID)
	require.NoError(t, err)
	assert.Equal(t, pinned, *got.OIDCIssuerPattern)

	// A legacy provider whose stored pattern is broad (written before the
	// rule): renaming it, or re-sending the same pattern, still saves.
	legacy, err := create("issuerpin-legacy", pinned)
	require.NoError(t, err)
	_, err = testpg.Pool(t).Exec(ctx, `UPDATE oauth_identity_providers SET oidc_issuer_pattern = $2 WHERE id = $1`, legacy.IdentityProviderID, broad)
	require.NoError(t, err)
	name := "Legacy renamed"
	same := broad
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{ID: legacy.IdentityProviderID, Name: &name, OIDCIssuerPattern: &same})
	require.NoError(t, err, "an unrelated edit of a legacy provider is not refused")
	got, err = d.Repo.FindByID(ctx, legacy.IdentityProviderID)
	require.NoError(t, err)
	assert.True(t, got.HasBroadIssuerPattern(), "the legacy pattern is kept, and flagged")

	// ... but turning multi-tenant ON puts a stored broad pattern in force:
	// refused.
	single := false
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{ID: legacy.IdentityProviderID, OIDCMultiTenant: &single})
	require.NoError(t, err)
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{ID: legacy.IdentityProviderID, OIDCMultiTenant: &multi})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ISSUER_PATTERN_NOT_PINNED")

	// Pinning it fixes it.
	p := pinned
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{ID: legacy.IdentityProviderID, OIDCMultiTenant: &multi, OIDCIssuerPattern: &p})
	require.NoError(t, err)
	got, err = d.Repo.FindByID(ctx, legacy.IdentityProviderID)
	require.NoError(t, err)
	assert.False(t, got.HasBroadIssuerPattern())
}

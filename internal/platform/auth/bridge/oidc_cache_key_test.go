package bridge

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
)

// The resolved-client cache must turn over when the IdP's secret (or any
// other field that shapes the client) changes — the 2026-09-23 prod incident:
// a secret saved after the first login never took effect, because the cache
// keyed on issuer|client id alone and never expired.
func TestCacheKeyChangesWithEverySecretOrShapeChange(t *testing.T) {
	issuer, clientID := "https://login.microsoftonline.com/t1/v2.0", "app-1"
	base := func() *identityprovider.IdentityProvider {
		return &identityprovider.IdentityProvider{OIDCIssuerURL: &issuer, OIDCClientID: &clientID}
	}
	noSecret := cacheKey(base())

	withSecret := base()
	ref := "encrypted:abc"
	withSecret.OIDCClientSecretRef = &ref
	assert.NotEqual(t, noSecret, cacheKey(withSecret), "adding a secret must yield a new cache entry")

	rotated := base()
	ref2 := "encrypted:def"
	rotated.OIDCClientSecretRef = &ref2
	assert.NotEqual(t, cacheKey(withSecret), cacheKey(rotated), "rotating the secret must yield a new cache entry")

	multi := base()
	multi.OIDCMultiTenant = true
	assert.NotEqual(t, noSecret, cacheKey(multi))

	pattern := base()
	p := "https://login.microsoftonline.com/{tenantid}/v2.0"
	pattern.OIDCIssuerPattern = &p
	assert.NotEqual(t, noSecret, cacheKey(pattern))

	assert.Equal(t, noSecret, cacheKey(base()), "an unchanged IdP keeps hitting its entry")
}

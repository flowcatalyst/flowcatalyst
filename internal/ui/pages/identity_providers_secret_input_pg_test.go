//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TB2 (/ui): the identity provider form refuses a stored ciphertext typed as
// the client secret (whichever kind is chosen), creating or updating nothing;
// a plaintext secret is saved encrypted as before.
func TestIdentityProviderForm_RefusesAStoredCiphertextAsTheClientSecret(t *testing.T) {
	ctx := context.Background()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	pool := testpg.Pool(t)
	idps := identityprovider.NewRepository(pool)
	clients := client.NewRepository(pool)
	ops := IdpOps{Repo: idps, Mappings: emaildomainmapping.NewRepository(pool), Principals: principal.NewRepository(pool), Enc: enc, UoW: testpg.NewUoW(t)}
	serve := func(q req) *httptest.ResponseRecorder {
		return serveMounted(t, anchorAdmin(), q,
			&IdentityProviders{Store: idps, Ops: ops, Clients: clients, ClientNames: clients, Shell: shellForTest()})
	}
	create := func(code string, kv ...string) *httptest.ResponseRecorder {
		base := []string{
			"code", code, "name", code, "type", "OIDC", "multiTenant", "true",
			"issuerUrl", "https://login.tb2ui.test/v2.0", "clientId", "tb2ui-app",
		}
		return serve(postForm("/ui/identity-providers/new", append(base, kv...)...))
	}
	sealed, err := enc.Encrypt("the-real-secret")
	require.NoError(t, err)
	const refusal = "A secret must be supplied as its value, not as a stored ciphertext"

	for i, c := range []struct{ kind, field, value string }{
		{"encrypt", "secretValue", "encrypted:" + sealed},
		{"encrypt", "secretValue", "  Encrypted:" + sealed},
		{"aws-sm", "secretRef", "ENCRYPTED:AAAA"},
	} {
		code := "tb2ui-refused-" + string(rune('a'+i))
		rec := create(code, "secretKind", c.kind, c.field, c.value)
		assert.Contains(t, rec.Body.String(), refusal, c.value)
		assert.NotContains(t, rec.Body.String(), sealed, "the pasted value is not echoed")
		found, err := idps.FindByCode(ctx, code)
		require.NoError(t, err)
		assert.Nil(t, found, "nothing is created: %s", c.value)
	}

	// Plaintext: saved, encrypted.
	rec := create("tb2ui-ok", "secretKind", "encrypt", "secretValue", "a-plain-client-secret")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	ip, err := idps.FindByCode(ctx, "tb2ui-ok")
	require.NoError(t, err)
	require.NotNil(t, ip)
	require.NotNil(t, ip.OIDCClientSecretRef)
	plain, err := enc.Decrypt(*ip.OIDCClientSecretRef)
	require.NoError(t, err)
	assert.Equal(t, "a-plain-client-secret", plain)
	before := *ip.OIDCClientSecretRef

	// Update: refused, stored secret unchanged; a plaintext replaces it.
	edit := func(kv ...string) *httptest.ResponseRecorder {
		base := []string{"name", "tb2ui-ok", "multiTenant", "true", "issuerUrl", "https://login.tb2ui.test/v2.0", "clientId", "tb2ui-app"}
		return serve(postForm("/ui/identity-providers/"+ip.ID+"/edit", append(base, kv...)...))
	}
	rec = edit("secretKind", "encrypt", "secretValue", "encrypted:"+sealed)
	assert.Contains(t, rec.Body.String(), refusal)
	ip, err = idps.FindByCode(ctx, "tb2ui-ok")
	require.NoError(t, err)
	assert.Equal(t, before, *ip.OIDCClientSecretRef)
	rec = edit("secretKind", "encrypt", "secretValue", "a-new-plain-secret")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	ip, err = idps.FindByCode(ctx, "tb2ui-ok")
	require.NoError(t, err)
	plain, err = enc.Decrypt(*ip.OIDCClientSecretRef)
	require.NoError(t, err)
	assert.Equal(t, "a-new-plain-secret", plain)
}

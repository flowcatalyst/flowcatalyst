//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TB2 (API): the client secret an API caller supplies is its value or an
// external reference, never a stored ciphertext, which the bridge would
// decrypt at sign-in.
func TestIdentityProviderAPI_RefusesAStoredCiphertextAsTheClientSecret(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	pool := testpg.Pool(t)
	repo := identityprovider.NewRepository(pool)
	s := &State{Repo: repo, UoW: testpg.NewUoW(t), Enc: enc}
	ctx := testpg.AnchorCtx()
	require.NoError(t, auth.CanCreateIdentityProviders(auth.FromContext(ctx)))

	body := func(code string, secret *string) CreateIdentityProviderRequest {
		return CreateIdentityProviderRequest{
			Code: code, Name: code, Type: "OIDC",
			OIDCIssuerURL: new("https://login.tb2api.test/v2.0"), OIDCClientID: new("tb2-app"), OIDCClientSecretRef: secret,
		}
	}

	// A ciphertext of the platform's own key: what a database dump holds.
	sealed, err := enc.Encrypt("the-real-secret")
	require.NoError(t, err)
	for i, v := range []string{"encrypted:" + sealed, " Encrypted:" + sealed, "ENCRYPTED:AAAA"} {
		code := "tb2api-refused-" + string(rune('a'+i))
		_, err := s.create(ctx, &apicommon.In[CreateIdentityProviderRequest]{Body: body(code, new(v))})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_INVALID")
		found, err := repo.FindByCode(context.Background(), code)
		require.NoError(t, err)
		assert.Nil(t, found, "nothing is created")
	}

	// A plaintext secret is stored encrypted, as before.
	_, err = s.create(ctx, &apicommon.In[CreateIdentityProviderRequest]{Body: body("tb2api-ok", new("a-plain-client-secret"))})
	require.NoError(t, err)
	ip, err := repo.FindByCode(context.Background(), "tb2api-ok")
	require.NoError(t, err)
	require.NotNil(t, ip)
	require.NotNil(t, ip.OIDCClientSecretRef)
	assert.Contains(t, *ip.OIDCClientSecretRef, "encrypted:")
	assert.NotContains(t, *ip.OIDCClientSecretRef, "a-plain-client-secret")
	plain, err := enc.Decrypt(*ip.OIDCClientSecretRef)
	require.NoError(t, err)
	assert.Equal(t, "a-plain-client-secret", plain)
	before := *ip.OIDCClientSecretRef

	// Update: the ciphertext is refused and the stored secret is untouched.
	for _, v := range []string{"encrypted:" + sealed, "  eNcRyPtEd:AAAA"} {
		_, err := s.update(ctx, &updateInput{ID: ip.ID, Body: UpdateIdentityProviderRequest{OIDCClientSecretRef: new(v)}})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_INVALID")
	}
	ip, err = repo.FindByCode(context.Background(), "tb2api-ok")
	require.NoError(t, err)
	assert.Equal(t, before, *ip.OIDCClientSecretRef)

	// Update with a plaintext replaces it; omitting the field keeps it.
	_, err = s.update(ctx, &updateInput{ID: ip.ID, Body: UpdateIdentityProviderRequest{OIDCClientSecretRef: new("a-new-plain-secret")}})
	require.NoError(t, err)
	ip, err = repo.FindByCode(context.Background(), "tb2api-ok")
	require.NoError(t, err)
	plain, err = enc.Decrypt(*ip.OIDCClientSecretRef)
	require.NoError(t, err)
	assert.Equal(t, "a-new-plain-secret", plain)
	_, err = s.update(ctx, &updateInput{ID: ip.ID, Body: UpdateIdentityProviderRequest{Name: new("Renamed")}})
	require.NoError(t, err)
	ip2, err := repo.FindByCode(context.Background(), "tb2api-ok")
	require.NoError(t, err)
	assert.Equal(t, *ip.OIDCClientSecretRef, *ip2.OIDCClientSecretRef, "an unrelated update leaves the stored secret as it is")
}

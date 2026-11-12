package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// TB3 (auth config): the OIDC client secret an API caller supplies for an auth
// config is its value, never a stored ciphertext (it is decrypted when used).
func TestEncryptOIDCSecretRef_RefusesAStoredCiphertext(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	sealed, err := enc.Encrypt("the-real-secret")
	require.NoError(t, err)

	for _, v := range []string{"encrypted:" + sealed, "  Encrypted:" + sealed, "ENCRYPTED:AAAA"} {
		out, err := encryptOIDCSecretRef(enc, &v)
		require.Error(t, err, v)
		assert.Nil(t, out)
		ue := usecase.AsError(err)
		require.NotNil(t, ue)
		assert.Equal(t, usecase.KindValidation, ue.Kind)
		assert.Equal(t, "SECRET_VALUE_INVALID", ue.Code)
	}

	plain := "a-plain-client-secret"
	out, err := encryptOIDCSecretRef(enc, &plain)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Contains(t, *out, "encrypted:")
	got, err := enc.Decrypt(*out)
	require.NoError(t, err)
	assert.Equal(t, plain, got)

	out, err = encryptOIDCSecretRef(enc, nil)
	require.NoError(t, err)
	assert.Nil(t, out, "an omitted secret keeps the stored one")
	ref := "aws-sm://prod/idp#secret"
	out, err = encryptOIDCSecretRef(enc, &ref)
	require.NoError(t, err)
	assert.Equal(t, ref, *out, "an external reference is stored as given")
}

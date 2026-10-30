package ingestion_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

func TestDeriveSecretName(t *testing.T) {
	for _, tc := range []struct{ key, path, want string }{
		{"mix-positions", "clientSecret", "mix-positions-client-secret"},
		{"vendor-feed", "credential", "vendor-feed-credential"},
		{"feed", "credential.secretAccessKey", "feed-credential-secret-access-key"},
		{"feed", "auth.clientSecret", "feed-auth-client-secret"},
	} {
		got, err := ingestion.DeriveSecretName(tc.key, tc.path)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got)
		assert.True(t, ingestion.ValidSecretName(got))
	}
	_, err := ingestion.DeriveSecretName("feed", "bad path")
	require.Error(t, err)
	long := make([]byte, 100)
	for i := range long {
		long[i] = 'a'
	}
	_, err = ingestion.DeriveSecretName(string(long), "password")
	require.Error(t, err, "a name past the limit is refused, not cut")
}

func TestSecretNameForKeepsTheNameThereIs(t *testing.T) {
	current := map[string]any{"auth": map[string]any{"clientSecret": map[string]any{"name": "mix-client-secret"}}, "token": "old-token"}
	got, err := ingestion.SecretNameFor(nil, current, "mix-positions", "auth.clientSecret")
	require.NoError(t, err)
	assert.Equal(t, "mix-client-secret", got, "an existing entry keeps its names")
	got, err = ingestion.SecretNameFor(nil, current, "mix-positions", "token")
	require.NoError(t, err)
	assert.Equal(t, "old-token", got, "an extension's name is plain text")
	got, err = ingestion.SecretNameFor(map[string]any{"token": "named-by-caller"}, current, "mix-positions", "token")
	require.NoError(t, err)
	assert.Equal(t, "named-by-caller", got, "the value being written wins")
	got, err = ingestion.SecretNameFor(nil, nil, "mix-positions", "password")
	require.NoError(t, err)
	assert.Equal(t, "mix-positions-password", got)
}

func TestBindSecretName(t *testing.T) {
	entry := map[string]any{"url": "u"}
	ingestion.BindSecretName(entry, "auth.password", "feed-auth-password", false)
	ingestion.BindSecretName(entry, "token", "feed-token", true)
	assert.Equal(t, map[string]any{"url": "u", "auth": map[string]any{"password": map[string]any{"name": "feed-auth-password"}}, "token": "feed-token"}, entry)
}

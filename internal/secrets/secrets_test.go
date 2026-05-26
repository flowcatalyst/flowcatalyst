package secrets_test

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/secrets"
)

func TestServiceResolvesLiterals(t *testing.T) {
	svc := secrets.NewService("env")
	got, err := svc.Resolve(context.Background(), "literal:hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", got)

	got, err = svc.Resolve(context.Background(), "bare-value")
	require.NoError(t, err)
	assert.Equal(t, "bare-value", got)
}

func TestServiceRoutesByScheme(t *testing.T) {
	svc := secrets.NewService("env")
	svc.Register(secrets.NewEnvProvider())

	t.Setenv("MY_SECRET", "from-env")
	got, err := svc.Resolve(context.Background(), "env://MY_SECRET")
	require.NoError(t, err)
	assert.Equal(t, "from-env", got)
}

func TestServiceMissingProvider(t *testing.T) {
	svc := secrets.NewService("env")
	_, err := svc.Resolve(context.Background(), "aws-sm://my-secret")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "aws-sm")
}

func TestEnvProviderNotFound(t *testing.T) {
	p := secrets.NewEnvProvider()
	_, err := p.Get(context.Background(), "DOES_NOT_EXIST_XYZ")
	require.Error(t, err)
}

func TestEncryptedFileRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "secrets.json")
	p, err := secrets.NewEncryptedFileProvider(path, key)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, p.Set(ctx, "db-password", "super-secret"))

	got, err := p.Get(ctx, "db-password")
	require.NoError(t, err)
	assert.Equal(t, "super-secret", got)

	// Reopen with same key — value still there.
	p2, err := secrets.NewEncryptedFileProvider(path, key)
	require.NoError(t, err)
	got2, err := p2.Get(ctx, "db-password")
	require.NoError(t, err)
	assert.Equal(t, "super-secret", got2)

	require.NoError(t, p2.Delete(ctx, "db-password"))
	_, err = p2.Get(ctx, "db-password")
	require.Error(t, err)
}

func TestEncryptedFileRequiresCorrectKeySize(t *testing.T) {
	_, err := secrets.NewEncryptedFileProvider("/tmp/whatever", []byte("short-key"))
	require.Error(t, err)
}

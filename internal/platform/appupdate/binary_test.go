package appupdate

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariantOrder(t *testing.T) {
	t.Parallel()
	v := []string{"qa", "debug", "beta", "release"}
	slices.SortFunc(v, VariantOrder)
	assert.Equal(t, []string{"release", "debug", "beta", "qa"}, v)
}

func TestValidVariant(t *testing.T) {
	t.Parallel()
	for _, ok := range []string{"release", "debug", "qa-2", "a"} {
		assert.True(t, ValidVariant(ok), ok)
	}
	for _, bad := range []string{"", "Release", "2fast", "a_b", "abcdefghijklmnopqrstu", "re lease"} {
		assert.False(t, ValidVariant(bad), bad)
	}
}

func TestBinaryFileName(t *testing.T) {
	t.Parallel()
	name := "1.4.0 (build/7)"
	b := Binary{AppKey: "driver-app", Variant: "debug"}
	b.VersionCode, b.VersionName = 42, &name
	assert.Equal(t, "driver-app-1.4.0__build_7_-debug.apk", b.FileName())
	b.VersionName = nil
	assert.Equal(t, "driver-app-42-debug.apk", b.FileName())
}

func TestUpdateURLNames(t *testing.T) {
	t.Parallel()
	code, key, ok := UpdateURLNames("https://platform.example.com/app-updates/acme/driver-app/manifest")
	assert.True(t, ok)
	assert.Equal(t, []string{"acme", "driver-app"}, []string{code, key})
	code, key, ok = UpdateURLNames("http://10.0.2.2:8080/base/app-updates/acme/driver-app/manifest")
	assert.True(t, ok, "behind a path prefix")
	assert.Equal(t, []string{"acme", "driver-app"}, []string{code, key})
	for _, other := range []string{"", "https://u.expo.dev/abc", "https://x.example.com/app-updates/acme/driver-app", "::"} {
		_, _, ok := UpdateURLNames(other)
		assert.False(t, ok, other)
	}
}

// A token is 256 bits from crypto/rand, stored as its SHA-256 only.
func TestInstallTokens(t *testing.T) {
	t.Parallel()
	tok, hash, err := NewInstallToken()
	require.NoError(t, err)
	assert.True(t, ValidInstallToken(tok))
	assert.Len(t, tok, 43)
	assert.Equal(t, HashInstallToken(tok), hash)
	assert.NotContains(t, hash, tok)
	other, _, err := NewInstallToken()
	require.NoError(t, err)
	assert.NotEqual(t, tok, other)
	assert.True(t, InstallTokenMatches(hash, HashInstallToken(tok)))
	assert.False(t, InstallTokenMatches(hash, HashInstallToken(other)))
	assert.False(t, InstallTokenMatches(hash, ""))
	assert.Equal(t, "https://p.example.com/app-updates/acme/driver-app/install/"+tok, InstallURL("https://p.example.com/", "acme", "driver-app", tok))
}

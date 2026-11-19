package apktest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWriteForTools writes APKs into the directory FC_APKTEST_OUT names, to
// check them with apksigner and aapt2 by hand (package apk, "Verified
// against"); skipped without it.
func TestWriteForTools(t *testing.T) {
	dir := os.Getenv("FC_APKTEST_OUT")
	if dir == "" {
		t.Skip("FC_APKTEST_OUT names no directory")
	}
	s, err := NewSigner("acme driver-app")
	require.NoError(t, err)
	base := Options{
		VersionCode: 42, VersionName: "1.4.0", MinSDK: 24, TargetSDK: 36, RuntimeVersion: "1.4.0",
		UpdateURL: "https://platform.example.com/app-updates/acme/driver-app/manifest", Channel: "production", KeyID: "main",
	}
	for name, o := range map[string]Options{"unsigned.apk": base, "v2.apk": with(base, s, false), "v2v3.apk": with(base, s, true)} {
		b, err := Build(o)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), b, 0o600))
	}
	t.Logf("certificate SHA-256 %s", s.CertificateSHA256())
}

func with(o Options, s *Signer, v3 bool) Options {
	o.Signer, o.V3 = s, v3
	return o
}

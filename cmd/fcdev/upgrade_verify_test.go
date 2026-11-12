package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verifyAsset = "fcdev-v1.2.3-linux-amd64.tar.gz"

var verifyArchive = []byte("the archive bytes")

func sha256Hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// fakeRelease serves its files from memory.
type fakeRelease struct {
	rel   *release
	files map[string][]byte
}

func newFakeRelease(assets ...string) *fakeRelease {
	f := &fakeRelease{rel: &release{version: "1.2.3", assets: map[string]string{}}, files: map[string][]byte{}}
	for _, a := range assets {
		f.rel.assets[a] = "https://release.test/" + a
	}
	return f
}

func (f *fakeRelease) get(_ context.Context, url string) ([]byte, error) {
	if b, ok := f.files[strings.TrimPrefix(url, "https://release.test/")]; ok {
		return b, nil
	}
	return nil, errors.New("404")
}

func runVerify(t *testing.T, f *fakeRelease, goos string, skip bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := verifyReleaseAsset(context.Background(), f.rel, verifyAsset, verifyArchive, "flowcatalyst/flowcatalyst", goos, skip, f.get, &out)
	return out.String(), err
}

// withFakeCosign puts a cosign on PATH that records its arguments and exits
// with the given status; with none given, PATH holds no cosign.
func withFakeCosign(t *testing.T, exit int) (argsFile string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if exit < 0 {
		return ""
	}
	if runtime.GOOS == "windows" {
		t.Skip("fake cosign is a shell script")
	}
	argsFile = filepath.Join(t.TempDir(), "args")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\necho cosign says no >&2\nexit " + string(rune('0'+exit)) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cosign"), []byte(script), 0o755))
	return argsFile
}

func TestVerifyReleaseAsset_Checksum(t *testing.T) {
	good := []byte(sha256Hex(verifyArchive) + "  " + verifyAsset + "\n")

	t.Run("matching sidecar passes", func(t *testing.T) {
		withFakeCosign(t, -1)
		f := newFakeRelease(verifyAsset+".sha256", verifyAsset)
		f.files[verifyAsset+".sha256"] = good
		out, err := runVerify(t, f, "darwin", false)
		require.NoError(t, err)
		assert.Contains(t, out, "sha256 verified")
	})

	t.Run("missing sidecar is an error", func(t *testing.T) {
		f := newFakeRelease(verifyAsset)
		_, err := runVerify(t, f, "darwin", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--insecure-skip-checksum")
	})

	t.Run("missing sidecar proceeds with the opt-out", func(t *testing.T) {
		withFakeCosign(t, -1)
		f := newFakeRelease(verifyAsset)
		out, err := runVerify(t, f, "darwin", true)
		require.NoError(t, err)
		assert.Contains(t, out, "warning")
	})

	t.Run("unreadable sidecar (download fails) is an error, waived by the opt-out", func(t *testing.T) {
		withFakeCosign(t, -1)
		f := newFakeRelease(verifyAsset + ".sha256") // listed but not downloadable
		_, err := runVerify(t, f, "darwin", false)
		require.Error(t, err)
		_, err = runVerify(t, f, "darwin", true)
		require.NoError(t, err)
	})

	t.Run("garbage sidecar is unreadable: error, waived by the opt-out", func(t *testing.T) {
		withFakeCosign(t, -1)
		f := newFakeRelease(verifyAsset + ".sha256")
		f.files[verifyAsset+".sha256"] = []byte("<html>not found</html>\n")
		_, err := runVerify(t, f, "darwin", false)
		require.ErrorIs(t, err, errSidecarUnreadable)
		_, err = runVerify(t, f, "darwin", true)
		require.NoError(t, err)
	})

	t.Run("a mismatch is an error even with the opt-out", func(t *testing.T) {
		f := newFakeRelease(verifyAsset + ".sha256")
		f.files[verifyAsset+".sha256"] = []byte(sha256Hex([]byte("something else")) + "  x\n")
		for _, skip := range []bool{false, true} {
			_, err := runVerify(t, f, "darwin", skip)
			require.ErrorIs(t, err, errChecksumMismatch, "skip=%v", skip)
		}
	})
}

func TestVerifyReleaseAsset_Cosign(t *testing.T) {
	sidecar := []byte(sha256Hex(verifyArchive) + "  " + verifyAsset + "\n")
	signed := func() *fakeRelease {
		f := newFakeRelease(verifyAsset, verifyAsset+".sha256", verifyAsset+".sig", verifyAsset+".pem")
		f.files[verifyAsset+".sha256"] = sidecar
		f.files[verifyAsset+".sig"] = []byte("sig")
		f.files[verifyAsset+".pem"] = []byte("pem")
		return f
	}

	t.Run("cosign absent: proceeds with one notice", func(t *testing.T) {
		withFakeCosign(t, -1)
		out, err := runVerify(t, signed(), "linux", false)
		require.NoError(t, err)
		assert.Equal(t, 1, strings.Count(out, "signature not verified"))
	})

	t.Run("cosign present and the signature verifies", func(t *testing.T) {
		args := withFakeCosign(t, 0)
		out, err := runVerify(t, signed(), "linux", false)
		require.NoError(t, err)
		assert.Contains(t, out, "cosign signature verified")
		b, err := os.ReadFile(args)
		require.NoError(t, err)
		got := string(b)
		assert.Contains(t, got, "verify-blob")
		assert.Contains(t, got, "--certificate-oidc-issuer\nhttps://token.actions.githubusercontent.com")
		assert.Contains(t, got, "--certificate-identity-regexp\n"+cosignIdentityRegexp("flowcatalyst/flowcatalyst"))
	})

	t.Run("a cosign that fails is an error", func(t *testing.T) {
		withFakeCosign(t, 1)
		_, err := runVerify(t, signed(), "linux", false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "signature verification failed")
		assert.Contains(t, err.Error(), "cosign says no")
	})

	t.Run("a failing signature is an error even with the checksum opt-out", func(t *testing.T) {
		withFakeCosign(t, 1)
		_, err := runVerify(t, signed(), "linux", true)
		require.Error(t, err)
	})

	t.Run("cosign present but no .sig/.pem published: notice, proceeds", func(t *testing.T) {
		withFakeCosign(t, 1) // would fail if it were run
		f := newFakeRelease(verifyAsset, verifyAsset+".sha256")
		f.files[verifyAsset+".sha256"] = sidecar
		out, err := runVerify(t, f, "linux", false)
		require.NoError(t, err)
		assert.Contains(t, out, "signature not verified")
	})

	t.Run("not checked off Linux", func(t *testing.T) {
		withFakeCosign(t, 1)
		_, err := runVerify(t, signed(), "darwin", false)
		require.NoError(t, err)
	})
}

// The identity the verification (and the release notes) use is the workflow's
// own path in the repository it runs in.
func TestCosignIdentityRegexp(t *testing.T) {
	re := regexp.MustCompile(cosignIdentityRegexp("flowcatalyst/flowcatalyst"))
	for _, ok := range []string{
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fcdev.yml@refs/tags/fcdev/v0.17.0",
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fcdev.yml@refs/heads/main",
	} {
		assert.True(t, re.MatchString(ok), ok)
	}
	for _, bad := range []string{
		"https://github.com/flowcatalyst/flowcatalyst-go/.github/workflows/release-fcdev.yml@refs/tags/fcdev/v0.17.0",
		"https://github.com/evil/flowcatalyst/.github/workflows/release-fcdev.yml@refs/tags/fcdev/v0.17.0",
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/other.yml@refs/tags/fcdev/v0.17.0",
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fcdev.yml@refs/heads/feature",
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fcdev.yml@refs/tags/other/v1",
		"https://github.com/flowcatalyst/flowcatalyst/.github/workflows/release-fcdev.yml@refs/heads/main/x",
	} {
		assert.False(t, re.MatchString(bad), bad)
	}
}

func TestUpgradeFlagRegistered(t *testing.T) {
	f := newUpgradeCmd().Flags().Lookup("insecure-skip-checksum")
	require.NotNil(t, f)
	assert.Equal(t, "false", f.DefValue)
}

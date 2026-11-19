//go:build integration

package main

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk/apktest"
	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
)

var linkLine = regexp.MustCompile(`(?m)^(http\S+/install/[A-Za-z0-9_-]{43})$`)

// The pipeline's and the operator's path end to end: a pipeline publishes a
// debug and a regular build with `fcdev updates binaries publish` (the
// application, app key, channel and variant read from the APKs), an operator
// makes a link and replaces it, and a person's download is the newest build.
func TestFcdevBinariesAndLinks(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	dir := t.TempDir()
	const rv = "c0ffee0123456789abcdef0123456789abcdef01"
	release := filepath.Join(dir, "app-release.apk")
	debug := filepath.Join(dir, "app-debug.apk")
	require.NoError(t, os.WriteFile(release, aut.APK(t, w, "release", w.AppA, "driver-app", "production", rv, 140, "1.4.0", false), 0o600))
	require.NoError(t, os.WriteFile(debug, aut.APK(t, w, "debug", w.AppA, "driver-app", "production", rv, 140, "1.4.0", true), 0o600))

	cmd := newUpdatesCmd()
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"binaries", "inspect", release})
	require.NoError(t, cmd.Execute(), buf.String())
	out := buf.String()
	assert.Contains(t, out, "com.example.driver")
	assert.Contains(t, out, rv+" (from fingerprint)")
	assert.Contains(t, out, "production")

	out, err := fcdevAgainst(t, w, "sa-a", "binaries", "publish", debug, "--message", "debug build", "--commit", "abc123")
	require.NoError(t, err, out)
	assert.Contains(t, out, "uploading")
	assert.Contains(t, out, "published debug 1.4.0 (140) of "+w.AppA+"/driver-app to channel production")
	out, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", release)
	require.NoError(t, err, out)
	assert.Contains(t, out, "published release 1.4.0 (140)")
	out, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", release)
	require.NoError(t, err, out)
	assert.Contains(t, out, "the platform has")
	assert.Contains(t, out, "already published release")

	// What the APK says wins over a flag that contradicts it.
	_, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", release, "--channel", "staging")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expo-channel-name")
	_, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", debug, "--variant", "release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "debuggable")
	bad := filepath.Join(dir, "broken.apk")
	require.NoError(t, os.WriteFile(bad, []byte("not an apk"), 0o600))
	_, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_AN_APK")

	out, err = fcdevAgainst(t, w, "sa-a", "binaries", "list", "--app", w.AppA)
	require.NoError(t, err, out)
	assert.Contains(t, out, "release")
	assert.Contains(t, out, "debug")

	// The pipeline cannot make a link; the operator can, and replace it.
	_, err = fcdevAgainst(t, w, "sa-a", "links", "create", "--app", w.AppA, "--app-key", "driver-app", "--channel", "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
	out, err = fcdevAgainst(t, w, "operator", "links", "create", "--app", w.AppA, "--app-key", "driver-app", "--channel", "production")
	require.NoError(t, err, out)
	first := linkLine.FindString(out)
	require.NotEmpty(t, first, out)
	resp, body := w.Get(strings.TrimPrefix(first, w.InstallBase), nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, string(body), "Regular build")
	assert.Contains(t, string(body), "Debug build")

	out, err = fcdevAgainst(t, w, "operator", "links", "list", "--app", w.AppA)
	require.NoError(t, err, out)
	linkID := regexp.MustCompile(`lnk_\S+`).FindString(out)
	require.NotEmpty(t, linkID, out)
	assert.NotContains(t, out, strings.TrimPrefix(first, w.InstallBase+"/app-updates/"+w.AppA+"/driver-app/install/"))

	out, err = fcdevAgainst(t, w, "operator", "links", "replace", linkID)
	require.NoError(t, err, out)
	second := linkLine.FindString(out)
	require.NotEmpty(t, second, out)
	assert.NotEqual(t, first, second)
	resp, _ = w.Get(strings.TrimPrefix(first, w.InstallBase), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the old address stopped working")
	resp, body = w.Get(strings.TrimPrefix(second, w.InstallBase)+"/release.apk", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	rel, err := os.ReadFile(release)
	require.NoError(t, err)
	assert.Equal(t, rel, body)

	out, err = fcdevAgainst(t, w, "operator", "links", "remove", linkID)
	require.NoError(t, err, out)
	resp, _ = w.Get(strings.TrimPrefix(second, w.InstallBase), nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// `binaries has` answers an update pipeline by its exit status, and
// `publish --require-binary` refuses an update no binary carries.
func TestFcdevBinariesHasAndRequireBinary(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	const rv = "4.0.0"
	has := []string{"binaries", "has", "--app", w.AppA, "--app-key", "driver-app", "--channel", "production", "--runtime-version", rv}
	exitOf := func(err error) int {
		if err == nil {
			return 0
		}
		var ee *exitError
		require.True(t, errors.As(err, &ee), "%v", err)
		return ee.Code
	}
	out, err := fcdevAgainst(t, w, "sa-a", has...)
	assert.Equal(t, 1, exitOf(err), out)
	assert.Contains(t, out, "no: no binary of runtime 4.0.0")

	exp := t.TempDir()
	writeExport(t, exp, goodMeta(), goodFiles())
	out, err = fcdevAgainst(t, w, "sa-a", "publish", exp, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production",
		"--os", "android", "--runtime-version", rv, "--require-binary")
	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "NO_BINARY_FOR_RUNTIME_VERSION")
	out, err = fcdevAgainst(t, w, "sa-a", "publish", exp, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production",
		"--os", "android", "--runtime-version", rv)
	require.NoError(t, err, out)
	assert.Contains(t, out, "warning: no binary of runtime version 4.0.0 is published on channel production")

	apkPath := filepath.Join(t.TempDir(), "app.apk")
	// The runtime version as a @string resource this time (the Expo config
	// plugin's way for a plain runtimeVersion).
	b, err := apktest.Build(apktest.Options{
		VersionCode: 3, VersionName: "4.0.0", RuntimeVersion: rv, Signer: aut.Signer(t, "release"),
		UpdateURL: w.Server.URL + "/app-updates/" + w.AppA + "/driver-app/manifest", Channel: "production",
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(apkPath, b, 0o600))
	out, err = fcdevAgainst(t, w, "sa-a", "binaries", "publish", apkPath)
	require.NoError(t, err, out)

	out, err = fcdevAgainst(t, w, "sa-a", has...)
	assert.Equal(t, 0, exitOf(err), out)
	assert.Contains(t, out, "yes: 1 binaries of runtime 4.0.0")
	out, err = fcdevAgainst(t, w, "sa-a", "publish", exp, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production",
		"--os", "android", "--runtime-version", rv, "--require-binary")
	require.NoError(t, err, out)
	assert.NotContains(t, out, "warning: no binary")

	out, err = fcdevAgainst(t, w, "sa-b", has...)
	assert.Equal(t, 2, exitOf(err), out)
}

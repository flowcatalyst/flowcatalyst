//go:build integration

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
)

// fcdevAgainst runs `fcdev updates ...` against w, authenticating as the
// principal named by clientID (the front server's token endpoint hands the
// client id back as the bearer the world knows).
func fcdevAgainst(t *testing.T, w *aut.World, clientID string, args ...string) (string, error) {
	t.Helper()
	t.Setenv(fnCLICredentialsEnv, filepath.Join(t.TempDir(), "none.json"))
	t.Setenv("FC_PROFILE", "")
	target, err := url.Parse(w.Server.URL)
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(target)
	front := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			require.NoError(t, r.ParseForm())
			rw.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(rw).Encode(map[string]any{"access_token": r.PostForm.Get("client_id"), "token_type": "Bearer", "expires_in": 3600})
			return
		}
		proxy.ServeHTTP(rw, r)
	}))
	t.Cleanup(front.Close)
	cmd := newUpdatesCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append(args, "--platform", front.URL, "--client-id", clientID, "--client-secret", "s"))
	err = cmd.Execute()
	return out.String() + errOut.String(), err
}

// The build-machine path end to end: an expo export published with `fcdev
// updates publish` as the application's service account reaches a phone,
// verified as the client verifies it; a rollout and a rollback follow.
func TestFcdevUpdatesPublishAndRollback(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	dir := t.TempDir()
	writeExport(t, dir, goodMeta(), goodFiles())
	writeExport(t, dir, goodMeta(), map[string]string{"expoConfig.json": `{"name":"Driver","runtimeVersion":"4.0.0"}`})

	out, err := fcdevAgainst(t, w, "sa-a", "publish", dir, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production",
		"--message", "first", "--commit", "abc123", "--json")
	require.NoError(t, err, out)
	assert.Contains(t, out, "4 files, 4 to upload")

	for _, platform := range []string{"android", "ios"} {
		phone := aut.NewPhone(w, aut.Branch{App: w.AppA, AppKey: "driver-app", Platform: platform, RuntimeVersion: "4.0.0", Channel: "production"})
		a := phone.Ask(t)
		require.NotNil(t, a.Manifest, string(a.Raw))
		files := phone.FetchAssets(t, a.Manifest)
		assert.Equal(t, platform+" bundle", string(files[a.Manifest.LaunchAsset.Key]))
		assert.Equal(t, map[string]any{"name": "Driver", "runtimeVersion": "4.0.0"}, a.Manifest.Extra["expoClient"])
	}

	// Publishing the same export again uploads nothing.
	out, err = fcdevAgainst(t, w, "sa-a", "publish", dir, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production", "--os", "android", "--rollout", "25")
	require.NoError(t, err, out)
	assert.Contains(t, out, "3 files, 0 to upload")
	assert.Contains(t, out, "rollout 25%")

	// Another application's service account cannot publish to this one.
	out, err = fcdevAgainst(t, w, "sa-b", "publish", dir, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SCOPE_FORBIDDEN", out)

	out, err = fcdevAgainst(t, w, "sa-a", "list", "--app", w.AppA, "--os", "android")
	require.NoError(t, err, out)
	assert.Contains(t, out, "25%")

	out, err = fcdevAgainst(t, w, "sa-a", "rollback", "--app", w.AppA, "--app-key", "driver-app", "--os", "android",
		"--runtime-version", "4.0.0", "--channel", "production", "--to-embedded")
	require.NoError(t, err, out)
	assert.Contains(t, out, "rolled back to the embedded bundle")
	phone := aut.NewPhone(w, aut.Branch{App: w.AppA, AppKey: "driver-app", Platform: "android", RuntimeVersion: "4.0.0", Channel: "production"})
	assert.Equal(t, "rollBackToEmbedded", phone.Ask(t).Directive["type"])

	out, err = fcdevAgainst(t, w, "sa-a", "branches", "--app", w.AppA)
	require.NoError(t, err, out)
	assert.Contains(t, out, "ROLLBACK_TO_EMBEDDED")
}

// No runtime version, or a malformed export: nothing is uploaded or published.
func TestFcdevUpdatesPublishRefusesWithoutRuntimeVersion(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	dir := t.TempDir()
	writeExport(t, dir, goodMeta(), goodFiles())
	out, err := fcdevAgainst(t, w, "sa-a", "publish", dir, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no runtime version", out)

	bad := t.TempDir()
	m := goodMeta()
	m["version"] = 7
	writeExport(t, bad, m, goodFiles())
	_, err = fcdevAgainst(t, w, "sa-a", "publish", bad, "--app", w.AppA, "--app-key", "driver-app", "--channel", "production", "--runtime-version", "1.0.0")
	require.Error(t, err)

	st, body := w.Do("operator", http.MethodGet, "/api/app-updates/updates?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusOK, st)
	assert.Contains(t, string(body), `"total":0`)
	st, body = w.Do("operator", http.MethodPost, "/api/app-updates/files/"+w.AppA+"/missing", map[string]any{"sha256": []string{aut.SHA256Hex([]byte("android bundle"))}})
	require.Equal(t, http.StatusOK, st)
	assert.Contains(t, string(body), aut.SHA256Hex([]byte("android bundle")), "nothing was uploaded")
}

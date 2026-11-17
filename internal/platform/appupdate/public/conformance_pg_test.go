//go:build integration

package public_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func branch(w *aut.World) aut.Branch {
	return aut.Branch{App: w.AppA, AppKey: "driver-app", Platform: "android", RuntimeVersion: "1.0.0", Channel: "production"}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err)
	return v
}

// The whole life of a branch, driven as the client drives it: nothing yet,
// an update (manifest verified, every asset fetched and hash-checked), a
// second one that replaces it, a staged rollout that holds a phone back, a
// rollback to the previous update, and a rollback to the embedded bundle.
func TestConformance_ClientLifecycle(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branch(w)
	phone := aut.NewPhone(w, b)

	a := phone.Ask(t)
	require.Equal(t, http.StatusOK, a.Status, string(a.Raw))
	assert.Equal(t, "noUpdateAvailable", a.Directive["type"])

	// First update.
	e1 := aut.NewExport("v1", 3)
	u1 := w.Publish("sa-a", b, e1, 100)
	a = phone.Ask(t)
	require.NotNil(t, a.Manifest, string(a.Raw))
	assert.Equal(t, u1.ManifestID, a.Manifest.ID)
	assert.Equal(t, u1.CreatedAt[:23], a.Manifest.CreatedAt[:23])
	assert.Equal(t, map[string]any{"name": "Driver", "slug": "driver-app"}, a.Manifest.Extra["expoClient"])
	files := phone.FetchAssets(t, a.Manifest)
	require.Len(t, files, 4)
	assert.Equal(t, e1.Bundle, files[a.Manifest.LaunchAsset.Key])
	assert.Len(t, a.Manifest.LaunchAsset.Key, 32, "the key is the file's MD5, as expo export names assets")
	assert.True(t, strings.HasPrefix(a.Manifest.LaunchAsset.URL, "assets/"), "relative to the manifest URL by default")

	// The same phone on iOS-style upper-case id, and with the app key in the
	// header form iOS sends, gets the same answer.
	ios := *phone
	ios.ClientID = strings.ToUpper(phone.ClientID)
	assert.Equal(t, a.Manifest.ID, ios.Ask(t).Manifest.ID)

	// A second update replaces it: a new id and a later createdAt.
	e2 := aut.NewExport("v2", 2)
	u2 := w.Publish("sa-a", b, e2, 100)
	a2 := phone.Ask(t)
	require.NotNil(t, a2.Manifest)
	assert.Equal(t, u2.ManifestID, a2.Manifest.ID)
	assert.True(t, parseTime(t, a2.Manifest.CreatedAt).After(parseTime(t, a.Manifest.CreatedAt)), "the client loads only a newer createdAt")
	phone.FetchAssets(t, a2.Manifest)

	// A staged rollout: a phone outside it is held on v2, a phone inside gets v3.
	e3 := aut.NewExport("v3", 1)
	u3 := w.Publish("sa-a", b, e3, 30)
	var in, out *aut.Phone
	for in == nil || out == nil {
		p := aut.NewPhone(w, b)
		if appupdate.Bucket(u3.ID, p.ClientID) < 30 {
			in = p
		} else {
			out = p
		}
	}
	assert.Equal(t, u3.ManifestID, in.Ask(t).Manifest.ID)
	assert.Equal(t, u2.ManifestID, out.Ask(t).Manifest.ID, "held back")
	assert.Equal(t, u2.ManifestID, out.Ask(t).Manifest.ID, "stable: the same phone gets the same answer")
	noID := aut.NewPhone(w, b)
	noID.ClientID = ""
	assert.Equal(t, u2.ManifestID, noID.Ask(t).Manifest.ID, "a phone without an id is not in a partial rollout")

	// Raising the rollout keeps the phone that was in, and reaches the other.
	st, body := w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u3.ID+"/rollout", map[string]int{"rolloutPercent": 100})
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Equal(t, u3.ManifestID, in.Ask(t).Manifest.ID)
	assert.Equal(t, u3.ManifestID, out.Ask(t).Manifest.ID)

	// Roll back to the previous update: v2's files as a new, newer update.
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/rollback", map[string]any{
		"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion,
		"channel": b.Channel, "to": "PREVIOUS",
	})
	require.Equal(t, http.StatusCreated, st, string(body))
	var rb struct {
		Update aut.UpdateRow `json:"update"`
	}
	require.NoError(t, json.Unmarshal(body, &rb))
	require.NotNil(t, rb.Update.RepublishedFrom)
	assert.Equal(t, u2.ID, *rb.Update.RepublishedFrom)
	a4 := in.Ask(t)
	require.NotNil(t, a4.Manifest)
	assert.Equal(t, rb.Update.ManifestID, a4.Manifest.ID)
	assert.NotEqual(t, u2.ManifestID, a4.Manifest.ID, "a new manifest id: the phone has v2's id stored already")
	assert.True(t, parseTime(t, a4.Manifest.CreatedAt).After(parseTime(t, u3.CreatedAt)), "newer than the update it replaces")
	assert.Equal(t, a2.Manifest.LaunchAsset, a4.Manifest.LaunchAsset, "v2's files")
	in.FetchAssets(t, a4.Manifest)

	// Roll back to the embedded bundle: a signed directive, commitTime later
	// than every update (the client acts only on a newer one).
	st, body = w.Do("operator", http.MethodPost, "/api/app-updates/rollback", map[string]any{
		"applicationCode": b.App, "appKey": b.AppKey, "platform": b.Platform, "runtimeVersion": b.RuntimeVersion,
		"channel": b.Channel, "to": "EMBEDDED", "message": "bad build",
	})
	require.Equal(t, http.StatusCreated, st, string(body))
	a5 := in.Ask(t)
	require.Equal(t, "rollBackToEmbedded", a5.Directive["type"], string(a5.Raw))
	commit := a5.Directive["parameters"].(map[string]any)["commitTime"].(string)
	assert.True(t, parseTime(t, commit).After(parseTime(t, a4.Manifest.CreatedAt)))

	// A new publish after the rollback is served again.
	u6 := w.Publish("sa-a", b, aut.NewExport("v6", 1), 100)
	assert.Equal(t, u6.ManifestID, in.Ask(t).Manifest.ID)
}

// Each phone gets only its own branch: another platform, runtime version,
// channel or app key is another branch.
func TestConformance_BranchesAreSeparate(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branch(w)
	u := w.Publish("sa-a", b, aut.NewExport("only", 1), 100)
	assert.Equal(t, u.ManifestID, aut.NewPhone(w, b).Ask(t).Manifest.ID)
	for _, other := range []aut.Branch{
		{App: b.App, AppKey: b.AppKey, Platform: "ios", RuntimeVersion: b.RuntimeVersion, Channel: b.Channel},
		{App: b.App, AppKey: b.AppKey, Platform: b.Platform, RuntimeVersion: "1.0.1", Channel: b.Channel},
		{App: b.App, AppKey: b.AppKey, Platform: b.Platform, RuntimeVersion: b.RuntimeVersion, Channel: "staging"},
		{App: b.App, AppKey: "yard-app", Platform: b.Platform, RuntimeVersion: b.RuntimeVersion, Channel: b.Channel},
		{App: w.AppB, AppKey: b.AppKey, Platform: b.Platform, RuntimeVersion: b.RuntimeVersion, Channel: b.Channel},
	} {
		a := aut.NewPhone(w, other).Ask(t)
		assert.Equal(t, "noUpdateAvailable", a.Directive["type"], "%+v", other)
	}
}

// A client with code signing off still gets an answer (signed: the header is
// there and ignored); one asking for another key id is refused clearly.
func TestConformance_SigningKeyNegotiation(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branch(w)
	w.Publish("sa-a", b, aut.NewExport("k", 1), 100)

	unsigned := aut.NewPhone(w, b)
	unsigned.KeyID = ""
	a := unsigned.Ask(t)
	require.NotNil(t, a.Manifest)
	aut.VerifyLikeClient(t, a.Parts["manifest"].Headers["expo-signature"], a.Parts["manifest"].Body, aut.KeyID, w.CertPEM)

	root := aut.NewPhone(w, b)
	root.KeyID = "root" // a client whose codeSigningMetadata names no keyid
	a = root.Ask(t)
	require.Equal(t, http.StatusBadRequest, a.Status)
	assert.JSONEq(t, `{"error":"UNKNOWN_KEY_ID","message":"the key id in expo-expect-signature is not a signing key of this server"}`, string(a.Raw))
}

func get(t *testing.T, w *aut.World, path string, header map[string]string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, w.Server.URL+path, nil)
	require.NoError(t, err)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	resp, err := w.Server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, resp.Header, string(body)
}

// The public routes are hostile-facing: malformed input is refused before any
// work, in one shape; another application's existence and files are not
// revealed; only GET.
func TestConformance_HostileRequests(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branch(w)
	u := w.Publish("sa-a", b, aut.NewExport("h", 2), 100)
	good := aut.NewPhone(w, b)
	a := good.Ask(t)
	require.NotNil(t, a.Manifest)
	assetPath := appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/" + a.Manifest.LaunchAsset.URL
	manifestPath := appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/manifest"

	headers := func(mut func(map[string]string)) map[string]string {
		h := map[string]string{}
		for k, v := range good.ManifestRequest(t).Header {
			h[k] = v[0]
		}
		mut(h)
		return h
	}
	type tc struct {
		name   string
		path   string
		hdr    map[string]string
		status int
		code   string
	}
	cases := []tc{
		{"protocol version missing", manifestPath, headers(func(h map[string]string) { delete(h, "Expo-Protocol-Version") }), 406, "UNSUPPORTED_PROTOCOL_VERSION"},
		{"protocol version 0", manifestPath, headers(func(h map[string]string) { h["Expo-Protocol-Version"] = "0" }), 406, "UNSUPPORTED_PROTOCOL_VERSION"},
		{"json only", manifestPath, headers(func(h map[string]string) { h["Accept"] = "application/expo+json, application/json" }), 406, "NOT_ACCEPTABLE"},
		{"multipart refused by q=0", manifestPath, headers(func(h map[string]string) { h["Accept"] = "multipart/mixed;q=0, application/json" }), 406, "NOT_ACCEPTABLE"},
		{"no platform", manifestPath, headers(func(h map[string]string) { delete(h, "Expo-Platform") }), 400, "INVALID_REQUEST"},
		{"web platform", manifestPath, headers(func(h map[string]string) { h["Expo-Platform"] = "web" }), 400, "INVALID_REQUEST"},
		{"no runtime version", manifestPath, headers(func(h map[string]string) { delete(h, "Expo-Runtime-Version") }), 400, "INVALID_REQUEST"},
		{"runtime version too long", manifestPath, headers(func(h map[string]string) { h["Expo-Runtime-Version"] = strings.Repeat("1", 129) }), 400, "INVALID_REQUEST"},
		{"runtime version traversal", manifestPath, headers(func(h map[string]string) { h["Expo-Runtime-Version"] = "../1" }), 400, "INVALID_REQUEST"},
		{"no channel", manifestPath, headers(func(h map[string]string) { delete(h, "Expo-Channel-Name") }), 400, "INVALID_REQUEST"},
		{"channel with a quote", manifestPath, headers(func(h map[string]string) { h["Expo-Channel-Name"] = `prod'--` }), 400, "INVALID_REQUEST"},
		{"malformed expect-signature", manifestPath, headers(func(h map[string]string) { h["Expo-Expect-Signature"] = `sig, keyid=main` }), 400, "INVALID_REQUEST"},
		{"other algorithm", manifestPath, headers(func(h map[string]string) { h["Expo-Expect-Signature"] = `sig, keyid="main", alg="ecdsa-p256-sha256"` }), 400, "UNSUPPORTED_SIGNATURE_ALGORITHM"},
		{"upper-case app code", appupdate.PublicPrefix + "/ACME/driver-app/manifest", headers(func(map[string]string) {}), 400, "INVALID_REQUEST"},
		{"bad app key", appupdate.PublicPrefix + "/" + b.App + "/Driver_App/manifest", headers(func(map[string]string) {}), 400, "INVALID_REQUEST"},
		{"unknown route", appupdate.PublicPrefix + "/" + b.App + "/driver-app/other", nil, 404, "NOT_FOUND"},
		{"prefix only", appupdate.PublicPrefix + "/", nil, 404, "NOT_FOUND"},
		{"asset bad hash", appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/assets/" + strings.Repeat("Z", 64), nil, 404, "NOT_FOUND"},
		{"asset unknown", appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/assets/" + strings.Repeat("0", 64), nil, 404, "NOT_FOUND"},
		{"asset of another app", appupdate.PublicPrefix + "/" + w.AppB + "/" + b.AppKey + "/" + a.Manifest.LaunchAsset.URL, nil, 404, "NOT_FOUND"},
		{"asset of another app key", appupdate.PublicPrefix + "/" + b.App + "/yard-app/" + a.Manifest.LaunchAsset.URL, nil, 404, "NOT_FOUND"},
		{"asset traversal", appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/assets/..%2F..%2Fetc%2Fpasswd", nil, 404, "NOT_FOUND"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, h, body := get(t, w, c.path, c.hdr)
			require.Equal(t, c.status, st, body)
			assert.Equal(t, "application/json", h.Get("Content-Type"))
			assert.Equal(t, "no-store", h.Get("Cache-Control"))
			var e map[string]string
			require.NoError(t, json.Unmarshal([]byte(body), &e), body)
			assert.Equal(t, c.code, e["error"])
			assert.Len(t, e, 2, "only error and message")
		})
	}

	// An unknown application is answered exactly as a known one with no
	// updates on the branch: a signed noUpdateAvailable.
	unknown := *good
	unknown.ManifestURL = w.Server.URL + appupdate.PublicPrefix + "/no-such-app-" + strings.ToLower(uuid.NewString()[:8]) + "/driver-app/manifest"
	known := aut.NewPhone(w, aut.Branch{App: w.AppB, AppKey: b.AppKey, Platform: b.Platform, RuntimeVersion: b.RuntimeVersion, Channel: b.Channel})
	ua, ka := unknown.Ask(t), known.Ask(t)
	require.Equal(t, "noUpdateAvailable", ua.Directive["type"])
	assert.Equal(t, ka.Parts["directive"], ua.Parts["directive"], "same bytes, same signature")

	// Only GET.
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req, err := http.NewRequest(m, w.Server.URL+manifestPath, strings.NewReader("{}"))
		require.NoError(t, err)
		resp, err := w.Server.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode, m)
	}

	// The asset route: conditional request, and no authentication needed.
	st, h, _ := get(t, w, assetPath, nil)
	require.Equal(t, http.StatusOK, st)
	etag := h.Get("ETag")
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	st, _, _ = get(t, w, assetPath, map[string]string{"If-None-Match": etag})
	assert.Equal(t, http.StatusNotModified, st)

	// An inactive application's phones are told nothing is available, and
	// its files are not served.
	_, err := testpg.Pool(t).Exec(t.Context(), `UPDATE app_applications SET active = false WHERE id = $1`, w.AppAID)
	require.NoError(t, err)
	assert.Equal(t, "noUpdateAvailable", good.Ask(t).Directive["type"])
	st, _, _ = get(t, w, assetPath, nil)
	assert.Equal(t, http.StatusNotFound, st)
	_ = u
}

// Without a signing key the routes fail closed.
func TestConformance_NoKeyIsUnavailable(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{NoKeys: true})
	a := aut.NewPhone(w, branch(w)).Ask(t)
	require.Equal(t, http.StatusServiceUnavailable, a.Status)
	assert.JSONEq(t, `{"error":"APP_UPDATES_UNCONFIGURED","message":"updates are not available"}`, string(a.Raw))
}

// A published update's manifest bytes and signature do not change between
// requests (served from the cache), and a rollout change does not change them.
func TestConformance_ManifestIsStable(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := branch(w)
	u := w.Publish("sa-a", b, aut.NewExport("s", 2), 100)
	p := aut.NewPhone(w, b)
	first := p.Ask(t).Parts["manifest"]
	st, body := w.Do("sa-a", http.MethodPut, "/api/app-updates/updates/"+u.ID+"/rollout", map[string]int{"rolloutPercent": 100})
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Equal(t, first, p.Ask(t).Parts["manifest"])
}

//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk/apktest"
	aut "github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/appupdatetest"
)

const fingerprint = "8b0e5c1f2a9d4e7b6c3a1f0e9d8c7b6a5f4e3d2c"

func releaseAPK(t *testing.T, w *aut.World, code int64, name string) []byte {
	t.Helper()
	return aut.APK(t, w, "release", w.AppA, "driver-app", "production", fingerprint, code, name, false)
}

func publishBody(w *aut.World, variant string, b []byte) map[string]any {
	return map[string]any{
		"applicationCode": w.AppA, "appKey": "driver-app", "platform": "android", "channel": "production",
		"variant": variant, "sha256": aut.SHA256Hex(b),
	}
}

// An upload is parsed: what the platform records is what the APK says.
// Uploading the same file again changes nothing.
func TestBinaryUpload_RecordsWhatTheAPKSays(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := releaseAPK(t, w, 42, "1.4.0")
	st, body := w.UploadBinary("sa-a", w.AppA, b)
	require.Equal(t, http.StatusOK, st, string(body))
	var got struct {
		SHA256  string `json:"sha256"`
		Size    int64  `json:"sizeBytes"`
		Created bool   `json:"created"`
		Facts   struct {
			PackageName       string `json:"packageName"`
			VersionCode       int64  `json:"versionCode"`
			VersionName       string `json:"versionName"`
			RuntimeVersion    string `json:"runtimeVersion"`
			UpdateURL         string `json:"updateUrl"`
			UpdateChannel     string `json:"updateChannel"`
			UpdateKeyID       string `json:"updateKeyId"`
			SigningCertSHA256 string `json:"signingCertificateSha256"`
			DebugCertificate  bool   `json:"debugCertificate"`
			SigningSchemes    []int  `json:"signingSchemes"`
		} `json:"facts"`
	}
	require.NoError(t, json.Unmarshal(body, &got))
	assert.True(t, got.Created)
	assert.Equal(t, int64(len(b)), got.Size)
	assert.Equal(t, "com.example.driver", got.Facts.PackageName)
	assert.Equal(t, int64(42), got.Facts.VersionCode)
	assert.Equal(t, "1.4.0", got.Facts.VersionName)
	assert.Equal(t, fingerprint, got.Facts.RuntimeVersion, "read from assets/fingerprint, as expo-updates reads it")
	assert.Equal(t, "production", got.Facts.UpdateChannel)
	assert.Equal(t, aut.KeyID, got.Facts.UpdateKeyID)
	assert.True(t, strings.HasSuffix(got.Facts.UpdateURL, "/app-updates/"+w.AppA+"/driver-app/manifest"))
	assert.Equal(t, aut.Signer(t, "release").CertificateSHA256(), got.Facts.SigningCertSHA256)
	assert.False(t, got.Facts.DebugCertificate)
	assert.Equal(t, []int{2}, got.Facts.SigningSchemes)

	st, body = w.Do("sa-a", http.MethodGet, "/api/app-updates/binary-files/"+w.AppA+"/"+aut.SHA256Hex(b), nil)
	require.Equal(t, http.StatusOK, st, string(body))
	st, body = w.UploadBinary("sa-a", w.AppA, b)
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Contains(t, string(body), `"created":false`)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM app_binary_files WHERE application_id = $1`, w.AppAID))
}

// A file that is not a well-formed, signed APK is refused and recorded nowhere.
func TestBinaryUpload_RefusesWhatIsNotASignedAPK(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{MaxBinaryBytes: 64 << 10})
	unsigned, err := apktest.Build(apktest.Options{VersionCode: 1, Fingerprint: fingerprint})
	require.NoError(t, err)
	corrupted := releaseAPK(t, w, 1, "1.0.0")
	corrupted[100] ^= 0xff
	big, err := apktest.Build(apktest.Options{VersionCode: 1, Filler: 80 << 10, Signer: aut.Signer(t, "release")})
	require.NoError(t, err)

	for name, c := range map[string]struct {
		body []byte
		code string
	}{
		"not a zip":             {[]byte("this is not an APK at all, just some text"), "NOT_AN_APK"},
		"unsigned":              {unsigned, "UNSIGNED"},
		"changed after signing": {corrupted, "SIGNATURE_INVALID"},
		"too large":             {big, "FILE_TOO_LARGE"},
	} {
		t.Run(name, func(t *testing.T) {
			st, body := w.UploadBinary("sa-a", w.AppA, c.body)
			require.Equal(t, http.StatusBadRequest, st, string(body))
			assert.Equal(t, c.code, errCode(t, body))
		})
	}
	good := releaseAPK(t, w, 1, "1.0.0")
	st, body := w.Do("sa-a", http.MethodPut, "/api/app-updates/binary-files/"+w.AppA+"/"+aut.SHA256Hex([]byte("other")), good)
	require.Equal(t, http.StatusBadRequest, st, string(body))
	assert.Equal(t, "DIGEST_MISMATCH", errCode(t, body))
	assert.Equal(t, 0, count(t, `SELECT count(*) FROM app_binary_files WHERE application_id = $1`, w.AppAID))
	ok, err := w.Store.Exists(context.Background(), aut.SHA256Hex(good))
	require.NoError(t, err)
	assert.False(t, ok, "nothing reached the store")
}

// Publishing writes the binary, its event and its audit row; publishing the
// same file to the same place again is a no-op.
func TestPublishBinary_RecordsAndIsIdempotent(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := releaseAPK(t, w, 42, "1.4.0")
	row := w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", b)
	assert.Equal(t, "driver-app-1.4.0-release.apk", row.FileName)
	assert.Equal(t, fingerprint, *row.Facts.RuntimeVersion)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:binary:published' AND subject = $1`, "platform.appbinary."+row.ID))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'PublishBinaryCommand'`, row.ID))

	st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/binaries", publishBody(w, "release", b))
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Contains(t, string(body), `"created":false`)
	assert.Contains(t, string(body), row.ID)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM app_binaries WHERE application_id = $1`, w.AppAID))
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = 'platform:app-update:binary:published' AND data->>'applicationId' = $1`, w.AppAID))

	// The same file in the debug variant is another binary.
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/binaries", publishBody(w, "debug", b))
	require.Equal(t, http.StatusCreated, st, string(body))
}

// An application's service account publishes its own binaries only, and
// never makes links; a client's administrator and a viewer do neither.
func TestBinaries_Permissions(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	b := releaseAPK(t, w, 1, "1.0.0")
	for _, p := range []string{"sa-b", "client-admin", "viewer", "nobody"} {
		st, body := w.UploadBinary(p, w.AppA, b)
		assert.Equal(t, http.StatusForbidden, st, "%s upload: %s", p, body)
		st, body = w.Do(p, http.MethodPost, "/api/app-updates/binaries", publishBody(w, "release", b))
		assert.Equal(t, http.StatusForbidden, st, "%s publish: %s", p, body)
	}
	w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", b)
	link := map[string]string{"applicationCode": w.AppA, "appKey": "driver-app", "channel": "production"}
	for _, p := range []string{"sa-a", "sa-b", "client-admin", "viewer"} {
		st, body := w.Do(p, http.MethodPost, "/api/app-updates/install-links", link)
		assert.Equal(t, http.StatusForbidden, st, "%s create link: %s", p, body)
	}
	l := w.CreateLink(w.AppA, "driver-app", "production")
	for _, p := range []string{"sa-a", "viewer"} {
		st, _ := w.Do(p, http.MethodPost, "/api/app-updates/install-links/"+l.ID+"/replace", nil)
		assert.Equal(t, http.StatusForbidden, st, p)
		st, _ = w.Do(p, http.MethodDelete, "/api/app-updates/install-links/"+l.ID, nil)
		assert.Equal(t, http.StatusForbidden, st, p)
	}
	st, body := w.Do("sa-a", http.MethodGet, "/api/app-updates/install-links?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Contains(t, string(body), l.ID, "the service account may see that a link exists")
	assert.NotContains(t, string(body), l.Token)
	st, _ = w.Do("sa-b", http.MethodGet, "/api/app-updates/binaries?applicationCode="+w.AppA, nil)
	assert.Equal(t, http.StatusForbidden, st)
}

// The publish rules read the file, not the publisher.
func TestPublishBinary_Rules(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	refuse := func(t *testing.T, variant string, b []byte, mutate func(map[string]any), code string) {
		t.Helper()
		st, body := w.UploadBinary("sa-a", w.AppA, b)
		require.Equal(t, http.StatusOK, st, string(body))
		req := publishBody(w, variant, b)
		if mutate != nil {
			mutate(req)
		}
		st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/binaries", req)
		require.Equal(t, http.StatusConflict, st, string(body))
		assert.Equal(t, code, errCode(t, body))
	}
	debuggable := aut.APK(t, w, "release", w.AppA, "driver-app", "production", fingerprint, 5, "1.0.0", true)
	debugSigned := aut.APK(t, w, "debug", w.AppA, "driver-app", "production", fingerprint, 5, "1.0.0", false)
	t.Run("release must not be debuggable", func(t *testing.T) { refuse(t, "release", debuggable, nil, "RELEASE_IS_DEBUGGABLE") })
	t.Run("release must not be debug-signed", func(t *testing.T) { refuse(t, "release", debugSigned, nil, "RELEASE_SIGNED_WITH_DEBUG_KEY") })
	t.Run("the binary's channel", func(t *testing.T) {
		refuse(t, "release", releaseAPK(t, w, 6, "1.0.0"), func(m map[string]any) { m["channel"] = "staging" }, "CHANNEL_MISMATCH")
	})
	t.Run("the binary's update URL", func(t *testing.T) {
		refuse(t, "release", releaseAPK(t, w, 6, "1.0.0"), func(m map[string]any) { m["appKey"] = "other-app" }, "UPDATE_URL_MISMATCH")
	})
	t.Run("not uploaded", func(t *testing.T) {
		st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/binaries", publishBody(w, "release", []byte("never uploaded")))
		require.Equal(t, http.StatusConflict, st, string(body))
		assert.Equal(t, "FILE_MISSING", errCode(t, body))
	})
	t.Run("android only", func(t *testing.T) {
		req := publishBody(w, "release", releaseAPK(t, w, 6, "1.0.0"))
		req["platform"] = "ios"
		st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/binaries", req)
		require.Equal(t, http.StatusBadRequest, st, string(body))
	})

	// The debug variant takes the debuggable and the debug-signed builds.
	w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "debug", debuggable)
	t.Run("one certificate per place", func(t *testing.T) { refuse(t, "debug", debugSigned, nil, "SIGNING_CERTIFICATE_CHANGED") })
	other, err := apktest.Build(apktest.Options{
		Package: "com.example.other", VersionCode: 9, VersionName: "1.0.0", Fingerprint: fingerprint, Signer: aut.Signer(t, "release"),
	})
	require.NoError(t, err)
	t.Run("one package per place", func(t *testing.T) { refuse(t, "debug", other, nil, "PACKAGE_MISMATCH") })
	sameCode := aut.APK(t, w, "release", w.AppA, "driver-app", "production", fingerprint, 5, "1.0.1", true)
	t.Run("one file per version code", func(t *testing.T) { refuse(t, "debug", sameCode, nil, "VERSION_CODE_TAKEN") })
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM app_binaries WHERE application_id = $1`, w.AppAID))
}

// The newest binary of a place is the one with the highest version code,
// whatever the order they were published in; older ones stay listed.
func TestBinaries_NewestByVersionCode(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	v20 := w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", releaseAPK(t, w, 20, "2.0.0"))
	v10 := w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", releaseAPK(t, w, 10, "1.0.0"))
	dbg := w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "debug",
		aut.APK(t, w, "debug", w.AppA, "driver-app", "production", fingerprint, 15, "1.5.0", true))

	st, body := w.Do("viewer", http.MethodGet, "/api/app-updates/binaries?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusOK, st, string(body))
	var page struct {
		Data  []aut.BinaryRow `json:"data"`
		Total int             `json:"total"`
	}
	require.NoError(t, json.Unmarshal(body, &page))
	require.Equal(t, 3, page.Total)
	newest := map[string]bool{}
	for _, b := range page.Data {
		newest[b.ID] = b.Newest
	}
	assert.Equal(t, map[string]bool{v20.ID: true, v10.ID: false, dbg.ID: true}, newest)

	st, body = w.Do("viewer", http.MethodGet, "/api/app-updates/binaries/"+v10.ID, nil)
	require.Equal(t, http.StatusOK, st, string(body))
	assert.Contains(t, string(body), `"newest":false`)
	st, _ = w.Do("viewer", http.MethodGet, "/api/app-updates/binaries/bin_000000000000x", nil)
	assert.Equal(t, http.StatusNotFound, st)
}

// The question an update pipeline asks, and the strict publish it can choose.
func TestBinaryCheck_AndRequireBinary(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	check := func() (bool, int) {
		st, body := w.Do("sa-a", http.MethodGet, "/api/app-updates/binary-check?applicationCode="+w.AppA+
			"&appKey=driver-app&platform=android&channel=production&runtimeVersion="+fingerprint, nil)
		require.Equal(t, http.StatusOK, st, string(body))
		var res struct {
			Published bool `json:"published"`
			Count     int  `json:"count"`
		}
		require.NoError(t, json.Unmarshal(body, &res))
		return res.Published, res.Count
	}
	b := aut.Branch{App: w.AppA, AppKey: "driver-app", Platform: "android", RuntimeVersion: fingerprint, Channel: "production"}
	e := aut.NewExport("strict", 1)
	w.Upload("sa-a", b.App, e)

	published, n := check()
	assert.False(t, published)
	assert.Equal(t, 0, n)
	strict := aut.PublishBody(b, e, 100, nil)
	strict["requireBinary"] = true
	st, body := w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", strict)
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "NO_BINARY_FOR_RUNTIME_VERSION", errCode(t, body))

	// The default publishes, and says no binary carries the runtime version.
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", aut.PublishBody(b, e, 100, nil))
	require.Equal(t, http.StatusCreated, st, string(body))
	assert.Contains(t, string(body), `"binariesOnBranch":0`)

	w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", releaseAPK(t, w, 1, "1.0.0"))
	published, n = check()
	assert.True(t, published)
	assert.Equal(t, 1, n)
	st, body = w.Do("sa-a", http.MethodPost, "/api/app-updates/updates", strict)
	require.Equal(t, http.StatusCreated, st, string(body))
	assert.Contains(t, string(body), `"binariesOnBranch":1`)
	// Reads of updates carry no count.
	st, body = w.Do("sa-a", http.MethodGet, "/api/app-updates/updates?applicationCode="+w.AppA, nil)
	require.Equal(t, http.StatusOK, st)
	assert.NotContains(t, string(body), "binariesOnBranch")
}

// A link is made once per channel, replaced (the old token dead at once) and
// removed; each is an event and an audit row, none carries the token.
func TestInstallLinks_Lifecycle(t *testing.T) {
	w := aut.NewWorld(t, aut.Options{})
	w.PublishBinary("sa-a", w.AppA, "driver-app", "production", "release", releaseAPK(t, w, 1, "1.0.0"))
	l := w.CreateLink(w.AppA, "driver-app", "production")
	assert.True(t, appupdate.ValidInstallToken(l.Token), l.Token)
	assert.Equal(t, w.InstallBase+"/app-updates/"+w.AppA+"/driver-app/install/"+l.Token, l.URL)
	assert.Equal(t, 1, l.Generation)
	assert.Equal(t, 1, count(t, `SELECT count(*) FROM app_install_links WHERE id = $1 AND token_hash = $2`, l.ID, appupdate.HashInstallToken(l.Token)))

	st, body := w.Do("operator", http.MethodPost, "/api/app-updates/install-links",
		map[string]string{"applicationCode": w.AppA, "appKey": "driver-app", "channel": "production"})
	require.Equal(t, http.StatusConflict, st, string(body))
	assert.Equal(t, "INSTALL_LINK_EXISTS", errCode(t, body))

	page := "/app-updates/" + w.AppA + "/driver-app/install/"
	resp, _ := w.Get(page+l.Token, nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	r := w.ReplaceLink(l.ID)
	assert.Equal(t, l.ID, r.ID)
	assert.Equal(t, 2, r.Generation)
	assert.NotEqual(t, l.Token, r.Token)
	resp, _ = w.Get(page+l.Token, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "the replaced token is dead at once")
	resp, _ = w.Get(page+r.Token, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	st, body = w.Do("operator", http.MethodDelete, "/api/app-updates/install-links/"+l.ID, nil)
	require.Equal(t, http.StatusNoContent, st, string(body))
	resp, _ = w.Get(page+r.Token, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "a removed link is dead at once")
	st, _ = w.Do("operator", http.MethodDelete, "/api/app-updates/install-links/"+l.ID, nil)
	assert.Equal(t, http.StatusNotFound, st)

	for typ, op := range map[string]string{"created": "CreateInstallLinkCommand", "replaced": "ReplaceInstallLinkCommand", "removed": "RemoveInstallLinkCommand"} {
		assert.Equal(t, 1, count(t, `SELECT count(*) FROM msg_events WHERE type = $1 AND subject = $2`,
			"platform:app-update:install-link:"+typ, "platform.appinstalllink."+l.ID), typ)
		assert.Equal(t, 1, count(t, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = $2`, l.ID, op), op)
	}
	for _, tok := range []string{l.Token, r.Token} {
		assert.Equal(t, 0, count(t, `SELECT count(*) FROM msg_events WHERE data::text LIKE '%'||$1||'%'`, tok), "no event carries a token")
		assert.Equal(t, 0, count(t, `SELECT count(*) FROM aud_logs WHERE operation_json::text LIKE '%'||$1||'%'`, tok), "no audit row carries a token")
	}

	// Another application's operator-less view: sa-b does not see A's links.
	st, _ = w.Do("sa-b", http.MethodGet, "/api/app-updates/install-links?applicationCode="+w.AppA, nil)
	assert.Equal(t, http.StatusForbidden, st)
}

//go:build integration

package appupdatetest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate/apk/apktest"
)

var (
	signersOnce sync.Once
	signers     map[string]*apktest.Signer
)

// Signer is one of three signers shared by a test binary (key generation is
// slow): "release" (the app's release key), "other" (a different key) and
// "debug" (subject CN "Android Debug", as the Android SDK's debug keystore).
func Signer(t *testing.T, name string) *apktest.Signer {
	t.Helper()
	signersOnce.Do(func() {
		signers = map[string]*apktest.Signer{}
		for n, cn := range map[string]string{"release": "Acme Release", "other": "Someone Else", "debug": "Android Debug"} {
			s, err := apktest.NewSigner(cn)
			if err != nil {
				panic(err)
			}
			signers[n] = s
		}
	})
	return signers[name]
}

// APK builds an APK of app (its update URL names app and appKey), signed by
// the named signer, on channel, with runtime version rv carried the way Expo's
// fingerprint policy writes it (assets/fingerprint).
func APK(t *testing.T, w *World, signer, app, appKey, channel, rv string, versionCode int64, versionName string, debuggable bool) []byte {
	t.Helper()
	b, err := apktest.Build(apktest.Options{
		Package: "com.example.driver", VersionCode: versionCode, VersionName: versionName, MinSDK: 26, TargetSDK: 36,
		Debuggable: debuggable, Fingerprint: rv, UpdateURL: w.Server.URL + "/app-updates/" + app + "/" + appKey + "/manifest",
		Channel: channel, KeyID: KeyID, Signer: Signer(t, signer),
	})
	require.NoError(t, err)
	return b
}

// UploadBinary uploads b for app as principal and returns the status and body.
func (w *World) UploadBinary(principal, app string, b []byte) (int, []byte) {
	w.T.Helper()
	return w.Do(principal, http.MethodPut, "/api/app-updates/binary-files/"+app+"/"+SHA256Hex(b), b)
}

// BinaryRow is the API's binary answer.
type BinaryRow struct {
	ID        string `json:"id"`
	Variant   string `json:"variant"`
	Channel   string `json:"channel"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"sizeBytes"`
	FileName  string `json:"fileName"`
	Newest    bool   `json:"newest"`
	Facts     struct {
		PackageName       string  `json:"packageName"`
		VersionCode       int64   `json:"versionCode"`
		VersionName       *string `json:"versionName"`
		RuntimeVersion    *string `json:"runtimeVersion"`
		UpdateChannel     *string `json:"updateChannel"`
		SigningCertSHA256 string  `json:"signingCertificateSha256"`
		Debuggable        bool    `json:"debuggable"`
		DebugCertificate  bool    `json:"debugCertificate"`
	} `json:"facts"`
}

// PublishBinary uploads b and publishes it as principal to app/appKey/channel
// in variant, and returns the binary.
func (w *World) PublishBinary(principal, app, appKey, channel, variant string, b []byte) BinaryRow {
	w.T.Helper()
	st, body := w.UploadBinary(principal, app, b)
	require.Equal(w.T, http.StatusOK, st, string(body))
	st, body = w.Do(principal, http.MethodPost, "/api/app-updates/binaries", map[string]any{
		"applicationCode": app, "appKey": appKey, "platform": "android", "channel": channel,
		"variant": variant, "sha256": SHA256Hex(b), "message": "test build", "sourceCommit": "abc123",
	})
	require.Equal(w.T, http.StatusCreated, st, string(body))
	var res struct {
		Binary BinaryRow `json:"binary"`
	}
	require.NoError(w.T, json.Unmarshal(body, &res))
	return res.Binary
}

// Link is a created or replaced install link.
type Link struct {
	ID         string
	URL        string
	Token      string
	Generation int
}

// CreateLink makes the install link of app/appKey/channel as the operator.
func (w *World) CreateLink(app, appKey, channel string) Link {
	w.T.Helper()
	st, body := w.Do("operator", http.MethodPost, "/api/app-updates/install-links",
		map[string]string{"applicationCode": app, "appKey": appKey, "channel": channel})
	require.Equal(w.T, http.StatusCreated, st, string(body))
	return linkOf(w.T, body)
}

// ReplaceLink gives a link a new token as the operator.
func (w *World) ReplaceLink(id string) Link {
	w.T.Helper()
	st, body := w.Do("operator", http.MethodPost, "/api/app-updates/install-links/"+id+"/replace", nil)
	require.Equal(w.T, http.StatusOK, st, string(body))
	return linkOf(w.T, body)
}

func linkOf(t *testing.T, body []byte) Link {
	t.Helper()
	var res struct {
		Link struct {
			ID         string `json:"id"`
			Generation int    `json:"generation"`
		} `json:"link"`
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	require.NoError(t, json.Unmarshal(body, &res))
	return Link{ID: res.Link.ID, URL: res.URL, Token: res.Token, Generation: res.Link.Generation}
}

// Get fetches an address of the world's server (no credentials) with the
// headers given, and returns the response with its body read.
func (w *World) Get(path string, headers map[string]string) (*http.Response, []byte) {
	w.T.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, w.Server.URL+path, nil)
	require.NoError(w.T, err)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := w.Server.Client().Do(req)
	require.NoError(w.T, err)
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

// StoredPath is where the world's file store keeps the file sha256Hex.
func (w *World) StoredPath(sha256Hex string) string {
	return filepath.Join(w.StoreDir, "sha256", sha256Hex[:2], sha256Hex)
}

// CorruptStored flips one byte of a stored file in place.
func (w *World) CorruptStored(sha256Hex string, at int64) {
	w.T.Helper()
	f, err := os.OpenFile(w.StoredPath(sha256Hex), os.O_RDWR, 0)
	require.NoError(w.T, err)
	defer func() { _ = f.Close() }()
	b := make([]byte, 1)
	_, err = f.ReadAt(b, at)
	require.NoError(w.T, err)
	b[0] ^= 0xff
	_, err = f.WriteAt(b, at)
	require.NoError(w.T, err)
}

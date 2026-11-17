package appupdate

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleUpdate() *Update {
	return &Update{
		ID: "upd_0000000000001", ApplicationCode: "acme",
		AppKey: "driver-app", Platform: "android", RuntimeVersion: "1.0.0", Channel: "production",
		Kind: KindUpdate, ManifestID: "0d3f9a2e-5c1b-4f7e-9a6d-2b8c4e1f0a93",
		CreatedAt:  time.Date(2026, 11, 17, 9, 30, 0, 250_000_000, time.UTC),
		ExpoClient: []byte(`{"name":"Driver","runtimeVersion":"1.0.0"}`),
		Assets: []Asset{
			{SHA256: strings.Repeat("ab", 32), MD5: strings.Repeat("1", 32), ContentType: LaunchAssetContentType, IsLaunchAsset: true},
			{SHA256: strings.Repeat("cd", 32), MD5: strings.Repeat("2", 32), FileExtension: ".png", ContentType: "image/png"},
		},
	}
}

// The manifest has every field the clients require (expo-manifests
// ExpoUpdatesManifest: id, createdAt, runtimeVersion, launchAsset; Android
// requires launchAsset.key and each asset's key, fileExtension, url).
func TestManifestJSON(t *testing.T) {
	t.Parallel()
	b, err := ManifestJSON(sampleUpdate(), "")
	require.NoError(t, err)
	var m struct {
		ID             string                     `json:"id"`
		CreatedAt      string                     `json:"createdAt"`
		RuntimeVersion string                     `json:"runtimeVersion"`
		LaunchAsset    map[string]any             `json:"launchAsset"`
		Assets         []map[string]any           `json:"assets"`
		Metadata       map[string]string          `json:"metadata"`
		Extra          map[string]json.RawMessage `json:"extra"`
	}
	require.NoError(t, json.Unmarshal(b, &m))
	assert.Equal(t, "0d3f9a2e-5c1b-4f7e-9a6d-2b8c4e1f0a93", m.ID)
	assert.Equal(t, "2026-11-17T09:30:00.250Z", m.CreatedAt)
	assert.Equal(t, "1.0.0", m.RuntimeVersion)
	assert.Equal(t, map[string]any{
		"hash": "q6urq6urq6urq6urq6urq6urq6urq6urq6urq6urq6s", "key": strings.Repeat("1", 32),
		"contentType": "application/javascript", "url": "assets/" + strings.Repeat("ab", 32),
	}, m.LaunchAsset, "no fileExtension on the launch asset")
	require.Len(t, m.Assets, 1)
	assert.Equal(t, ".png", m.Assets[0]["fileExtension"])
	assert.Equal(t, "assets/"+strings.Repeat("cd", 32), m.Assets[0]["url"])
	assert.Equal(t, map[string]string{}, m.Metadata)
	assert.JSONEq(t, `{"name":"Driver","runtimeVersion":"1.0.0"}`, string(m.Extra["expoClient"]))

	abs, err := ManifestJSON(sampleUpdate(), "https://updates.example.com/")
	require.NoError(t, err)
	assert.Contains(t, string(abs), `"url":"https://updates.example.com/app-updates/acme/driver-app/assets/`+strings.Repeat("cd", 32)+`"`)

	again, err := ManifestJSON(sampleUpdate(), "")
	require.NoError(t, err)
	assert.Equal(t, b, again, "the bytes are deterministic, so a signature over them can be cached")

	u := sampleUpdate()
	u.Assets = u.Assets[1:]
	_, err = ManifestJSON(u, "")
	require.Error(t, err, "no launch asset")
}

func TestDirectives(t *testing.T) {
	t.Parallel()
	assert.JSONEq(t, `{"type":"noUpdateAvailable"}`, string(NoUpdateAvailableJSON()))
	rb := &Update{Kind: KindRollbackToEmbedded, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 6_000_000, time.UTC)}
	assert.JSONEq(t, `{"type":"rollBackToEmbedded","parameters":{"commitTime":"2026-01-02T03:04:05.006Z"}}`, string(RollBackToEmbeddedJSON(rb)))
}

// The multipart body: parts the clients find by content-disposition name,
// part header names in lower case (iOS looks expo-signature up
// case-sensitively), and the closing delimiter followed by CRLF (the iOS
// reader needs it).
func TestWriteMultipart(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ct, err := WriteMultipart(&buf, []Part{
		{Name: "manifest", Body: []byte(`{"id":"x"}`), Signature: `sig="abc", keyid="main", alg="rsa-v1_5-sha256"`},
		{Name: "extensions", Body: ExtensionsJSON()},
	})
	require.NoError(t, err)
	mt, params, err := mime.ParseMediaType(ct)
	require.NoError(t, err)
	assert.Equal(t, "multipart/mixed", mt)
	boundary := params["boundary"]
	require.NotEmpty(t, boundary)
	body := buf.String()
	assert.True(t, strings.HasPrefix(body, "--"+boundary+"\r\n"))
	assert.True(t, strings.HasSuffix(body, "\r\n--"+boundary+"--\r\n"))
	assert.Regexp(t, regexp.MustCompile(`(?m)^expo-signature: sig="abc", keyid="main", alg="rsa-v1_5-sha256"\r$`), body)
	assert.Regexp(t, regexp.MustCompile(`(?m)^content-disposition: form-data; name="manifest"\r$`), body)
	assert.NotContains(t, body, "Expo-Signature")

	r := multipart.NewReader(strings.NewReader(body), boundary)
	p, err := r.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "manifest", p.FormName())
	got, err := io.ReadAll(p)
	require.NoError(t, err)
	assert.Equal(t, `{"id":"x"}`, string(got), "the part body is exactly the signed bytes")
	p, err = r.NextPart()
	require.NoError(t, err)
	assert.Equal(t, "extensions", p.FormName())
	_, err = r.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

func TestHashBase64URL(t *testing.T) {
	t.Parallel()
	// SHA-256 of the empty string.
	h, err := HashBase64URL("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
	require.NoError(t, err)
	assert.Equal(t, "47DEQpj8HBSa-_TImW-5JCeuQeRkm5NMpJWZG3hSuFU", h)
	_, err = HashBase64URL("zz")
	require.Error(t, err)
}

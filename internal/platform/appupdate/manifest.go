package appupdate

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// HashBase64URL turns a hex SHA-256 into the manifest's asset hash: base64url
// without padding (the clients compare UpdatesUtils.toBase64Url(sha256)).
func HashBase64URL(sha256Hex string) (string, error) {
	b, err := hex.DecodeString(sha256Hex)
	if err != nil || len(b) != 32 {
		return "", errors.New("not a hex SHA-256")
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// manifestAsset is one asset of the manifest body. Field order is the wire
// order; fileExtension is omitted for the launch asset only.
type manifestAsset struct {
	Hash          string `json:"hash"`
	Key           string `json:"key"`
	ContentType   string `json:"contentType"`
	FileExtension string `json:"fileExtension,omitempty"`
	URL           string `json:"url"`
}

type manifestBody struct {
	ID             string            `json:"id"`
	CreatedAt      string            `json:"createdAt"`
	RuntimeVersion string            `json:"runtimeVersion"`
	LaunchAsset    manifestAsset     `json:"launchAsset"`
	Assets         []manifestAsset   `json:"assets"`
	Metadata       map[string]string `json:"metadata"`
	Extra          map[string]any    `json:"extra"`
}

// AssetURL is the address of a file in a manifest: relative to the manifest
// URL ("assets/<sha256>", which both clients resolve against the update URL)
// when base is empty, else base + "/app-updates/<app>/<appKey>/assets/<sha256>".
func AssetURL(base, applicationCode, appKey, sha256Hex string) string {
	if base == "" {
		return "assets/" + sha256Hex
	}
	return strings.TrimRight(base, "/") + PublicPrefix + "/" + applicationCode + "/" + appKey + "/assets/" + sha256Hex
}

// PublicPrefix is the public routes' path prefix.
const PublicPrefix = "/app-updates"

// ManifestJSON is the manifest body of update u (kind UPDATE, assets loaded,
// ApplicationCode set). The bytes are what is signed and sent; they depend
// only on the stored update and assetBase, so a signature over them can be
// cached.
func ManifestJSON(u *Update, assetBase string) ([]byte, error) {
	if u.Kind != KindUpdate {
		return nil, errors.New("not an update")
	}
	m := manifestBody{
		ID:             u.ManifestID,
		CreatedAt:      FormatTime(u.CreatedAt),
		RuntimeVersion: u.RuntimeVersion,
		Assets:         []manifestAsset{},
		Metadata:       map[string]string{},
		Extra:          map[string]any{},
	}
	launch := false
	for _, a := range u.Assets {
		h, err := HashBase64URL(a.SHA256)
		if err != nil {
			return nil, err
		}
		ma := manifestAsset{
			Hash: h, Key: a.MD5, ContentType: a.ContentType,
			URL: AssetURL(assetBase, u.ApplicationCode, u.AppKey, a.SHA256),
		}
		if a.IsLaunchAsset {
			if launch {
				return nil, errors.New("two launch assets")
			}
			launch = true
			m.LaunchAsset = ma
			continue
		}
		ma.FileExtension = a.FileExtension
		m.Assets = append(m.Assets, ma)
	}
	if !launch {
		return nil, errors.New("no launch asset")
	}
	if len(u.ExpoClient) > 0 {
		m.Extra["expoClient"] = json.RawMessage(u.ExpoClient)
	}
	return json.Marshal(m)
}

// NoUpdateAvailableJSON is the noUpdateAvailable directive body.
func NoUpdateAvailableJSON() []byte { return []byte(`{"type":"noUpdateAvailable"}`) }

// RollBackToEmbeddedJSON is the rollBackToEmbedded directive of a rollback row:
// its commitTime is the row's, so a phone acts on it only while its launched
// update is older (LoaderSelectionPolicyFilterAware).
func RollBackToEmbeddedJSON(u *Update) []byte {
	return []byte(`{"type":"rollBackToEmbedded","parameters":{"commitTime":"` + FormatTime(u.CreatedAt) + `"}}`)
}

// ExtensionsJSON is the extensions part sent with a manifest: no asset
// request headers.
func ExtensionsJSON() []byte { return []byte(`{"assetRequestHeaders":{}}`) }

// Part is one part of a multipart answer.
type Part struct {
	// Name is the content-disposition name: manifest, extensions or directive.
	Name string
	Body []byte
	// Signature is the expo-signature header value; "" sends none.
	Signature string
}

// WriteMultipart writes parts as a multipart/mixed body and returns its
// content type (with the boundary). Part header names are written in lower
// case: the iOS client looks expo-signature up case-sensitively
// (FileDownloader.swift, optionalValue(forKey: "expo-signature")), so Go's
// canonical "Expo-Signature" would be missed. The body ends with the closing
// delimiter and CRLF, which the iOS reader requires.
func WriteMultipart(buf *bytes.Buffer, parts []Part) (string, error) {
	w := multipart.NewWriter(buf)
	for _, p := range parts {
		// A map literal, not Set: Set canonicalises the key.
		h := textproto.MIMEHeader{
			"content-disposition": {`form-data; name="` + p.Name + `"`},
			"content-type":        {"application/json"},
		}
		if p.Signature != "" {
			h["expo-signature"] = []string{p.Signature}
		}
		pw, err := w.CreatePart(h)
		if err != nil {
			return "", err
		}
		if _, err := pw.Write(p.Body); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return "multipart/mixed; boundary=" + w.Boundary(), nil
}

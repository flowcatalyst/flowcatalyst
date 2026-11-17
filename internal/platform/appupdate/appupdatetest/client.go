//go:build integration

package appupdatetest

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

// Phone is one installation of a phone app, asking as expo-updates 58 does
// (android/.../loader/FileDownloader.kt createRequestForRemoteUpdate and
// createRequestForAsset; ios/EXUpdates/AppLoader/FileDownloader.swift).
type Phone struct {
	ManifestURL    string
	Platform       string
	RuntimeVersion string
	// RequestHeaders are the app config's updates.requestHeaders.
	RequestHeaders map[string]string
	// ClientID is EAS-Client-ID (iOS upper-case, Android lower-case).
	ClientID string
	// KeyID and CertPEM are the app's codeSigningMetadata.keyid and embedded
	// certificate; empty KeyID: code signing not configured.
	KeyID   string
	CertPEM []byte
	HTTP    *http.Client
}

// NewPhone is an Android phone with code signing on channel, for b.
func NewPhone(w *World, b Branch) *Phone {
	return &Phone{
		ManifestURL:    w.Server.URL + appupdate.PublicPrefix + "/" + b.App + "/" + b.AppKey + "/manifest",
		Platform:       b.Platform,
		RuntimeVersion: b.RuntimeVersion,
		RequestHeaders: map[string]string{"expo-channel-name": b.Channel},
		ClientID:       uuid.NewString(),
		KeyID:          KeyID,
		CertPEM:        w.CertPEM,
		HTTP:           w.Server.Client(),
	}
}

// ManifestRequest is the request the phone sends.
func (p *Phone) ManifestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, p.ManifestURL, nil)
	require.NoError(t, err)
	h := req.Header
	h.Set("Accept", "multipart/mixed,application/expo+json,application/json")
	h.Set("Expo-Platform", p.Platform)
	h.Set("Expo-Protocol-Version", "1")
	h.Set("Expo-API-Version", "1")
	h.Set("Expo-Updates-Environment", "BARE")
	h.Set("Expo-JSON-Error", "true")
	h.Set("EAS-Client-ID", p.ClientID)
	h.Set("Accept-Encoding", "zstd, br, gzip")
	if p.RuntimeVersion != "" {
		h.Set("Expo-Runtime-Version", p.RuntimeVersion)
	}
	for k, v := range p.RequestHeaders {
		h.Set(k, v)
	}
	if p.KeyID != "" {
		// CodeSigningConfiguration.getAcceptSignatureHeader / createAcceptSignatureHeader.
		h.Set("expo-expect-signature", `sig, keyid="`+p.KeyID+`", alg="rsa-v1_5-sha256"`)
	}
	return req
}

// Answer is a manifest response as the client reads it.
type Answer struct {
	Status   int
	Header   http.Header
	Raw      []byte
	Parts    map[string]ClientPart
	Manifest *Manifest
	// Directive is the directive part's JSON, when there is one.
	Directive map[string]any
}

// ClientPart is one multipart part as the iOS reader sees it: headers keyed
// exactly as written, the body bytes.
type ClientPart struct {
	Headers map[string]string
	Body    []byte
}

// Manifest is the manifest body.
type Manifest struct {
	ID             string            `json:"id"`
	CreatedAt      string            `json:"createdAt"`
	RuntimeVersion string            `json:"runtimeVersion"`
	LaunchAsset    ManifestAsset     `json:"launchAsset"`
	Assets         []ManifestAsset   `json:"assets"`
	Metadata       map[string]string `json:"metadata"`
	Extra          map[string]any    `json:"extra"`
}

// ManifestAsset is one asset entry.
type ManifestAsset struct {
	Hash          string  `json:"hash"`
	Key           string  `json:"key"`
	ContentType   string  `json:"contentType"`
	FileExtension *string `json:"fileExtension"`
	URL           string  `json:"url"`
}

var dateShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`)

// Ask sends the manifest request and checks the answer as the client does:
// the outer headers, the multipart body read by both clients' parsers, the
// signature of the manifest or directive part verified over its exact bytes
// with the embedded certificate, and the manifest's required fields.
func (p *Phone) Ask(t *testing.T) *Answer {
	t.Helper()
	resp, err := p.HTTP.Do(p.ManifestRequest(t))
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	a := &Answer{Status: resp.StatusCode, Header: resp.Header, Raw: raw}
	if resp.StatusCode != http.StatusOK {
		return a
	}
	// UpdateFactory.getUpdate: no expo-protocol-version is "Legacy manifests
	// are no longer supported"; only 0 and 1 are read.
	require.Equal(t, "1", resp.Header.Get("expo-protocol-version"))
	require.Equal(t, "0", resp.Header.Get("expo-sfv-version"))
	require.Equal(t, "private, max-age=0", resp.Header.Get("cache-control"))
	require.Empty(t, resp.Header.Get("content-encoding"))

	mt, params, err := mime.ParseMediaType(resp.Header.Get("content-type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/mixed", mt)
	boundary := params["boundary"]
	require.NotEmpty(t, boundary)

	// The iOS reader (UpdatesMultipartStreamReader), ported; and the Android
	// one is OkHttp's MultipartReader, for which Go's reader stands in: both
	// must see the same parts and bytes.
	ios, ok := ReadAllPartsIOS(raw, boundary)
	require.True(t, ok, "the iOS multipart reader must complete (closing delimiter + CRLF)")
	a.Parts = map[string]ClientPart{}
	for _, part := range ios {
		name := contentDispositionName(part.Headers)
		require.NotEmpty(t, name)
		a.Parts[name] = part
	}
	mr := multipart.NewReader(bytes.NewReader(raw), boundary)
	n := 0
	for {
		gp, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(gp)
		require.NoError(t, err)
		require.Equal(t, a.Parts[gp.FormName()].Body, body, "both readers see the same %s bytes", gp.FormName())
		n++
	}
	require.Len(t, a.Parts, n)

	signed := ""
	switch {
	case a.Parts["manifest"].Body != nil:
		signed = "manifest"
		require.Nil(t, a.Parts["directive"].Body, "one of manifest and directive")
	case a.Parts["directive"].Body != nil:
		signed = "directive"
	default:
		t.Fatalf("no manifest and no directive part")
	}
	part := a.Parts[signed]
	require.Equal(t, "application/json", part.Headers["content-type"])
	if p.KeyID != "" {
		// iOS reads the part header with a case-sensitive key.
		sigHeader, ok := part.Headers["expo-signature"]
		require.True(t, ok, "the %s part carries expo-signature (lower case: iOS looks it up case-sensitively)", signed)
		VerifyLikeClient(t, sigHeader, part.Body, p.KeyID, p.CertPEM)
	}
	if signed == "directive" {
		require.NoError(t, json.Unmarshal(part.Body, &a.Directive))
		switch a.Directive["type"] {
		case "noUpdateAvailable":
		case "rollBackToEmbedded":
			params, _ := a.Directive["parameters"].(map[string]any)
			ct, _ := params["commitTime"].(string)
			require.Regexp(t, dateShape, ct, "parameters.commitTime in the one shape Android parses")
		default:
			t.Fatalf("unknown directive %v", a.Directive["type"])
		}
		return a
	}
	var m Manifest
	require.NoError(t, json.Unmarshal(part.Body, &m))
	_, err = uuid.Parse(m.ID)
	require.NoError(t, err, "id must be a UUID")
	require.Regexp(t, dateShape, m.CreatedAt, "createdAt in the one shape Android parses")
	require.Equal(t, p.RuntimeVersion, m.RuntimeVersion)
	require.NotEmpty(t, m.LaunchAsset.Key, "Android requires launchAsset.key")
	require.NotEmpty(t, m.LaunchAsset.URL)
	require.NotEmpty(t, m.LaunchAsset.Hash)
	for _, as := range m.Assets {
		require.NotNil(t, as.FileExtension, "both clients require fileExtension on an asset")
		require.True(t, strings.HasPrefix(*as.FileExtension, "."))
		name := as.Key + *as.FileExtension // UpdatesUtils.createFilenameForAsset / isSafeFilename
		require.NotContains(t, name, "/")
		require.NotContains(t, name, `\`)
		require.NotEmpty(t, as.URL)
		require.NotEmpty(t, as.Hash)
	}
	require.NotNil(t, m.Metadata)
	ext := a.Parts["extensions"]
	require.NotNil(t, ext.Body, "the extensions part accompanies the manifest")
	var exts map[string]any
	require.NoError(t, json.Unmarshal(ext.Body, &exts))
	a.Manifest = &m
	return a
}

// VerifyLikeClient is CodeSigningConfiguration.validateSignatureInternal with
// the embedded certificate only (no chain): parse expo-signature, require its
// keyid to be the configured one, check the certificate as CertificateChain
// does (validity now, self-signed, key usage digital signature, extended key
// usage code signing), then SHA256withRSA over the bytes.
func VerifyLikeClient(t *testing.T, header string, body []byte, keyID string, certPEMBytes []byte) {
	t.Helper()
	d, err := appupdate.ParseSFVDictionary(header)
	require.NoError(t, err)
	require.Equal(t, "string", d["sig"].Kind, "sig is a string item")
	gotKey := appupdate.DefaultClientKeyID
	if v, ok := d["keyid"]; ok && v.Kind == "string" {
		gotKey = v.Str
	}
	require.Equal(t, keyID, gotKey, "Key with keyid from signature not found in client configuration")
	if v, ok := d["alg"]; ok {
		require.Equal(t, appupdate.AlgRSASHA256, v.Str)
	}
	block, _ := pem.Decode(certPEMBytes)
	require.NotNil(t, block)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	now := time.Now()
	require.False(t, now.Before(cert.NotBefore) || now.After(cert.NotAfter), "checkValidity")
	require.Equal(t, cert.Issuer.String(), cert.Subject.String(), "root must be self-signed")
	require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature), "verify(own key)")
	require.NotZero(t, cert.KeyUsage&x509.KeyUsageDigitalSignature, "keyUsage[0]")
	hasCodeSigning := false
	for _, u := range cert.ExtKeyUsage {
		hasCodeSigning = hasCodeSigning || u == x509.ExtKeyUsageCodeSigning
	}
	require.True(t, hasCodeSigning, "extendedKeyUsage contains 1.3.6.1.5.5.7.3.3")
	sig, err := base64.StdEncoding.DecodeString(d["sig"].Str)
	require.NoError(t, err)
	// Android verifies bodyString.toByteArray(): the part read as UTF-8 and
	// encoded again, which is the same bytes only for valid UTF-8.
	require.True(t, bytes.Equal([]byte(string(body)), body))
	sum := sha256.Sum256(body)
	require.NoError(t, rsa.VerifyPKCS1v15(cert.PublicKey.(*rsa.PublicKey), crypto.SHA256, sum[:], sig), "SHA256withRSA verifies")
}

// FetchAssets downloads every asset of the manifest the way the client does
// (the URL resolved against the manifest URL, ExpoUpdatesUpdate
// .resolveManifestAssetUrls; the asset request headers) and checks each body's
// base64url SHA-256 against the manifest (UpdatesUtils.verifySHA256AndWriteToFile).
func (p *Phone) FetchAssets(t *testing.T, m *Manifest) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	base, err := url.Parse(p.ManifestURL)
	require.NoError(t, err)
	for _, as := range append([]ManifestAsset{m.LaunchAsset}, m.Assets...) {
		ref, err := url.Parse(as.URL)
		require.NoError(t, err)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base.ResolveReference(ref).String(), nil)
		require.NoError(t, err)
		req.Header.Set("Expo-Platform", p.Platform)
		req.Header.Set("Expo-Protocol-Version", "1")
		req.Header.Set("Expo-API-Version", "1")
		req.Header.Set("Expo-Updates-Environment", "BARE")
		req.Header.Set("EAS-Client-ID", p.ClientID)
		req.Header.Set("Accept", "*/*")
		req.Header.Set("Accept-Encoding", "zstd, br, gzip")
		req.Header.Set("Expo-Runtime-Version", p.RuntimeVersion)
		for k, v := range p.RequestHeaders {
			req.Header.Set(k, v)
		}
		resp, err := p.HTTP.Do(req)
		require.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		require.Equal(t, as.ContentType, resp.Header.Get("Content-Type"), "the asset is served with its manifest content type")
		require.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
		sum := sha256.Sum256(body)
		require.Equal(t, as.Hash, base64.RawURLEncoding.EncodeToString(sum[:]), "base64url SHA-256 matches the manifest")
		out[as.Key] = body
	}
	return out
}

func contentDispositionName(h map[string]string) string {
	for k, v := range h {
		if strings.EqualFold(k, "content-disposition") { // stringValueForCaseInsensitiveKey
			_, params, err := mime.ParseMediaType(v)
			if err == nil {
				return params["name"]
			}
		}
	}
	return ""
}

// ReadAllPartsIOS is expo-updates' iOS UpdatesMultipartStreamReader.readAllParts,
// ported: the first delimiter "--b\r\n" (no preceding CRLF needed), then
// "\r\n--b\r\n", closed by "\r\n--b--\r\n". It returns false where the Swift
// returns false (the input ended before the closing delimiter).
func ReadAllPartsIOS(data []byte, boundary string) ([]ClientPart, bool) {
	first := []byte("--" + boundary + "\r\n")
	rest := []byte("\r\n--" + boundary + "\r\n")
	closing := []byte("\r\n--" + boundary + "--\r\n")
	delimiter := first
	chunkStart := 0
	var out []ClientPart
	for {
		idx := indexFrom(data, delimiter, chunkStart)
		isClose := false
		if idx < 0 {
			isClose = true
			idx = indexFrom(data, closing, chunkStart)
		}
		if idx < 0 {
			return out, false
		}
		if chunkStart > 0 {
			out = append(out, emitIOS(data[chunkStart:idx]))
		}
		if isClose {
			return out, true
		}
		chunkStart = idx + len(delimiter)
		if bytes.Equal(delimiter, first) {
			delimiter = rest
		}
	}
}

func indexFrom(data, sep []byte, from int) int {
	if from > len(data) {
		return -1
	}
	i := bytes.Index(data[from:], sep)
	if i < 0 {
		return -1
	}
	return from + i
}

// emitIOS splits a chunk at the first blank line: headers (each line split at
// its first ':', the value trimmed, the key kept as written), then the body.
func emitIOS(chunk []byte) ClientPart {
	marker := []byte("\r\n\r\n")
	before, after, ok := bytes.Cut(chunk, marker)
	if !ok {
		return ClientPart{Body: chunk}
	}
	headers := map[string]string{}
	for line := range strings.SplitSeq(string(before), "\r\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		headers[k] = strings.TrimSpace(v)
	}
	return ClientPart{Headers: headers, Body: after}
}

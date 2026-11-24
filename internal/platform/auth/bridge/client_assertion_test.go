package bridge

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // test: the x5t thumbprint
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// tokenCapture is what the fake token endpoint received.
type tokenCapture struct {
	form      url.Values
	basicUser string
	hasBasic  bool
}

// tokenEndpoint is a fake IdP: discovery plus a /token that records each
// request and answers a bearer token.
type tokenEndpoint struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []tokenCapture
}

func newTokenEndpoint(t *testing.T) *tokenEndpoint {
	t.Helper()
	te := &tokenEndpoint{}
	mux := http.NewServeMux()
	te.srv = httptest.NewServer(mux)
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer": te.srv.URL, "authorization_endpoint": te.srv.URL + "/auth",
			"token_endpoint": te.srv.URL + "/token", "jwks_uri": te.srv.URL + "/jwks",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		u, _, ok := r.BasicAuth()
		te.mu.Lock()
		te.reqs = append(te.reqs, tokenCapture{form: r.PostForm, basicUser: u, hasBasic: ok})
		te.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"at","token_type":"Bearer","expires_in":3600}`))
	})
	t.Cleanup(te.srv.Close)
	return te
}

func (te *tokenEndpoint) last(t *testing.T) tokenCapture {
	t.Helper()
	te.mu.Lock()
	defer te.mu.Unlock()
	require.NotEmpty(t, te.reqs)
	return te.reqs[len(te.reqs)-1]
}

func (te *tokenEndpoint) idp(method identityprovider.ClientAuthMethod) *identityprovider.IdentityProvider {
	issuer, client := te.srv.URL, "entra-client-id"
	return &identityprovider.IdentityProvider{
		ID: "idp_test1", Code: "entra", Type: identityprovider.TypeOIDC,
		OIDCIssuerURL: &issuer, OIDCClientID: &client, OIDCClientAuthMethod: method,
	}
}

func testEnc(t *testing.T) *encryption.Service {
	t.Helper()
	k, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(k)
	require.NoError(t, err)
	return enc
}

func TestExchangeClientSecretSendsASecretNotAnAssertion(t *testing.T) {
	te := newTokenEndpoint(t)
	enc := testEnc(t)
	b := NewBridge(nil, nil, enc)
	idp := te.idp(identityprovider.ClientAuthSecret)
	ref, err := enc.Encrypt("s3cr3t")
	require.NoError(t, err)
	idp.OIDCClientSecretRef = &ref

	r, _, err := b.buildResolved(context.Background(), idp, nil)
	require.NoError(t, err)
	_, err = r.Exchange(context.Background(), "the-code", "https://app/cb", "verifier")
	require.NoError(t, err)

	got := te.last(t)
	assert.Empty(t, got.form.Get("client_assertion"))
	assert.Empty(t, got.form.Get("client_assertion_type"))
	// The secret is sent in the form or as Basic credentials (oauth2 detects
	// which the provider takes); either way it is the secret, not an assertion.
	if got.hasBasic {
		assert.Equal(t, "entra-client-id", got.basicUser)
	} else {
		assert.Equal(t, "s3cr3t", got.form.Get("client_secret"))
	}
}

func TestExchangeClientSecretUnsetIsAPublicClient(t *testing.T) {
	te := newTokenEndpoint(t)
	b := NewBridge(nil, nil, nil)
	r, _, err := b.buildResolved(context.Background(), te.idp(""), nil) // "" = CLIENT_SECRET
	require.NoError(t, err)
	_, err = r.Exchange(context.Background(), "c", "https://app/cb", "v")
	require.NoError(t, err)
	got := te.last(t)
	assert.Empty(t, got.form.Get("client_secret"))
	assert.Empty(t, got.form.Get("client_assertion"))
}

func TestExchangeCertificateSendsAVerifiableAssertion(t *testing.T) {
	te := newTokenEndpoint(t)
	enc := testEnc(t)
	cert, err := identityprovider.GenerateCertificate(enc, "idp_test1", "entra", time.Now())
	require.NoError(t, err)
	cert.Status = identityprovider.CertificateActive
	b := NewBridge(nil, nil, enc)

	r, _, err := b.buildResolved(context.Background(), te.idp(identityprovider.ClientAuthCertificate), cert)
	require.NoError(t, err)
	_, err = r.Exchange(context.Background(), "the-code", "https://app/cb", "verifier")
	require.NoError(t, err)
	_, err = r.Exchange(context.Background(), "the-code2", "https://app/cb", "verifier")
	require.NoError(t, err)

	te.mu.Lock()
	reqs := append([]tokenCapture(nil), te.reqs...)
	te.mu.Unlock()
	require.Len(t, reqs, 2)

	block, _ := pem.Decode([]byte(cert.CertificatePEM))
	x509Cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	pub := x509Cert.PublicKey.(*rsa.PublicKey)
	s1 := sha1.Sum(block.Bytes) //nolint:gosec // test
	s256 := sha256.Sum256(block.Bytes)

	jtis := map[string]bool{}
	for _, got := range reqs {
		assert.False(t, got.hasBasic, "no Basic header for an assertion method")
		assert.Empty(t, got.form.Get("client_secret"))
		assert.Equal(t, "entra-client-id", got.form.Get("client_id"))
		assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", got.form.Get("client_assertion_type"))
		assert.Equal(t, "verifier", got.form.Get("code_verifier"))

		claims := jwt.MapClaims{}
		parsed, err := jwt.ParseWithClaims(got.form.Get("client_assertion"), claims,
			func(*jwt.Token) (any, error) { return pub, nil }, jwt.WithValidMethods([]string{"RS256"}))
		require.NoError(t, err, "the assertion verifies against the certificate's public key")
		assert.Equal(t, "JWT", parsed.Header["typ"])
		assert.Equal(t, base64.RawURLEncoding.EncodeToString(s1[:]), parsed.Header["x5t"])
		assert.Equal(t, base64.RawURLEncoding.EncodeToString(s256[:]), parsed.Header["x5t#S256"])
		assert.Equal(t, "entra-client-id", claims["iss"])
		assert.Equal(t, "entra-client-id", claims["sub"])
		assert.Equal(t, te.srv.URL+"/token", claims["aud"])
		exp, iat, nbf := claims["exp"].(float64), claims["iat"].(float64), claims["nbf"].(float64)
		assert.InDelta(t, 300, exp-iat, 0)
		assert.Equal(t, iat, nbf)
		jti, _ := claims["jti"].(string)
		assert.NotEmpty(t, jti)
		jtis[jti] = true
	}
	assert.Len(t, jtis, 2, "a fresh assertion (new jti) is minted for every exchange")
}

func TestExchangeFederatedSendsAPlatformSignedAssertion(t *testing.T) {
	te := newTokenEndpoint(t)
	signKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	privDER := x509.MarshalPKCS1PrivateKey(signKey)
	pubDER, err := x509.MarshalPKIXPublicKey(&signKey.PublicKey)
	require.NoError(t, err)
	svc, err := authservice.NewWithRSA(authservice.Config{Issuer: "https://fc.example.test"},
		string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})))
	require.NoError(t, err)

	b := NewBridge(nil, nil, nil)
	b.SetAssertionSigner(svc)
	r, _, err := b.buildResolved(context.Background(), te.idp(identityprovider.ClientAuthFederated), nil)
	require.NoError(t, err)
	_, err = r.Exchange(context.Background(), "the-code", "https://app/cb", "verifier")
	require.NoError(t, err)

	got := te.last(t)
	assert.False(t, got.hasBasic)
	assert.Empty(t, got.form.Get("client_secret"))
	assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", got.form.Get("client_assertion_type"))
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(got.form.Get("client_assertion"), claims,
		func(*jwt.Token) (any, error) { return &signKey.PublicKey, nil }, jwt.WithValidMethods([]string{"RS256"}))
	require.NoError(t, err, "the assertion verifies against the platform JWKS key")
	assert.Equal(t, svc.KeyID(), parsed.Header["kid"])
	assert.Equal(t, "https://fc.example.test", claims["iss"])
	assert.Equal(t, "identity-provider:idp_test1", claims["sub"])
	assert.Equal(t, "api://AzureADTokenExchange", claims["aud"])
	assert.Equal(t, "client_assertion", claims["token_use"])
	assert.NotEmpty(t, claims["jti"])
}

func TestBuildResolvedAssertionMethodErrorsNameTheProvider(t *testing.T) {
	te := newTokenEndpoint(t)
	b := NewBridge(nil, nil, testEnc(t))

	_, _, err := b.buildResolved(context.Background(), te.idp(identityprovider.ClientAuthFederated), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idp_test1")
	assert.Contains(t, err.Error(), "signing key")

	_, _, err = b.buildResolved(context.Background(), te.idp(identityprovider.ClientAuthCertificate), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idp_test1")
	assert.Contains(t, err.Error(), "no ACTIVE certificate")

	// A certificate whose key the encryption service cannot open: the error
	// names the provider and carries no key material.
	other, err := identityprovider.GenerateCertificate(testEnc(t), "idp_test1", "entra", time.Now())
	require.NoError(t, err)
	_, _, err = b.buildResolved(context.Background(), te.idp(identityprovider.ClientAuthCertificate), other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "idp_test1")
	assert.NotContains(t, err.Error(), other.PrivateKeyRef)
}

func TestCacheKeyTurnsOverWithMethodAndActiveCertificate(t *testing.T) {
	issuer, clientID := "https://login.microsoftonline.com/t1/v2.0", "app-1"
	mk := func(m identityprovider.ClientAuthMethod) *identityprovider.IdentityProvider {
		return &identityprovider.IdentityProvider{OIDCIssuerURL: &issuer, OIDCClientID: &clientID, OIDCClientAuthMethod: m}
	}
	secret := cacheKey(mk(identityprovider.ClientAuthSecret), "")
	assert.Equal(t, secret, cacheKey(mk(""), ""), "an unset method is CLIENT_SECRET")
	fed := cacheKey(mk(identityprovider.ClientAuthFederated), "")
	cert1 := cacheKey(mk(identityprovider.ClientAuthCertificate), "ipc_1")
	cert2 := cacheKey(mk(identityprovider.ClientAuthCertificate), "ipc_2")
	assert.NotEqual(t, secret, fed, "changing the method changes the key")
	assert.NotEqual(t, secret, cert1)
	assert.NotEqual(t, fed, cert1)
	assert.NotEqual(t, cert1, cert2, "promoting a certificate changes the key")
	assert.Equal(t, cert1, cacheKey(mk(identityprovider.ClientAuthCertificate), "ipc_1"))
}

func TestIssuerIsPublicHTTPS(t *testing.T) {
	for issuer, want := range map[string]bool{
		"https://fc.example.com":      true,
		"https://fc.example.com:8443": true,
		"http://fc.example.com":       false,
		"https://localhost:8080":      false,
		"https://127.0.0.1":           false,
		"https://[::1]:8443":          false,
		"https://app.localhost":       false,
		"":                            false,
		"not a url":                   false,
	} {
		assert.Equal(t, want, IssuerIsPublicHTTPS(issuer), issuer)
	}
}

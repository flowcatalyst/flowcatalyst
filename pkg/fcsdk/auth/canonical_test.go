package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/auth"
	"github.com/flowcatalyst/go-sdk/internal/compactjws/compactjwstest"
)

// A token verifies only in the spelling it was issued in (docs/wire-contract.md,
// Authentication, "Token spelling"): every verifier in this package refuses the
// issued token re-spelled (another last character of the signature, padding,
// the standard alphabet, a line break) and changed in content, and still
// accepts the issued token afterwards.

// rsaIssuer serves OIDC discovery and a JWKS for one RSA key of the given size.
type rsaIssuer struct {
	url  string
	sign jwk.Key
}

func newRSAIssuer(t *testing.T, bits int) *rsaIssuer {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, bits)
	require.NoError(t, err)
	priv, err := jwk.Import[jwk.Key](raw)
	require.NoError(t, err)
	require.NoError(t, priv.Set(jwk.KeyIDKey, "canonical-key"))
	require.NoError(t, priv.Set(jwk.AlgorithmKey, jwa.RS256()))
	pub, err := priv.PublicKey()
	require.NoError(t, err)
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(pub))
	iss := &rsaIssuer{sign: priv}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": iss.url, "jwks_uri": iss.url + "/jwks"})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(set) })
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	iss.url = srv.URL
	return iss
}

func (i *rsaIssuer) mint(t *testing.T, typ string, claims map[string]any) string {
	t.Helper()
	tok := jwt.New()
	for k, v := range claims {
		require.NoError(t, tok.Set(k, v))
	}
	h := jws.NewHeaders()
	if typ != "" {
		require.NoError(t, h.Set(jws.TypeKey, typ))
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), i.sign, jws.WithProtectedHeaders(h)))
	require.NoError(t, err)
	return string(signed)
}

func accessClaims(iss, aud, sub string) map[string]any {
	now := time.Now()
	return map[string]any{
		jwt.SubjectKey: sub, jwt.IssuerKey: iss, jwt.AudienceKey: aud,
		jwt.ExpirationKey: now.Add(time.Hour), jwt.IssuedAtKey: now.Add(-time.Minute),
		"token_use": "api", "type": "SERVICE", "scope": "CLIENT", "clients": []string{"clt_1"},
	}
}

func TestTokenValidatorAcceptsOnlyTheIssuedSpelling(t *testing.T) {
	// 2048-bit: 256-byte signature, 4 spare bits; 3072: none; 4096: 2.
	for _, bits := range []int{2048, 3072, 4096} {
		t.Run(map[int]string{2048: "RS256 2048", 3072: "RS256 3072", 4096: "RS256 4096"}[bits], func(t *testing.T) {
			iss := newRSAIssuer(t, bits)
			v := auth.NewTokenValidator(auth.TokenValidatorConfig{IssuerURL: iss.url, Audience: "my-app"})
			k := compactjwstest.Kind{
				Token: iss.mint(t, "", accessClaims(iss.url, "my-app", "prn_canonical")),
				Verify: func(tok string) error {
					_, err := v.Validate(context.Background(), tok)
					return err
				},
				Claim: "prn_canonical", OtherClaim: "prn_somebody",
			}
			t.Run("tampered", func(t *testing.T) { compactjwstest.Tampered(t, k) })
			t.Run("re-spelled", func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
			t.Run("re-spelled, through ValidateBearer", func(t *testing.T) {
				b := k
				b.Verify = func(tok string) error {
					_, err := v.ValidateBearer(context.Background(), "Bearer "+tok)
					return err
				}
				compactjwstest.NonCanonical(t, b)
			})
		})
	}
}

func TestHmacValidatorAcceptsOnlyTheIssuedSpelling(t *testing.T) {
	// HS256: a 32-byte signature, 43 characters, 2 spare bits.
	v := auth.NewHmacTokenValidator(testSecret, "fc", "fc")
	k := compactjwstest.Kind{
		Token:  mintHS256(t, testSecret, "fc", "fc", "prn_canonical", time.Now().Add(time.Hour), nil),
		Verify: func(tok string) error { _, err := v.Validate(tok); return err },
		Claim:  "prn_canonical", OtherClaim: "prn_somebody",
	}
	t.Run("tampered", func(t *testing.T) { compactjwstest.Tampered(t, k) })
	t.Run("re-spelled", func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
	t.Run("re-spelled, through ValidateBearer", func(t *testing.T) {
		b := k
		b.Verify = func(tok string) error { _, err := v.ValidateBearer("Bearer " + tok); return err }
		compactjwstest.NonCanonical(t, b)
	})
}

// fieldRig mints field tokens and a device's proofs for a FieldTokenVerifier.
type fieldRig struct {
	iss    *rsaIssuer
	device *ecdsa.PrivateKey
	jwk    map[string]string
	jkt    string
	v      *auth.FieldTokenVerifier
}

const (
	rigApp = "canonical-app"
	rigURL = "https://app.example.test/api/orders"
)

func newFieldRig(t *testing.T) *fieldRig {
	t.Helper()
	iss := newRSAIssuer(t, 2048)
	device, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pt, err := device.PublicKey.Bytes() // 0x04 || x || y
	require.NoError(t, err)
	x := base64.RawURLEncoding.EncodeToString(pt[1:33])
	y := base64.RawURLEncoding.EncodeToString(pt[33:])
	tp := sha256.Sum256([]byte(`{"crv":"P-256","kty":"EC","x":"` + x + `","y":"` + y + `"}`))
	return &fieldRig{
		iss: iss, device: device,
		jwk: map[string]string{"kty": "EC", "crv": "P-256", "x": x, "y": y},
		jkt: base64.RawURLEncoding.EncodeToString(tp[:]),
		v:   auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{Issuer: iss.url, AppCode: rigApp}),
	}
}

func (r *fieldRig) token(t *testing.T) string {
	now := time.Now()
	return r.iss.mint(t, "fc-field+jwt", map[string]any{
		jwt.SubjectKey: "fwu_canonical", jwt.IssuerKey: r.iss.url, jwt.AudienceKey: rigApp,
		jwt.ExpirationKey: now.Add(10 * time.Minute), jwt.IssuedAtKey: now.Add(-time.Minute), jwt.JwtIDKey: "jti-1",
		"token_use": "field", "cnf": map[string]string{"jkt": r.jkt}, "device_mode": "PERSONAL", "amr": []string{"pin"},
	})
}

// proof signs a DPoP proof for token (its ath is the hash of token's exact text).
func (r *fieldRig) proof(t *testing.T, token string) string {
	t.Helper()
	enc := func(v any) string {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	sum := sha256.Sum256([]byte(token))
	input := enc(map[string]any{"alg": "ES256", "typ": "dpop+jwt", "jwk": r.jwk}) + "." +
		enc(map[string]any{
			"htm": "POST", "htu": rigURL, "iat": time.Now().Unix(), "jti": "proof-jti-1",
			"ath": base64.RawURLEncoding.EncodeToString(sum[:]),
		})
	digest := sha256.Sum256([]byte(input))
	rr, ss, err := ecdsa.Sign(rand.Reader, r.device, digest[:])
	require.NoError(t, err)
	sig := append(rr.FillBytes(make([]byte, 32)), ss.FillBytes(make([]byte, 32))...)
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestFieldVerifierTokenRefusesTamperedTokens(t *testing.T) {
	r := newFieldRig(t)
	// Each changed token comes with a proof made for its exact text, so the
	// proof's ath matches and only the token's own check can refuse it.
	compactjwstest.Tampered(t, compactjwstest.Kind{
		Token: r.token(t),
		Verify: func(tok string) error {
			_, err := r.v.Verify(context.Background(), "DPoP "+tok, r.proof(t, tok), "POST", rigURL)
			return err
		},
		Claim: "fwu_canonical", OtherClaim: "fwu_somebodyy",
	})
}

// TestFieldVerifierTokenAcceptsOnlyTheIssuedSpelling: the re-spelled field
// token is refused as malformed, with a proof made for its exact text. Space
// before or after the token is outside it (the Authorization header's
// framing, trimmed as the platform does), so it is not a case here.
func TestFieldVerifierTokenAcceptsOnlyTheIssuedSpelling(t *testing.T) {
	r := newFieldRig(t)
	tok := r.token(t)
	verify := func(tok string) error {
		_, err := r.v.Verify(context.Background(), "DPoP "+tok, r.proof(t, tok), "POST", rigURL)
		return err
	}
	require.NoError(t, verify(tok))
	parts := splitCompact(t, tok)
	spellings := compactjwstest.NonCanonicalSpellings(parts[2])
	require.Len(t, spellings, 15) // a 256-byte signature: 4 spare bits
	mid := len(parts[2]) / 2
	cases := map[string]string{
		"signature padded":                   tok + "==",
		"signature in the standard alphabet": parts[0] + "." + parts[1] + "." + stdAlphabet(parts[2]),
		"line feed inside the signature":     parts[0] + "." + parts[1] + "." + parts[2][:mid] + "\n" + parts[2][mid:],
		"carriage return inside the signature": parts[0] + "." + parts[1] + "." +
			parts[2][:mid] + "\r" + parts[2][mid:],
		"CRLF inside the signature": parts[0] + "." + parts[1] + "." + parts[2][:mid] + "\r\n" + parts[2][mid:],
	}
	for _, s := range spellings {
		cases["signature ends in "+s[len(s)-1:]] = parts[0] + "." + parts[1] + "." + s
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			require.NotEqual(t, tok, bad)
			err := verify(bad)
			var fe *auth.FieldError
			require.ErrorAs(t, err, &fe, "must be refused")
			assert.Equal(t, auth.FieldReasonMalformed, fe.Reason)
		})
	}
	require.NoError(t, verify(tok))
}

func TestFieldVerifierProofAcceptsOnlyTheIssuedSpelling(t *testing.T) {
	r := newFieldRig(t)
	tok := r.token(t)
	k := compactjwstest.Kind{
		Token: r.proof(t, tok),
		Verify: func(proof string) error {
			_, err := r.v.Verify(context.Background(), "DPoP "+tok, proof, "POST", rigURL)
			if fe, ok := errors.AsType[*auth.FieldError](err); ok && fe.Reason != auth.FieldReasonInvalidProof {
				t.Errorf("refused as %s, want %s", fe.Reason, auth.FieldReasonInvalidProof)
			}
			return err
		},
		Claim: "proof-jti-1", OtherClaim: "proof-jti-2",
	}
	t.Run("tampered", func(t *testing.T) { compactjwstest.Tampered(t, k) })
	t.Run("re-spelled", func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
	t.Run("whitespace around it", func(t *testing.T) {
		for _, bad := range []string{" " + k.Token, k.Token + " ", "\t" + k.Token, k.Token + "\n"} {
			assert.Error(t, k.Verify(bad), "%q", bad)
		}
	})
}

func splitCompact(t *testing.T, tok string) [3]string {
	t.Helper()
	var out [3]string
	n := 0
	start := 0
	for i := 0; i <= len(tok); i++ {
		if i == len(tok) || tok[i] == '.' {
			require.Less(t, n, 3, "more than three segments")
			out[n] = tok[start:i]
			n++
			start = i + 1
		}
	}
	require.Equal(t, 3, n)
	return out
}

func stdAlphabet(s string) string {
	b := []byte(s)
	for i, c := range b {
		switch c {
		case '-':
			b[i] = '+'
		case '_':
			b[i] = '/'
		}
	}
	return string(b)
}

// TestFieldNonCanonicalFixtures runs the shared non-canonical vectors
// (api/field-fixtures/noncanonical.json): the valid fixture's token and proof
// re-spelled, each refused with the reason the vector names.
func TestFieldNonCanonicalFixtures(t *testing.T) {
	issuer, appCode, jwks, _ := loadFixtures(t)
	raw, err := os.ReadFile(filepath.Join(fixtureDir, "noncanonical.json"))
	require.NoError(t, err)
	var f struct {
		Issuer  string        `json:"issuer"`
		AppCode string        `json:"appCode"`
		Cases   []fixtureCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &f))
	require.Equal(t, issuer, f.Issuer)
	require.Equal(t, appCode, f.AppCode)
	require.NotEmpty(t, f.Cases)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(jwks) }))
	t.Cleanup(srv.Close)
	for _, c := range f.Cases {
		t.Run(c.Name, func(t *testing.T) {
			require.NotEqual(t, "valid", c.Expect)
			now, err := time.Parse(time.RFC3339, c.Now)
			require.NoError(t, err)
			v := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
				Issuer: f.Issuer, AppCode: f.AppCode, JWKSURL: srv.URL, Now: func() time.Time { return now },
			})
			_, err = v.Verify(context.Background(), "DPoP "+c.AccessToken, c.DPoPProof, c.Method, c.URL)
			var fe *auth.FieldError
			require.ErrorAs(t, err, &fe, "must be refused")
			assert.Equal(t, c.Expect, fe.Reason, fe.Error())
		})
	}
}

package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

func TestExtractBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{"", ""},
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"bearer abc.def.ghi", "abc.def.ghi"}, // case-insensitive scheme
		{"Bearer  spaced", "spaced"},          // trims
		{"Basic dXNlcjpwYXNz", ""},            // wrong scheme
		{"Bearer", ""},                        // no token
		{"BearerNoSpace", ""},                 // malformed
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set("Authorization", tc.header)
		}
		got, _ := extractToken(r)
		if got != tc.want {
			t.Errorf("extractToken(%q) = %q; want %q", tc.header, got, tc.want)
		}
	}
}

func TestExtractBearerTokenSessionCookie(t *testing.T) {
	// No Authorization header — cookie wins.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, fromCookie := extractToken(r); got != "cookie-jwt" || !fromCookie {
		t.Errorf("cookie path: got %q fromCookie=%v want cookie-jwt/true", got, fromCookie)
	}

	// Both present — Authorization wins (and is not flagged as a cookie).
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Bearer header-jwt")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, fromCookie := extractToken(r); got != "header-jwt" || fromCookie {
		t.Errorf("header preferred: got %q fromCookie=%v want header-jwt/false", got, fromCookie)
	}

	// Non-Bearer Authorization — do NOT fall through to cookie.
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "cookie-jwt"})
	if got, _ := extractToken(r); got != "" {
		t.Errorf("non-bearer header should suppress cookie fallback: got %q", got)
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{"a, b , c ", []string{"a", "b", "c"}}, // whitespace trimmed
		{",,a,,b,", []string{"a", "b"}},        // empty segments dropped
	}
	for _, tc := range cases {
		got := splitCSV(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCSV(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

// TestAuthenticatorRejectsIdentityBearer is the enforcement test for fix (e):
// an interactive-login (identity) access token presented as an API bearer must
// be rejected outright — never producing an AuthContext, even an empty one —
// while an authority-bearing (api) token from the same signer is accepted.
func TestAuthenticatorRejectsIdentityBearer(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: mustPKIX(t, &key.PublicKey)}))

	// Provider (validates bearers) and authservice (mints them) share the key,
	// issuer, and audience so signature + audience checks line up. nil repos are
	// safe: the identity-reject path returns before ResolveClaims/FlattenPermissions,
	// and the api token below carries an explicit scope so no role lookup runs.
	prov, err := provider.NewProvider(provider.Config{
		Issuer:     "flowcatalyst",
		Audience:   "flowcatalyst",
		SigningKey: privPEM,
	}, nil, nil)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	svc, err := authservice.NewWithRSA(authservice.DefaultConfig(), string(privPEM), pubPEM)
	if err != nil {
		t.Fatalf("authservice: %v", err)
	}

	p := principal.NewUser("u@example.com", principal.ScopeAnchor)
	identityTok, err := svc.GenerateIdentityAccessToken(p)
	if err != nil {
		t.Fatalf("identity mint: %v", err)
	}
	apiTok, err := svc.GenerateAccessTokenWithScope(p, []string{"platform:iam:role:view"})
	if err != nil {
		t.Fatalf("api mint: %v", err)
	}

	run := func(bearer string) (status int, hadCtx bool) {
		var seen bool
		h := Authenticator(AuthConfig{Provider: prov})(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				seen = auth.FromContext(r.Context()) != nil
				w.WriteHeader(http.StatusOK)
			}))
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
		r.Header.Set("Authorization", "Bearer "+bearer)
		h.ServeHTTP(rec, r)
		return rec.Code, seen
	}

	if status, hadCtx := run(identityTok); status != http.StatusUnauthorized || hadCtx {
		t.Errorf("identity bearer: status=%d hadCtx=%v; want 401 and no AuthContext", status, hadCtx)
	}
	if status, hadCtx := run(apiTok); status != http.StatusOK || !hadCtx {
		t.Errorf("api bearer: status=%d hadCtx=%v; want 200 with AuthContext", status, hadCtx)
	}
}

func mustPKIX(t *testing.T, pub *rsa.PublicKey) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal pkix: %v", err)
	}
	return der
}

func TestStringSliceFromJWTExtra(t *testing.T) {
	// JWT round-tripped through JSON arrives as []interface{}; freshly
	// minted tokens hand back []string. Both must coerce.
	if got := stringSlice([]string{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("stringSlice([]string): %v", got)
	}
	if got := stringSlice([]interface{}{"a", "b"}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("stringSlice([]interface{}): %v", got)
	}
	if got := stringSlice("nope"); got != nil {
		t.Errorf("stringSlice(string): %v", got)
	}
	if got := stringSlice(nil); got != nil {
		t.Errorf("stringSlice(nil): %v", got)
	}
}

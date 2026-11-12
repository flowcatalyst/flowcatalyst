package middleware

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

type classRig struct {
	prov *provider.Provider
	key  *rsa.PrivateKey
}

func newClassRig(t *testing.T) *classRig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	prov, err := provider.NewProvider(provider.Config{Issuer: "flowcatalyst", Audience: "flowcatalyst", SigningKey: privPEM}, nil, nil)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	return &classRig{prov: prov, key: key}
}

// session is what the platform sets in fc_session.
func (c *classRig) session(t *testing.T) string {
	t.Helper()
	tok, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_x", Email: "u@example.com"}, c.key, "flowcatalyst", time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	return tok
}

// api is an API access token shaped like authservice's: marker + platform aud,
// carrying whatever narrowing the caller gave it.
func (c *classRig) api(t *testing.T, scope string) string {
	t.Helper()
	mc := jwt.MapClaims{
		"iss": "flowcatalyst", "aud": "flowcatalyst", "sub": "prn_x", "token_use": "api",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(), "tier": "ANCHOR", "type": "USER",
		"roles": []string{}, "clients": []string{},
	}
	if scope != "" {
		mc["scope"] = scope
	}
	out, err := jwt.NewWithClaims(jwt.SigningMethodRS256, mc).SignedString(c.key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return out
}

type classProbe struct {
	code   int
	ac     *auth.AuthContext
	cookie bool
	bearer bool
}

func (c *classRig) send(cookie, bearer string) classProbe {
	var out classProbe
	h := Authenticator(AuthConfig{Provider: c.prov})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		out.ac = auth.FromContext(r.Context())
		out.cookie = AuthenticatedBySessionCookie(r.Context())
		out.bearer = AuthenticatedByBearer(r.Context())
	}))
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/thing", nil)
	if cookie != "" {
		r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: cookie})
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	h.ServeHTTP(rec, r)
	out.code = rec.Code
	return out
}

// T2: an API token (here narrowed to nothing, then to one scope) sent as the
// fc_session cookie yields no AuthContext. It is treated like a stale cookie:
// the request proceeds unauthenticated, no 401, and above all no database
// authority (a nil-repo provider would panic if the cookie path resolved it).
func TestAPITokenInCookieIsUnauthenticated(t *testing.T) {
	c := newClassRig(t)
	for name, tok := range map[string]string{
		"roles narrowed to nothing": c.api(t, ""),
		"scoped":                    c.api(t, "platform:iam:role:view"),
	} {
		got := c.send(tok, "")
		if got.ac != nil || got.cookie || got.bearer {
			t.Errorf("%s: API token in the cookie authenticated (ac=%v cookie=%v bearer=%v)", name, got.ac, got.cookie, got.bearer)
		}
		if got.code != http.StatusOK {
			t.Errorf("%s: status %d; a rejected cookie must proceed unauthenticated, not 401", name, got.code)
		}
	}
}

// T3: a session token presented as a Bearer is rejected like an identity token.
func TestSessionTokenAsBearerIsRejected(t *testing.T) {
	c := newClassRig(t)
	got := c.send("", c.session(t))
	if got.code != http.StatusUnauthorized || got.ac != nil {
		t.Errorf("session token as Bearer: status=%d ac=%v; want 401 and no AuthContext", got.code, got.ac)
	}
}

// T4 (bearer half; the cookie half needs the database and lives in
// internal/server/token_authority_pg_test.go): an API bearer authenticates with
// its own claims, only the Bearer marker is set, and Authorization takes
// precedence over a cookie.
func TestAPIBearerKeepsItsOwnClaimsAndMarker(t *testing.T) {
	c := newClassRig(t)
	got := c.send("", c.api(t, "platform:iam:role:view"))
	if got.code != http.StatusOK || got.ac == nil {
		t.Fatalf("api bearer: status=%d ac=%v; want 200 with AuthContext", got.code, got.ac)
	}
	if !got.bearer || got.cookie {
		t.Errorf("markers: bearer=%v cookie=%v; want bearer only", got.bearer, got.cookie)
	}
	if len(got.ac.Permissions) != 1 || got.ac.Permissions[0] != "platform:iam:role:view" {
		t.Errorf("permissions = %v; want the token's own narrowed scope", got.ac.Permissions)
	}

	// Authorization wins over the cookie, and a cookie that would be refused
	// beside it does not matter.
	both := c.send(c.api(t, ""), c.api(t, "platform:iam:role:view"))
	if both.ac == nil || !both.bearer || both.cookie {
		t.Errorf("bearer+cookie: ac=%v bearer=%v cookie=%v; want the bearer's context", both.ac, both.bearer, both.cookie)
	}
}

func TestBearerMarkerAbsentWithoutBearer(t *testing.T) {
	c := newClassRig(t)
	if got := c.send("", ""); got.ac != nil || got.bearer || got.cookie {
		t.Errorf("anonymous: ac=%v bearer=%v cookie=%v", got.ac, got.bearer, got.cookie)
	}
}

func TestRefuseBearer(t *testing.T) {
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	if err := RefuseBearer(ctx); err != nil {
		t.Errorf("plain context: %v", err)
	}
	if err := RefuseBearer(WithSessionCookieAuth(ctx)); err != nil {
		t.Errorf("cookie context: %v", err)
	}
	if err := RefuseBearer(WithBearerAuth(ctx)); err == nil {
		t.Error("bearer context must be refused")
	}
}

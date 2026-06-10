package authservice

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

func genRSAPEMs(t *testing.T) (privPEM, pubPEM string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privDER := x509.MarshalPKCS1PrivateKey(key)
	privPEM = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER}))
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public: %v", err)
	}
	pubPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	return privPEM, pubPEM
}

func anchorUser() *principal.Principal {
	p := principal.NewUser("admin@example.com", principal.ScopeAnchor)
	p.Roles = []serviceaccount.RoleAssignment{{Role: "platform:admin"}, {Role: "operant:viewer"}}
	return p
}

func newRS256(t *testing.T) *AuthService {
	t.Helper()
	priv, pub := genRSAPEMs(t)
	cfg := DefaultConfig()
	cfg.RSAPrivateKeyPEM = priv
	cfg.RSAPublicKeyPEM = pub
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Algorithm() != "RS256" {
		t.Fatalf("want RS256, got %s", svc.Algorithm())
	}
	return svc
}

func TestGenerateAndValidateAccessToken(t *testing.T) {
	svc := newRS256(t)
	p := anchorUser()

	tok, err := svc.GenerateAccessToken(p)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Subject != p.ID {
		t.Errorf("sub = %q, want %q", claims.Subject, p.ID)
	}
	if claims.Tier != "ANCHOR" {
		t.Errorf("tier = %q, want ANCHOR", claims.Tier)
	}
	if claims.PrincipalType != "USER" {
		t.Errorf("type = %q, want USER", claims.PrincipalType)
	}
	if claims.ID == "" {
		t.Error("jti is empty")
	}
	if claims.Aud != "flowcatalyst" {
		t.Errorf("aud = %q, want flowcatalyst", claims.Aud)
	}
	if len(claims.Clients) != 1 || claims.Clients[0] != "*" {
		t.Errorf("clients = %v, want [*]", claims.Clients)
	}
	if len(claims.Applications) != 2 || claims.Applications[0] != "platform" || claims.Applications[1] != "operant" {
		t.Errorf("applications = %v, want [platform operant]", claims.Applications)
	}
}

// TestAccessTokenWireShape asserts the raw JSON payload: aud is a bare string
// (not an array), type/jti present, clients/roles/applications always present.
// Tenancy rides "tier" (was "scope"); "scope" now carries granted permissions
// and is omitted when a token is minted without one (the GenerateAccessToken
// path here). The legacy "permissions" array claim is never emitted.
func TestAccessTokenWireShape(t *testing.T) {
	svc := newRS256(t)
	tok, err := svc.GenerateAccessToken(anchorUser())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	payload := decodeJWTPayload(t, tok)

	if _, ok := payload["aud"].(string); !ok {
		t.Errorf("aud must be a bare string, got %T (%v)", payload["aud"], payload["aud"])
	}
	for _, k := range []string{"type", "jti", "nbf", "iat", "exp", "iss", "sub", "tier", "name", "clients", "roles", "applications"} {
		if _, ok := payload[k]; !ok {
			t.Errorf("missing required claim %q", k)
		}
	}
	if payload["type"] != "USER" {
		t.Errorf(`type = %v, want "USER"`, payload["type"])
	}
	if _, ok := payload["scope"]; ok {
		t.Error("scope claim must be omitted when no scope is granted")
	}
	if _, ok := payload["permissions"]; ok {
		t.Error("permissions claim must NOT be present")
	}
}

// TestAccessTokenScopeClaim asserts that a token minted with a granted scope
// carries those permissions on the space-delimited "scope" claim and round-
// trips through validation.
func TestAccessTokenScopeClaim(t *testing.T) {
	svc := newRS256(t)
	granted := []string{"platform:messaging:event-type:view", "platform:billing:invoice:read"}

	tok, err := svc.GenerateAccessTokenWithScope(anchorUser(), granted)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	payload := decodeJWTPayload(t, tok)
	if got, want := payload["scope"], strings.Join(granted, " "); got != want {
		t.Errorf("scope = %v, want %q", got, want)
	}

	claims, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Scope != strings.Join(granted, " ") {
		t.Errorf("claims.Scope = %q, want %q", claims.Scope, strings.Join(granted, " "))
	}
}

func TestClientScopeClients(t *testing.T) {
	svc := newRS256(t)
	cid := "clt_123"
	p := principal.NewUser("u@example.com", principal.ScopeClient)
	p.ClientID = &cid
	p.ClientIdentifierMap = map[string]string{cid: "acme"}

	tok, err := svc.GenerateAccessToken(p)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := svc.ValidateToken(tok)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if claims.Tier != "CLIENT" {
		t.Errorf("tier = %q, want CLIENT", claims.Tier)
	}
	if len(claims.Clients) != 1 || claims.Clients[0] != "clt_123:acme" {
		t.Errorf(`clients = %v, want ["clt_123:acme"]`, claims.Clients)
	}
}

func TestServicePrincipalOmitsEmail(t *testing.T) {
	svc := newRS256(t)
	p := principal.NewService("svc_1", "ci-bot")
	tok, err := svc.GenerateAccessToken(p)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	payload := decodeJWTPayload(t, tok)
	if _, ok := payload["email"]; ok {
		t.Error("SERVICE principal token must omit email")
	}
	if payload["type"] != "SERVICE" {
		t.Errorf(`type = %v, want "SERVICE"`, payload["type"])
	}
}

func TestHS256Fallback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SecretKey = "dev-secret-key-for-tests-only-32b!"
	svc, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.Algorithm() != "HS256" {
		t.Fatalf("want HS256, got %s", svc.Algorithm())
	}
	tok, err := svc.GenerateAccessToken(anchorUser())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.ValidateToken(tok); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if len(svc.AllJWKSKeys()) != 0 {
		t.Error("HS256 service must expose no JWKS keys")
	}
}

func TestKeyRotationValidatesPreviousKey(t *testing.T) {
	// A token minted by the "previous" service must validate against a
	// "current" service that lists the previous key for rotation.
	prevPriv, prevPub := genRSAPEMs(t)
	curPriv, curPub := genRSAPEMs(t)

	prevCfg := DefaultConfig()
	prevCfg.RSAPrivateKeyPEM = prevPriv
	prevCfg.RSAPublicKeyPEM = prevPub
	prevSvc, err := New(prevCfg)
	if err != nil {
		t.Fatalf("prev New: %v", err)
	}
	oldToken, err := prevSvc.GenerateAccessToken(anchorUser())
	if err != nil {
		t.Fatalf("generate old token: %v", err)
	}

	curCfg := DefaultConfig()
	curCfg.RSAPrivateKeyPEM = curPriv
	curCfg.RSAPublicKeyPEM = curPub
	curCfg.RSAPublicKeyPreviousPEM = prevPub
	curSvc, err := New(curCfg)
	if err != nil {
		t.Fatalf("cur New: %v", err)
	}

	if _, err := curSvc.ValidateToken(oldToken); err != nil {
		t.Fatalf("current service should validate token signed by previous key: %v", err)
	}
	if got := len(curSvc.AllJWKSKeys()); got != 2 {
		t.Errorf("JWKS should expose current + previous = 2 keys, got %d", got)
	}
}

func TestKeyIDIsDeterministicSHA256Prefix(t *testing.T) {
	_, pub := genRSAPEMs(t)
	want := independentKeyID(pub)
	if got := generateKeyID(pub); got != want {
		t.Errorf("kid = %q, want %q", got, want)
	}
	if len(generateKeyID(pub)) != 22 {
		t.Errorf("kid length = %d, want 22 (base64url of 16 bytes)", len(generateKeyID(pub)))
	}
}

func TestIDTokenShape(t *testing.T) {
	svc := newRS256(t)
	nonce := "n-0S6_WzA2Mj"
	tok, err := svc.GenerateIDToken(anchorUser(), "clt_rp", &nonce)
	if err != nil {
		t.Fatalf("generate id token: %v", err)
	}
	payload := decodeJWTPayload(t, tok)
	if payload["aud"] != "clt_rp" {
		t.Errorf("aud = %v, want clt_rp", payload["aud"])
	}
	if payload["azp"] != "clt_rp" {
		t.Errorf("azp = %v, want clt_rp", payload["azp"])
	}
	if payload["nonce"] != nonce {
		t.Errorf("nonce = %v, want %v", payload["nonce"], nonce)
	}
	if _, ok := payload["nbf"]; ok {
		t.Error("ID token must not carry nbf (Rust parity)")
	}
	if _, ok := payload["jti"]; ok {
		t.Error("ID token must not carry jti (Rust parity)")
	}
	if payload["email_verified"] != true {
		t.Errorf("email_verified = %v, want true", payload["email_verified"])
	}
}

// ─── helpers ────────────────────────────────────────────────────────────

func decodeJWTPayload(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return m
}

func independentKeyID(pubPEM string) string {
	h := sha256.Sum256([]byte(pubPEM))
	return base64.RawURLEncoding.EncodeToString(h[:16])
}

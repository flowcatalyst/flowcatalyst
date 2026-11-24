package authservice

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

func assertionService(t *testing.T) (*AuthService, *rsa.PublicKey) {
	t.Helper()
	priv, pub := genRSAPEMs(t)
	svc, err := NewWithRSA(Config{Issuer: "https://fc.example.test"}, priv, pub)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode([]byte(pub))
	k, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return svc, k.(*rsa.PublicKey)
}

func TestSignClientAssertionClaimsAndKid(t *testing.T) {
	svc, pub := assertionService(t)
	tok, err := svc.SignClientAssertion("identity-provider:idp_1", "api://AzureADTokenExchange", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, func(*jwt.Token) (any, error) { return pub, nil },
		jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		t.Fatalf("assertion does not verify against the platform key: %v", err)
	}
	if parsed.Header["kid"] != svc.KeyID() || svc.KeyID() == "" {
		t.Fatalf("kid header %v, want the JWKS key id %q", parsed.Header["kid"], svc.KeyID())
	}
	want := map[string]any{
		"iss": "https://fc.example.test", "sub": "identity-provider:idp_1",
		"aud": "api://AzureADTokenExchange", "token_use": "client_assertion",
	}
	for k, v := range want {
		if claims[k] != v {
			t.Errorf("claim %s = %v, want %v", k, claims[k], v)
		}
	}
	for _, k := range []string{"jti", "iat", "nbf", "exp"} {
		if claims[k] == nil {
			t.Errorf("claim %s missing", k)
		}
	}
	exp, _ := claims["exp"].(float64)
	iat, _ := claims["iat"].(float64)
	if d := exp - iat; d != 300 {
		t.Errorf("exp-iat = %v, want 300", d)
	}
}

func TestSignClientAssertionRefusesHS256(t *testing.T) {
	svc := NewWithSecret(Config{Issuer: "https://fc.example.test", SecretKey: "s"})
	if _, err := svc.SignClientAssertion("s", "a", time.Minute); !errors.Is(err, ErrClientAssertionNeedsRSA) {
		t.Fatalf("got %v, want ErrClientAssertionNeedsRSA", err)
	}
}

// A client assertion is signed with the platform key and issuer: it must never
// pass as a platform credential in any validator.
func TestClientAssertionIsNeverAPlatformCredential(t *testing.T) {
	svc, pub := assertionService(t)
	cfg := svc.config
	for name, aud := range map[string]string{
		"external audience": "api://AzureADTokenExchange",
		// Even an assertion minted with the platform audience (a hostile or
		// misconfigured caller of SignClientAssertion) is refused on token_use.
		"platform audience": cfg.Audience,
	} {
		t.Run(name, func(t *testing.T) {
			tok, err := svc.SignClientAssertion("identity-provider:idp_1", aud, 5*time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ValidateToken(tok); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("ValidateToken accepted it: %v", err)
			}
			if _, err := svc.ValidateFieldToken(tok); !errors.Is(err, ErrInvalidToken) {
				t.Errorf("ValidateFieldToken accepted it: %v", err)
			}
			if _, err := sessiontoken.Validate(tok, pub, sessiontoken.Expect{Issuer: cfg.Issuer, Audience: cfg.Audience}); err == nil {
				t.Errorf("sessiontoken.Validate accepted it")
			}
			// And with no expectations at all, the token_use guard still refuses.
			if _, err := sessiontoken.Validate(tok, pub, sessiontoken.Expect{}); err == nil {
				t.Errorf("sessiontoken.Validate (no expectations) accepted it")
			}
		})
	}
}

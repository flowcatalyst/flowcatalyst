package oauthapi

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// TestExpiresInTracksConfiguredTTL pins that the advertised expires_in is the
// configured access-token lifetime, not a 3600 literal. The two used to be
// independent: tokens were minted with AccessTokenExpirySecs while every token
// response reported a hardcoded 3600, so any non-default TTL made the server
// lie about when its own token expires — clients would keep using a token past
// its exp, or discard a still-valid one.
func TestExpiresInTracksConfiguredTTL(t *testing.T) {
	for _, ttl := range []int64{60, 900, 3600, 7200} {
		svc := authservice.NewWithSecret(authservice.Config{
			Issuer:                "https://fc.example",
			Audience:              "https://fc.example",
			SecretKey:             "dev-secret-not-for-production",
			AccessTokenExpirySecs: ttl,
		})
		if got := svc.AccessTokenTTLSecs(); got != ttl {
			t.Errorf("AccessTokenTTLSecs() = %d, want %d", got, ttl)
		}

		// The minted token's own lifetime must agree with what we advertise.
		p := principal.NewUser("u@example.com", principal.ScopeAnchor)
		tok, err := svc.GenerateAccessToken(p)
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		claims, err := svc.ValidateToken(tok)
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		lifetime := int64(claims.ExpiresAt.Sub(claims.IssuedAt.Time).Seconds())
		if lifetime != svc.AccessTokenTTLSecs() {
			t.Errorf("minted lifetime %ds disagrees with advertised expires_in %ds",
				lifetime, svc.AccessTokenTTLSecs())
		}
	}
}

// TestAccessTokenTTLDefault: an unset TTL still falls back to the historical
// 3600, so the wire contract is unchanged for anyone not setting the env var.
func TestAccessTokenTTLDefault(t *testing.T) {
	svc := authservice.NewWithSecret(authservice.Config{
		Issuer:    "https://fc.example",
		Audience:  "https://fc.example",
		SecretKey: "dev-secret-not-for-production",
	})
	if got := svc.AccessTokenTTLSecs(); got != 3600 {
		t.Errorf("default AccessTokenTTLSecs() = %d, want 3600", got)
	}
}

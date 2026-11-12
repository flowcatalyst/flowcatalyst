package sessiontoken_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

// T1: the two transport predicates over every token shape the platform sees.
// A session token is what Mint produces (no marker, no aud); an API token
// carries token_use "api"; a legacy API token has no marker but an aud; an
// identity token is neither; "session" is accepted for forward compatibility.
func TestTokenClassPredicates(t *testing.T) {
	key := mustKey(t)
	sign := func(mc jwt.MapClaims) *sessiontoken.Claims {
		t.Helper()
		mc["iss"] = "flowcatalyst"
		mc["sub"] = "prn_x"
		mc["exp"] = time.Now().Add(time.Hour).Unix()
		raw, err := jwt.NewWithClaims(jwt.SigningMethodRS256, mc).SignedString(key)
		if err != nil {
			t.Fatalf("sign: %v", err)
		}
		c, err := sessiontoken.Validate(raw, &key.PublicKey, sessiontoken.Expect{Issuer: "flowcatalyst", Audience: "flowcatalyst"})
		if err != nil {
			t.Fatalf("validate: %v", err)
		}
		return c
	}

	minted, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_x", Email: "u@example.com"}, key, "flowcatalyst", time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	mintedClaims, err := sessiontoken.Validate(minted, &key.PublicKey, sessiontoken.Expect{Issuer: "flowcatalyst", Audience: "flowcatalyst"})
	if err != nil {
		t.Fatalf("validate minted: %v", err)
	}

	for _, tc := range []struct {
		name          string
		c             *sessiontoken.Claims
		session, bear bool
	}{
		{"Mint output (no marker, no aud)", mintedClaims, true, false},
		{"api token (marker + aud)", sign(jwt.MapClaims{"aud": "flowcatalyst", "token_use": "api"}), false, true},
		{"api token, aud as array", sign(jwt.MapClaims{"aud": []string{"flowcatalyst"}, "token_use": "api"}), false, true},
		{"legacy api token (no marker, aud)", sign(jwt.MapClaims{"aud": "flowcatalyst"}), false, true},
		{"identity token", sign(jwt.MapClaims{"aud": "flowcatalyst", "token_use": "identity"}), false, false},
		{"explicit session marker", sign(jwt.MapClaims{"token_use": "session"}), true, false},
		{"session marker with an aud", sign(jwt.MapClaims{"aud": "flowcatalyst", "token_use": "session"}), true, false},
		{"unknown class", sign(jwt.MapClaims{"aud": "flowcatalyst", "token_use": "field"}), false, false},
		{"api marker, no aud", sign(jwt.MapClaims{"token_use": "api"}), false, true},
	} {
		if got := sessiontoken.IsSessionToken(tc.c); got != tc.session {
			t.Errorf("%s: IsSessionToken = %v, want %v", tc.name, got, tc.session)
		}
		if got := sessiontoken.IsAPIBearer(tc.c); got != tc.bear {
			t.Errorf("%s: IsAPIBearer = %v, want %v", tc.name, got, tc.bear)
		}
	}
	if sessiontoken.IsSessionToken(nil) || sessiontoken.IsAPIBearer(nil) {
		t.Error("a nil claims set is neither class")
	}
	if mintedClaims.HasAudience {
		t.Error("a minted session token must carry no aud claim (old instances read it by that rule)")
	}
}

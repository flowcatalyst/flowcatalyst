package authservice

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
)

// tokenKinds is every kind of token the service verifies, each with its
// verifier, for the shared JWS checks: access tokens (RS256, HS256, and RS256
// signed by the previous key during a rotation) and field access tokens.
func tokenKinds(t *testing.T) map[string]compactjwstest.Kind {
	t.Helper()
	out := map[string]compactjwstest.Kind{}

	access := func(name string, mint, verify *AuthService) {
		p := anchorUser()
		tok, err := mint.GenerateAccessToken(p)
		if err != nil {
			t.Fatalf("%s: generate: %v", name, err)
		}
		out[name] = compactjwstest.Kind{
			Token: tok,
			Verify: func(token string) error {
				_, err := verify.ValidateToken(token)
				return err
			},
			Claim: string(p.ID), OtherClaim: "prn_0000000000000",
		}
	}
	rs := newRS256(t)
	access("access RS256", rs, rs)

	cfg := DefaultConfig()
	cfg.SecretKey = "dev-secret-key-for-tests-only-32b!"
	hs, err := New(cfg)
	if err != nil || hs.Algorithm() != "HS256" {
		t.Fatalf("HS256 service: %v", err)
	}
	access("access HS256", hs, hs)

	prevPriv, prevPub := genRSAPEMs(t)
	curPriv, curPub := genRSAPEMs(t)
	prev, err := NewWithRSA(DefaultConfig(), prevPriv, prevPub)
	if err != nil {
		t.Fatal(err)
	}
	curCfg := DefaultConfig()
	curCfg.RSAPrivateKeyPEM, curCfg.RSAPublicKeyPEM, curCfg.RSAPublicKeyPreviousPEM = curPriv, curPub, prevPub
	cur, err := New(curCfg)
	if err != nil {
		t.Fatal(err)
	}
	access("access RS256 previous key", prev, cur)

	field, err := rs.GenerateFieldAccessToken(FieldSubject{WorkerID: "fwu_alice", AppCode: "app", ClientID: "clt_1"})
	if err != nil {
		t.Fatal(err)
	}
	out["field RS256"] = compactjwstest.Kind{
		Token: field,
		Verify: func(token string) error {
			_, err := rs.ValidateFieldToken(token)
			return err
		},
		Claim: "fwu_alice", OtherClaim: "fwu_mallory",
	}
	return out
}

func TestValidate_RefusesTamperedTokens(t *testing.T) {
	for name, k := range tokenKinds(t) {
		t.Run(name, func(t *testing.T) { compactjwstest.Tampered(t, k) })
	}
}

func TestValidate_AcceptsOnlyTheIssuedSpelling(t *testing.T) {
	for name, k := range tokenKinds(t) {
		t.Run(name, func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
	}
}

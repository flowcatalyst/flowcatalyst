package main

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"maps"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldauth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
)

// TestCommittedFixturesAreCurrent: the generator is deterministic and the
// committed api/field-fixtures files are exactly its output. Regenerate with
// `go run ./tools/field-fixtures` after changing it.
func TestCommittedFixturesAreCurrent(t *testing.T) {
	first, second := generate(), generate()
	for name, data := range first {
		if !bytes.Equal(data, second[name]) {
			t.Fatalf("%s: generator output differs between runs", name)
		}
		committed, err := os.ReadFile(filepath.Join("..", "..", "api", "field-fixtures", name))
		if err != nil {
			t.Fatalf("read committed %s: %v", name, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("api/field-fixtures/%s is stale; run `go run ./tools/field-fixtures`", name)
		}
	}
}

// TestLinkedFixtureCarriesTheClaim: valid_linked is the one case whose token
// carries linked_principal_id, and it lists the same id as the principal a
// verifier must expose (docs/field-native-session-spec.md section 5).
func TestLinkedFixtureCarriesTheClaim(t *testing.T) {
	var f struct {
		Cases []fixtureCase `json:"cases"`
	}
	if err := json.Unmarshal(generate()["cases.json"], &f); err != nil {
		t.Fatal(err)
	}
	linked := 0
	for _, c := range f.Cases {
		payload, err := base64.RawURLEncoding.DecodeString(splitToken(c.AccessToken)[1])
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		var claims map[string]any
		if err := json.Unmarshal(payload, &claims); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		id, has := claims["linked_principal_id"]
		if c.Name != "valid_linked" {
			if has {
				t.Errorf("%s carries linked_principal_id", c.Name)
			}
			continue
		}
		linked++
		if id != "prn_0FIXTUREPRINCIPAL" || c.Principal == nil || c.Principal.LinkedPrincipalID != id || c.Expect != "valid" {
			t.Errorf("valid_linked: claim %v, principal %+v, expect %s", id, c.Principal, c.Expect)
		}
	}
	if linked != 1 {
		t.Errorf("valid_linked appears %d times, want 1", linked)
	}
}

// lenient decodes a segment the way a forgiving decoder does: whitespace and
// '=' dropped, the standard alphabet read as base64url, spare bits ignored.
func lenient(t *testing.T, what, seg string) []byte {
	t.Helper()
	s := strings.NewReplacer(" ", "", "\r", "", "\n", "", "=", "", "+", "-", "/", "_").Replace(seg)
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("%s: %q does not decode even leniently: %v", what, seg, err)
	}
	return b
}

// TestNonCanonicalVectorsDifferOnlyInSpelling: every vector in
// noncanonical.json is the valid case with its token or its proof re-spelled,
// and nothing else wrong. Read leniently, each token is the valid token's
// header and claims under a signature that verifies, each proof is the valid
// proof's claims (with the ath of the token it is sent with) under a signature
// that verifies, from the device key; read strictly (compactjws, the
// platform's rule), each re-spelled token or proof is refused and the other
// half of the pair is canonical. So a verifier that refuses a vector refuses
// it for the spelling alone.
func TestNonCanonicalVectorsDifferOnlyInSpelling(t *testing.T) {
	out := generate()
	var valid, nc fixtureFile
	if err := json.Unmarshal(out["cases.json"], &valid); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(out["noncanonical.json"], &nc); err != nil {
		t.Fatal(err)
	}
	base := valid.Cases[0]
	if base.Name != "valid" || nc.Issuer != valid.Issuer || nc.AppCode != valid.AppCode {
		t.Fatalf("the base case or the configuration differs: %s, %s, %s", base.Name, nc.Issuer, nc.AppCode)
	}
	rsaPub := &rsaSigner().PublicKey
	device := newKey("fc-field-fixture-device-key")
	bh, bp := splitToken(base.AccessToken), splitToken(base.DPoPProof)
	baseClaims := lenient(t, "valid proof claims", bp[1])
	var baseProofClaims map[string]any
	if err := json.Unmarshal(baseClaims, &baseProofClaims); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{base.AccessToken + " " + base.DPoPProof: true}
	tokens, proofs := 0, 0
	for _, c := range nc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			if c.Method != base.Method || c.URL != base.URL || c.Now != base.Now || c.Principal != nil {
				t.Fatalf("request differs from the valid case: %+v", c)
			}
			if key := c.AccessToken + " " + c.DPoPProof; seen[key] {
				t.Fatal("duplicate of the valid case or of another vector")
			} else {
				seen[key] = true
			}
			// The proof as a forgiving verifier reads it (it trims the header).
			pp := splitToken(strings.TrimSpace(c.DPoPProof))
			sig := lenient(t, "proof signature", pp[2])
			digest := sha256.Sum256([]byte(pp[0] + "." + pp[1]))
			if len(sig) != 64 || !ecdsa.Verify(device.public(), digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
				t.Fatal("the proof's signature does not verify from the device key")
			}
			var hdr struct {
				Typ, Alg string
				JWK      map[string]string
			}
			if err := json.Unmarshal(lenient(t, "proof header", pp[0]), &hdr); err != nil {
				t.Fatal(err)
			}
			if hdr.Typ != "dpop+jwt" || hdr.Alg != "ES256" || len(hdr.JWK) != 4 || hdr.JWK["kty"] != "EC" || hdr.JWK["crv"] != "P-256" ||
				!bytes.Equal(lenient(t, "x", hdr.JWK["x"]), decode(device.x)) || !bytes.Equal(lenient(t, "y", hdr.JWK["y"]), decode(device.y)) {
				t.Fatalf("the proof header is not the device key's: %+v", hdr)
			}
			var claims map[string]any
			if err := json.Unmarshal(lenient(t, "proof claims", pp[1]), &claims); err != nil {
				t.Fatal(err)
			}
			if claims["ath"] != ath(c.AccessToken) {
				t.Fatal("the proof's ath is not the hash of the token it is sent with")
			}
			canonicalProof := compactjws.Check(c.DPoPProof) == nil &&
				func() bool { _, ex := compactjws.DecodeSegment(hdr.JWK["x"]); return ex == nil }() &&
				func() bool { _, ey := compactjws.DecodeSegment(hdr.JWK["y"]); return ey == nil }()

			switch {
			case strings.HasPrefix(c.Name, "token_"):
				tokens++
				if c.Expect != expectTokenRespelled {
					t.Fatalf("expect %q, want %q", c.Expect, expectTokenRespelled)
				}
				if c.AccessToken == base.AccessToken || compactjws.Check(c.AccessToken) == nil {
					t.Fatal("the token is spelled canonically")
				}
				if !canonicalProof {
					t.Fatal("the proof of a re-spelled token must be canonical")
				}
				tp := splitToken(c.AccessToken)
				if !bytes.Equal(lenient(t, "header", tp[0]), lenient(t, "valid header", bh[0])) ||
					!bytes.Equal(lenient(t, "claims", tp[1]), lenient(t, "valid claims", bh[1])) {
					t.Fatal("the token's header or claims differ from the valid token's")
				}
				d := sha256.Sum256([]byte(tp[0] + "." + tp[1]))
				if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, d[:], lenient(t, "signature", tp[2])); err != nil {
					t.Fatalf("the token's signature does not verify: %v", err)
				}
			case strings.HasPrefix(c.Name, "proof_"):
				proofs++
				if c.Expect != expectProofRespelled {
					t.Fatalf("expect %q, want %q", c.Expect, expectProofRespelled)
				}
				if c.AccessToken != base.AccessToken {
					t.Fatal("a re-spelled proof must come with the valid token")
				}
				if canonicalProof && c.DPoPProof == strings.TrimSpace(c.DPoPProof) {
					t.Fatal("the proof is spelled canonically")
				}
				delete(claims, "ath")
				want := maps.Clone(baseProofClaims)
				delete(want, "ath")
				if !reflect.DeepEqual(claims, want) {
					t.Fatalf("the proof's claims differ from the valid proof's: %v", claims)
				}
			default:
				t.Fatal("a vector re-spells the token or the proof")
			}
		})
	}
	// Every other last character of a 256-byte RS256 and a 64-byte ES256
	// signature (4 spare bits each), padding, CR/LF, the re-signed segments.
	if tokens < 25 || proofs < 40 {
		t.Errorf("%d token and %d proof vectors; expected at least 25 and 40", tokens, proofs)
	}
}

// TestUnlockFixturesAgreeWithThePlatform: every unlock.json vector is judged
// by the platform's own verifier as it says, its jws is its signingInput signed,
// and the attestation challenge is the platform's.
func TestUnlockFixturesAgreeWithThePlatform(t *testing.T) {
	var f unlockFile
	if err := json.Unmarshal(generate()["unlock.json"], &f); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(f.UnlockKey)
	if err != nil {
		t.Fatal(err)
	}
	pub, _, _, err := fieldauth.ParseECJWK(raw)
	if err != nil {
		t.Fatal(err)
	}
	valid := 0
	for _, c := range f.Cases {
		if !strings.HasPrefix(c.JWS, c.SigningInput+".") {
			t.Errorf("%s: jws does not sign its signingInput", c.Name)
		}
		now, err := time.Parse(time.RFC3339, c.Now)
		if err != nil {
			t.Fatal(err)
		}
		w := c.VerifyWith
		_, err = fieldauth.VerifyUnlockJWS(c.JWS, pub, fieldauth.UnlockExpect{
			Type: w.Type, KeyID: w.KeyID, Aud: w.Aud, Nonce: w.Nonce, Sub: w.Sub, DeviceID: w.DeviceID, App: w.App,
		}, now)
		switch {
		case c.Expect == "valid" && err != nil:
			t.Errorf("%s: refused: %v", c.Name, err)
		case c.Expect != "valid" && err == nil:
			t.Errorf("%s: accepted, want %s", c.Name, c.Expect)
		case c.Expect == "valid":
			valid++
		}
	}
	if valid != 2 {
		t.Errorf("want the two valid cases, got %d", valid)
	}
	got := attestation.Challenge("fws_0FIXTUREFAMILY", f.WorkerID, f.DeviceID)
	if hex.EncodeToString(got) != f.AttestationChallenge.Hex {
		t.Errorf("attestation challenge %x, vector %s", got, f.AttestationChallenge.Hex)
	}
}

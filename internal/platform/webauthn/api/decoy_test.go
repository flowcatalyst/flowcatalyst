package api

import (
	"encoding/json"
	"testing"
)

func decoyID(t *testing.T, email string) string {
	t.Helper()
	raw, err := json.Marshal(decoyChallenge("localhost", email))
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		PublicKey struct {
			Challenge        string `json:"challenge"`
			RPID             string `json:"rpId"`
			AllowCredentials []struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			} `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.PublicKey.RPID != "localhost" || len(out.PublicKey.AllowCredentials) != 1 || out.PublicKey.AllowCredentials[0].Type != "public-key" {
		t.Fatalf("decoy shape changed: %s", raw)
	}
	return out.PublicKey.AllowCredentials[0].ID
}

// L4: the decoy credential id is a function of the (normalised) email, so
// asking twice about an unknown address returns the same id, as a real account
// does; different addresses differ; the id keeps today's length (32 bytes,
// 43 base64url characters).
func TestDecoyCredentialIDIsStablePerEmail(t *testing.T) {
	a1 := decoyID(t, "nobody-l4@example.test")
	a2 := decoyID(t, "nobody-l4@example.test")
	if a1 != a2 {
		t.Fatalf("decoy id changed between calls: %q vs %q", a1, a2)
	}
	if n := decoyID(t, "  NoBody-L4@Example.TEST "); n != a1 {
		t.Fatalf("decoy id depends on case/whitespace: %q vs %q", n, a1)
	}
	if b := decoyID(t, "someone-else-l4@example.test"); b == a1 {
		t.Fatal("different emails produced the same decoy id")
	}
	if len(a1) != 43 {
		t.Fatalf("decoy id length = %d, want 43 (32 bytes, base64url)", len(a1))
	}
}

func TestDecoyChallengeStillDiffersPerCall(t *testing.T) {
	c := func() string {
		return decoyChallenge("localhost", "x@y.test")["publicKey"].(map[string]any)["challenge"].(string)
	}
	first, second := c(), c()
	if first == second {
		t.Fatal("challenge must stay random per call")
	}
}

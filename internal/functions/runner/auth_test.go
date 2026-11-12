package runner

import (
	"encoding/json"
	"testing"
)

// The invoke caller check is an allowlist: only an API bearer ("api", or a
// legacy token without the marker that carries an aud) is admitted. A
// marker-less token with no aud is a session token and is refused.
func TestCallerFromClaims_TokenUseAllowlist(t *testing.T) {
	for use, ok := range map[string]bool{
		"": true, "api": true,
		"identity": false, "field": false, "foo": false,
	} {
		c, err := callerFromClaims(platformClaims{Sub: "prn_x", TokenUse: use, Aud: json.RawMessage(`"https://platform"`)})
		if ok && (err != nil || c == nil) {
			t.Errorf("token_use=%q: want admitted, got err=%v", use, err)
		}
		if !ok && (err == nil || c != nil) {
			t.Errorf("token_use=%q: want refused, got caller=%v", use, c)
		}
	}
}

func TestCallerFromClaims_SessionTokenRefused(t *testing.T) {
	for _, aud := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`""`)} {
		if c, err := callerFromClaims(platformClaims{Sub: "prn_x", Aud: aud}); err == nil || c != nil {
			t.Errorf("aud=%s: a session-shaped token (no marker, no aud) must be refused", aud)
		}
	}
}

func TestCallerFromClaims_ClientsAndCodes(t *testing.T) {
	c, err := callerFromClaims(platformClaims{Sub: "p", Aud: json.RawMessage(`"x"`), Clients: []string{"clt_a:bevco", "clt_b", "*", "clt_c:"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Clients; len(got) != 4 || got[0] != "clt_a" || got[1] != "clt_b" || got[2] != "*" || got[3] != "clt_c" {
		t.Errorf("clients (ids) changed: %v", got)
	}
	if got := c.ClientCodes; len(got) != 2 || got[0] != "bevco" || got[1] != "*" {
		t.Errorf("clientCodes = %v, want [bevco *]", got)
	}
}

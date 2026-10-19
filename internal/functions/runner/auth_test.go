package runner

import "testing"

// The invoke caller check is an allowlist: only an API credential ("api" or a
// legacy token without the marker) is admitted.
func TestCallerFromClaims_TokenUseAllowlist(t *testing.T) {
	for use, ok := range map[string]bool{
		"": true, "api": true,
		"identity": false, "field": false, "foo": false,
	} {
		c, err := callerFromClaims(platformClaims{Sub: "prn_x", TokenUse: use})
		if ok && (err != nil || c == nil) {
			t.Errorf("token_use=%q: want admitted, got err=%v", use, err)
		}
		if !ok && (err == nil || c != nil) {
			t.Errorf("token_use=%q: want refused, got caller=%v", use, c)
		}
	}
}

func TestCallerFromClaims_ClientsAndCodes(t *testing.T) {
	c, err := callerFromClaims(platformClaims{Sub: "p", Clients: []string{"clt_a:bevco", "clt_b", "*", "clt_c:"}})
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

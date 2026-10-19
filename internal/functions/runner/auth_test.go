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

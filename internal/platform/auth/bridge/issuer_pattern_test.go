package bridge

import "testing"

func TestIsValidIssuer(t *testing.T) {
	t.Cleanup(func() { SetIssuerPatternStrict(false) })
	const base = "https://login.microsoftonline.com/organizations/v2.0"
	anchored := `^https://login\.microsoftonline\.com/[0-9a-f-]{36}/v2\.0$`
	unanchored := `https://login\.microsoftonline\.com/[0-9a-f-]{36}/v2\.0`
	tenant := "https://login.microsoftonline.com/11111111-2222-3333-4444-555555555555/v2.0"

	// Legitimate forms: exact IdP issuer, anchored and unanchored patterns
	// matching the whole issuer, a local http issuer, a Google-style issuer.
	legit := []struct{ iss, issuerURL, pattern string }{
		{base, base, ""},
		{tenant, base, anchored},
		{tenant, base, unanchored},
		{"http://localhost:8081/realms/dev", "http://x", `^http://localhost:\d+/realms/[a-z]+$`},
		{"https://accounts.google.com", "http://x", `^https://accounts\.google\.com$`},
		{"https://accounts.google.com", "http://x", `https://accounts\.google\.com`},
	}
	// Near-miss issuers that only a partial match would accept.
	nearMiss := []struct{ iss, pattern string }{
		{"https://evil.example/?next=" + tenant, unanchored},
		{tenant + "/extra", unanchored},
		{"x" + tenant, unanchored},
		{"https://accounts.google.com.evil.example", `https://accounts\.google\.com`},
		{"https://evil.example/https://accounts.google.com", `https://accounts\.google\.com`},
	}

	for _, strict := range []bool{false, true} {
		SetIssuerPatternStrict(strict)
		for _, c := range legit {
			p := c.pattern
			if got := isValidIssuer(c.iss, c.issuerURL, true, &p); !got {
				t.Errorf("strict=%v legit issuer %q rejected by %q", strict, c.iss, c.pattern)
			}
		}
		for _, c := range nearMiss {
			p := c.pattern
			got := isValidIssuer(c.iss, "https://other", true, &p)
			if strict && got {
				t.Errorf("strict: near-miss %q accepted by %q", c.iss, c.pattern)
			}
			if !strict && !got && issuerPatternAnchored(c.pattern) {
				t.Errorf("lax: anchored pattern must still reject %q", c.iss)
			}
		}
		// Anchored patterns reject near-misses in both modes.
		a := anchored
		if isValidIssuer("https://evil.example/?"+tenant, "https://other", true, &a) {
			t.Errorf("strict=%v anchored pattern accepted a near-miss", strict)
		}
		// Single-tenant: pattern never applies.
		if isValidIssuer(tenant, "https://other", false, &a) {
			t.Errorf("strict=%v pattern applied to a single-tenant IdP", strict)
		}
	}
}

func TestClaimExplicitlyFalse(t *testing.T) {
	for in, want := range map[string]bool{
		`false`: true, `"false"`: true, `"FALSE"`: true, `true`: false, `"true"`: false,
		``: false, `null`: false, `"yes"`: false, `0`: false, `{}`: false,
	} {
		if got := claimExplicitlyFalse([]byte(in)); got != want {
			t.Errorf("claimExplicitlyFalse(%q)=%v want %v", in, got, want)
		}
	}
}

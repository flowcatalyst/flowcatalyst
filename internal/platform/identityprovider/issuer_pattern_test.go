package identityprovider

import (
	"errors"
	"testing"
)

func TestCheckIssuerPattern(t *testing.T) {
	pinned := []string{
		`^https://login\.microsoftonline\.com/11111111-2222-3333-4444-555555555555/v2\.0$`,
		// Two tenants, or the v1 and v2 issuer of one: still spelled out.
		`^https://login\.microsoftonline\.com/(11111111-2222-3333-4444-555555555555|66666666-7777-8888-9999-000000000000)/v2\.0$`,
		`^https://login\.microsoftonline\.com/tenant1/v2\.0$|^https://sts\.windows\.net/tenant1/$`,
		// The parser factors "tenant1|tenant2" into "tenant[12]": countable.
		`^https://idp\.example/(tenant1|tenant2)$`,
		`^https://accounts\.google\.com/?$`,
		`(?i)^https://Login\.MicrosoftOnline\.com/abc/v2\.0$`,
	}
	for _, p := range pinned {
		if err := CheckIssuerPattern(p); err != nil {
			t.Errorf("pinned pattern %q refused: %v", p, err)
		}
	}

	broad := []string{
		"",
		// The audit's example: anchored, but any tenant.
		`^https://login\.microsoftonline\.com/[^/]+/v2\.0$`,
		`^https://login\.microsoftonline\.com/[0-9a-f-]{36}/v2\.0$`,
		`^https://login\.microsoftonline\.com/.*/v2\.0$`,
		`^https://login\.microsoftonline\.com/\w+/v2\.0$`,
		// Unescaped dots match any character.
		`^https://login.microsoftonline.com/abc/v2.0$`,
		// Unanchored, at either end or both.
		`https://login\.microsoftonline\.com/abc/v2\.0`,
		`^https://login\.microsoftonline\.com/abc/v2\.0`,
		`https://login\.microsoftonline\.com/abc/v2\.0$`,
		// An alternative without its anchors matches a substring.
		`^https://a\.example/t1|https://b\.example/t2$`,
		// The old placeholder convention is not a regex the platform expands.
		`https://login.microsoftonline.com/{tenantId}/v2.0`,
		// Line anchors match inside a multi-line string.
		`(?m)^https://a\.example/t1$`,
		// Invalid.
		`^https://a\.example/(t1$`,
	}
	for _, p := range broad {
		err := CheckIssuerPattern(p)
		if err == nil {
			t.Errorf("broad pattern %q accepted", p)
			continue
		}
		if !errors.Is(err, ErrIssuerPatternNotPinned) {
			t.Errorf("pattern %q: error %v is not ErrIssuerPatternNotPinned", p, err)
		}
	}
}

func TestHasBroadIssuerPattern(t *testing.T) {
	broad := `^https://login\.microsoftonline\.com/[^/]+/v2\.0$`
	pinned := `^https://login\.microsoftonline\.com/abc/v2\.0$`
	empty := ""
	cases := []struct {
		name string
		idp  IdentityProvider
		want bool
	}{
		{"multi-tenant broad", IdentityProvider{Type: TypeOIDC, OIDCMultiTenant: true, OIDCIssuerPattern: &broad}, true},
		{"multi-tenant pinned", IdentityProvider{Type: TypeOIDC, OIDCMultiTenant: true, OIDCIssuerPattern: &pinned}, false},
		{"multi-tenant no pattern (issuer URL exactly)", IdentityProvider{Type: TypeOIDC, OIDCMultiTenant: true}, false},
		{"multi-tenant empty pattern", IdentityProvider{Type: TypeOIDC, OIDCMultiTenant: true, OIDCIssuerPattern: &empty}, false},
		{"single-tenant: the pattern is never consulted", IdentityProvider{Type: TypeOIDC, OIDCIssuerPattern: &broad}, false},
		{"internal", IdentityProvider{Type: TypeInternal, OIDCMultiTenant: true, OIDCIssuerPattern: &broad}, false},
	}
	for _, c := range cases {
		if got := c.idp.HasBroadIssuerPattern(); got != c.want {
			t.Errorf("%s: HasBroadIssuerPattern = %v, want %v", c.name, got, c.want)
		}
	}
}

package oauthapi

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzMatchRedirectURI checks the security property of the matcher rather
// than specific answers: whatever string a client sends as redirect_uri, a
// match against a wildcard registration implies the URI really is an
// https://<one label>.example.com/... URL with no userinfo.
func FuzzMatchRedirectURI(f *testing.F) {
	registered := []string{
		"https://*.example.com/callback",
		"https://qa-*.example.com/cb/*",
		"https://app.example.com/exact",
		"http://127.0.0.1/cb",
	}
	for _, s := range []string{
		"https://a.example.com/callback",
		"https://evil%2Ecom/.example.com/callback",
		"https://evil%2Ecom#.example.com/callback",
		"https://evil.com\\@a.example.com/callback",
		"https://a.example.com@evil.com/callback",
		"https://user:pw@a.example.com/callback",
		"https://a.b.example.com/callback",
		"https://example.com/callback",
		"https://a.example.com.evil.com/callback",
		"https://a.example.com:444/callback",
		"HTTPS://A.EXAMPLE.COM/callback",
		"https://a.example.com/callback/../x",
		"https://qa-x.example.com/cb/deep/path",
		"//a.example.com/callback",
		"https://a.example.com\t/callback",
		"https://a.example.com%00.evil.com/callback",
		"", "*", "https://*", "https://*.com/",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, uri string) {
		if !MatchRedirectURI(uri, registered) {
			return
		}
		for _, r := range registered {
			if uri == r {
				return // exact registration
			}
		}
		u, err := url.Parse(uri)
		if err != nil {
			t.Fatalf("matched an unparseable URI %q: %v", uri, err)
		}
		if u.User != nil {
			t.Fatalf("matched a URI with userinfo %q", uri)
		}
		host := strings.ToLower(u.Hostname())
		if !strings.HasSuffix(host, ".example.com") && host != "app.example.com" {
			t.Fatalf("matched %q whose host %q is outside every registered domain", uri, host)
		}
		label := strings.TrimSuffix(host, ".example.com")
		if strings.ContainsAny(label, "./\\@#?") || label == "" {
			t.Fatalf("matched %q: wildcard consumed %q", uri, label)
		}
	})
}

package oauthapi

import "testing"

func TestMatchRedirectURI(t *testing.T) {
	cases := []struct {
		name       string
		uri        string
		registered []string
		want       bool
	}{
		// exact
		{"exact match", "https://app.inhanceapps.com/done", []string{"https://app.inhanceapps.com/done"}, true},
		{"no match", "https://evil.com/x", []string{"https://app.inhanceapps.com/done"}, false},

		// leftmost subdomain wildcard, any path
		{"subdomain wildcard host-only matches bare", "https://app.inhanceapps.com", []string{"https://*.inhanceapps.com"}, true},
		{"subdomain wildcard host-only matches path", "https://app.inhanceapps.com/auth/logged-out", []string{"https://*.inhanceapps.com"}, true},
		{"subdomain wildcard different sub", "https://portal.inhanceapps.com/x", []string{"https://*.inhanceapps.com"}, true},

		// partial-label leftmost wildcard (multi-tenant: one pattern, many tenants)
		{"partial wildcard matches", "https://qa-acme.inhanceapps.com/done", []string{"https://qa-*.inhanceapps.com"}, true},
		{"partial wildcard matches any path", "https://qa-acme.inhanceapps.com/x/y", []string{"https://qa-*.inhanceapps.com"}, true},
		{"partial wildcard wrong prefix", "https://prod-acme.inhanceapps.com/x", []string{"https://qa-*.inhanceapps.com"}, false},
		{"partial wildcard empty expansion rejected", "https://qa-.inhanceapps.com/x", []string{"https://qa-*.inhanceapps.com"}, false},
		{"partial wildcard cannot cross dot", "https://qa-a.b.inhanceapps.com/x", []string{"https://qa-*.inhanceapps.com"}, false},
		{"suffix-label wildcard", "https://acme-qa.inhanceapps.com/x", []string{"https://*-qa.inhanceapps.com"}, true},

		// host-confusion / over-broad — all must fail
		{"bare base domain rejected", "https://inhanceapps.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"multi-label sub rejected", "https://a.b.inhanceapps.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"suffix-append attack", "https://inhanceapps.com.evil.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"prefix-append attack", "https://app.inhanceapps.com.evil.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"substring (no dot) attack", "https://evilinhanceapps.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"userinfo host confusion", "https://app.inhanceapps.com@evil.com/x", []string{"https://*.inhanceapps.com"}, false},
		{"bare wildcard rejected", "https://anything.com", []string{"https://*"}, false},
		{"wildcard on single-label base rejected", "https://x.com", []string{"https://*.com"}, false},
		{"scheme downgrade rejected", "http://app.inhanceapps.com", []string{"https://*.inhanceapps.com"}, false},
		{"port mismatch rejected", "https://app.inhanceapps.com:8443/x", []string{"https://*.inhanceapps.com"}, false},

		// explicit path patterns
		{"exact path pattern matches", "https://app.inhanceapps.com/cb", []string{"https://*.inhanceapps.com/cb"}, true},
		{"exact path pattern other path", "https://app.inhanceapps.com/other", []string{"https://*.inhanceapps.com/cb"}, false},
		{"path-prefix wildcard", "https://app.inhanceapps.com/auth/done", []string{"https://*.inhanceapps.com/auth/*"}, true},
		{"path-prefix wildcard miss", "https://app.inhanceapps.com/admin", []string{"https://*.inhanceapps.com/auth/*"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MatchRedirectURI(c.uri, c.registered); got != c.want {
				t.Errorf("MatchRedirectURI(%q, %v) = %v, want %v", c.uri, c.registered, got, c.want)
			}
		})
	}
}

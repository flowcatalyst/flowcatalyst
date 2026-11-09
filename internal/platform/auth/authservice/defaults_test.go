package authservice

import "testing"

// A zero Config gets exactly DefaultConfig's lifetimes. The refresh-token
// default was once 30 days here against 7 days in DefaultConfig, the env
// config (OIDC_REFRESH_TOKEN_TTL) and the env doc.
func TestApplyDefaultsMatchesDefaultConfig(t *testing.T) {
	var c Config
	c.applyDefaults()
	d := DefaultConfig()
	if c.RefreshTokenExpirySecs != 7*86400 {
		t.Fatalf("refresh default = %ds, want 7 days (%ds)", c.RefreshTokenExpirySecs, 7*86400)
	}
	if c.RefreshTokenExpirySecs != d.RefreshTokenExpirySecs ||
		c.AccessTokenExpirySecs != d.AccessTokenExpirySecs ||
		c.SessionTokenExpirySecs != d.SessionTokenExpirySecs ||
		c.IDTokenExpirySecs != d.IDTokenExpirySecs ||
		c.Issuer != d.Issuer || c.Audience != d.Audience {
		t.Fatalf("applyDefaults %+v disagrees with DefaultConfig %+v", c, d)
	}

	// An explicit value is kept.
	c = Config{RefreshTokenExpirySecs: 3600}
	c.applyDefaults()
	if c.RefreshTokenExpirySecs != 3600 {
		t.Fatalf("explicit refresh lifetime overwritten: %d", c.RefreshTokenExpirySecs)
	}
}

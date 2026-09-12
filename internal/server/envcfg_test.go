package server

import "testing"

// The ECS task definitions set these deployment names; LoadEnv must honour
// them, with the FC_* name winning where one exists.
func TestLoadEnv_DeployedAliases(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		for _, k := range []string{
			"FC_API_PORT", "PORT",
			"FC_DISPATCH_PROCESSING_ENDPOINT", "DISPATCH_SCHEDULER_PROCESSING_ENDPOINT",
			"OIDC_SESSION_TTL", "FC_JWT_ACCESS_TOKEN_TTL_SECS", "OIDC_ACCESS_TOKEN_TTL", "OIDC_REFRESH_TOKEN_TTL",
		} {
			t.Setenv(k, "")
		}
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://localhost:8080/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.SessionTTLSecs != 86400 || c.AccessTokenTTLSecs != 3600 || c.RefreshTokenTTLSecs != 604800 {
			t.Errorf("TTLs = %d/%d/%d, want 86400/3600/604800", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})

	t.Run("deployed names", func(t *testing.T) {
		t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "")
		t.Setenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT", "http://fc-platform:8080/api/dispatch/process")
		t.Setenv("OIDC_SESSION_TTL", "28800")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "1800")
		t.Setenv("OIDC_REFRESH_TOKEN_TTL", "2592000")
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://fc-platform:8080/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.SessionTTLSecs != 28800 || c.AccessTokenTTLSecs != 1800 || c.RefreshTokenTTLSecs != 2592000 {
			t.Errorf("TTLs = %d/%d/%d, want 28800/1800/2592000", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})

	t.Run("FC name wins", func(t *testing.T) {
		t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "http://a/api/dispatch/process")
		t.Setenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT", "http://b/api/dispatch/process")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "600")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "1800")
		c := LoadEnv()
		if c.DispatchProcessingEndpoint != "http://a/api/dispatch/process" {
			t.Errorf("DispatchProcessingEndpoint = %q", c.DispatchProcessingEndpoint)
		}
		if c.AccessTokenTTLSecs != 600 {
			t.Errorf("AccessTokenTTLSecs = %d, want 600", c.AccessTokenTTLSecs)
		}
	})

	t.Run("non-positive falls back", func(t *testing.T) {
		t.Setenv("OIDC_SESSION_TTL", "0")
		t.Setenv("FC_JWT_ACCESS_TOKEN_TTL_SECS", "-5")
		t.Setenv("OIDC_ACCESS_TOKEN_TTL", "")
		t.Setenv("OIDC_REFRESH_TOKEN_TTL", "nope")
		c := LoadEnv()
		if c.SessionTTLSecs != 86400 || c.AccessTokenTTLSecs != 3600 || c.RefreshTokenTTLSecs != 604800 {
			t.Errorf("TTLs = %d/%d/%d, want defaults", c.SessionTTLSecs, c.AccessTokenTTLSecs, c.RefreshTokenTTLSecs)
		}
	})
}

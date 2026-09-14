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

// The deployed router task sets NOTIFICATION_BATCH_INTERVAL and
// FLOWCATALYST_CONFIG_INTERVAL (in seconds); LoadEnv must honour both, with
// the FC_* name winning where set, and leave 0 (router default) when unset.
func TestLoadEnv_RouterIntervalAliases(t *testing.T) {
	t.Run("unset leaves router defaults", func(t *testing.T) {
		for _, k := range []string{
			"FC_NOTIFY_BATCH_INTERVAL_SECONDS", "NOTIFICATION_BATCH_INTERVAL",
			"FC_ROUTER_CONFIG_INTERVAL_SECONDS", "FLOWCATALYST_CONFIG_INTERVAL",
		} {
			t.Setenv(k, "")
		}
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 0 || c.RouterConfigIntervalSec != 0 {
			t.Errorf("intervals = %d/%d, want 0/0", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
	t.Run("deployed names", func(t *testing.T) {
		t.Setenv("FC_NOTIFY_BATCH_INTERVAL_SECONDS", "")
		t.Setenv("NOTIFICATION_BATCH_INTERVAL", "300")
		t.Setenv("FC_ROUTER_CONFIG_INTERVAL_SECONDS", "")
		t.Setenv("FLOWCATALYST_CONFIG_INTERVAL", "120")
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 300 || c.RouterConfigIntervalSec != 120 {
			t.Errorf("intervals = %d/%d, want 300/120", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
	t.Run("FC name wins", func(t *testing.T) {
		t.Setenv("FC_NOTIFY_BATCH_INTERVAL_SECONDS", "45")
		t.Setenv("NOTIFICATION_BATCH_INTERVAL", "300")
		t.Setenv("FC_ROUTER_CONFIG_INTERVAL_SECONDS", "60")
		t.Setenv("FLOWCATALYST_CONFIG_INTERVAL", "120")
		c := LoadEnv()
		if c.RouterNotifyBatchIntervalSec != 45 || c.RouterConfigIntervalSec != 60 {
			t.Errorf("intervals = %d/%d, want 45/60", c.RouterNotifyBatchIntervalSec, c.RouterConfigIntervalSec)
		}
	})
}

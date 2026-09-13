package main

import (
	"testing"
)

var devCreds = routerCredentials{ClientID: "fcdev-router", Secret: "s3cret"}

// Dev and prod run one code path: the router learns its queues and pools from
// a served router-config document, so fcdev points its own router at its own
// platform. The document is an authenticated route on the API listener, so the
// URL is built off the API port and carries the bootstrapped credential.
func TestDevEnvCfg_DefaultsConfigURLToItsOwnServedDocument(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "")
	t.Setenv("FC_ROUTER_CLIENT_ID", "")
	t.Setenv("FC_ROUTER_CLIENT_SECRET", "")
	t.Setenv("FC_ROUTER_PLATFORM_URL", "")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 9090}, "postgres://localhost/fc", devCreds)

	want := "http://localhost:8080/api/dispatch/router-config"
	if cfg.RouterConfigURL != want {
		t.Errorf("RouterConfigURL = %q, want %q", cfg.RouterConfigURL, want)
	}
	if cfg.RouterClientID != "fcdev-router" || cfg.RouterClientSecret != "s3cret" {
		t.Errorf("credentials = %q/%q, want the bootstrapped pair", cfg.RouterClientID, cfg.RouterClientSecret)
	}
	// The credential belongs to a platform, and this names which one — without
	// it the router would not know which config URL may receive the token.
	if cfg.RouterPlatformURL != "http://localhost:8080" {
		t.Errorf("RouterPlatformURL = %q, want the local platform", cfg.RouterPlatformURL)
	}
}

// An operator who has already pointed the router somewhere else (at another
// config service, say) is never overridden.
func TestDevEnvCfg_KeepsAnExplicitConfigURL(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "http://integral.test/config")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 9090}, "postgres://localhost/fc", devCreds)

	if cfg.RouterConfigURL != "http://integral.test/config" {
		t.Errorf("RouterConfigURL = %q, want the explicitly configured URL", cfg.RouterConfigURL)
	}
}

// An operator's own credential wins over the bootstrapped one.
func TestDevEnvCfg_KeepsExplicitCredentials(t *testing.T) {
	t.Setenv("FC_ROUTER_CLIENT_ID", "operators-own")
	t.Setenv("FC_ROUTER_CLIENT_SECRET", "operators-secret")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 9090}, "postgres://localhost/fc", devCreds)

	if cfg.RouterClientID != "operators-own" || cfg.RouterClientSecret != "operators-secret" {
		t.Errorf("credentials = %q/%q, want the operator's own", cfg.RouterClientID, cfg.RouterClientSecret)
	}
}

// An ephemeral API port is not knowable here — EnvCfg is built before the
// listener binds — so no URL is synthesised rather than one naming port 0,
// which could never work.
func TestDevEnvCfg_EphemeralAPIPortSynthesisesNothing(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "")
	t.Setenv("FC_ROUTER_CLIENT_ID", "")
	t.Setenv("FC_ROUTER_CLIENT_SECRET", "")
	cfg := devEnvCfg(startOpts{APIPort: 0, MetricsPort: 9090}, "postgres://localhost/fc", devCreds)

	if cfg.RouterConfigURL != "" {
		t.Errorf("RouterConfigURL = %q, want empty for an ephemeral API port", cfg.RouterConfigURL)
	}
	if cfg.RouterClientID != "" {
		t.Errorf("RouterClientID = %q, want no credential without a knowable platform URL", cfg.RouterClientID)
	}
}

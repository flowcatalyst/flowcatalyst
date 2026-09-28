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

func TestDevFunctionRunner(t *testing.T) {
	opts := startOpts{APIPort: 8080, FunctionsEnabled: true, FunctionsPort: 8095, FunctionsPublicPort: 8096, FunctionsMemoryMB: 256}
	c := devFunctionRunner(opts, routerCredentials{ClientID: "fcdev-functions", Secret: "s"})
	if !c.Enabled || c.Bind != "127.0.0.1" || c.Port != 8095 || c.PublicPort != 8096 || c.PlatformURL != "http://localhost:8080" ||
		c.ClientID != "fcdev-functions" || c.MemoryLimitMB != 256 || c.ReserveMB != 32 || c.SetMemoryLimit {
		t.Fatalf("config = %+v", c)
	}
	if devFunctionRunner(opts, routerCredentials{}).Enabled {
		t.Error("enabled without a credential")
	}
	opts.APIPort = 0
	if devFunctionRunner(opts, routerCredentials{ClientID: "x", Secret: "y"}).Enabled {
		t.Error("enabled without a knowable platform URL")
	}
}

// TestDevEnvCfg_DispatchCallbackFollowsAPIPort: an fcdev on a non-default
// --api-port must have its dispatch jobs called back on its own port, not
// on whatever listens on 8080.
func TestDevEnvCfg_DispatchCallbackFollowsAPIPort(t *testing.T) {
	t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "")
	t.Setenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT", "")
	cfg := devEnvCfg(startOpts{APIPort: 18180}, "postgres://x", routerCredentials{})
	if cfg.DispatchProcessingEndpoint != "http://localhost:18180/api/dispatch/process" {
		t.Fatalf("callback = %q", cfg.DispatchProcessingEndpoint)
	}
	t.Setenv("FC_DISPATCH_PROCESSING_ENDPOINT", "http://elsewhere/api/dispatch/process")
	if cfg := devEnvCfg(startOpts{APIPort: 18180}, "postgres://x", routerCredentials{}); cfg.DispatchProcessingEndpoint != "http://elsewhere/api/dispatch/process" {
		t.Fatalf("an explicit endpoint was overridden: %q", cfg.DispatchProcessingEndpoint)
	}
}

// TestDevEnvCfg_RunnerURLFollowsFunctionsPort: promote wires deliveries to
// the runner on the port this fcdev's runner listens on.
func TestDevEnvCfg_RunnerURLFollowsFunctionsPort(t *testing.T) {
	t.Setenv("FC_FUNCTIONS_RUNNER_URL", "")
	cfg := devEnvCfg(startOpts{APIPort: 18180, FunctionsPort: 18195}, "postgres://x", routerCredentials{})
	if cfg.FunctionsRunnerURL != "http://127.0.0.1:18195" {
		t.Fatalf("runner URL = %q", cfg.FunctionsRunnerURL)
	}
}

// TestDevEnvCfg_IssuerFollowsAPIPort: tokens an fcdev on a non-default port
// mints must name that fcdev as their issuer.
func TestDevEnvCfg_IssuerFollowsAPIPort(t *testing.T) {
	for _, k := range []string{"FC_JWT_ISSUER", "FC_EXTERNAL_BASE_URL", "EXTERNAL_BASE_URL"} {
		t.Setenv(k, "")
	}
	if cfg := devEnvCfg(startOpts{APIPort: 18180}, "postgres://x", routerCredentials{}); cfg.JWTIssuer != "http://localhost:18180" {
		t.Fatalf("issuer = %q", cfg.JWTIssuer)
	}
	t.Setenv("FC_EXTERNAL_BASE_URL", "https://dev.example.test")
	if cfg := devEnvCfg(startOpts{APIPort: 18180}, "postgres://x", routerCredentials{}); cfg.JWTIssuer != "https://dev.example.test" {
		t.Fatalf("an explicit base URL was overridden: %q", cfg.JWTIssuer)
	}
}

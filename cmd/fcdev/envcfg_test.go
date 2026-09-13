package main

import (
	"testing"
)

// Dev and prod run one code path: the router learns its queues and pools from
// a served router-config document, so fcdev points its own router at its own
// platform. The document lives on the INTERNAL listener, so the URL must be
// built off the metrics port — the API port would 404.
func TestDevEnvCfg_DefaultsConfigURLToItsOwnServedDocument(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 9090}, "postgres://localhost/fc")

	want := "http://localhost:9090/api/dispatch/router-config"
	if cfg.RouterConfigURL != want {
		t.Errorf("RouterConfigURL = %q, want %q", cfg.RouterConfigURL, want)
	}
}

// An operator who has already pointed the router somewhere else (at another
// config service, say) is never overridden.
func TestDevEnvCfg_KeepsAnExplicitConfigURL(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "http://integral.test/config")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 9090}, "postgres://localhost/fc")

	if cfg.RouterConfigURL != "http://integral.test/config" {
		t.Errorf("RouterConfigURL = %q, want the explicitly configured URL", cfg.RouterConfigURL)
	}
}

// An ephemeral metrics port is not knowable here — EnvCfg is built before the
// listener binds — so no URL is synthesised rather than emitting one pointing
// at port 0, which could never work.
func TestDevEnvCfg_EphemeralMetricsPortSynthesisesNothing(t *testing.T) {
	t.Setenv("FLOWCATALYST_CONFIG_URL", "")
	cfg := devEnvCfg(startOpts{APIPort: 8080, MetricsPort: 0}, "postgres://localhost/fc")

	if cfg.RouterConfigURL != "" {
		t.Errorf("RouterConfigURL = %q, want empty for an ephemeral metrics port", cfg.RouterConfigURL)
	}
}

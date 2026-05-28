package main

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

// devEnvCfg builds the server.EnvCfg fc-dev hands to server.Run. Starts
// from the env-driven LoadEnv() so explicit FC_* overrides win, then
// applies dev-friendly defaults: every subsystem on, embedded broker
// (Postgres-backed queue on the shared pool), and the X-FC-Test-Principal
// escape hatch so engineers can hit /api/* without a real token.
//
// JWT signing keys stay ephemeral in dev — pin them with
// FC_JWT_SIGNING_KEY_PATH if needed.
func devEnvCfg(opts startOpts, databaseURL string) server.EnvCfg {
	cfg := server.LoadEnv()
	cfg.DatabaseURL = databaseURL
	cfg.APIPort = opts.APIPort
	cfg.MetricsPort = opts.MetricsPort

	// Always-on in dev.
	cfg.PlatformEnabled = true
	cfg.AuthAllowTestHeaders = true

	// Subsystem toggles follow the CLI flags. Defaults (in flag config)
	// match the historical fc-dev: scheduler+stream on, outbox+router off.
	cfg.SchedulerEnabled = opts.SchedulerEnabled
	cfg.StreamEnabled = opts.StreamEnabled
	cfg.OutboxEnabled = opts.OutboxEnabled
	cfg.RouterEnabled = opts.RouterEnabled
	cfg.MCPEnabled = opts.MCPEnabled

	// Default broker: dev runs the Postgres queue against the same pool
	// the platform uses. Override with FC_DEFAULT_BROKER=none to turn
	// off the in-process fallback (router then needs FLOWCATALYST_CONFIG_URL).
	if cfg.DefaultBroker == "" {
		cfg.DefaultBroker = "postgres"
	}
	return cfg
}

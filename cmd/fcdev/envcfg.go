package main

import (
	"fmt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

// devEnvCfg builds the server.EnvCfg fcdev hands to server.Run. Starts
// from the env-driven LoadEnv() so explicit FC_* overrides win, then
// applies dev-friendly defaults: every subsystem on, embedded broker
// (Postgres-backed queue on the shared pool), and the X-FC-Test-Principal
// escape hatch so engineers can hit /api/* without a real token.
//
// JWT signing keys stay ephemeral in dev — pin them with
// FC_JWT_SIGNING_KEY_PATH if needed.
func devEnvCfg(opts startOpts, databaseURL string, routerCreds routerCredentials) server.EnvCfg {
	cfg := server.LoadEnv()
	cfg.DatabaseURL = databaseURL
	cfg.APIPort = opts.APIPort
	cfg.MetricsPort = opts.MetricsPort

	// Always-on in dev.
	cfg.PlatformEnabled = true
	cfg.AuthAllowTestHeaders = true

	// Subsystem toggles follow the CLI flags. Defaults (in flag config)
	// match the historical fcdev: scheduler+stream on, outbox+router off.
	cfg.SchedulerEnabled = opts.SchedulerEnabled
	cfg.ScheduledJobEnabled = opts.ScheduledJobEnabled
	cfg.StreamEnabled = opts.StreamEnabled
	cfg.OutboxEnabled = opts.OutboxEnabled
	cfg.RouterEnabled = opts.RouterEnabled
	cfg.MCPEnabled = opts.MCPEnabled

	// Dev and prod run ONE code path: the router learns its queues and pools
	// from a served router-config document. So point it at this fcdev's own
	// platform, whose document names Postgres-backed queues rather than SQS
	// ones — the only dev/prod difference is the queue type inside it.
	//
	// The document is an authenticated route on the API listener, so the URL
	// is built off APIPort and the credential bootstrapped alongside it is
	// what the router presents. Only defaulted: an operator who has already
	// pointed FLOWCATALYST_CONFIG_URL somewhere else (at another config
	// service, say), or supplied their own credential, is never overridden.
	//
	// An ephemeral --api-port 0 is not knowable here — EnvCfg is built before
	// the listener binds — so nothing is synthesised rather than a URL naming
	// port 0, which could never work. The router then runs with no queues, as
	// it did before.
	if opts.APIPort > 0 {
		base := fmt.Sprintf("http://localhost:%d", opts.APIPort)
		if cfg.RouterConfigURL == "" {
			cfg.RouterConfigURL = base + "/api/dispatch/router-config"
		}
		if cfg.RouterClientID == "" && routerCreds.ClientID != "" {
			cfg.RouterClientID = routerCreds.ClientID
			cfg.RouterClientSecret = routerCreds.Secret
			// The credential belongs to one platform, and this names which.
			// Setting it also turns on the A-01 settled-group hook against
			// that platform — the intended behaviour whenever the router
			// consumes its dispatch queues.
			if cfg.RouterPlatformURL == "" {
				cfg.RouterPlatformURL = base
			}
		}
	}
	return cfg
}

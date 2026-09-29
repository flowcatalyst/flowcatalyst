package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
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

	// The debug surface (pprof, expvar, the router dump) is on in dev unless
	// FC_DEBUG_ENDPOINTS_ENABLED says otherwise. It stays authenticated: an
	// anchor login (the dev admin) or router Basic auth.
	if _, set := os.LookupEnv("FC_DEBUG_ENDPOINTS_ENABLED"); !set {
		cfg.DebugEndpointsEnabled = true
	}

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
		// LoadEnv derived the dispatch callback from FC_API_PORT's value
		// before --api-port applied; re-derive it from the port this
		// instance actually serves, or every dispatch job's delivery is
		// called back on whatever listens on the default port.
		if os.Getenv("FC_DISPATCH_PROCESSING_ENDPOINT") == "" && os.Getenv("DISPATCH_SCHEDULER_PROCESSING_ENDPOINT") == "" {
			cfg.DispatchProcessingEndpoint = base + "/api/dispatch/process"
		}
		// Likewise the token issuer (and audience, which follows it): the
		// default names port 8080, so tokens minted here claimed another
		// server's issuer, and anything validating by issuer discovery
		// fetched that server's keys.
		if os.Getenv("FC_JWT_ISSUER") == "" && os.Getenv("FC_EXTERNAL_BASE_URL") == "" && os.Getenv("EXTERNAL_BASE_URL") == "" {
			cfg.JWTIssuer = base
		}
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
	// Promote wires subscriptions and schedules to this instance's own
	// runner, on the port it actually listens on.
	if os.Getenv("FC_FUNCTIONS_RUNNER_URL") == "" && opts.FunctionsPort > 0 {
		cfg.FunctionsRunnerURL = fmt.Sprintf("http://127.0.0.1:%d", opts.FunctionsPort)
	}
	// Function artifacts live beside fcdev's other data, not under the
	// deployed default (/var/lib), unless FC_FUNCTIONS_ARTIFACT_STORE says.
	if cfg.FunctionsArtifactStore == "" {
		cfg.FunctionsArtifactStore = "file://" + filepath.Join(userDataDir(), "flowcatalyst", "functions", "artifacts")
	}
	// Developers run webhook receivers on their own machine and network, so the
	// delivery guard allows loopback and private targets here unless the
	// environment says otherwise. Cloud metadata and link-local stay blocked.
	if os.Getenv("FC_DELIVERY_ALLOW_LOOPBACK") == "" {
		netguard.Default.AllowLoopback = true
	}
	if os.Getenv("FC_DELIVERY_ALLOW_PRIVATE") == "" {
		netguard.Default.AllowPrivate = true
	}
	cfg.ApplyDeliveryPolicy(netguard.Default)
	return cfg
}

// functionsLocalClientID is the fixed client_id of the dev function runner's
// OAuth client (see bootstrapLocalCredentials).
const functionsLocalClientID = "fcdev-functions"

// fnCLILocalClientID is the fixed client_id of the dev `fcdev fn` CLI.
const fnCLILocalClientID = "fcdev-fn-cli"

// devFunctionRunner configures the in-process function runner: loopback
// listeners, this fcdev's own platform, the bootstrapped credential, a
// memory budget sized for a machine running everything, and caches in the
// OS cache directory. FC_FUNCTIONS_* overrides still apply through the flags.
func devFunctionRunner(opts startOpts, creds routerCredentials) *server.FunctionRunnerConfig {
	c := server.LoadFunctionRunnerEnv(server.EnvCfg{PlatformEnabled: true, APIPort: opts.APIPort})
	c.Enabled = opts.FunctionsEnabled && creds.ClientID != "" && opts.APIPort > 0
	c.Bind = "127.0.0.1"
	c.Port = opts.FunctionsPort
	c.PublicPort = opts.FunctionsPublicPort
	c.PlatformURL = fmt.Sprintf("http://localhost:%d", opts.APIPort)
	c.ClientID, c.ClientSecret = creds.ClientID, creds.Secret
	c.MemoryLimitMB = opts.FunctionsMemoryMB
	if c.ReserveMB == 0 {
		// The platform's own memory is outside the runner's budget here, so
		// only a small reserve for the runner itself.
		c.ReserveMB = 32
	}
	c.SetMemoryLimit = false // shares the process with the platform
	if dir, err := os.UserCacheDir(); err == nil && c.CacheDir == "" {
		c.CacheDir = filepath.Join(dir, "flowcatalyst", "fcdev-functions")
	}
	return &c
}

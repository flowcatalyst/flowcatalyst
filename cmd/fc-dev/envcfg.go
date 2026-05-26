package main

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

// devEnvCfg returns a server.EnvCfg with dev-friendly defaults. The
// real OAuth signing key + global secret are ephemeral (regenerated
// every restart) so engineers don't need to manage a keyring locally.
//
// Override with FC_JWT_SIGNING_KEY_PATH or FC_OAUTH_GLOBAL_SECRET to
// pin them across restarts.
func devEnvCfg(databaseURL string) server.EnvCfg {
	cfg := server.LoadEnv()
	cfg.DatabaseURL = databaseURL
	// In dev we always run the platform API.
	cfg.PlatformEnabled = true
	// Dev defaults the X-FC-Test-Principal escape hatch ON so engineers
	// can hit /api/* without minting a real token. Production never sets
	// this — fc-server reads it from FC_AUTH_ALLOW_TEST_HEADERS.
	cfg.AuthAllowTestHeaders = true
	return cfg
}

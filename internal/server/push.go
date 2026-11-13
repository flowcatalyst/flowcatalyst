package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// PushAuth is what the push gateway authenticates a subscriber with: the platform's token
// validators. WirePlatform hands the ones it built, so a gateway in the same process shares the
// signing key (an ephemeral dev key is generated once); a gateway running alone builds its own
// from the same configuration (FC_JWT_*), which must then name the platform's key.
type PushAuth struct {
	Provider *provider.Provider
	Auth     *authservice.AuthService
}

// pushEnvPrefix is the prefix of the gateway's settings in the environment: FC_PUSH_STREAM_SECRET
// is the framework module's push.stream.secret (PUSH_STREAM_SECRET), and so on for every
// push.* setting of the module (framework contract section 10).
const pushEnvPrefix = "FC_PUSH_"

// PushEnvironment returns the gateway's settings from environ ("NAME=value" pairs) as the
// framework module reads them: every FC_PUSH_X becomes PUSH_X, except the component flag
// FC_PUSH_ENABLED. APP_ENV is production so that no local configuration file is read.
func PushEnvironment(environ []string) map[string]string {
	env := map[string]string{"APP_ENV": "production"}
	for _, kv := range environ {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, pushEnvPrefix) || name == "FC_PUSH_ENABLED" || name == "FC_PUSH_DB_POOL_MAX" {
			continue
		}
		env["PUSH_"+strings.TrimPrefix(name, pushEnvPrefix)] = val
	}
	return env
}

// startPushIfEnabled starts the push gateway (cfg.PushEnabled) and mounts its routes on r at
// /api/push/*. It returns nil when the component is off. The gateway needs the platform database
// (pool); with the platform API in this process (handles.PushAuth set) it shares its token
// validators, otherwise it builds its own.
func startPushIfEnabled(ctx context.Context, r chi.Router, pool *pgxpool.Pool, cfg EnvCfg, handles PlatformHandles) (*pushgw.Gateway, error) {
	if !cfg.PushEnabled {
		return nil, nil
	}
	if pool == nil {
		return nil, errors.New("push gateway: FC_PUSH_ENABLED needs the platform database")
	}
	pa := handles.PushAuth
	if pa == nil {
		if err := requireStandaloneSigningKey(cfg); err != nil {
			return nil, err
		}
		var err error
		pa = &PushAuth{}
		if pa.Provider, pa.Auth, err = newAuthCore(cfg, principal.NewRepository(pool), role.NewRepository(pool)); err != nil {
			return nil, fmt.Errorf("push gateway: %w", err)
		}
	}
	csrf, err := platformmw.SessionCSRFChecker(publicOrigins(cfg.JWTIssuer))
	if err != nil {
		return nil, fmt.Errorf("push gateway: session CSRF rule: %w", err)
	}
	gw, err := pushgw.Start(ctx, pushgw.Config{
		Pool:        pool,
		DatabaseURL: cfg.DatabaseURL,
		Env:         PushEnvironment(os.Environ()),
		RedisURL:    os.Getenv("FC_REDIS_URL"),
		Authenticator: &pushgw.Authenticator{
			Provider: pa.Provider,
			Auth:     pa.Auth,
			Field:    &fieldidentity.Ops{R: fieldidentity.NewRepos(pool)},
			CSRF:     csrf,
			BaseURL:  cfg.JWTIssuer,
			Replay:   pushgw.PGReplayGuard{Pool: pool},
		},
		Origins:   cors.NewRepository(pool),
		DBPoolMax: envInt("FC_PUSH_DB_POOL_MAX", 10),
	})
	if err != nil {
		return nil, err
	}
	// ingest (POST /api/push/batch): bearer credentials only, never the session cookie
	authn := platformmw.Authenticator(platformmw.AuthConfig{Provider: pa.Provider, AllowTestHeaders: cfg.AuthAllowTestHeaders})
	r.Handle(pushgw.IngestPath, gw.IngestHandler(authn))
	r.Handle(pushgw.Mount, gw.Handler())
	r.Get("/health/push", func(w http.ResponseWriter, _ *http.Request) {
		// readiness of the gateway: started, the hub follows the log, not shutting down
		if !gw.Ready() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	slog.Info("push gateway started", "mount", pushgw.Mount)
	return gw, nil
}

// requireStandaloneSigningKey refuses a gateway that runs without the platform API (it validates
// tokens the platform minted) when no JWT signing key is configured: it would generate an
// ephemeral key of its own and refuse every token. The configuration must name the platform's key
// (FLOWCATALYST_JWT_PRIVATE_KEY, FC_JWT_SIGNING_KEY_PEM or FC_JWT_SIGNING_KEY_PATH), the same
// settings as the platform's. A development process (FLOWCATALYST_DEV_MODE) may generate one.
func requireStandaloneSigningKey(cfg EnvCfg) error {
	if SigningKeyConfigured(cfg.JWTSigningKeyPath) || cfg.RouterDevMode {
		return nil
	}
	return errors.New("push gateway: FC_PLATFORM_ENABLED is off and no JWT signing key is configured; " +
		"set the platform's key (FLOWCATALYST_JWT_PRIVATE_KEY, FC_JWT_SIGNING_KEY_PEM or FC_JWT_SIGNING_KEY_PATH) " +
		"so that the tokens the platform issues validate here (an ephemeral key would refuse every token)")
}

// pushSignOut returns the hook every sign-out calls (the login endpoint's /auth/logout, the OIDC
// session end, the console's /ui/logout): it ends the signed-out principal's push streams on every
// gateway task. The gateway may run in another process, so it is a NOTIFY on the platform database
// (pushgw.NotifyPrincipalEnd), sent whether or not a gateway runs here; it is best effort, as a
// sign-out must not fail for it.
func pushSignOut(p *provider.Provider, pool *pgxpool.Pool) func(*http.Request) {
	return func(r *http.Request) {
		// Sign-out only: whichever name is present identifies who signs out.
		tok, _ := platformmw.ReadSessionCookie(r)
		if tok == "" {
			return
		}
		claims, err := p.ValidateSessionToken(r.Context(), tok)
		if err != nil || claims == nil || claims.Subject == "" {
			return
		}
		ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
		defer cancel()
		if err := pushgw.NotifyPrincipalEnd(ctx, pool, claims.Subject); err != nil {
			slog.Warn("push gateway: could not announce a sign-out", "err", err)
		}
	}
}

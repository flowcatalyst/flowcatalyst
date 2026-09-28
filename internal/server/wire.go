package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	platformsink "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// WirePlatform instantiates every subdomain's repository + operations +
// HTTP routes against the supplied pool and registers them on r. The
// resulting router is the platform's complete public API surface.
//
// The wiring is phase-aligned across sibling files:
//
//	wire_repos.go    — buildRepos: one repository per subdomain
//	wire_services.go — buildServices: auth provider, OAuth token service,
//	                   webauthn, email/2FA, login endpoint
//	wire_public.go   — registerPublicRoutes: everything OUTSIDE the auth
//	                   middleware (login, publicapi, password reset, and the
//	                   OAuth/OIDC provider surface: /oauth/* + /.well-known)
//	wire_routes.go   — registerPlatformAPI: the auth Group, the huma API,
//	                   and every <pkg>api.Register call. Adding a new
//	                   subdomain is a four-line ritual there: build the
//	                   repo, build the use cases, build the api.State,
//	                   register it on the huma API.
//	wire_spec.go     — registerSpecRoutes: unauthenticated OpenAPI/Swagger
//
// PlatformHandles are what the rest of the server needs from a wired
// platform.
type PlatformHandles struct {
	// Authenticate is the platform's bearer/session authenticator, without
	// the X-FC-Test-Principal dev fallback: it attaches the caller's
	// AuthContext (or none) to the request. The debug surface's gate uses it.
	Authenticate func(http.Handler) http.Handler
}

func WirePlatform(r chi.Router, pool *pgxpool.Pool, cfg EnvCfg, dispatchSettings dispatch.Settings) (PlatformHandles, error) {
	// Wire the huma error transformer so handler-returned *usecase.Error
	// values flow out as the canonical {code, message, details} envelope.
	httpcompat.Init()

	sink := platformsink.New()
	uow := usecasepgx.New(pool, sink)

	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	if err != nil {
		return PlatformHandles{}, err
	}

	registerPublicRoutes(r, cfg, pool, uow, repos, svcs)
	humaAPI := registerPlatformAPI(r, cfg, pool, uow, repos, svcs, dispatchSettings)
	registerSpecRoutes(r, humaAPI)
	return PlatformHandles{
		Authenticate: platformmw.Authenticator(platformmw.AuthConfig{Provider: svcs.authProvider}),
	}, nil
}

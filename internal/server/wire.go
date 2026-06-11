package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	platformsink "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// WirePlatform instantiates every subdomain's repository + operations +
// HTTP routes against the supplied pool and registers them on r. The
// resulting router is the same surface the Rust fc-platform exposes.
//
// The wiring is phase-aligned across sibling files:
//
//	wire_repos.go    — buildRepos: one repository per subdomain
//	wire_services.go — buildServices: auth provider, OAuth token service,
//	                   webauthn, email/2FA, login endpoint
//	wire_public.go   — registerPublicRoutes: everything OUTSIDE the auth
//	                   middleware (login, publicapi, password reset,
//	                   /oauth/authorize)
//	wire_routes.go   — registerPlatformAPI: the auth Group, the huma API,
//	                   and every <pkg>api.Register call. Adding a new
//	                   subdomain is a four-line ritual there: build the
//	                   repo, build the use cases, build the api.State,
//	                   register it on the huma API.
//	wire_spec.go     — registerSpecRoutes: unauthenticated OpenAPI/Swagger
func WirePlatform(r chi.Router, pool *pgxpool.Pool, cfg EnvCfg) error {
	// Wire the huma error transformer so handler-returned *usecase.Error
	// values flow out as the canonical {code, message, details} envelope.
	httpcompat.Init()

	sink := platformsink.New()
	uow := usecasepgx.New(pool, sink)

	repos := buildRepos(pool)
	svcs, err := buildServices(cfg, pool, repos)
	if err != nil {
		return err
	}

	registerPublicRoutes(r, cfg, pool, uow, repos, svcs)
	humaAPI := registerPlatformAPI(r, cfg, pool, uow, repos, svcs)
	registerSpecRoutes(r, humaAPI)
	return nil
}

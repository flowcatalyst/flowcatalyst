package server

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	functioncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	platformsink "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
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
	// FunctionControlListener fans out `NOTIFY fng_desired` to the function
	// control plane's held long-polls (docs/function-runner-plan.md §8.3,
	// internal/platform/function/control). The caller (run.go) starts its
	// Run(ctx) as a background subsystem; nil when platform wiring built no
	// pool.
	FunctionControlListener *functioncontrol.Listener
	// PushAuth is what the push gateway authenticates with when it runs in this process: the
	// platform's own token validators, so an ephemeral dev signing key is shared, not regenerated.
	PushAuth *PushAuth
	// Payloads is the payload service the platform's readers and writers
	// share; Run reports its health (GET /health/payload-store and the
	// metrics listener's /ready).
	Payloads *payloadstore.Service
	// FieldAttestationSweep checks each fetched attestation key status list
	// against the registered unlock keys; Run hands it to the list's fetcher.
	FieldAttestationSweep func(context.Context, *attestation.StatusList)
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
	// The platform's own events follow the payload service's switch
	// (FC_PAYLOAD_TABLE_WRITES); set before anything is served.
	sink.UsePayloads(svcs.payloads)

	registerPublicRoutes(r, cfg, pool, uow, repos, svcs)
	humaAPI := registerPlatformAPI(r, cfg, pool, uow, repos, svcs, dispatchSettings)
	registerSpecRoutes(r, humaAPI)
	if err := registerUI(r, humaAPI, cfg, pool, uow, repos, svcs); err != nil {
		return PlatformHandles{}, err
	}
	return PlatformHandles{
		Authenticate:            platformmw.Authenticator(platformmw.AuthConfig{Provider: svcs.authProvider, StepUpMaxAge: cfg.stepUpMaxAge()}),
		FunctionControlListener: svcs.functionControlListener,
		PushAuth:                &PushAuth{Provider: svcs.authProvider, Auth: svcs.authSvc},
		Payloads:                svcs.payloads,
		FieldAttestationSweep:   fieldAttestationSweep(repos, uow),
	}, nil
}

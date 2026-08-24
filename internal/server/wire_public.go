package server

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	passwordresetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/publicapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// registerPublicRoutes mounts everything that must live OUTSIDE the
// bearer-token middleware: the SPA login surface, pre-sign-in read-only
// endpoints, the password-reset flow, and the whole OAuth/OIDC provider
// surface (/oauth/* plus the /.well-known documents).
func registerPublicRoutes(r chi.Router, cfg EnvCfg, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, repos *repoSet, svcs *serviceSet) {
	// Public auth surface: SPA login + cookie acquisition. MUST live
	// outside the bearer-token middleware below — a stale fc_session
	// cookie from a previous run would otherwise 401 the request before
	// the SPA could re-authenticate.
	svcs.loginEP.RegisterPublicRoutes(r)

	// Public read-only endpoints the SPA hits before sign-in
	// (login-theme branding, platform feature flags). Mounted outside
	// the auth middleware for the same reason as the login surface.
	// The client resolver turns the ?client=<identifier> branding hint
	// (passed through /oauth/authorize) into the client whose theme
	// overrides the platform one.
	publicapi.New(repos.platformConfigRepo).
		WithClientResolver(func(ctx context.Context, identifier string) (string, error) {
			c, err := repos.clientRepo.FindByIdentifier(ctx, identifier)
			if err != nil || c == nil {
				return "", err
			}
			return c.ID, nil
		}).
		RegisterRoutes(r)

	// Unauthenticated password-reset flow (request/validate/confirm). Public
	// like /auth/login. Email is delivered via the SMTP_* env (SendGrid in
	// prod); when SMTP isn't configured the message is logged instead. Delivery
	// is best-effort — a send failure never fails the request.
	// emailSvc is the shared mailer constructed with the 2FA services.
	passwordresetapi.RegisterRoutes(r, &passwordresetapi.State{
		Principals:       repos.principalRepo,
		PortalIdentities: repos.portalIdentityRepo,
		Tokens:           repos.resetTokenRepo,
		UoW:              uow,
		ExternalBaseURL:  cfg.JWTIssuer,
		Emailer:          passwordresetapi.NewEmailer(svcs.emailSvc, repos.platformConfigRepo),
		// Post-reset hygiene: refresh tokens minted under the old
		// credential are revoked (matches change-password).
		RefreshTokens: grantstore.NewRefreshTokenRepository(pool),
		// 2FA hand-off: clear-on-reset_2fa, revoke remembered devices, and
		// return enrollment_required when the domain compels a second factor.
		MFA:       svcs.mfaSvc,
		MFATokens: svcs.mfaTokens,
		Policy:    svcs.twofaPolicy,
		Notifier:  svcs.notifier,
		// Phase 8: a self-service reset with no strong factor queues for
		// client-admin approval and notifies them, instead of issuing a token.
		Approvals:    repos.resetApprovalRepo,
		ClientAdmins: repos.principalRepo,
	})

	// The OAuth/OIDC provider surface is mounted OUTSIDE the bearer-token
	// middleware in its entirety. These endpoints ARE the authentication
	// system; putting them behind the platform's API-credential middleware
	// gains nothing and actively breaks callers:
	//
	//   - Every one of them authenticates its own caller — client_secret
	//     basic/post on token/introspect/revoke, the access token itself on
	//     userinfo — or is unauthenticated by design (discovery, JWKS). None
	//     reads the AuthContext, so the middleware contributes no protection.
	//   - Authenticator hard-fails an explicit Authorization: Bearer it won't
	//     accept, before the handler runs. That 401s the RFC-mandated call
	//     shape: /oauth/userinfo is invoked WITH the access token as a bearer,
	//     and an ordinary (non-APIAccess) OIDC client holds a token_use=identity
	//     token, which the middleware rejects outright. Same foot-gun for a
	//     refresh_token call that leaves a stale bearer attached, or a JWKS
	//     fetch from a client that blanket-attaches credentials.
	//
	// /oauth/authorize additionally needs this placement because an absent or
	// expired session must redirect to login rather than 401; it validates the
	// session cookie itself.
	//
	// CorrelationID is applied here so these routes keep the X-Correlation-ID
	// echo they had inside the auth group; only Authenticator is dropped.
	r.Group(func(r chi.Router) {
		r.Use(platformmw.CorrelationID)

		svcs.oauthTokenEP.RegisterAuthorizeRoutes(r.With(ratelimit.IPLimitMiddleware(svcs.rlStore, ratelimit.BucketOAuthAuthorizeIP, svcs.rlPolicies.OAuthAuthorizeIP)))
		svcs.oauthTokenEP.RegisterTokenRoutes(r.With(
			ratelimit.GovernorMiddleware(svcs.oauthTokenIPGov, "rate limit exceeded for this IP"),
			ratelimit.IPLimitMiddleware(svcs.rlStore, ratelimit.BucketOAuthTokenIP, svcs.rlPolicies.OAuthTokenIP),
		))
		svcs.oauthTokenEP.RegisterIntrospectRoutes(r)
		svcs.oauthTokenEP.RegisterRevokeRoutes(r)
		svcs.oauthTokenEP.RegisterUserinfoRoutes(r)
		svcs.oauthTokenEP.RegisterDiscoveryRoutes(r)
	})

	// POST /api/dispatch/process — the message router's delivery callback.
	// MUST be outside the bearer middleware: the router authenticates with the
	// scheduler's HMAC job token (verified inside the handler), not a platform
	// JWT. Skipped only when the dispatch-auth secret can't be derived (no
	// FLOWCATALYST_APP_KEY) — same fail-closed condition as StartScheduler.
	if secret, err := dispatchAuthSecret(); err == nil {
		dispatchprocessing.New(repos.dispatchJobRepo, scheduler.NewDispatchAuthService(secret)).
			WithDeliveryCredsResolver(dispatchDeliveryCredsResolver(repos)).
			Mount(r)
	} else {
		slog.Warn("dispatch-processing callback not mounted: cannot derive dispatch-auth secret", "err", err)
	}
}

// dispatchDeliveryCredsResolver resolves a dispatch job's delivery
// credentials (bearer token + signing secret): job → subscription →
// application (by code) → the application service account's webhook
// credentials. Each hop's result is cached inside the shared SA resolver
// (60s TTL); subscription/application lookups are cheap indexed reads. Jobs
// without a subscription (or whose chain resolves no credentials) deliver
// bare.
func dispatchDeliveryCredsResolver(repos *repoSet) dispatchprocessing.DeliveryCredsResolver {
	byAppID := serviceaccount.NewCachedOutboundCredsResolver(repos.serviceAccountRepo, time.Minute)
	return func(ctx context.Context, job *dispatchjob.DispatchJob) (serviceaccount.OutboundCreds, error) {
		appCode := ""
		if job.SubscriptionID != nil && *job.SubscriptionID != "" {
			sub, err := repos.subscriptionRepo.FindByID(ctx, *job.SubscriptionID)
			if err != nil {
				return serviceaccount.OutboundCreds{}, err
			}
			if sub != nil && sub.ApplicationCode != nil {
				appCode = *sub.ApplicationCode
			}
		}
		if appCode == "" {
			// Direct dispatch jobs (explicit targetUrl, no subscription) have
			// no subscription to walk — but a fully qualified code's first
			// segment IS the application code, so they still resolve signing
			// credentials. Bare legacy codes fall through to an unknown-app
			// lookup and deliver unsigned, which the SDKs now prevent at
			// emission (QualifiedCode / assertQualifiedCode).
			if seg, _, ok := strings.Cut(job.Code, ":"); ok && seg != "" {
				appCode = seg
			}
		}
		if appCode == "" {
			return serviceaccount.OutboundCreds{}, nil
		}
		app, err := repos.applicationRepo.FindByCode(ctx, appCode)
		if err != nil || app == nil {
			return serviceaccount.OutboundCreds{}, err
		}
		return byAppID(ctx, app.ID)
	}
}

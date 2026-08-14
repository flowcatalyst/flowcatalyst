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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// registerPublicRoutes mounts everything that must live OUTSIDE the
// bearer-token middleware: the SPA login surface, pre-sign-in read-only
// endpoints, the password-reset flow, and /oauth/authorize (which
// redirects to login instead of 401-ing).
func registerPublicRoutes(r chi.Router, cfg EnvCfg, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, repos *repoSet, svcs *serviceSet) {
	// Public auth surface: SPA login + cookie acquisition. MUST live
	// outside the bearer-token middleware below — a stale fc_session
	// cookie from a previous run would otherwise 401 the request before
	// the SPA could re-authenticate.
	svcs.loginEP.RegisterPublicRoutes(r)

	// Public read-only endpoints the SPA hits before sign-in
	// (login-theme branding, platform feature flags). Mounted outside
	// the auth middleware for the same reason as the login surface.
	publicapi.New(repos.platformConfigRepo).RegisterRoutes(r)

	// Unauthenticated password-reset flow (request/validate/confirm). Public
	// like /auth/login. Email is delivered via the SMTP_* env (SendGrid in
	// prod); when SMTP isn't configured the message is logged instead. Delivery
	// is best-effort — a send failure never fails the request (matching Rust).
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

	// /oauth/authorize is mounted OUTSIDE the auth middleware: an absent or
	// expired session must redirect to login (not 401), and the handler
	// validates the session cookie itself. Wrapped in the per-IP throttle.
	svcs.oauthTokenEP.RegisterAuthorizeRoutes(r.With(ratelimit.IPLimitMiddleware(svcs.rlStore, ratelimit.BucketOAuthAuthorizeIP, svcs.rlPolicies.OAuthAuthorizeIP)))

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

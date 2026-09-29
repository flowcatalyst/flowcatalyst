package server

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	functioncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/control"
	functionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/versioncache"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
)

// serviceSet bundles the shared services WirePlatform threads through the
// public and authenticated route registrations. Field names match the
// original wire.go locals so the wiring code reads as `svcs.<old name>`.
type serviceSet struct {
	authProvider        *provider.Provider
	authSvc             *authservice.AuthService
	encSvc              *encryption.Service
	rlStore             ratelimit.Store
	rlPolicies          ratelimit.Policies
	oauthTokenIPGov     *ratelimit.Governor
	oauthTokenClientGov *ratelimit.Governor
	oauthTokenEP        *oauthapi.State
	webauthnService     *webauthn.Service
	emailSvc            email.Service
	platformName        func(context.Context) string
	mfaSvc              *mfa.Service
	mfaTokens           *mfatoken.Issuer
	notifier            *notify.Notifier
	twofaPolicy         twofa.Policy
	loginEP             *login.Endpoint
	principalVersions   *versioncache.Reader
	functionArtifacts   artifact.Store
	// functionControlListener fans out NOTIFY fng_desired to the control
	// plane's held long-polls (docs/function-runner-plan.md §8.3). Started
	// by run.go via PlatformHandles.FunctionControlListener; nil only when
	// buildServices was handed a nil pool.
	functionControlListener *functioncontrol.Listener
	// functionLoader reads a published artifact's fc_describe document the
	// way the runner will (plan §3, §8.2 WP4 task 2): the platform's own
	// tiny, no-capability wazero engine, built once here and shared by
	// every publish.
	functionLoader *runtimes.Loader
	// functionWiring bundles the repositories + FC_FUNCTIONS_RUNNER_URL
	// template the promote wiring reconciliation (WP8) composes against.
	functionWiring functionops.WiringDeps
}

// functionDescribeMemoryLimitBytes / functionDescribeMemoryReserveBytes size
// the platform's own tiny engine for reading fc_describe at publish (plan
// §3: "at publish it instantiates the module with no capabilities to read
// its manifest... That is safe because the module is sandboxed"). Far more
// than any describe call needs; reserve 0 because this engine hosts nothing
// else (plan §8.2 WP4 task 2: "its own small budget (e.g. 256 MiB, reserve
// 0)").
const (
	functionDescribeMemoryLimitBytes   = 256 << 20
	functionDescribeMemoryReserveBytes = 0
)

// defaultFunctionArtifactDir is the file-store root used when
// FC_FUNCTIONS_ARTIFACT_STORE is unset (docs/function-runner-plan.md §8.4).
// A deployed environment is expected to set FC_FUNCTIONS_ARTIFACT_STORE to
// an s3:// URL; this is only the fallback for a boot with neither set.
// fc-dev wiring its own state directory in here is a later work package
// (§12.1 WP9) — out of WP3's scope.
const defaultFunctionArtifactDir = "/var/lib/flowcatalyst/functions/artifacts"

func buildServices(cfg EnvCfg, pool *pgxpool.Pool, repos *repoSet) (*serviceSet, error) {
	svcs := &serviceSet{}

	// Session + refresh-token lifetimes are package-level settings read at
	// issue time; set them before any route that mints either is wired.
	login.SessionTTL = time.Duration(cfg.SessionTTLSecs) * time.Second
	grantstore.RefreshTokenTTL = time.Duration(cfg.RefreshTokenTTLSecs) * time.Second

	// ── Auth provider (claims projection + session JWTs) ───────────────
	// SigningKey is supplied via cfg.JWTSigningKeyPath in production. In
	// dev we fall back to a generated ephemeral key so the binary can
	// boot without filesystem deps. See fcdev for the persistent-key
	// path used by local development.
	signingKey := LoadSigningKeyOrEphemeral(cfg.JWTSigningKeyPath)
	authProvider, err := provider.NewProvider(provider.Config{
		Issuer: cfg.JWTIssuer,
		// Must match authservice's access-token `aud` below so bearers it
		// mints validate, while OIDC ID tokens (aud = an RP's client_id,
		// same signing key) are rejected by the middleware.
		Audience:   cfg.JWTIssuer,
		SigningKey: signingKey,
	}, repos.principalRepo, repos.roleRepo)
	if err != nil {
		return nil, fmt.Errorf("auth provider init: %w", err)
	}
	svcs.authProvider = authProvider

	// ── Hand-rolled OAuth token service (/oauth/token) ────────────────
	// authservice signs/validates with the same RSA key the auth provider
	// loaded, so the JWKS + session-cookie paths line up. encSvc verifies
	// confidential client secrets (decrypt + compare).
	// cfg.JWTPreviousPublicKey is the validation-only previous public key
	// for zero-downtime key rotation — tokens signed with the prior key
	// still verify (see envcfg.go normalizedPreviousPublicKey).
	svcs.authSvc, err = authservice.New(authservice.Config{
		Issuer:                  cfg.JWTIssuer,
		Audience:                cfg.JWTIssuer,
		RSAPrivateKeyPEM:        string(signingKey),
		RSAPublicKeyPreviousPEM: cfg.JWTPreviousPublicKey,
		// Token responses derive expires_in from this, so raising it moves
		// both the minted exp and the advertised lifetime together.
		AccessTokenExpirySecs:  int64(cfg.AccessTokenTTLSecs),
		SessionTokenExpirySecs: int64(cfg.SessionTTLSecs),
		RefreshTokenExpirySecs: int64(cfg.RefreshTokenTTLSecs),
		IDTokenExpirySecs:      300,
	})
	if err != nil {
		return nil, fmt.Errorf("authservice init: %w", err)
	}
	svcs.encSvc, err = encryption.FromEnv()
	if err != nil {
		return nil, fmt.Errorf("encryption init: %w", err)
	}
	// Distributed rate-limit store: Redis when FC_REDIS_URL is reachable,
	// else Postgres, else Noop (FC_RATE_LIMIT_DISABLE=1). Throttles
	// /oauth/{token,authorize} per-client_id (+ per-IP via middleware).
	svcs.rlStore = ratelimit.Build(context.Background(), pool)
	svcs.rlPolicies = ratelimit.PoliciesFromEnv()
	// In-memory per-instance governors layered in front of the distributed
	// store on /oauth/token (defence-in-depth). They shed a local flood
	// before the network
	// round-trip; the distributed store remains the cluster-wide ceiling.
	svcs.oauthTokenIPGov = ratelimit.NewGovernor(ratelimit.OAuthTokenIPGovernorFromEnv())
	svcs.oauthTokenClientGov = ratelimit.NewGovernor(ratelimit.OAuthTokenClientGovernorFromEnv())

	// Principal version cache: backs GET /api/principals/{id}/version, which
	// SDKs (e.g. the Laravel SDK's opt-in revocation check) poll to catch a
	// role/permission change before the caller's access token naturally
	// expires. Same Redis instance/env var as the rate-limit store above;
	// degrades to DB-only (still correct, just uncached) when unset/down.
	versionStore := versioncache.Build(context.Background())
	repos.principalRepo.VersionCache = versionStore
	svcs.principalVersions = versioncache.NewReader(
		versionStore,
		envutil.Int("FC_PRINCIPAL_VERSION_CACHE_SIZE", 10_000),
		time.Duration(envutil.Int("FC_PRINCIPAL_VERSION_CACHE_TTL_SECS", 30))*time.Second,
		func(ctx context.Context, principalID string) (time.Time, error) {
			return repos.principalRepo.LookupVersion(ctx, ids.PrincipalID(principalID))
		},
	)
	svcs.oauthTokenEP = &oauthapi.State{
		// Records rotation-overlap secret use for the client drawer's status line.
		OAuthClientWrites: repos.authRepo.OAuthClients,
		// Persists the lazy migration of a verified client secret to its
		// hashed:v1: form.
		SecretRewrites:    repos.authRepo.OAuthClients,
		OAuthClients:      repos.authRepo.OAuthClients,
		Principals:        repos.principalRepo,
		PortalIdentities:  repos.portalIdentityRepo,
		PortalApps:        repos.portalAppRepo,
		Auth:              svcs.authSvc,
		AuthCodes:         grantstore.NewAuthorizationCodeRepository(pool),
		RefreshTokens:     grantstore.NewRefreshTokenRepository(pool),
		PendingAuth:       grantstore.NewPendingAuthRepository(pool),
		Encryption:        svcs.encSvc,
		BaseURL:           cfg.JWTIssuer,
		LoginAttempts:     repos.loginAttemptRepo,
		RateLimit:         svcs.rlStore,
		RateLimitPolicies: svcs.rlPolicies,
		ClientGovernor:    svcs.oauthTokenClientGov,
		// /oauth/authorize treats an invalid/absent session as
		// redirect-to-login, so it validates the session cookie itself
		// (it's mounted outside the rejecting auth middleware).
		ValidateSession: func(token string) (string, time.Time, bool) {
			c, err := authProvider.ValidateSessionToken(context.Background(), token)
			if err != nil || c == nil {
				return "", time.Time{}, false
			}
			return c.Subject, c.IssuedAt, true
		},
		// Flatten roles → permission ceiling for the granted "scope" claim and
		// requested-scope narrowing on /oauth/token.
		FlattenPermissions: authProvider.FlattenPermissions,
		// Narrows a minted ID token's "roles" claim to an app-scoped OAuth
		// client's own application(s).
		FilterRolesForApplications: authProvider.FilterRolesForApplications,
		// A service account authenticating via client_credentials is its
		// day-to-day use — the liveness signal an operator reads before
		// revoking. Best-effort; a failed stamp must not fail the token.
		TouchServiceAccountUsed: func(ctx context.Context, serviceAccountID string) {
			_ = repos.serviceAccountRepo.TouchLastUsed(ctx, serviceAccountID)
		},
	}

	// ── Webauthn service ───────────────────────────────────────────────
	// go-webauthn matches the browser's origin against RPOrigins by exact
	// scheme+host (no wildcard/subdomain support), so every allowed origin must
	// be listed verbatim. RPID is the registrable parent domain (e.g.
	// inhanceapps.com) and validly covers any subdomain origin. Origins come from
	// FC_WEBAUTHN_ORIGINS (comma-separated, the deploy env's name); the singular
	// FC_WEBAUTHN_RP_ORIGIN is kept as a fallback for older configs.
	svcs.webauthnService, err = webauthn.NewService(webauthn.Config{
		// Read once at startup: the passkey prompt shows this. A platform-name
		// change takes effect on next restart (the library fixes RPDisplayName at
		// construction); the live-read paths (2FA issuer, emails) update instantly.
		RPDisplayName: branding.PlatformName(context.Background(), repos.platformConfigRepo),
		RPID:          envOr("FC_WEBAUTHN_RP_ID", "localhost"),
		RPOrigins:     webauthnOrigins(),
	}, repos.webauthnCredRepo, repos.webauthnCeremonyRepo)
	if err != nil {
		return nil, fmt.Errorf("webauthn service init: %w", err)
	}

	// Email + 2FA services. emailSvc is shared by the MFA challenge mailer and
	// the password-reset mailer. mfaSvc carries TOTP/email-PIN/recovery-code/
	// trusted-device logic; TOTP secrets are encrypted with encSvc (TOTP
	// degrades gracefully if no key). mfaTokens signs the short-lived
	// pending/enroll tokens with a secret derived from the session-signing key
	// (rejected by the RS256 middleware).
	svcs.emailSvc = email.FromEnv()
	// Resolve the configurable platform/brand name live for the authenticator-app
	// issuer and security emails (re-read per use, so a change applies instantly).
	svcs.platformName = branding.Provider(repos.platformConfigRepo)
	mfaCfg := mfa.DefaultConfig()
	mfaCfg.PlatformName = svcs.platformName
	svcs.mfaSvc = mfa.NewService(mfa.NewRepository(pool), svcs.encSvc, svcs.emailSvc, mfaCfg)
	svcs.mfaTokens = mfatoken.NewIssuer(authProvider.SigningKey(), authProvider.Issuer())
	svcs.notifier = notify.New(svcs.emailSvc).WithName(svcs.platformName)
	svcs.twofaPolicy = twofa.Policy{Mappings: repos.edmRepo, IDPs: repos.idpRepo}

	// Public auth surface: SPA login + cookie acquisition. MUST live
	// outside the bearer-token middleware — a stale fc_session cookie from
	// a previous run would otherwise 401 the request before the SPA could
	// re-authenticate. Registered in registerPublicRoutes; the
	// authenticated /auth/me half registers inside the platform group.
	svcs.loginEP = login.New(login.Config{
		Provider:          authProvider,
		Principals:        repos.principalRepo,
		Mappings:          repos.edmRepo,
		IdentityProviders: repos.idpRepo,
		CookieSecure:      !cfg.AuthAllowTestHeaders,
		LoginAttempts:     repos.loginAttemptRepo,
		BackoffPolicy:     loginbackoff.PolicyFromEnv(),
		// /auth/refresh shares the OAuth refresh-token store + access-token
		// signer so a token issued via either path rotates identically.
		RefreshTokens: svcs.oauthTokenEP.RefreshTokens,
		Auth:          svcs.authSvc,
		// 2FA: challenge/enroll endpoints. (A passkey does not exempt the
		// password path, so no webauthn dependency here.)
		MFA:       svcs.mfaSvc,
		MFATokens: svcs.mfaTokens,
		Notifier:  svcs.notifier,
		Audit:     repos.auditRepo,
	})

	// Function-runner artifact store (docs/function-runner-plan.md §8.4):
	// file:// or s3://, per FC_FUNCTIONS_ARTIFACT_STORE.
	svcs.functionArtifacts, err = artifact.FromURL(context.Background(), cfg.FunctionsArtifactStore, defaultFunctionArtifactDir)
	if err != nil {
		return nil, fmt.Errorf("function artifact store init: %w", err)
	}

	// The platform's own tiny engine for reading a published artifact's
	// fc_describe document at publish time (plan §3, §8.2 WP4 task 2) —
	// built ONCE here, no capabilities, a small dedicated budget distinct
	// from the runner's own (internal/server/functions.go): this engine
	// only ever instantiates one no-capability instance per publish call.
	functionBudget, err := budget.New(functionDescribeMemoryLimitBytes, functionDescribeMemoryReserveBytes)
	if err != nil {
		return nil, fmt.Errorf("function describe budget init: %w", err)
	}
	functionEngine, err := engine.New(context.Background(), engine.Config{Budget: functionBudget})
	if err != nil {
		return nil, fmt.Errorf("function describe engine init: %w", err)
	}
	svcs.functionLoader = runtimes.NewLoader(functionEngine)

	// Promote wiring deps (WP8, plan §8.5): the repositories + runner URL
	// template SetAlias/DeleteAlias/DeleteFunction reconcile against.
	svcs.functionWiring = functionops.WiringDeps{
		Subscriptions:     repos.subscriptionRepo,
		ScheduledJobs:     repos.scheduledJobRepo,
		DispatchPools:     repos.dispatchPoolRepo,
		ServiceAccounts:   repos.serviceAccountRepo,
		RunnerURLTemplate: cfg.FunctionsRunnerURL,
	}

	// Function control-plane NOTIFY listener (docs/function-runner-plan.md
	// §8.3): fans out `NOTIFY fng_desired` to held long-polls. Built here
	// (Run is started by the caller — see run.go's PlatformHandles.
	// FunctionControlListener) so it shares this process's pool like every
	// other service; a nil pool (no database) leaves it unusable, matching
	// how the rest of buildServices behaves under FC_PLATFORM_ENABLED with
	// no DB configured.
	if pool != nil {
		svcs.functionControlListener = functioncontrol.NewListener(pool, nil)
	}

	return svcs, nil
}

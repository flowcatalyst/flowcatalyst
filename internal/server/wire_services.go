package server

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/branding"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
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
}

func buildServices(cfg EnvCfg, pool *pgxpool.Pool, repos *repoSet) (*serviceSet, error) {
	svcs := &serviceSet{}

	// ── Auth provider (claims projection + session JWTs) ───────────────
	// SigningKey is supplied via cfg.JWTSigningKeyPath in production. In
	// dev we fall back to a generated ephemeral key so the binary can
	// boot without filesystem deps. See fc-dev for the persistent-key
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
	// Validation-only previous public key for zero-downtime key rotation —
	// tokens signed with the prior key still verify. Matches Rust's
	// FLOWCATALYST_JWT_PREVIOUS_PUBLIC_KEY. Normalize the SSM/env PEM (same \n
	// mangling as the private key) and skip it unless it's a real PEM: it's
	// optional, so a missing or unparseable value must NOT stop the platform
	// booting. (The current key's public half is derived from signingKey.)
	prevPubKey := NormalizePEM(os.Getenv("FLOWCATALYST_JWT_PREVIOUS_PUBLIC_KEY"))
	if !strings.Contains(prevPubKey, "-----BEGIN") {
		prevPubKey = ""
	}
	svcs.authSvc, err = authservice.New(authservice.Config{
		Issuer:                  cfg.JWTIssuer,
		Audience:                cfg.JWTIssuer,
		RSAPrivateKeyPEM:        string(signingKey),
		RSAPublicKeyPreviousPEM: prevPubKey,
		AccessTokenExpirySecs:   3600,
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
	// store on /oauth/token (defence-in-depth; 1:1 with Rust's
	// rate_limit_middleware.rs). They shed a local flood before the network
	// round-trip; the distributed store remains the cluster-wide ceiling.
	svcs.oauthTokenIPGov = ratelimit.NewGovernor(ratelimit.OAuthTokenIPGovernorFromEnv())
	svcs.oauthTokenClientGov = ratelimit.NewGovernor(ratelimit.OAuthTokenClientGovernorFromEnv())
	svcs.oauthTokenEP = &oauthapi.State{
		OAuthClients:      repos.authRepo.OAuthClients,
		Principals:        repos.principalRepo,
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

	return svcs, nil
}

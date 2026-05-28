// Package provider hosts the OAuth/OIDC provider runtime — FlowCatalyst
// issues access/refresh/ID tokens via this provider for SDK consumers
// (client_credentials grant) and users (authorization_code grant).
//
// Wiring shape:
//
//	provider.go         — Config, Claims, BuildClaims, Provider+New
//	hasher.go           — Argon2id fosite.Hasher for client-secret verify
//	client_adapter.go   — auth.OAuthClient → fosite.Client adapter
//	client_manager.go   — fosite.ClientManager (GetClient + JTI replay store)
//	session.go          — FCSession (JWTSession + Extra claims)
//	storage.go          — fosite.Storage + oauth2.CoreStorage + revocation
//	token_endpoint.go   — POST /oauth/token (delegates to fosite)
//	payload/            — oauth_oidc_payloads-backed artifact store
//
// Today's compose lights up the client_credentials grant. The
// authorization_code + refresh_token grants are wired into the storage
// adapter already; switching them on is a matter of adding their
// factories to NewProvider's compose call once we expose
// /oauth/authorize.
package provider

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ory/fosite"
	"github.com/ory/fosite/compose"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/payload"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
)

// Config bundles the construction-time settings for the OAuth provider.
type Config struct {
	// Issuer is the JWT iss claim, e.g. "https://flowcatalyst.example.com".
	Issuer string

	// AccessTokenTTL is how long access tokens are valid.
	AccessTokenTTL time.Duration

	// RefreshTokenTTL is how long refresh tokens are valid.
	RefreshTokenTTL time.Duration

	// AuthorizationCodeTTL is how long authorization codes are valid.
	AuthorizationCodeTTL time.Duration

	// SigningKey is the RS256 private key used to sign JWTs. PEM-encoded.
	SigningKey []byte

	// SigningKeyID is the kid claim in issued JWTs.
	SigningKeyID string

	// GlobalSecret is the HMAC secret used by fosite for non-JWT tokens
	// (refresh tokens, authorize codes). 32 bytes minimum.
	GlobalSecret []byte
}

// DefaultConfig returns the canonical defaults, matching Rust
// fc-platform's auth_service.rs: 1h access, 30d refresh, 10min auth code.
func DefaultConfig() Config {
	return Config{
		AccessTokenTTL:       1 * time.Hour,
		RefreshTokenTTL:      30 * 24 * time.Hour,
		AuthorizationCodeTTL: 10 * time.Minute,
	}
}

// Claims is the FlowCatalyst-specific JWT payload. The fields land in
// fosite's JWT under "extra" (which fosite serializes top-level — see
// jwt.JWTClaims.ToMap). Keep names in sync with what SDK consumers
// expect.
type Claims struct {
	Issuer       string
	Subject      string
	Audience     string
	Scope        string   // "ANCHOR" | "PARTNER" | "CLIENT"
	Clients      []string // tenant IDs accessible
	Roles        []string
	Applications []string
	Permissions  []string // de-duplicated, flattened from Roles
	Email        string
	Name         string // user display name (OIDC "name" claim)

	// OIDC ID-token-specific claims. These are populated only when the
	// caller is minting an ID token, not a plain access token. Rust's
	// auth_service.rs ships them on the ID token; we match the same set
	// (nonce, azp, auth_time, email_verified). acr/amr are not populated
	// by Rust either, so we leave them off.
	Nonce           string // OIDC nonce echoed from the authorize request
	AuthorizedParty string // OIDC "azp" — typically the client_id
	AuthTime        int64  // OIDC "auth_time" — Unix seconds
	EmailVerified   *bool  // OIDC "email_verified" — pointer to distinguish "unset"
}

// BuildClaims projects a principal onto our Claims shape. Called by the
// /oauth/token handler before fosite mints the JWT. roles may be nil —
// in that case Permissions is left empty (handlers without permission
// gates still work, gated handlers reject with PERMISSION_REQUIRED).
func BuildClaims(ctx context.Context, cfg Config, principals *principal.Repository, roles *role.Repository, principalID string) (*Claims, error) {
	p, err := principals.FindByID(ctx, principalID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errors.New("principal not found")
	}
	if !p.Active {
		return nil, errors.New("principal is deactivated")
	}
	roleNames := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		roleNames = append(roleNames, ra.Role)
	}
	clients := append([]string(nil), p.AssignedClients...)
	if p.ClientID != nil && *p.ClientID != "" {
		clients = append(clients, *p.ClientID)
	}
	apps := append([]string(nil), p.AccessibleApplicationIDs...)
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	perms, err := flattenPermissions(ctx, roles, roleNames)
	if err != nil {
		return nil, fmt.Errorf("flatten permissions: %w", err)
	}
	return &Claims{
		Issuer:       cfg.Issuer,
		Subject:      p.ID,
		Scope:        string(p.Scope),
		Clients:      clients,
		Roles:        roleNames,
		Applications: apps,
		Permissions:  perms,
		Email:        email,
		Name:         p.Name,
	}, nil
}

// FlattenPermissions resolves a principal's role names into their
// de-duplicated permission set. The auth middleware calls this to derive
// permissions for tokens that carry roles but no permissions claim
// (e.g. OAuth access tokens minted by authservice, matching Rust which
// never bakes permissions into the JWT).
func (p *Provider) FlattenPermissions(ctx context.Context, roleNames []string) ([]string, error) {
	return flattenPermissions(ctx, p.roles, roleNames)
}

// flattenPermissions looks up each role by name and concatenates its
// permissions, de-duplicated. Skips roles the repo can't find (a known
// role was deleted out from under the principal) — the principal keeps
// whatever permissions the remaining roles grant.
func flattenPermissions(ctx context.Context, roles *role.Repository, roleNames []string) ([]string, error) {
	if roles == nil || len(roleNames) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, name := range roleNames {
		r, err := roles.FindByName(ctx, name)
		if err != nil {
			return nil, err
		}
		if r == nil {
			continue
		}
		for _, p := range r.Permissions {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out, nil
}

// Provider bundles the live fosite OAuth2Provider plus the deps the
// HTTP layer needs (principal + role repos for BuildClaims, config for
// TTLs).
type Provider struct {
	cfg             Config
	OAuth2          fosite.OAuth2Provider
	storage         *Storage
	principals      *principal.Repository
	roles           *role.Repository
	SessionResolver func(*http.Request) string
	// signingKey is the RSA key shared by fosite (for /oauth/token JWTs)
	// and the sessiontoken package (for /auth/login cookies). The
	// session-cookie path lives outside fosite entirely — see ADR-0001.
	signingKey *rsa.PrivateKey
}

// SetSessionResolver lets callers plug in the principal-id resolver
// the /oauth/authorize endpoint uses to detect logged-in users.
func (p *Provider) SetSessionResolver(resolver func(*http.Request) string) {
	p.SessionResolver = resolver
}

// NewProvider wires fosite end-to-end. Returns an error if the RSA key
// is missing or malformed.
func NewProvider(cfg Config, authRepo *auth.Repository, payloads *payload.Repository, principals *principal.Repository, roles *role.Repository) (*Provider, error) {
	key, err := parseRSAPrivateKey(cfg.SigningKey)
	if err != nil {
		return nil, fmt.Errorf("auth provider: %w", err)
	}
	if len(cfg.GlobalSecret) < 32 {
		return nil, errors.New("auth provider: GlobalSecret must be at least 32 bytes")
	}
	if cfg.AccessTokenTTL == 0 {
		cfg.AccessTokenTTL = 1 * time.Hour
	}
	if cfg.RefreshTokenTTL == 0 {
		cfg.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if cfg.AuthorizationCodeTTL == 0 {
		cfg.AuthorizationCodeTTL = 10 * time.Minute
	}

	fc := &fosite.Config{
		AccessTokenLifespan:        cfg.AccessTokenTTL,
		RefreshTokenLifespan:       cfg.RefreshTokenTTL,
		AuthorizeCodeLifespan:      cfg.AuthorizationCodeTTL,
		IDTokenIssuer:              cfg.Issuer,
		GlobalSecret:               cfg.GlobalSecret,
		ClientSecretsHasher:        Argon2idHasher{},
		SendDebugMessagesToClients: false,
	}

	storage := NewStorage(authRepo.OAuthClients, payloads)

	keyGetter := func(_ context.Context) (any, error) { return key, nil }
	hmacStrategy := compose.NewOAuth2HMACStrategy(fc)
	jwtStrategy := compose.NewOAuth2JWTStrategy(keyGetter, hmacStrategy, fc)

	provider := compose.Compose(
		fc,
		storage,
		jwtStrategy,
		compose.OAuth2AuthorizeExplicitFactory,
		compose.OAuth2ClientCredentialsGrantFactory,
		compose.OAuth2RefreshTokenGrantFactory,
		compose.OAuth2TokenRevocationFactory,
		compose.OAuth2TokenIntrospectionFactory,
		compose.OAuth2PKCEFactory,
	)

	return &Provider{
		cfg:             cfg,
		OAuth2:          provider,
		signingKey:      key,
		storage:         storage,
		principals:      principals,
		roles:           roles,
		SessionResolver: func(*http.Request) string { return "" },
	}, nil
}

// SigningKey exposes the RSA private key the provider was constructed
// with. Used by sessiontoken-aware callers (the auth middleware, the
// /auth/login handler) to share the same key pair fosite uses, without
// reaching into fosite's strategy.
func (p *Provider) SigningKey() *rsa.PrivateKey { return p.signingKey }

// Issuer returns the configured JWT issuer claim.
func (p *Provider) Issuer() string { return p.cfg.Issuer }

// AccessTokenTTL returns the configured access-token lifetime.
func (p *Provider) AccessTokenTTL() time.Duration { return p.cfg.AccessTokenTTL }

// ResolveClaims is the exported BuildClaims wrapper bound to this
// provider's repos + config. Used by callers that need the flattened
// claim set for a principal (e.g. /auth/login's response body, which
// includes the `permissions` list so the SPA's route guards can run).
func (p *Provider) ResolveClaims(ctx context.Context, principalID string) (*Claims, error) {
	return BuildClaims(ctx, p.cfg, p.principals, p.roles, principalID)
}

// MintSessionToken issues a self-contained JWT access token for the
// supplied principal. Used by /auth/login: the resulting token is set
// as the session cookie. Now backed by the sessiontoken package
// directly — no fosite reach-through — so the claim shape is exactly
// what we put in.
//
// ttl=0 falls back to AccessTokenTTL.
func (p *Provider) MintSessionToken(ctx context.Context, principalID string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = p.cfg.AccessTokenTTL
	}
	c, err := BuildClaims(ctx, p.cfg, p.principals, p.roles, principalID)
	if err != nil {
		return "", fmt.Errorf("build claims: %w", err)
	}
	return sessiontoken.Mint(sessiontoken.Claims{
		Subject:      c.Subject,
		Scope:        c.Scope,
		Email:        c.Email,
		Clients:      c.Clients,
		Roles:        c.Roles,
		Applications: c.Applications,
		Permissions:  c.Permissions,
	}, p.signingKey, p.cfg.Issuer, ttl)
}

// ValidateSessionToken verifies a session-cookie JWT (signature + std
// claim checks) and returns the parsed claims. Used by the platform's
// auth middleware to verify both Authorization: Bearer tokens and
// fc_session cookies. Both transports carry tokens minted by
// MintSessionToken (or its fosite-grant equivalent for /oauth/token —
// fosite signs with the same RSA key so the signature path lines up).
func (p *Provider) ValidateSessionToken(_ context.Context, token string) (*sessiontoken.Claims, error) {
	return sessiontoken.Validate(token, &p.signingKey.PublicKey)
}

// parseRSAPrivateKey accepts PKCS#1 or PKCS#8 PEM blocks.
func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	if len(pemBytes) == 0 {
		return nil, errors.New("signing key is empty")
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any8, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse pkcs8: %w", err)
	}
	rsaKey, ok := any8.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rsaKey, nil
}

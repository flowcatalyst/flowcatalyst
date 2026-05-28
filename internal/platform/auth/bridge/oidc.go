// Package bridge implements the OIDC client / bridge side of auth:
// FlowCatalyst as an OIDC client of external IDPs (Entra, Keycloak,
// Google). On login, the user is redirected to the external IDP; on
// callback we exchange the auth code, validate the ID token, and
// resolve the FlowCatalyst principal via the configured ClientAuthConfig
// or EmailDomainMapping.
//
// Library: github.com/coreos/go-oidc/v3 + golang.org/x/oauth2.
//
// Phase 3d scope: the OIDC client construction is wired; the per-IDP
// configuration lookup (resolve issuer URL + client ID for an email
// domain) is in place; the actual login/callback HTTP handlers ship in
// the auth-runtime follow-up alongside the session-cookie middleware.
package bridge

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// Bridge constructs and caches OIDC clients per (issuerURL, clientID)
// pair. The cache is keyed on the issuer URL + client ID because
// constructing a *oidc.Provider involves a discovery HTTP round-trip
// (to /.well-known/openid-configuration) that we only want to do once
// per IDP per process.
type Bridge struct {
	authRepo *auth.Repository
	enc      *encryption.Service // optional; decrypts OIDCClientSecretRef

	mu    sync.Mutex
	cache map[string]*resolved
}

type resolved struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// NewBridge wires the bridge. enc may be nil — confidential OIDC clients
// will fail to authenticate against the external IDP in that case, but
// public clients (no client secret) still work.
func NewBridge(authRepo *auth.Repository, enc *encryption.Service) *Bridge {
	return &Bridge{authRepo: authRepo, enc: enc, cache: make(map[string]*resolved)}
}

// ResolveForEmail picks the right ClientAuthConfig (and therefore the
// right OIDC issuer) for the user with the supplied email. Returns the
// OIDC client + token verifier + oauth2 config; the caller drives the
// redirect / callback flow.
func (b *Bridge) ResolveForEmail(ctx context.Context, email string) (*resolved, *auth.ClientAuthConfig, error) {
	domain := emailDomain(email)
	if domain == "" {
		return nil, nil, errors.New("invalid email: no domain")
	}
	cfg, err := b.authRepo.ClientAuthConfigs.FindByEmailDomain(ctx, domain)
	if err != nil {
		return nil, nil, fmt.Errorf("client_auth_config lookup: %w", err)
	}
	if cfg == nil {
		return nil, nil, errors.New("no auth config for domain " + domain)
	}
	if cfg.AuthProvider != auth.ProviderOIDC {
		return nil, cfg, nil // internal provider; no OIDC bridge needed
	}
	if cfg.OIDCIssuerURL == nil || cfg.OIDCClientID == nil {
		return nil, cfg, errors.New("OIDC config missing issuer or client ID")
	}

	key := *cfg.OIDCIssuerURL + "|" + *cfg.OIDCClientID
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.cache[key]; ok {
		return r, cfg, nil
	}

	provider, err := oidc.NewProvider(ctx, *cfg.OIDCIssuerURL)
	if err != nil {
		return nil, cfg, fmt.Errorf("oidc.NewProvider: %w", err)
	}
	clientSecret, err := b.resolveClientSecret(cfg)
	if err != nil {
		return nil, cfg, err
	}
	r := &resolved{
		provider: provider,
		verifier: provider.Verifier(&oidc.Config{ClientID: *cfg.OIDCClientID}),
		oauth: &oauth2.Config{
			ClientID:     *cfg.OIDCClientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}
	b.cache[key] = r
	return r, cfg, nil
}

// resolveClientSecret decrypts cfg.OIDCClientSecretRef using the
// configured encryption service. Empty ref → no secret (public client).
// If a ref is present but no encryption service is configured, or
// decryption fails, returns an error so the caller surfaces a clear
// misconfiguration rather than silently mis-authing.
func (b *Bridge) resolveClientSecret(cfg *auth.ClientAuthConfig) (string, error) {
	if cfg.OIDCClientSecretRef == nil || *cfg.OIDCClientSecretRef == "" {
		return "", nil
	}
	if b.enc == nil {
		return "", errors.New("OIDC client_secret_ref present but no encryption service configured (set FLOWCATALYST_APP_KEY)")
	}
	pt, err := b.enc.Decrypt(*cfg.OIDCClientSecretRef)
	if err != nil {
		return "", fmt.Errorf("decrypt OIDC client secret: %w", err)
	}
	return pt, nil
}

// VerifyIDToken validates a raw ID token JWT against the bridge cache.
// The verifier checks signature, issuer, audience (== ClientID),
// expiration, and not-before.
func (r *resolved) VerifyIDToken(ctx context.Context, raw string) (*oidc.IDToken, error) {
	return r.verifier.Verify(ctx, raw)
}

// AuthCodeURL builds the redirect URL for an OIDC login start. The state
// param is a CSRF token the caller persists in the session.
func (r *resolved) AuthCodeURL(state, redirectURI string) string {
	cfg := *r.oauth
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state)
}

// Exchange swaps an authorization code for tokens.
func (r *resolved) Exchange(ctx context.Context, code, redirectURI string) (*oauth2.Token, error) {
	cfg := *r.oauth
	cfg.RedirectURL = redirectURI
	return cfg.Exchange(ctx, code)
}

func emailDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return ""
}

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
	"regexp"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// Bridge constructs and caches OIDC clients per (issuerURL, clientID)
// pair. The cache is keyed on the issuer URL + client ID because
// constructing a *oidc.Provider involves a discovery HTTP round-trip
// (to /.well-known/openid-configuration) that we only want to do once
// per IDP per process.
type Bridge struct {
	mappings *emaildomainmapping.Repository
	idps     *identityprovider.Repository
	enc      *encryption.Service // optional; decrypts OIDCClientSecretRef

	mu    sync.Mutex
	cache map[string]*resolved
}

type resolved struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config

	// Multi-tenant issuer validation (Entra "common"/"organizations" etc.):
	// the verifier's built-in issuer check is skipped, so VerifyIDToken
	// validates the token's issuer against issuerPattern after the fact.
	issuerURL     string
	multiTenant   bool
	issuerPattern *string
}

// NewBridge wires the bridge. enc may be nil — confidential OIDC clients
// will fail to authenticate against the external IDP in that case, but
// public clients (no client secret) still work.
func NewBridge(mappings *emaildomainmapping.Repository, idps *identityprovider.Repository, enc *encryption.Service) *Bridge {
	return &Bridge{mappings: mappings, idps: idps, enc: enc, cache: make(map[string]*resolved)}
}

// ResolveForEmail resolves the OIDC client for the user's email domain via the
// email-domain mapping → identity provider chain, exactly as Rust's oidc_login
// does (find_by_email_domain → identity_provider.find_by_id). Returns the OIDC
// client + the IdP + the mapping; the caller drives the redirect / callback and
// persists the IdP + mapping ids in the login state.
func (b *Bridge) ResolveForEmail(ctx context.Context, email string) (*resolved, *identityprovider.IdentityProvider, *emaildomainmapping.EmailDomainMapping, error) {
	domain := emailDomain(email)
	if domain == "" {
		return nil, nil, nil, errors.New("invalid email: no domain")
	}
	mapping, err := b.mappings.FindByEmailDomain(ctx, domain)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("email_domain_mapping lookup: %w", err)
	}
	if mapping == nil {
		return nil, nil, nil, errors.New("no email-domain mapping for " + domain)
	}
	idp, err := b.idps.FindByID(ctx, mapping.IdentityProviderID)
	if err != nil {
		return nil, nil, mapping, fmt.Errorf("identity_provider lookup: %w", err)
	}
	if idp == nil {
		return nil, nil, mapping, errors.New("identity provider not found: " + mapping.IdentityProviderID)
	}
	if idp.Type != identityprovider.TypeOIDC {
		return nil, idp, mapping, nil // internal provider; no OIDC bridge needed
	}
	if idp.OIDCIssuerURL == nil || idp.OIDCClientID == nil {
		return nil, idp, mapping, errors.New("OIDC config missing issuer or client ID")
	}

	key := *idp.OIDCIssuerURL + "|" + *idp.OIDCClientID
	b.mu.Lock()
	defer b.mu.Unlock()
	if r, ok := b.cache[key]; ok {
		return r, idp, mapping, nil
	}

	// Multi-tenant IdPs (Entra "common"/"organizations", …) report a
	// {tenantid}-templated issuer in discovery and mint tokens with a
	// tenant-specific iss/aud, so the standard issuer/audience checks reject
	// them. Mirror Rust: accept the discovery-doc issuer (don't fail on the
	// mismatch), skip the built-in iss/aud checks, and validate the token's
	// issuer against oidc_issuer_pattern after verification (see VerifyIDToken).
	discoveryCtx := ctx
	verifierCfg := &oidc.Config{ClientID: *idp.OIDCClientID}
	if idp.OIDCMultiTenant {
		discoveryCtx = oidc.InsecureIssuerURLContext(ctx, *idp.OIDCIssuerURL)
		verifierCfg.SkipIssuerCheck = true
		verifierCfg.SkipClientIDCheck = true
	}
	provider, err := oidc.NewProvider(discoveryCtx, *idp.OIDCIssuerURL)
	if err != nil {
		return nil, idp, mapping, fmt.Errorf("oidc.NewProvider: %w", err)
	}
	clientSecret, err := b.resolveClientSecret(idp.OIDCClientSecretRef)
	if err != nil {
		return nil, idp, mapping, err
	}
	r := &resolved{
		provider:      provider,
		verifier:      provider.Verifier(verifierCfg),
		issuerURL:     *idp.OIDCIssuerURL,
		multiTenant:   idp.OIDCMultiTenant,
		issuerPattern: idp.OIDCIssuerPattern,
		oauth: &oauth2.Config{
			ClientID:     *idp.OIDCClientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}
	b.cache[key] = r
	return r, idp, mapping, nil
}

// resolveClientSecret decrypts the IdP's OIDCClientSecretRef using the
// configured encryption service. Empty ref → no secret (public client).
// If a ref is present but no encryption service is configured, or
// decryption fails, returns an error so the caller surfaces a clear
// misconfiguration rather than silently mis-authing.
func (b *Bridge) resolveClientSecret(secretRef *string) (string, error) {
	if secretRef == nil || *secretRef == "" {
		return "", nil
	}
	if b.enc == nil {
		return "", errors.New("OIDC client_secret_ref present but no encryption service configured (set FLOWCATALYST_APP_KEY)")
	}
	pt, err := b.enc.Decrypt(*secretRef)
	if err != nil {
		return "", fmt.Errorf("decrypt OIDC client secret: %w", err)
	}
	return pt, nil
}

// VerifyIDToken validates a raw ID token JWT. The verifier checks signature,
// expiration, and not-before; for a single-tenant IdP it also checks issuer +
// audience. For a multi-tenant IdP those built-in checks are skipped (the iss
// is tenant-specific), so the token's issuer is instead validated against the
// configured pattern here — 1:1 with Rust is_valid_issuer_for_idp.
func (r *resolved) VerifyIDToken(ctx context.Context, raw string) (*oidc.IDToken, error) {
	tok, err := r.verifier.Verify(ctx, raw)
	if err != nil {
		return nil, err
	}
	if r.multiTenant && !isValidIssuer(tok.Issuer, r.issuerURL, true, r.issuerPattern) {
		return nil, fmt.Errorf("invalid issuer for multi-tenant IdP: %s", tok.Issuer)
	}
	return tok, nil
}

// isValidIssuer mirrors Rust is_valid_issuer_for_idp: an exact match against the
// configured issuer URL, else (multi-tenant only) a regex match against the
// configured pattern.
func isValidIssuer(iss, issuerURL string, multiTenant bool, pattern *string) bool {
	if issuerURL == iss {
		return true
	}
	if multiTenant && pattern != nil && *pattern != "" {
		if re, err := regexp.Compile(*pattern); err == nil {
			return re.MatchString(iss)
		}
	}
	return false
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

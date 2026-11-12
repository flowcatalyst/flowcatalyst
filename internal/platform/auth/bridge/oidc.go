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
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

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

	// mu guards cache, inflight and failed, and is NEVER held across a network
	// call: a discovery that stalls must not block a sign-in through another,
	// already cached, provider.
	mu       sync.Mutex
	cache    map[string]*resolved
	inflight map[string]*discovery // discoveries running now, by cache key
	failed   map[string]discoveryFailure

	// client is the one http.Client the bridge uses for OIDC discovery, JWKS
	// and the token exchange (bounded: see newBoundedClient).
	client *http.Client
	// now and failTTL drive the negative cache; tests replace them.
	now     func() time.Time
	failTTL time.Duration
}

// discovery is one in-progress OIDC discovery; every resolve of the same key
// waits on done and reads r/err after it closes.
type discovery struct {
	done chan struct{}
	r    *resolved
	err  error
}

// discoveryFailure is a remembered failed discovery (the negative cache).
type discoveryFailure struct {
	err   error
	until time.Time
}

const (
	// discoveryTimeout bounds each request of discovery, JWKS and the token
	// exchange.
	discoveryTimeout = 10 * time.Second
	// maxDiscoveryBody caps any response body read from an identity provider.
	maxDiscoveryBody = 1 << 20
	// maxDiscoveryRedirects is the most redirects a request follows.
	maxDiscoveryRedirects = 3
	// discoveryFailureTTL is how long a failed discovery is remembered.
	discoveryFailureTTL = 30 * time.Second
)

// errResponseTooLarge is the read error of a response body over the cap.
var errResponseTooLarge = errors.New("identity provider response exceeds the size limit")

// cappedBody errors, rather than truncating silently, once more than max bytes
// are read: a truncated JSON document must fail, not parse as something else.
type cappedBody struct {
	rc   io.ReadCloser
	left int64
}

func (b *cappedBody) Read(p []byte) (int, error) {
	if b.left < 0 {
		return 0, errResponseTooLarge
	}
	if int64(len(p)) > b.left+1 {
		p = p[:b.left+1]
	}
	n, err := b.rc.Read(p)
	b.left -= int64(n)
	if b.left < 0 {
		return n, errResponseTooLarge
	}
	return n, err
}

func (b *cappedBody) Close() error { return b.rc.Close() }

// cappingTransport limits the body of every response it returns.
type cappingTransport struct {
	base http.RoundTripper
	max  int64
}

func (t cappingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	if resp.ContentLength > t.max {
		_ = resp.Body.Close()
		return nil, errResponseTooLarge
	}
	resp.Body = &cappedBody{rc: resp.Body, left: t.max}
	return resp, nil
}

// newBoundedClient builds the client for talking to identity providers: the
// given per-request timeout, response bodies capped at maxDiscoveryBody, at
// most maxDiscoveryRedirects redirects, TLS verification as the default
// transport does it (base nil = a clone of http.DefaultTransport).
func newBoundedClient(base http.RoundTripper, timeout time.Duration) *http.Client {
	if base == nil {
		base = http.DefaultTransport.(*http.Transport).Clone()
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: cappingTransport{base: base, max: maxDiscoveryBody},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxDiscoveryRedirects {
				return errors.New("identity provider: too many redirects")
			}
			return nil
		},
	}
}

type resolved struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config

	// Multi-tenant issuer/audience validation (Entra "common"/
	// "organizations" etc.): the verifier's built-in issuer + client-ID
	// checks are skipped, so VerifyIDToken validates the token's issuer
	// against issuerPattern and its audience against clientID after the
	// fact.
	issuerURL     string
	clientID      string
	multiTenant   bool
	issuerPattern *string

	// client carries the token exchange and verification requests (the bridge's
	// bounded client).
	client *http.Client
}

// NewBridge wires the bridge. enc may be nil — confidential OIDC clients
// will fail to authenticate against the external IDP in that case, but
// public clients (no client secret) still work.
func NewBridge(mappings *emaildomainmapping.Repository, idps *identityprovider.Repository, enc *encryption.Service) *Bridge {
	return &Bridge{
		mappings: mappings, idps: idps, enc: enc,
		cache:    make(map[string]*resolved),
		inflight: make(map[string]*discovery),
		failed:   make(map[string]discoveryFailure),
		client:   newBoundedClient(nil, discoveryTimeout),
		now:      time.Now,
		failTTL:  discoveryFailureTTL,
	}
}

// SetHTTPClient replaces the client used for discovery, JWKS and the token
// exchange (tests point it at an httptest server). Call before serving.
func (b *Bridge) SetHTTPClient(c *http.Client) { b.client = c }

// ErrEmailDomainNotMapped is ResolveForEmail's answer for a domain with no
// email-domain mapping: a user's typo or a stale link, not a server fault.
// The login endpoint answers it 404 EMAIL_DOMAIN_NOT_MAPPED; a mapped domain
// whose provider is broken is still an init failure (500).
var ErrEmailDomainNotMapped = errors.New("no email-domain mapping")

// ResolveForEmail resolves the OIDC client for the user's email domain via the
// email-domain mapping → identity provider chain. Returns the OIDC
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
		return nil, nil, nil, fmt.Errorf("%w: %s", ErrEmailDomainNotMapped, domain)
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
	r, err := b.resolveIdP(ctx, idp)
	if err != nil {
		return nil, idp, mapping, err
	}
	return r, idp, mapping, nil
}

// ErrProviderHasNoEmailDomains is ResolveByProviderID's refusal of an OIDC
// identity provider that owns no email domain (no mapping points at it).
var ErrProviderHasNoEmailDomains = errors.New("identity provider has no email domain mapped to it")

// ResolveByProviderID resolves the OIDC client for an identity provider by its
// id — the provider-direct login path (docs/portal-identity-plan.md): the
// caller (a portal app via /oauth/authorize?provider= or /auth/oidc/login?
// provider_id=) names the IdP explicitly. An IdP authenticates the email
// domains it owns, nothing else, so the rule is, fail-closed:
//
//   - the IdP must exist and be TypeOIDC (INTERNAL providers have no bridge);
//   - the IdP must own at least one email domain (a mapping pointing at it),
//     whatever its type — otherwise ErrProviderHasNoEmailDomains. The callback
//     binds the token's email domain and tenant to those mappings, and with
//     none an IdP could assert any email, including an existing account's.
func (b *Bridge) ResolveByProviderID(ctx context.Context, providerID string) (*resolved, *identityprovider.IdentityProvider, error) {
	idp, err := b.idps.FindByID(ctx, providerID)
	if err != nil {
		return nil, nil, fmt.Errorf("identity_provider lookup: %w", err)
	}
	if idp == nil {
		return nil, nil, errors.New("identity provider not found: " + providerID)
	}
	if idp.Type != identityprovider.TypeOIDC {
		return nil, idp, errors.New("identity provider is not OIDC: " + providerID)
	}
	if len(idp.AllowedEmailDomains) == 0 {
		return nil, idp, fmt.Errorf("%w: %s", ErrProviderHasNoEmailDomains, providerID)
	}
	r, err := b.resolveIdP(ctx, idp)
	if err != nil {
		return nil, idp, err
	}
	return r, idp, nil
}

// resolveIdP returns the cached OIDC client for an OIDC-type IdP, constructing
// (and caching) it on first use. Shared by the email-domain and provider-direct
// resolution paths so both hit the same per-(issuer, client id) cache.
//
// The lock covers only the maps. A cache hit returns at once; a miss starts one
// discovery for the key (concurrent resolves of the same key share it) outside
// the lock, so a provider whose discovery endpoint stalls delays nobody else. A
// failed discovery is remembered for failTTL, so a dead endpoint is not
// re-contacted on every sign-in. A waiter whose own context ends returns that
// error and leaves the discovery running for the others.
func (b *Bridge) resolveIdP(ctx context.Context, idp *identityprovider.IdentityProvider) (*resolved, error) {
	if idp.OIDCIssuerURL == nil || idp.OIDCClientID == nil {
		return nil, errors.New("OIDC config missing issuer or client ID")
	}

	key := cacheKey(idp)
	b.mu.Lock()
	if r, ok := b.cache[key]; ok {
		b.mu.Unlock()
		return r, nil
	}
	if f, ok := b.failed[key]; ok {
		if b.now().Before(f.until) {
			b.mu.Unlock()
			return nil, f.err
		}
		delete(b.failed, key)
	}
	d, running := b.inflight[key]
	if !running {
		d = &discovery{done: make(chan struct{})}
		b.inflight[key] = d
	}
	b.mu.Unlock()

	if !running {
		// Detached from this request's cancellation: other resolves wait on
		// the result. The client's timeout bounds it.
		go b.discover(context.WithoutCancel(ctx), key, idp, d)
	}
	select {
	case <-d.done:
		return d.r, d.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// discover runs one discovery for key and publishes the outcome: a success into
// the cache, a discovery failure into the negative cache.
func (b *Bridge) discover(ctx context.Context, key string, idp *identityprovider.IdentityProvider, d *discovery) {
	r, discoveryErr, err := b.buildResolved(ctx, idp)
	b.mu.Lock()
	delete(b.inflight, key)
	switch {
	case err == nil:
		b.cache[key] = r
		delete(b.failed, key)
	case discoveryErr:
		b.failed[key] = discoveryFailure{err: err, until: b.now().Add(b.failTTL)}
	}
	b.mu.Unlock()
	d.r, d.err = r, err
	close(d.done)
}

// buildResolved performs the discovery round-trip and assembles the client.
// discoveryErr reports that err came from the network discovery (the only
// failure worth remembering).
func (b *Bridge) buildResolved(ctx context.Context, idp *identityprovider.IdentityProvider) (r *resolved, discoveryErr bool, err error) {
	// Multi-tenant IdPs (Entra "common"/"organizations", …) report a
	// {tenantid}-templated issuer in discovery and mint tokens with a
	// tenant-specific iss/aud, so the standard issuer/audience checks reject
	// them. So: accept the discovery-doc issuer (don't fail on the
	// mismatch), skip the built-in iss/aud checks, and validate the token's
	// issuer against oidc_issuer_pattern after verification (see VerifyIDToken).
	discoveryCtx := oidc.ClientContext(ctx, b.client)
	verifierCfg := &oidc.Config{ClientID: *idp.OIDCClientID}
	if idp.OIDCMultiTenant {
		discoveryCtx = oidc.InsecureIssuerURLContext(discoveryCtx, *idp.OIDCIssuerURL)
		verifierCfg.SkipIssuerCheck = true
		verifierCfg.SkipClientIDCheck = true
	}
	provider, err := oidc.NewProvider(discoveryCtx, *idp.OIDCIssuerURL)
	if err != nil {
		return nil, true, fmt.Errorf("oidc.NewProvider: %w", err)
	}
	clientSecret, err := b.resolveClientSecret(idp.OIDCClientSecretRef)
	if err != nil {
		return nil, false, err
	}
	if clientSecret == "" {
		// Legal (a public client), but a confidential IdP registration will
		// refuse the code exchange with nothing more helpful than Entra's
		// AADSTS7000218 — so say it here, where the operator can find it.
		slog.Info("OIDC provider has no client secret configured; the code exchange will run as a public client",
			"identity_provider", idp.ID, "code", idp.Code, "issuer", *idp.OIDCIssuerURL)
	}
	return &resolved{
		provider:      provider,
		verifier:      provider.Verifier(verifierCfg),
		issuerURL:     *idp.OIDCIssuerURL,
		clientID:      *idp.OIDCClientID,
		multiTenant:   idp.OIDCMultiTenant,
		issuerPattern: idp.OIDCIssuerPattern,
		client:        b.client,
		oauth: &oauth2.Config{
			ClientID:     *idp.OIDCClientID,
			ClientSecret: clientSecret,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
	}, false, nil
}

// cacheKey identifies a resolved OIDC client by EVERY IdP field that shapes
// it — issuer, client id, the client-secret ref, multi-tenancy and the issuer
// pattern — so that editing any of them yields a new entry on the next login
// instead of the stale one. The cache used to key on issuer|client id alone
// and never expire: an IdP created without its secret (or logged in to
// before the secret was entered) cached a secret-less oauth2.Config, and
// saving the secret afterwards changed the database but not the running
// process — every exchange failed with AADSTS7000218 until a restart, and
// "saving it in the front end does not save it" was the reasonable
// conclusion (owner, 2026-09-23). The ref is the stored ciphertext (or
// literal:/scheme:// reference), never the plaintext; it is a map key, not
// a log field. Superseded entries are never evicted — they are a handful of
// small structs per edit, and an edit is rare.
func cacheKey(idp *identityprovider.IdentityProvider) string {
	secretRef := ""
	if idp.OIDCClientSecretRef != nil {
		secretRef = *idp.OIDCClientSecretRef
	}
	pattern := ""
	if idp.OIDCIssuerPattern != nil {
		pattern = *idp.OIDCIssuerPattern
	}
	return *idp.OIDCIssuerURL + "|" + *idp.OIDCClientID + "|" + secretRef +
		"|" + strconv.FormatBool(idp.OIDCMultiTenant) + "|" + pattern
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
// is tenant-specific), so both are re-applied manually after verification:
// the issuer against the configured pattern (see isValidIssuer),
// and the audience against our registered client
// ID — only the issuer varies per tenant; the aud is OUR client_id at every
// tenant, so skipping it permanently would accept an ID token minted for a
// completely different relying party at the same IdP.
func (r *resolved) VerifyIDToken(ctx context.Context, raw string) (*oidc.IDToken, error) {
	tok, err := r.verifier.Verify(r.clientContext(ctx), raw)
	if err != nil {
		return nil, err
	}
	if r.multiTenant {
		if !isValidIssuer(tok.Issuer, r.issuerURL, true, r.issuerPattern) {
			return nil, fmt.Errorf("invalid issuer for multi-tenant IdP: %s", tok.Issuer)
		}
		if !audienceContains(tok.Audience, r.clientID) {
			return nil, fmt.Errorf("ID token audience %v does not include this client", tok.Audience)
		}
	}
	return tok, nil
}

// audienceContains reports whether the token's aud list includes our
// registered client ID.
func audienceContains(auds []string, clientID string) bool {
	return slices.Contains(auds, clientID)
}

// isValidIssuer accepts an exact match against the configured issuer URL,
// else (multi-tenant only) a match against the configured pattern.
//
// A pinned pattern (identityprovider.CheckIssuerPattern: anchored, exact
// issuers only — the tenant spelled out) is the issuer check the audit asks
// for (docs/oidc-security-audit.md §1): a token from any other tenant fails
// here, before the email-domain and tid checks. A legacy pattern that does not
// pin the issuer (stored before save refused such patterns) is refused when
// FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED is on; otherwise it is honoured as
// before — full-match under FC_OIDC_ISSUER_PATTERN_STRICT, else the historical
// partial match — and warned about once per pattern, so no existing tenant is
// locked out without the operator choosing it.
func isValidIssuer(iss, issuerURL string, multiTenant bool, pattern *string) bool {
	if issuerURL == iss {
		return true
	}
	if !multiTenant || pattern == nil || *pattern == "" {
		return false
	}
	p := *pattern
	if identityprovider.IssuerPatternPinned(p) {
		// Pinned implies anchored: MatchString is a whole-issuer match.
		re, err := regexp.Compile(p)
		return err == nil && re.MatchString(iss)
	}
	if issuerPatternRequirePinned.Load() {
		warnBroadOnce(p, true)
		return false
	}
	warnBroadOnce(p, false)
	if issuerPatternStrict.Load() {
		// Full-match semantics: the pattern must cover the WHOLE issuer,
		// so an unanchored pattern cannot be satisfied by a substring
		// (https://evil.example/?x=login.microsoftonline.com/…).
		if re, err := regexp.Compile(`^(?:` + p + `)$`); err == nil {
			return re.MatchString(iss)
		}
		return false
	}
	if re, err := regexp.Compile(p); err == nil {
		return re.MatchString(iss)
	}
	return false
}

// issuerPatternStrict switches isValidIssuer to full-match semantics for a
// legacy (not pinned) pattern (FC_OIDC_ISSUER_PATTERN_STRICT). Off by default:
// existing patterns may rely on the historical partial-match behaviour.
var issuerPatternStrict atomic.Bool

// SetIssuerPatternStrict enables or disables full-match issuer patterns.
func SetIssuerPatternStrict(v bool) { issuerPatternStrict.Store(v) }

// issuerPatternRequirePinned refuses every token of a multi-tenant IdP whose
// pattern does not pin the issuer (FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED). Off
// by default: turn it on once the console shows no provider warning.
var issuerPatternRequirePinned atomic.Bool

// SetIssuerPatternRequirePinned enables or disables refusing legacy broad
// issuer patterns at sign-in.
func SetIssuerPatternRequirePinned(v bool) { issuerPatternRequirePinned.Store(v) }

var warnedIssuerPatterns sync.Map

// warnBroadOnce logs, once per pattern and process, that a multi-tenant IdP's
// issuer pattern does not pin the issuer.
func warnBroadOnce(p string, refused bool) {
	if _, dup := warnedIssuerPatterns.LoadOrStore(p, true); dup {
		return
	}
	reason := ""
	if err := identityprovider.CheckIssuerPattern(p); err != nil {
		reason = err.Error()
	}
	if refused {
		slog.Error("OIDC sign-in refused: the identity provider's issuer pattern does not pin the issuer and FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED is on; pin it to your tenant",
			"pattern", p, "reason", reason, "example", identityprovider.IssuerPatternExample)
		return
	}
	slog.Warn("OIDC issuer pattern does not pin the issuer: a token from any tenant it admits passes the issuer check (only the email-domain and tid checks remain). Pin it to your tenant, then set FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED=true",
		"pattern", p, "reason", reason, "example", identityprovider.IssuerPatternExample)
}

// WarnBroadIssuerPatterns logs every multi-tenant identity provider whose
// stored issuer pattern does not pin the issuer — the migration path for
// patterns saved before save-time validation refused them. Called once at
// startup; the console shows the same warning on the provider. Never fails
// the boot.
func WarnBroadIssuerPatterns(ctx context.Context, idps *identityprovider.Repository) {
	all, err := idps.FindAll(ctx)
	if err != nil {
		slog.Warn("could not check identity providers' issuer patterns", "err", err)
		return
	}
	for i := range all {
		if !all[i].HasBroadIssuerPattern() {
			continue
		}
		reason := ""
		if err := identityprovider.CheckIssuerPattern(*all[i].OIDCIssuerPattern); err != nil {
			reason = err.Error()
		}
		slog.Warn("identity provider's issuer pattern does not pin the issuer to a tenant; pin it (save refuses such patterns now; FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED=true refuses its sign-ins)",
			"identity_provider", all[i].ID, "code", all[i].Code, "pattern", *all[i].OIDCIssuerPattern,
			"reason", reason, "example", identityprovider.IssuerPatternExample)
	}
}

// roleSyncRequireAllowList makes a sign-in through a provider that serves a
// client or partner domain, syncs roles and has no allow-list grant no roles
// (FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST). Off by default: turn it on once the
// console and the start-up log show no provider in that state.
var roleSyncRequireAllowList atomic.Bool

// SetRoleSyncRequireAllowList enables or disables refusing unrestricted role
// sync for non-anchor providers at sign-in.
func SetRoleSyncRequireAllowList(v bool) { roleSyncRequireAllowList.Store(v) }

var warnedRoleSyncIdPs sync.Map

// warnRoleSyncNoAllowListOnce logs, once per provider and process, that role
// sync runs for a client or partner domain without an allow-list.
func warnRoleSyncNoAllowListOnce(idp *identityprovider.IdentityProvider, refused bool) {
	if _, dup := warnedRoleSyncIdPs.LoadOrStore(idp.ID, true); dup {
		return
	}
	if refused {
		slog.Error("OIDC role sync grants nothing: the identity provider serves a client or partner domain, syncs roles and has no allow-list, and FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST is on; add an allow-list of the roles it may grant",
			"identity_provider", idp.ID, "code", idp.Code)
		return
	}
	slog.Warn("OIDC role sync has no allow-list on an identity provider that serves a client or partner domain: it may grant any mapped platform role. Add an allow-list, then set FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST=true",
		"identity_provider", idp.ID, "code", idp.Code)
}

// WarnRoleSyncWithoutAllowList logs every identity provider that
// RoleSyncNeedsAllowList — the migration path for providers saved before save
// refused that state. Called once at startup; the console shows the same
// warning on the provider. Never fails the boot.
func WarnRoleSyncWithoutAllowList(ctx context.Context, idps *identityprovider.Repository) {
	all, err := idps.FindAll(ctx)
	if err != nil {
		slog.Warn("could not check identity providers' role-sync allow-lists", "err", err)
		return
	}
	for i := range all {
		if !all[i].RoleSyncNeedsAllowList() {
			continue
		}
		slog.Warn("identity provider serves a client or partner domain, syncs roles and has no allow-list, so it may grant any mapped platform role; add an allow-list (save refuses this now; FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST=true makes its sign-ins grant nothing)",
			"identity_provider", all[i].ID, "code", all[i].Code)
	}
}

// MappingForDomain returns the email-domain mapping for domain, or nil when
// none exists. The provider-direct callback uses it to pin the token's tenant.
func (b *Bridge) MappingForDomain(ctx context.Context, domain string) (*emaildomainmapping.EmailDomainMapping, error) {
	m, err := b.mappings.FindByEmailDomain(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("email_domain_mapping lookup: %w", err)
	}
	return m, nil
}

// WarnIdPsWithoutEmailDomains logs every OIDC identity provider that owns no
// email domain: it cannot be used to sign in until one is mapped to it. Called
// once at startup; the console shows the same warning on the provider. Never
// fails the boot.
func WarnIdPsWithoutEmailDomains(ctx context.Context, idps *identityprovider.Repository) {
	all, err := idps.FindAll(ctx)
	if err != nil {
		slog.Warn("could not check identity providers' email domains", "err", err)
		return
	}
	for i := range all {
		if all[i].Type != identityprovider.TypeOIDC || len(all[i].AllowedEmailDomains) > 0 {
			continue
		}
		slog.Warn("identity provider has no email domain mapped to it, so it cannot be used to sign in; map a domain to it",
			"identity_provider", all[i].ID, "code", all[i].Code)
	}
}

// AuthCodeURL builds the redirect URL for an OIDC login start. The state
// param is a CSRF token the caller persists in the session.
func (r *resolved) AuthCodeURL(state, redirectURI string) string {
	cfg := *r.oauth
	cfg.RedirectURL = redirectURI
	return cfg.AuthCodeURL(state)
}

// Exchange swaps an authorization code for tokens. The PKCE code_verifier is
// REQUIRED: the login redirect sends a code_challenge, so the IdP rejects the
// exchange ("code_verifier does not match") unless the matching verifier is
// presented here. (This both completes PKCE and is what unblocks the token
// exchange against providers that enforce it, e.g. Entra.)
func (r *resolved) Exchange(ctx context.Context, code, redirectURI, codeVerifier string) (*oauth2.Token, error) {
	cfg := *r.oauth
	cfg.RedirectURL = redirectURI
	var opts []oauth2.AuthCodeOption
	if codeVerifier != "" {
		opts = append(opts, oauth2.SetAuthURLParam("code_verifier", codeVerifier))
	}
	return cfg.Exchange(r.clientContext(ctx), code, opts...)
}

// clientContext makes requests made under ctx use the bridge's bounded client.
func (r *resolved) clientContext(ctx context.Context) context.Context {
	if r.client == nil {
		return ctx
	}
	return oidc.ClientContext(ctx, r.client)
}

func emailDomain(email string) string {
	for i := len(email) - 1; i >= 0; i-- {
		if email[i] == '@' {
			return email[i+1:]
		}
	}
	return ""
}

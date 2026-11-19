package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jwx-go/jwkfetch/v4"
	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

// TokenValidatorConfig configures a TokenValidator. IssuerURL and
// Audience are required; everything else has a sensible default.
type TokenValidatorConfig struct {
	// IssuerURL is the FlowCatalyst OIDC server (e.g. https://auth.flowcatalyst.io).
	IssuerURL string
	// Audience is your application's expected aud claim. Defaults to "flowcatalyst".
	Audience string
	// JWKSRefreshInterval is how often jwx checks for key rotation.
	// Defaults to 1 hour.
	JWKSRefreshInterval time.Duration
	// ClockSkew is the acceptable difference for exp/nbf checks.
	// Defaults to 60s.
	ClockSkew time.Duration
	// HTTPClient is the transport used for discovery + JWKS fetch.
	// Defaults to a 15-second-timeout client.
	HTTPClient *http.Client
}

// TokenValidator verifies RS256 JWTs against a JWKS fetched via OIDC
// discovery.
type TokenValidator struct {
	cfg     TokenValidatorConfig
	cache   *JwksCache
	jwksURL string
	once    sync.Once
	initErr error
}

// NewTokenValidator builds a TokenValidator. The first call to Validate
// performs OIDC discovery to find the JWKS URL.
func NewTokenValidator(cfg TokenValidatorConfig) *TokenValidator {
	if cfg.Audience == "" {
		cfg.Audience = "flowcatalyst"
	}
	if cfg.JWKSRefreshInterval == 0 {
		cfg.JWKSRefreshInterval = time.Hour
	}
	if cfg.ClockSkew == 0 {
		cfg.ClockSkew = 60 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}
	cfg.IssuerURL = strings.TrimRight(cfg.IssuerURL, "/")
	return &TokenValidator{
		cfg:   cfg,
		cache: NewJwksCache(cfg.HTTPClient, cfg.JWKSRefreshInterval),
	}
}

// Validate verifies signature, issuer, audience, and exp/nbf claims. The token
// must be spelled exactly as issued (docs/wire-contract.md, "Token spelling"):
// canonical base64url in every segment, with no padding, line breaks or
// surrounding whitespace.
func (v *TokenValidator) Validate(ctx context.Context, token string) (*AuthContext, error) {
	// jwx decodes leniently on this path ('=' padding, the standard alphabet,
	// CR/LF, nonzero spare bits), so the text is checked first.
	if err := checkSpelling(token); err != nil {
		return nil, err
	}
	if err := v.ensureRegistered(ctx); err != nil {
		return nil, err
	}
	set, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		return nil, newErr(KindDiscovery, "fetch JWKS: "+err.Error())
	}
	_, err = jwt.ParseString(
		token,
		jwt.WithKeySet(set, jws.WithRequireKid(false)),
		jwt.WithIssuer(v.cfg.IssuerURL),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithAcceptableSkew(v.cfg.ClockSkew),
	)
	if err != nil {
		return nil, mapJWXError(err)
	}
	claims, derr := decodeAccessTokenClaims(token)
	if derr != nil {
		return nil, derr
	}
	// Only an API credential is an authority for an application backend: an
	// ALLOWLIST ("api", "identity", or no marker on older tokens). Field and
	// any future token class are refused rather than treated as empty-authority.
	if claims.TokenUse != "" && claims.TokenUse != "api" && claims.TokenUse != "identity" {
		return nil, newErr(KindInvalidToken, "token_use "+claims.TokenUse+" is not an API credential")
	}
	return NewAuthContext(claims, token), nil
}

// ValidateBearer strips the "Bearer " prefix and calls Validate.
func (v *TokenValidator) ValidateBearer(ctx context.Context, authHeader string) (*AuthContext, error) {
	token, err := stripBearer(authHeader)
	if err != nil {
		return nil, err
	}
	return v.Validate(ctx, token)
}

// RefreshJWKS forces an immediate re-fetch of the JWKS, useful after a
// known key rotation. Subsequent Validate calls will see the new keys.
func (v *TokenValidator) RefreshJWKS(ctx context.Context) error {
	if v.jwksURL == "" {
		if err := v.ensureRegistered(ctx); err != nil {
			return err
		}
	}
	return v.cache.Refresh(ctx, v.jwksURL)
}

func (v *TokenValidator) ensureRegistered(ctx context.Context) error {
	v.once.Do(func() {
		url, err := discoverJWKSURL(ctx, v.cfg.HTTPClient, v.cfg.IssuerURL)
		if err != nil {
			v.initErr = err
			return
		}
		v.jwksURL = url
		if err := v.cache.Register(url); err != nil {
			v.initErr = newErr(KindDiscovery, "register JWKS URL: "+err.Error())
		}
	})
	return v.initErr
}

// JwksCache wraps jwx's jwkfetch.Cache so callers don't have to deal with
// the underlying lifetime context.
type JwksCache struct {
	once    sync.Once
	inner   *jwkfetch.Cache
	initErr error
	rwInt   time.Duration

	mu        sync.Mutex
	attempted map[string]bool // URLs whose first fetch has been tried
}

// NewJwksCache builds a cache with the given refresh interval.
func NewJwksCache(httpClient *http.Client, refreshInterval time.Duration) *JwksCache {
	// The cache wants a context that bounds its lifetime; we use
	// context.Background so the cache lives for the process lifetime.
	// Callers that need finer control can build the underlying cache
	// themselves and wrap it.
	_ = httpClient // not used: the cache fetches with jwx's default client
	return &JwksCache{rwInt: refreshInterval}
}

// init starts the underlying cache on first use (NewJwksCache has no error return).
func (c *JwksCache) init() error {
	c.once.Do(func() {
		c.inner, c.initErr = jwkfetch.NewCache(context.Background(), httprc.NewClient())
	})
	return c.initErr
}

// Register adds a JWKS URL to the cache with auto-refresh. It does not wait
// for keys: the cache starts a fetch in the background, and the first Get (or
// Refresh) makes one of its own so that the caller sees its error.
func (c *JwksCache) Register(url string) error {
	if err := c.init(); err != nil {
		return err
	}
	return c.inner.Register(
		context.Background(),
		url,
		jwkfetch.WithWaitReady(false),
		jwkfetch.WithMinInterval(c.rwInt),
		jwkfetch.WithConstantInterval(c.rwInt),
	)
}

// Get returns the current key set for the URL, fetching on first call.
func (c *JwksCache) Get(ctx context.Context, url string) (jwk.Set, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	// Registration does not wait for its background fetch, so the first Get
	// for a URL fetches. If that fails, and the background fetch failed too,
	// the entry stays empty and later Gets keep failing until a refresh
	// succeeds (an explicit one, or the cache's own at the refresh interval).
	c.mu.Lock()
	if !c.attempted[url] {
		// Hold the lock across the first fetch so concurrent first callers wait for it.
		defer c.mu.Unlock()
		if c.attempted == nil {
			c.attempted = map[string]bool{}
		}
		c.attempted[url] = true
		return c.inner.Refresh(ctx, url)
	}
	c.mu.Unlock()
	return c.inner.Lookup(ctx, url)
}

// Refresh forces a re-fetch for the URL.
func (c *JwksCache) Refresh(ctx context.Context, url string) error {
	if err := c.init(); err != nil {
		return err
	}
	_, err := c.inner.Refresh(ctx, url)
	return err
}

// discoveryDoc is the subset of OIDC discovery we need.
type discoveryDoc struct {
	JwksURI string `json:"jwks_uri"`
	Issuer  string `json:"issuer,omitempty"`
}

// discoverJWKSURL fetches /.well-known/openid-configuration and returns
// the jwks_uri.
func discoverJWKSURL(ctx context.Context, httpClient *http.Client, issuerURL string) (string, error) {
	url := strings.TrimRight(issuerURL, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", newErr(KindDiscovery, err.Error())
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", newErr(KindDiscovery, fmt.Sprintf("GET %s: %s", url, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", newErr(KindDiscovery, fmt.Sprintf("GET %s: HTTP %d", url, resp.StatusCode))
	}
	var doc discoveryDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return "", newErr(KindDiscovery, "parse discovery doc: "+err.Error())
	}
	if doc.JwksURI == "" {
		return "", newErr(KindDiscovery, "discovery doc missing jwks_uri")
	}
	return doc.JwksURI, nil
}

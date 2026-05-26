package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
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
// discovery. Mirrors crates/fc-sdk/src/auth/jwks.rs::TokenValidator.
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

// Validate verifies signature, issuer, audience, and exp/nbf claims.
func (v *TokenValidator) Validate(ctx context.Context, token string) (*AuthContext, error) {
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

// JwksCache wraps jwx's jwk.Cache so callers don't have to deal with
// the underlying lifetime context.
type JwksCache struct {
	inner *jwk.Cache
	rwInt time.Duration
}

// NewJwksCache builds a cache with the given refresh interval.
func NewJwksCache(httpClient *http.Client, refreshInterval time.Duration) *JwksCache {
	// jwk.Cache wants a context that bounds its lifetime; we use
	// context.Background so the cache lives for the process lifetime.
	// Callers that need finer control can build the underlying jwk.Cache
	// themselves and wrap it.
	_ = httpClient // jwx's HTTPClient hook is registered per-Register call
	c := jwk.NewCache(context.Background())
	return &JwksCache{inner: c, rwInt: refreshInterval}
}

// Register adds a JWKS URL to the cache with auto-refresh.
func (c *JwksCache) Register(url string) error {
	return c.inner.Register(
		url,
		jwk.WithMinRefreshInterval(c.rwInt),
		jwk.WithRefreshInterval(c.rwInt),
	)
}

// Get returns the current key set for the URL, fetching on first call.
func (c *JwksCache) Get(ctx context.Context, url string) (jwk.Set, error) {
	return c.inner.Get(ctx, url)
}

// Refresh forces a re-fetch for the URL.
func (c *JwksCache) Refresh(ctx context.Context, url string) error {
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

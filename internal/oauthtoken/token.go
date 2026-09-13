// Package oauthtoken mints and caches OAuth2 client_credentials access tokens
// for one platform, for the components that call the platform as a client:
// the MCP server, and the router fetching its configuration document.
package oauthtoken

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// tokenRefreshBuffer is how far ahead of expiry the manager refreshes the
// cached token. The buffer covers clock skew and in-flight request latency so
// the platform never sees an already-expired token.
const tokenRefreshBuffer = 60 * time.Second

// Manager mints and caches an OAuth2 client_credentials access token for
// calls to one platform's API. It POSTs the standard form-encoded grant to
// {baseURL}/oauth/token, caches the access token in memory, and refreshes it
// tokenRefreshBuffer before expiry. Safe for concurrent use.
type Manager struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client

	mu     sync.Mutex
	cached *cachedToken

	// now is injectable for tests; defaults to time.Now.
	now func() time.Time
}

type cachedToken struct {
	accessToken string
	expiresAt   time.Time
}

// New builds a token manager for the given platform base URL and client
// credentials. httpClient may be nil (http.DefaultClient is used).
func New(baseURL, clientID, clientSecret string, httpClient *http.Client) *Manager {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Manager{
		tokenURL:     strings.TrimRight(baseURL, "/") + "/oauth/token",
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient:   httpClient,
		now:          time.Now,
	}
}

// Token returns a valid bearer token (no "Bearer " prefix), fetching a fresh
// one when the cache is empty or within tokenRefreshBuffer of expiry. Its
// signature matches client.TokenProvider, so it can be passed directly to
// client.WithTokenProvider.
func (m *Manager) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.cached != nil && m.now().Add(tokenRefreshBuffer).Before(m.cached.expiresAt) {
		return m.cached.accessToken, nil
	}

	tok, err := m.fetch(ctx)
	if err != nil {
		return "", err
	}
	m.cached = tok
	return tok.accessToken, nil
}

// Invalidate drops the cached token so the next Token call mints a fresh one.
//
// For the caller that has just been told its token is not accepted — a 401
// from the resource — where waiting for the refresh buffer would mean every
// attempt until expiry fails the same way. Minting is the only way to find out
// whether the rejection was about the token at all.
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.cached = nil
	m.mu.Unlock()
}

// tokenEndpointResponse is the subset of the RFC-6749 token response we use.
type tokenEndpointResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func (m *Manager) fetch(ctx context.Context) (*cachedToken, error) {
	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {m.clientID},
		"client_secret": {m.clientSecret},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var tr tokenEndpointResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return nil, fmt.Errorf("token endpoint returned no access_token")
	}

	// Default a missing/zero expiry to one hour, matching the platform's
	// access-token TTL — without it the token would be treated as already
	// stale and refetched on every call.
	ttl := time.Duration(tr.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &cachedToken{accessToken: tr.AccessToken, expiresAt: m.now().Add(ttl)}, nil
}

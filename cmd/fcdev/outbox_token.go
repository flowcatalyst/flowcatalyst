package main

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

// clientCredentialsTokenSource mints a platform access token via the OAuth
// client_credentials grant (POST <tokenURL>) and caches it until shortly before
// it expires, re-minting on demand. It implements the outbox TokenSource (and
// its optional Invalidate) so the standalone poller can authenticate with a
// service-account client_id/secret instead of a hand-pasted static token.
type clientCredentialsTokenSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	scope        string // optional requested-scope narrowing; "" = client ceiling
	client       *http.Client

	mu     sync.Mutex
	cached string
	expiry time.Time
}

// newClientCredentialsTokenSource builds a token source. tokenURL must be the
// full /oauth/token endpoint.
func newClientCredentialsTokenSource(tokenURL, clientID, clientSecret, scope string) *clientCredentialsTokenSource {
	return &clientCredentialsTokenSource{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		scope:        scope,
		client:       &http.Client{Timeout: 15 * time.Second},
	}
}

// tokenRefreshSkew re-mints this long before the cached token actually expires,
// so a token never expires mid-flight between the check and the platform call.
const tokenRefreshSkew = 60 * time.Second

// Token returns a valid cached token, minting a fresh one when the cache is
// empty or within tokenRefreshSkew of expiry.
func (s *clientCredentialsTokenSource) Token(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cached != "" && time.Now().Before(s.expiry.Add(-tokenRefreshSkew)) {
		return s.cached, nil
	}

	tok, ttl, err := s.mint(ctx)
	if err != nil {
		return "", err
	}
	s.cached = tok
	// Default to a conservative 5-minute TTL if the platform omits expires_in.
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	s.expiry = time.Now().Add(ttl)
	return tok, nil
}

// Invalidate drops the cached token so the next Token() re-mints. Called by the
// dispatcher after a 401.
func (s *clientCredentialsTokenSource) Invalidate() {
	s.mu.Lock()
	s.cached = ""
	s.expiry = time.Time{}
	s.mu.Unlock()
}

// mint performs the client_credentials exchange and returns the access token
// and its lifetime.
func (s *clientCredentialsTokenSource) mint(ctx context.Context) (string, time.Duration, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)
	if s.scope != "" {
		form.Set("scope", s.scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("token endpoint %s: %s", resp.Status, truncateToken(string(body), 200))
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", 0, fmt.Errorf("decode token response: %w", err)
	}
	if out.AccessToken == "" {
		return "", 0, fmt.Errorf("token endpoint returned no access_token")
	}
	return out.AccessToken, time.Duration(out.ExpiresIn) * time.Second, nil
}

func truncateToken(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

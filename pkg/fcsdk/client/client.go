// Package client is the FlowCatalyst platform HTTP API client. Resource
// access mirrors the Rust SDK shape: each aggregate has its own type
// returned by an accessor method on *FlowCatalystClient
// (c.EventTypes(), c.Subscriptions(), …) that holds a reference back
// to the client and exposes List/Get/Create/Update/Delete/Sync methods.
//
// Authentication: pass a static bearer token via WithToken, or a
// dynamic provider via WithTokenProvider (e.g. an OAuth2 client-
// credentials grant that refreshes ahead of expiry). Both forms feed
// the same Authorization header on every request.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// FlowCatalystClient is the entry point to the platform API.
type FlowCatalystClient struct {
	baseURL       string
	routerBaseURL string
	http          *http.Client
	tokenProvider TokenProvider
	timeout       time.Duration
	retryAttempts int
	retryDelay    time.Duration
}

// TokenProvider returns a fresh bearer token (without the "Bearer "
// prefix) each time the client needs to make a request. Implementations
// MUST be safe for concurrent use.
type TokenProvider func(ctx context.Context) (string, error)

// Option configures a FlowCatalystClient.
type Option func(*FlowCatalystClient)

// WithToken sets a static bearer token. Convenience over
// WithTokenProvider for cases where the token doesn't expire.
func WithToken(token string) Option {
	return func(c *FlowCatalystClient) {
		c.tokenProvider = func(_ context.Context) (string, error) { return token, nil }
	}
}

// WithTokenProvider sets a dynamic token source. Use this with an
// OAuth2 client-credentials manager that refreshes ahead of expiry.
func WithTokenProvider(p TokenProvider) Option {
	return func(c *FlowCatalystClient) { c.tokenProvider = p }
}

// WithHTTPClient overrides the underlying *http.Client. Use this for
// custom transports, proxies, or TLS configuration.
func WithHTTPClient(h *http.Client) Option {
	return func(c *FlowCatalystClient) { c.http = h }
}

// WithTimeout sets a per-request timeout (default 30s). Applied via
// context.WithTimeout inside do.
func WithTimeout(d time.Duration) Option {
	return func(c *FlowCatalystClient) { c.timeout = d }
}

// WithRetry configures transient-error retry. attempts is the total
// number of attempts (including the first), delay is the base delay
// between attempts (exponential backoff is applied internally).
// Default: 3 attempts, 100ms base.
func WithRetry(attempts int, baseDelay time.Duration) Option {
	return func(c *FlowCatalystClient) {
		c.retryAttempts = attempts
		c.retryDelay = baseDelay
	}
}

// WithRouterBaseURL overrides the message-router base URL for
// /monitoring/in-flight-messages/* endpoints. Default: same as baseURL.
func WithRouterBaseURL(u string) Option {
	return func(c *FlowCatalystClient) { c.routerBaseURL = strings.TrimRight(u, "/") }
}

// New builds a FlowCatalystClient.
func New(baseURL string, opts ...Option) *FlowCatalystClient {
	c := &FlowCatalystClient{
		baseURL:       strings.TrimRight(baseURL, "/"),
		http:          http.DefaultClient,
		timeout:       30 * time.Second,
		retryAttempts: 3,
		retryDelay:    100 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.routerBaseURL == "" {
		c.routerBaseURL = c.baseURL
	}
	return c
}

// BaseURL returns the configured platform base URL.
func (c *FlowCatalystClient) BaseURL() string { return c.baseURL }

// RouterBaseURL returns the configured router base URL.
func (c *FlowCatalystClient) RouterBaseURL() string { return c.routerBaseURL }

// ─── HTTP plumbing — exported for use by resource packages ────────────

// Get performs an authenticated GET against the platform. out is
// json-unmarshalled into. If out is nil, the response body is discarded.
func (c *FlowCatalystClient) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, c.baseURL+path, nil, out)
}

// GetRouter is Get against the router base URL (different host).
func (c *FlowCatalystClient) GetRouter(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, c.routerBaseURL+path, nil, out)
}

// postRouter is the POST equivalent of GetRouter. Internal — exposed
// here for the router resource only.
func (c *FlowCatalystClient) postRouter(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, c.routerBaseURL+path, body, out)
}

// Post performs an authenticated POST with a JSON body.
func (c *FlowCatalystClient) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, c.baseURL+path, body, out)
}

// Put performs an authenticated PUT with a JSON body.
func (c *FlowCatalystClient) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, c.baseURL+path, body, out)
}

// Patch performs an authenticated PATCH with a JSON body.
func (c *FlowCatalystClient) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, c.baseURL+path, body, out)
}

// Delete performs an authenticated DELETE. If out is non-nil, the body
// is decoded into it (some delete endpoints return the updated entity).
func (c *FlowCatalystClient) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, c.baseURL+path, nil, out)
}

// EncodeQuery builds a "?a=1&b=2" suffix from name/value pairs, omitting
// empty values. Pass an even number of strings.
func EncodeQuery(pairs ...string) string {
	if len(pairs)%2 != 0 {
		return ""
	}
	v := url.Values{}
	for i := 0; i < len(pairs); i += 2 {
		k, val := pairs[i], pairs[i+1] //nolint:gosec // G602: pairs is key/value (even length) by contract; the loop steps by 2
		if val == "" {
			continue
		}
		v.Set(k, val)
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// QueryBuilder accumulates optional query parameters and renders them
// as "?a=1&b=2" (or "" if nothing was set). Use Set methods that match
// the source type so callers don't have to format pointer types
// themselves.
type QueryBuilder struct {
	v url.Values
}

// NewQuery returns an empty QueryBuilder.
func NewQuery() *QueryBuilder { return &QueryBuilder{v: url.Values{}} }

// String appends a string param if non-empty.
func (q *QueryBuilder) String(name, value string) *QueryBuilder {
	if value != "" {
		q.v.Set(name, value)
	}
	return q
}

// Bool appends a *bool param if non-nil ("true"/"false").
func (q *QueryBuilder) Bool(name string, value *bool) *QueryBuilder {
	if value != nil {
		if *value {
			q.v.Set(name, "true")
		} else {
			q.v.Set(name, "false")
		}
	}
	return q
}

// Uint32 appends a *uint32 param if non-nil.
func (q *QueryBuilder) Uint32(name string, value *uint32) *QueryBuilder {
	if value != nil {
		q.v.Set(name, strconv.FormatUint(uint64(*value), 10))
	}
	return q
}

// Encode renders the query suffix, including the leading "?".
// Returns "" if no params were set.
func (q *QueryBuilder) Encode() string {
	if len(q.v) == 0 {
		return ""
	}
	return "?" + q.v.Encode()
}

// do issues the HTTP request, applying auth, JSON marshalling, retry on
// transient failures, and APIError mapping.
func (c *FlowCatalystClient) do(ctx context.Context, method, fullURL string, body, out any) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
	}

	var lastErr error
	delay := c.retryDelay
	for attempt := 0; attempt < c.retryAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if c.tokenProvider != nil {
			token, err := c.tokenProvider(ctx)
			if err != nil {
				return fmt.Errorf("token provider: %w", err)
			}
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			if !shouldRetry(err) || attempt == c.retryAttempts-1 {
				return fmt.Errorf("http: %w", err)
			}
			sleep(ctx, delay)
			delay *= 2
			continue
		}

		err = handleResponse(resp, out)
		if err == nil {
			return nil
		}

		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Retryable() && attempt < c.retryAttempts-1 {
			lastErr = err
			sleep(ctx, delay)
			delay *= 2
			continue
		}
		return err
	}
	return lastErr
}

func handleResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
		dec := json.NewDecoder(resp.Body)
		if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return &APIError{
		StatusCode: resp.StatusCode,
		Body:       string(body),
	}
}

func shouldRetry(err error) bool {
	// Network-level errors are typically transient. Context cancellation
	// is not — let it surface.
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return true
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

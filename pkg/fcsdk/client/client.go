// Package client is the FlowCatalyst platform HTTP API client. Resource
// access is aggregate-scoped: each aggregate has its own type
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

// WithTimeout sets the timeout of each attempt of a request (default 30s),
// applied with context.WithTimeout around the attempt. An attempt that runs
// out of it is a transport failure after the request may have been sent: a
// repeatable request is retried, any other ends with *OutcomeUnknownError.
// The caller's own context bounds the call as a whole.
func WithTimeout(d time.Duration) Option {
	return func(c *FlowCatalystClient) { c.timeout = d }
}

// WithRetry configures transient-error retry. attempts is the total
// number of attempts (including the first; 0 and 1 both mean no retry),
// delay is the base delay between attempts (exponential backoff is
// applied internally). Default: 3 attempts, 100ms base.
//
// What is retried depends on whether the request is repeatable (GET, HEAD,
// OPTIONS, PUT, DELETE, and the POSTs a method marks; see PostRepeatable):
// 408, 425 and 429, and a transport failure before the request was sent,
// are retried for every request; 500, 502, 503, 504 and a transport failure
// after the request may have been sent only for a repeatable one. A
// non-repeatable request that ends with 502, 503, 504 or such a transport
// failure is not sent again and returns *OutcomeUnknownError.
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

// postRouterRepeatable is the POST equivalent of GetRouter, for a POST the
// router answers without changing anything (so it may be sent again).
// Internal — exposed here for the router resource only.
func (c *FlowCatalystClient) postRouterRepeatable(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPost, c.routerBaseURL+path, body, out, true)
}

// Post performs an authenticated POST with a JSON body. The request is not
// repeatable: a gateway status (502, 503, 504) or a transport failure after
// it may have been sent is not retried and returns *OutcomeUnknownError,
// and a 500 is returned as it is. Use PostRepeatable for a POST that may be
// sent twice.
func (c *FlowCatalystClient) Post(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPost, c.baseURL+path, body, out, false)
}

// PostRepeatable is Post for a POST the caller knows is safe to send again:
// one whose repeat leaves the platform in the same state (a declarative
// sync) or changes nothing (a read-only query). It is retried like a GET.
// Never use it for a create, a provision or a state change: when the answer
// is lost, a repeat creates a second record or is refused.
func (c *FlowCatalystClient) PostRepeatable(ctx context.Context, path string, body, out any) error {
	return c.send(ctx, http.MethodPost, c.baseURL+path, body, out, true)
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

// yamlBody is an out value for an endpoint that answers a YAML document: the
// body is kept as it is instead of decoded as JSON.
type yamlBody string

func handleResponse(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil || resp.StatusCode == http.StatusNoContent {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
		if y, ok := out.(*yamlBody); ok {
			b, err := io.ReadAll(resp.Body)
			*y = yamlBody(b)
			return err
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

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

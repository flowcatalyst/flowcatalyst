package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync/atomic"
	"syscall"
)

// The retry rule. A request is repeatable when its method is GET, HEAD,
// OPTIONS, PUT or DELETE, or when it is a POST its caller marks repeatable
// (PostRepeatable). Then:
//
//   - 408, 425, 429 (refused before it was processed): retried, any request.
//   - 502, 503, 504: retried when repeatable; otherwise not sent again,
//     *OutcomeUnknownError (the platform may have applied it).
//   - 500: retried when repeatable; otherwise returned as the *APIError.
//   - a transport failure before anything was sent (DNS, connection
//     refused, a dial or TLS handshake failure, no connection obtained):
//     retried, any request.
//   - any other transport failure (reset, EOF, the attempt's timeout):
//     retried when repeatable; otherwise *OutcomeUnknownError.
//   - the caller's own context ending: never retried.
//   - a DELETE that answers 404 after an earlier attempt's outcome was
//     unknown succeeds: the earlier attempt most likely deleted it.

// do issues a request whose repeatability follows from its method alone (a
// POST through do is never repeatable).
func (c *FlowCatalystClient) do(ctx context.Context, method, fullURL string, body, out any) error {
	return c.send(ctx, method, fullURL, body, out, false)
}

// repeatableMethod reports whether a request of this method may be sent
// again whatever its path: the methods HTTP defines as idempotent.
func repeatableMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	}
	return false
}

// send issues the HTTP request, applying auth, JSON marshalling, the retry
// rule above and APIError mapping. markedRepeatable is the caller's mark
// that this POST may be sent again; it adds to the method's own rule.
func (c *FlowCatalystClient) send(ctx context.Context, method, fullURL string, body, out any, markedRepeatable bool) error {
	repeatable := markedRepeatable || repeatableMethod(method)

	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal body: %w", err)
		}
	}

	onBehalfOf, err := onBehalfOfFrom(ctx)
	if err != nil {
		return err
	}

	attempts := max(c.retryAttempts, 1)
	delay := c.retryDelay
	// uncertain: an earlier attempt may have been applied by the platform.
	uncertain := false
	for attempt := 1; ; attempt++ {
		last := attempt >= attempts
		res := c.attempt(ctx, method, fullURL, bodyBytes, out, onBehalfOf)
		if res.err == nil {
			return nil
		}
		if res.final {
			return res.err
		}

		if res.transport {
			if ctx.Err() != nil {
				// The caller's cancellation or deadline: never repeated.
				return fmt.Errorf("http: %w", res.err)
			}
			if res.maybeSent {
				if !repeatable {
					return &OutcomeUnknownError{Method: method, URL: fullURL, Err: res.err}
				}
				uncertain = true
			}
			if last {
				return fmt.Errorf("http: %w", res.err)
			}
		} else {
			var apiErr *APIError
			if !errors.As(res.err, &apiErr) {
				return res.err // a 2xx whose body did not decode
			}
			if uncertain && method == http.MethodDelete && apiErr.StatusCode == http.StatusNotFound {
				return nil
			}
			switch {
			case refusedUnprocessed(apiErr.StatusCode):
				// retried for every request
			case gatewayStatus(apiErr.StatusCode):
				if !repeatable {
					return &OutcomeUnknownError{
						Method: method, URL: fullURL,
						StatusCode: apiErr.StatusCode, Body: apiErr.Body, Err: apiErr,
					}
				}
				uncertain = true
			case apiErr.StatusCode == http.StatusInternalServerError && repeatable:
				// a repeatable request may meet a healthier instance
			default:
				return res.err
			}
			if last {
				return res.err
			}
		}
		sleep(ctx, delay)
		delay *= 2
	}
}

// attemptResult is the end of one attempt.
type attemptResult struct {
	err error
	// final: err is not about the platform's answer (building the request,
	// the token provider) and ends the call as it is.
	final bool
	// transport: err came from sending the request (no HTTP answer).
	transport bool
	// maybeSent: for a transport error, the request may have reached the
	// platform. False only when it provably did not.
	maybeSent bool
}

// attempt makes one attempt under its own timeout. A response's body is read
// (by handleResponse) before the attempt's context is cancelled.
func (c *FlowCatalystClient) attempt(ctx context.Context, method, fullURL string, bodyBytes []byte, out any, onBehalfOf string) attemptResult {
	actx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// The connection hooks tell "no connection was obtained, so nothing was
	// written" apart from a failure on an open connection. They compose with
	// any trace already on the caller's context.
	var gotConnStarted, gotConn atomic.Bool
	actx = httptrace.WithClientTrace(actx, &httptrace.ClientTrace{
		GetConn: func(string) { gotConnStarted.Store(true) },
		GotConn: func(httptrace.GotConnInfo) { gotConn.Store(true) },
	})

	req, err := http.NewRequestWithContext(actx, method, fullURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return attemptResult{err: fmt.Errorf("build request: %w", err), final: true}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if _, ok := out.(*yamlBody); ok {
		req.Header.Set("Accept", "application/yaml")
	}
	if onBehalfOf != "" {
		req.Header.Set(OnBehalfOfHeader, onBehalfOf)
	}
	if c.tokenProvider != nil {
		token, err := c.tokenProvider(actx)
		if err != nil {
			return attemptResult{err: fmt.Errorf("token provider: %w", err), final: true}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		notSent := notSentError(err) || (gotConnStarted.Load() && !gotConn.Load())
		return attemptResult{err: err, transport: true, maybeSent: !notSent}
	}
	return attemptResult{err: handleResponse(resp, out)}
}

// notSentError reports whether a transport error happened before any byte of
// the request could have been written: name resolution, a refused or failed
// dial (including a connect timeout), or a failed TLS handshake.
func notSentError(err error) bool {
	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	if opErr, ok := errors.AsType[*net.OpError](err); ok && opErr.Op == "dial" {
		return true
	}
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	_, ok := errors.AsType[tls.RecordHeaderError](err)
	return ok
}

// refusedUnprocessed: statuses that say the request was not processed, so
// it may be sent again whatever it is.
func refusedUnprocessed(status int) bool {
	switch status {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests:
		return true
	}
	return false
}

// gatewayStatus: statuses a gateway answers when it cannot say what became
// of the request.
func gatewayStatus(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

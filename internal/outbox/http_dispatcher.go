package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// batchResponse / itemResult mirror the platform batch-ingest response
// {results:[{id,status,error?}]} (status is SCREAMING_SNAKE_CASE per item).
type batchResponse struct {
	Results []itemResult `json:"results"`
}

type itemResult struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// parseItemStatus maps the per-item wire status to an OutboxStatus. The wire
// strings are exactly OutboxStatus.String() (SCREAMING_SNAKE_CASE).
func parseItemStatus(s string) (common.OutboxStatus, bool) {
	switch s {
	case "SUCCESS":
		return common.OutboxSuccess, true
	case "SKIPPED":
		// The platform deliberately did not store the item (e.g. audit-log with
		// an unresolvable applicationCode/clientCode, or no client access). This
		// is a terminal, acknowledged outcome — retrying is futile and must not
		// block the group, so treat it as success (the row is cleared).
		return common.OutboxSuccess, true
	case "BAD_REQUEST":
		return common.OutboxBadRequest, true
	case "INTERNAL_ERROR":
		return common.OutboxInternalError, true
	case "UNAUTHORIZED":
		return common.OutboxUnauthorized, true
	case "FORBIDDEN":
		return common.OutboxForbidden, true
	case "GATEWAY_ERROR":
		return common.OutboxGatewayError, true
	}
	return 0, false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TokenSource supplies the bearer token for the platform Authorization header.
// Implementations should cache and refresh internally; Token is called once per
// request. A nil TokenSource makes the dispatcher fall back to the static
// AuthToken. A TokenSource may additionally implement tokenInvalidator, whose
// Invalidate() the dispatcher calls on a 401 so the next request re-mints.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// tokenInvalidator is an optional capability of a TokenSource: drop any cached
// token so the next Token() re-fetches. Used to recover from an early 401
// (e.g. a revoked-before-expiry token) without waiting for the cached TTL.
type tokenInvalidator interface{ Invalidate() }

// HTTPDispatcher sends outbox items to the FlowCatalyst platform API.
// Mirrors fc-outbox/src/http_dispatcher.rs.
type HTTPDispatcher struct {
	platformURL string
	authToken   string
	tokenSource TokenSource
	client      *http.Client
}

// NewHTTPDispatcher wires a dispatcher.
func NewHTTPDispatcher(platformURL, authToken string, timeout time.Duration) *HTTPDispatcher {
	return &HTTPDispatcher{
		platformURL: platformURL,
		authToken:   authToken,
		client:      &http.Client{Timeout: timeout},
	}
}

// setAuthHeader sets the bearer Authorization header. When a TokenSource is
// configured it supplies the token (self-refreshing); otherwise the static
// authToken is used. A TokenSource error is returned so the caller can fail the
// dispatch retryably — the platform is reachable, only our token mint failed.
func (d *HTTPDispatcher) setAuthHeader(ctx context.Context, req *http.Request) error {
	if d.tokenSource != nil {
		tok, err := d.tokenSource.Token(ctx)
		if err != nil {
			return err
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		return nil
	}
	if d.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.authToken)
	}
	return nil
}

// onUnauthorized drops a cached token (best-effort) after a 401 so the next
// request re-mints. No-op for a static token or a non-invalidatable source.
func (d *HTTPDispatcher) onUnauthorized() {
	if inv, ok := d.tokenSource.(tokenInvalidator); ok {
		inv.Invalidate()
	}
}

// DispatchOutcome is the result of sending one item.
type DispatchOutcome struct {
	Status  common.OutboxStatus
	Message string
}

// SendBatch POSTs one or more items of the SAME ItemType in a single request
// and returns a per-item outcome keyed by outbox item id. This is the ONE
// dispatch path — Send (single item) delegates here too, so grouped and
// ungrouped items are classified identically.
//
// Result matching is POSITIONAL: the platform returns exactly one result per
// submitted item, in submission order (the events, dispatch-jobs, and
// audit-logs batch endpoints all do this). The result `id` is the platform
// RESOURCE id (event/job/audit id), NOT the outbox row id, so matching by id is
// wrong — results[i] is the outcome for items[i]. A transport/parse error or a
// non-2xx status fails the whole batch with the mapped status.
func (d *HTTPDispatcher) SendBatch(ctx context.Context, items []Item) map[string]DispatchOutcome {
	if len(items) == 0 {
		return map[string]DispatchOutcome{}
	}

	endpoint := d.platformURL + items[0].ItemType.APIPath()
	payloads := make([]json.RawMessage, len(items))
	for i, it := range items {
		payloads[i] = it.Payload
	}
	body, err := json.Marshal(map[string]any{"items": payloads})
	if err != nil {
		return failAll(items, common.OutboxBadRequest, "marshal: "+err.Error())
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return failAll(items, common.OutboxInternalError, "build: "+err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	if err := d.setAuthHeader(ctx, req); err != nil {
		// Couldn't mint a token → treat like an upstream gateway issue (retryable),
		// so the batch is re-tried once the token endpoint recovers.
		return failAll(items, common.OutboxGatewayError, "auth: "+err.Error())
	}

	resp, err := d.client.Do(req)
	if err != nil {
		// Transport failure (connect/DNS/timeout) → GATEWAY_ERROR, matching Rust
		// http_dispatcher.rs send() Err arm. Retryable.
		return failAll(items, common.OutboxGatewayError, "request: "+err.Error())
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		// OB5: a 2xx is NOT blanket success — the platform reports a PER-ITEM
		// outcome in {results:[{id,status,error?}]} and a batch can return 2xx
		// while individual items are BAD_REQUEST/SKIPPED/etc.
		var br batchResponse
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		if err := json.Unmarshal(raw, &br); err != nil {
			return failAll(items, common.OutboxInternalError, "parse results: "+truncate(string(raw), 200))
		}
		// Positional matching: results[i] is the outcome for items[i]. A count
		// mismatch means the contract was violated — fail the whole batch
		// retryably rather than risk mis-assigning outcomes to the wrong rows.
		if len(br.Results) != len(items) {
			return failAll(items, common.OutboxInternalError,
				fmt.Sprintf("result count mismatch: got %d for %d items", len(br.Results), len(items)))
		}
		out := make(map[string]DispatchOutcome, len(items))
		for i, it := range items {
			r := br.Results[i]
			st, ok := parseItemStatus(r.Status)
			if !ok {
				out[it.ID] = DispatchOutcome{Status: common.OutboxInternalError, Message: "unknown item status: " + r.Status}
				continue
			}
			out[it.ID] = DispatchOutcome{Status: st, Message: r.Error}
		}
		return out
	case resp.StatusCode == http.StatusUnauthorized:
		// 401 is retryable; drop any cached token so the next attempt re-mints.
		d.onUnauthorized()
		return failAll(items, common.OutboxUnauthorized, "401")
	case resp.StatusCode == http.StatusForbidden:
		return failAll(items, common.OutboxForbidden, "403")
	case resp.StatusCode == http.StatusBadGateway,
		resp.StatusCode == http.StatusServiceUnavailable,
		resp.StatusCode == http.StatusGatewayTimeout:
		return failAll(items, common.OutboxGatewayError, fmt.Sprintf("%d", resp.StatusCode))
	case resp.StatusCode == http.StatusBadRequest:
		// Only an exact 400 is terminal BAD_REQUEST. Every other unmatched
		// status (other 4xx — 404/409/422/429 — and other 5xx) falls through to
		// INTERNAL_ERROR (retryable), byte-matching Rust http_dispatcher.rs's
		// match arms (400/401/403/500/502..=504, _ => INTERNAL_ERROR). This keeps
		// a transient 429/404 retryable instead of permanently blocking a group.
		return failAll(items, common.OutboxBadRequest, fmt.Sprintf("%d", resp.StatusCode))
	default:
		return failAll(items, common.OutboxInternalError, fmt.Sprintf("%d", resp.StatusCode))
	}
}

// failAll assigns the same outcome to every item (transport/HTTP-level failure).
func failAll(items []Item, st common.OutboxStatus, msg string) map[string]DispatchOutcome {
	m := make(map[string]DispatchOutcome, len(items))
	for _, it := range items {
		m[it.ID] = DispatchOutcome{Status: st, Message: msg}
	}
	return m
}

// Send POSTs a single item and classifies the response into an OutboxStatus.
// It delegates to SendBatch (a 1-item batch) so single and multi-item dispatch
// share exactly one request/response path — there is no separate single-item
// classifier that can drift from the batch one.
func (d *HTTPDispatcher) Send(ctx context.Context, item Item) DispatchOutcome {
	out := d.SendBatch(ctx, []Item{item})
	if o, ok := out[item.ID]; ok {
		return o
	}
	// Unreachable in practice (SendBatch always keys every item), but never
	// return a zero-value SUCCESS by accident — treat as retryable.
	return DispatchOutcome{Status: common.OutboxInternalError, Message: "no outcome for item"}
}

package outbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
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
		// The platform did not store the item (an audit row whose clientCode or
		// applicationCode it could not resolve, on platforms before 2026-11-18).
		// That is a refusal, not a delivery: the row is kept, terminal and not
		// retried, for an operator to see and requeue (skippedMessage). Clearing
		// it as delivered lost the entry on both sides. Audit rows have no
		// message group, so this blocks none.
		return common.OutboxBadRequest, true
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

// skippedMessage is the stored message of a row the platform answered SKIPPED: it says the item
// was not stored, then the platform's own text when it sent one.
func skippedMessage(platformText string) string {
	const msg = "SKIPPED: the platform did not store this item"
	if platformText == "" {
		return msg
	}
	return msg + ": " + platformText
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
type HTTPDispatcher struct {
	platformURL string
	authToken   string
	tokenSource TokenSource
	client      *http.Client
	// gate, when set, is asked immediately before each request is sent; false sends nothing
	// and every item's outcome is NotSent (the processor lost its leadership).
	gate func() bool
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
	// RetryAfter is set when the platform answered 429 (an application over its rate, or paused:
	// the push gateway): the rows are not failed. The processor holds the message group (or the
	// type, for ungrouped rows) for this long and releases the rows without an attempt penalty, so
	// a pause waits in the application's own outbox however long it lasts.
	RetryAfter time.Duration
	// NotSent: the request was not sent because the processor is no longer the leader. The row
	// goes back to PENDING untouched.
	NotSent bool
}

// Bounds of a 429's Retry-After.
const (
	defaultRetryAfter = 5 * time.Second
	maxRetryAfter     = 5 * time.Minute
)

// retryAfter reads a Retry-After header in seconds (an HTTP date is not used by the platform): the
// default when absent or unreadable, at least a second and at most [maxRetryAfter].
func retryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || secs < 0 {
		return defaultRetryAfter
	}
	return min(max(time.Duration(secs)*time.Second, time.Second), maxRetryAfter)
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
//
// A request the platform refuses as too large (413) is not sent again as it
// was: one of several items is split in two and the halves are sent in order
// (the second only when the first did not fail as a whole, see sendParts); a
// single item is a terminal BAD_REQUEST (tooLargeMessage), because the
// platform will never take it. An item whose payload is not JSON is a
// terminal BAD_REQUEST of its own and is not sent.
func (d *HTTPDispatcher) SendBatch(ctx context.Context, items []Item) map[string]DispatchOutcome {
	if len(items) == 0 {
		return map[string]DispatchOutcome{}
	}
	parts, out := encodeItems(items)
	d.sendParts(ctx, parts, out)
	return out
}

// sendCut sends items of one ItemType in as many requests as it takes: they
// are cut, in order, at maxItems items or at maxBytes bytes of request body,
// whichever comes first (<= 0: no limit of that kind), and a request always
// carries at least one item, so an item larger than maxBytes is sent on its
// own. The requests go one after the other, in order. Once one fails as a
// whole in a way that holds the batch (a retryable failure, a 429, the
// leadership lost), the later ones are not sent: their items are NotSent, to
// be put back untouched. failed reports that a request failed as a whole with
// a retryable status, so the caller holds the type.
func (d *HTTPDispatcher) sendCut(ctx context.Context, items []Item, maxItems, maxBytes int) (map[string]DispatchOutcome, bool) {
	parts, out := encodeItems(items)
	stop, failed := false, false
	for _, chunk := range cutParts(parts, maxItems, maxBytes) {
		if stop {
			markNotSent(chunk, out, notSentEarlierFailed)
			continue
		}
		if k := d.sendParts(ctx, chunk, out); k.stops() {
			stop, failed = true, k == requestFailed
		}
	}
	return out, failed
}

// notSentEarlierFailed is the message of an item left unsent because an
// earlier request of its batch failed as a whole.
const notSentEarlierFailed = "not sent: an earlier request of the batch failed"

// tooLargeMessage is the stored error of an item the platform refused as too
// large to take in a request of its own (413): terminal, like a per-item
// PAYLOAD_TOO_LARGE.
func tooLargeMessage(size int) string {
	return fmt.Sprintf("PAYLOAD_TOO_LARGE: the item (%d bytes as sent) is larger than the platform accepts in one request (HTTP 413)", size)
}

// part is one item as it goes into a request: the JSON the batch route
// decodes (normalizeDispatchJobPayload for a dispatch job), as json.Marshal
// writes it into the request body.
type part struct {
	item Item
	body []byte
}

// Bytes of a request body around its items: {"items":[ and ]}.
const emptyRequestBytes = len(`{"items":[]}`)

// requestBytes is the size of the request body that carries parts.
func requestBytes(parts []part) int {
	n := emptyRequestBytes
	for i, p := range parts {
		if i > 0 {
			n++ // the comma
		}
		n += len(p.body)
	}
	return n
}

// encodeItems encodes each item for the request. An item whose payload is not
// JSON gets a terminal BAD_REQUEST in out and no part; the others keep their
// order.
func encodeItems(items []Item) ([]part, map[string]DispatchOutcome) {
	out := make(map[string]DispatchOutcome, len(items))
	parts := make([]part, 0, len(items))
	for _, it := range items {
		payload := it.Payload
		if it.ItemType == common.OutboxItemDispatchJob {
			payload = normalizeDispatchJobPayload(payload)
		}
		b, err := json.Marshal(payload)
		if err != nil {
			out[it.ID] = DispatchOutcome{Status: common.OutboxBadRequest, Message: "marshal: " + err.Error()}
			continue
		}
		parts = append(parts, part{item: it, body: b})
	}
	return parts, out
}

// cutParts cuts parts, in order, into requests of at most maxItems parts and
// maxBytes bytes of body (<= 0: no limit of that kind); a request holds at
// least one part.
func cutParts(parts []part, maxItems, maxBytes int) [][]part {
	var chunks [][]part
	start, size := 0, emptyRequestBytes
	for i, p := range parts {
		n := i - start
		grow := len(p.body)
		if n > 0 {
			grow++ // the comma
		}
		if n > 0 && ((maxItems > 0 && n >= maxItems) || (maxBytes > 0 && size+grow > maxBytes)) {
			chunks = append(chunks, parts[start:i:i])
			start, size, grow = i, emptyRequestBytes, len(p.body)
		}
		size += grow
	}
	if start < len(parts) {
		chunks = append(chunks, parts[start:])
	}
	return chunks
}

func markNotSent(parts []part, out map[string]DispatchOutcome, msg string) {
	for _, p := range parts {
		out[p.item.ID] = DispatchOutcome{NotSent: true, Message: msg}
	}
}

// requestKind is how a request went, as a whole.
type requestKind int

const (
	// requestAnswered: a 2xx with a result per item, or a refusal of the
	// whole request that is terminal (400, 403); the next request is sent.
	requestAnswered requestKind = iota
	// requestFailed: the request failed as a whole with a retryable status
	// (transport, 5xx, 401, any other status, an unreadable answer).
	requestFailed
	// requestWait: 429, the items carry RetryAfter.
	requestWait
	// requestNotSent: the gate was closed (the leadership is lost).
	requestNotSent
	// requestTooLarge: 413, the platform will not take a body this large.
	requestTooLarge
)

// stops reports whether a request that went this way stops its batch: the
// later requests are not sent.
func (k requestKind) stops() bool {
	switch k {
	case requestFailed, requestWait, requestNotSent:
		return true
	case requestAnswered, requestTooLarge:
		return false
	}
	return true
}

// sendParts sends parts in one request and records each part's outcome in
// out. A 413 of several parts sends the two halves instead, in order; the
// second only when the first did not stop (a whole failure, a 429, the gate
// closed), else its parts are NotSent. A 413 of one part is that item's
// terminal BAD_REQUEST. It returns how the batch must go on: the kind of the
// request that stopped it, or requestAnswered.
func (d *HTTPDispatcher) sendParts(ctx context.Context, parts []part, out map[string]DispatchOutcome) requestKind {
	if len(parts) == 0 {
		return requestAnswered
	}
	res, kind := d.post(ctx, parts)
	if kind != requestTooLarge {
		maps.Copy(out, res)
		return kind
	}
	if len(parts) == 1 {
		out[parts[0].item.ID] = DispatchOutcome{Status: common.OutboxBadRequest, Message: tooLargeMessage(len(parts[0].body))}
		return requestAnswered
	}
	h := len(parts) / 2
	if k := d.sendParts(ctx, parts[:h:h], out); k.stops() {
		markNotSent(parts[h:], out, notSentEarlierFailed)
		return k
	}
	return d.sendParts(ctx, parts[h:], out)
}

// post sends parts in one request and maps the answer to an outcome per item,
// and how the request went as a whole.
func (d *HTTPDispatcher) post(ctx context.Context, parts []part) (map[string]DispatchOutcome, requestKind) {
	items := make([]Item, len(parts))
	body := make([]byte, 0, requestBytes(parts))
	body = append(body, `{"items":[`...)
	for i, p := range parts {
		items[i] = p.item
		if i > 0 {
			body = append(body, ',')
		}
		body = append(body, p.body...)
	}
	body = append(body, `]}`...)

	endpoint := d.platformURL + items[0].ItemType.APIPath()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return failAll(items, common.OutboxInternalError, "build: "+err.Error()), requestFailed
	}
	req.Header.Set("Content-Type", "application/json")
	if err := d.setAuthHeader(ctx, req); err != nil {
		// Couldn't mint a token → treat like an upstream gateway issue (retryable),
		// so the batch is re-tried once the token endpoint recovers.
		return failAll(items, common.OutboxGatewayError, "auth: "+err.Error()), requestFailed
	}

	if d.gate != nil && !d.gate() {
		out := make(map[string]DispatchOutcome, len(items))
		for _, it := range items {
			out[it.ID] = DispatchOutcome{NotSent: true, Message: "not the leader"}
		}
		return out, requestNotSent
	}
	resp, err := d.client.Do(req)
	if err != nil {
		// Transport failure (connect/DNS/timeout) → GATEWAY_ERROR. Retryable.
		return failAll(items, common.OutboxGatewayError, "request: "+err.Error()), requestFailed
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
			return failAll(items, common.OutboxInternalError, "parse results: "+truncate(string(raw), 200)), requestFailed
		}
		// Positional matching: results[i] is the outcome for items[i]. A count
		// mismatch means the contract was violated — fail the whole batch
		// retryably rather than risk mis-assigning outcomes to the wrong rows.
		if len(br.Results) != len(items) {
			return failAll(items, common.OutboxInternalError,
				fmt.Sprintf("result count mismatch: got %d for %d items", len(br.Results), len(items))), requestFailed
		}
		out := make(map[string]DispatchOutcome, len(items))
		for i, it := range items {
			r := br.Results[i]
			st, ok := parseItemStatus(r.Status)
			if !ok {
				out[it.ID] = DispatchOutcome{Status: common.OutboxInternalError, Message: "unknown item status: " + r.Status}
				continue
			}
			msg := r.Error
			if r.Status == "SKIPPED" {
				msg = skippedMessage(r.Error)
			}
			out[it.ID] = DispatchOutcome{Status: st, Message: msg}
		}
		return out, requestAnswered
	case resp.StatusCode == http.StatusUnauthorized:
		// 401 is retryable; drop any cached token so the next attempt re-mints.
		d.onUnauthorized()
		return failAll(items, common.OutboxUnauthorized, "401"), requestFailed
	case resp.StatusCode == http.StatusForbidden:
		return failAll(items, common.OutboxForbidden, "403"), requestAnswered
	case resp.StatusCode == http.StatusBadGateway,
		resp.StatusCode == http.StatusServiceUnavailable,
		resp.StatusCode == http.StatusGatewayTimeout:
		return failAll(items, common.OutboxGatewayError, fmt.Sprintf("%d", resp.StatusCode)), requestFailed
	case resp.StatusCode == http.StatusTooManyRequests:
		// back-pressure, not failure: ask again after Retry-After, without spending an attempt
		out := failAll(items, common.OutboxInternalError, "429")
		d := retryAfter(resp.Header.Get("Retry-After"))
		for id, o := range out {
			o.RetryAfter = d
			out[id] = o
		}
		return out, requestWait
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		// The body is over the platform's cap: split (sendParts), never sent
		// again as it was. Should a caller ever read these outcomes, they are
		// the retryable INTERNAL_ERROR any other unmatched status gets.
		return failAll(items, common.OutboxInternalError, "413"), requestTooLarge
	case resp.StatusCode == http.StatusBadRequest:
		// Only an exact 400 is terminal BAD_REQUEST. Every other unmatched
		// status (other 4xx — 404/409/422 — and other 5xx) falls through to
		// INTERNAL_ERROR (retryable): the mapped statuses are 400/401/403/
		// 413/429/500/502-504, everything else → INTERNAL_ERROR. This keeps a
		// transient 404 retryable instead of permanently blocking a group.
		return failAll(items, common.OutboxBadRequest, fmt.Sprintf("%d", resp.StatusCode)), requestAnswered
	default:
		return failAll(items, common.OutboxInternalError, fmt.Sprintf("%d", resp.StatusCode)), requestFailed
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

// normalizeDispatchJobPayload puts a DISPATCH_JOB row's payload into the shape
// POST /api/dispatch-jobs/batch decodes. Every field is forwarded as the SDK
// wrote it (connectionId, idempotencyKey, clientCode/clientId and the rest are
// batch item fields already) except `metadata`: the batch takes a list of
// {key, value}, and Laravel and TypeScript SDK releases before 2026-11-06
// wrote an object. Such a row would fail its whole batch with a 400 and sit
// terminal BAD_REQUEST, so an object is turned into the list here, key-sorted
// (the order the singular route stores a metadata map in); a non-string value
// is carried as its JSON text. Rows still pending in an application's outbox
// table from an older SDK therefore deliver.
//
// The row's own client_id column is not read: it is the outbox's owner (the
// SDK's configured FlowCatalyst client), not the job's business client, which
// the SDK writes in the payload as clientCode/clientId.
//
// Anything that does not parse, or has no object metadata, is returned
// verbatim: the platform's answer for it is the one to record.
func normalizeDispatchJobPayload(raw json.RawMessage) json.RawMessage {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return raw
	}
	meta, ok := fields["metadata"]
	if !ok {
		return raw
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(meta, &obj); err != nil || obj == nil {
		return raw // already a list, null, or not metadata at all
	}
	type entry struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	list := make([]entry, 0, len(obj))
	for _, k := range slices.Sorted(maps.Keys(obj)) {
		v := obj[k]
		var str string
		if err := json.Unmarshal(v, &str); err != nil {
			str = string(bytes.TrimSpace(v))
		}
		list = append(list, entry{Key: k, Value: str})
	}
	enc, err := json.Marshal(list)
	if err != nil {
		return raw
	}
	fields["metadata"] = enc
	out, err := json.Marshal(fields)
	if err != nil {
		return raw
	}
	return out
}

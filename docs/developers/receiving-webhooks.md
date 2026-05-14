# Receiving Webhooks

How to write a webhook receiver that FlowCatalyst can deliver to. Covers the request shape, HMAC signature validation, the ack/nack contract, and retry semantics.

---

## The request shape

FlowCatalyst POSTs to your receiver. Body: the event payload, exactly as published.

```http
POST /webhooks/shipments
Content-Type: application/json
Authorization: Bearer <token>                           ← if connection has bearer auth
X-FLOWCATALYST-SIGNATURE: c3a5b2…                       ← if connection has signing secret
X-FLOWCATALYST-TIMESTAMP: 2026-05-13T15:42:08.331Z      ← always when signing
X-FlowCatalyst-Event-Id: mev_0HZXEQ7D5E6F7
X-FlowCatalyst-Event-Type: orders:fulfillment:shipment:shipped
X-FlowCatalyst-Dispatch-Job-Id: djb_0HZXEQ9J1K2L3
X-FlowCatalyst-Subscription-Id: sub_0HZXEQ8G8H9I0
X-FlowCatalyst-Client-Id: clt_0HZXEQ5Y8JY5Z
X-FlowCatalyst-Correlation-Id: req_…                    ← propagated from publisher
X-FlowCatalyst-Attempt: 1                                ← attempt number, 1-based
traceparent: 00-…                                        ← W3C tracing context if present

{
  "id": "mev_0HZXEQ7D5E6F7",
  "type": "orders:fulfillment:shipment:shipped",
  "source": "/orders/fulfillment",
  "time": "2026-05-13T15:42:00Z",
  "data": {
    "shipmentId": "shp_abc",
    "orderId": "ord_xyz",
    "trackingNumber": "1Z999AA1"
  }
}
```

The headers are how your receiver knows what to do without parsing the body. The body is the source of truth for the event payload.

---

## Verifying the HMAC signature

Connections configured with a signing secret produce signed requests. The signature is HMAC-SHA256 over `timestamp + body`:

```python
import hmac, hashlib, time

SIGNING_SECRET = "your-secret"     # provisioned in the FC admin UI

def verify(request):
    timestamp = request.headers["X-FLOWCATALYST-TIMESTAMP"]
    signature = request.headers["X-FLOWCATALYST-SIGNATURE"]
    body = request.body            # raw bytes; not re-encoded JSON

    # Replay protection: reject anything older than 5 minutes
    ts = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
    if (datetime.utcnow() - ts).total_seconds() > 300:
        return False

    # Constant-time compare
    expected = hmac.new(
        SIGNING_SECRET.encode(),
        (timestamp + body.decode("utf-8")).encode(),
        hashlib.sha256
    ).hexdigest()
    return hmac.compare_digest(expected, signature)
```

Critical:

- **Use the raw body bytes.** Re-encoding through your framework's JSON parser changes whitespace and field order; signature won't match.
- **Use constant-time compare.** Avoid `==` on signatures — it leaks timing information that allows brute-forcing the signature.
- **Validate timestamp freshness.** Without it, an attacker who captured a request once can replay it forever. 5 minutes is the standard window.

### Per-language helpers

- **TypeScript SDK** — `import { verifyWebhook } from "@flowcatalyst/sdk/webhook"` does all the above.
- **Laravel SDK** — middleware `flowcatalyst.verify-webhook` provided.
- **Rust SDK** — `fc-sdk::webhook::verify_request` (requires `webhook` feature).

---

## The ack/nack contract

Your response tells FlowCatalyst what to do next. Three options:

### Acknowledge — `2xx` with `{"ack": true}` (or empty body)

The message is done. FlowCatalyst marks the dispatch job COMPLETED.

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"ack": true}
```

Or just:

```http
HTTP/1.1 204 No Content
```

### Negative-ack — `2xx` with `{"ack": false, "delaySeconds": N}`

You succeeded HTTP-wise but ask FlowCatalyst to retry. This is the "polite NACK" — your endpoint is healthy, but it can't process this particular message right now (rate limit upstream, business state not yet ready, etc.).

```http
HTTP/1.1 200 OK

{"ack": false, "delaySeconds": 60}
```

FlowCatalyst will retry in `delaySeconds` (clipped to the platform's max retry delay). If `delaySeconds` is omitted, defaults to 30 s.

### Failure — non-2xx

| Status | FC interprets as | Action |
|---|---|---|
| 200, 204 (with ack) | Success | Mark COMPLETED |
| 200 with `ack=false` | Throttling | Retry after delaySeconds |
| 400 | Permanent failure (bad request) | Mark FAILED, no retry. Operator must investigate. |
| 401, 403 | Permanent failure (auth) | Mark FAILED, no retry. Operator must fix connection auth. |
| 404 | Permanent failure (no endpoint) | Mark FAILED, no retry. |
| 429 | Throttling | Retry after `Retry-After` header value (or 30s default) |
| 500, 502, 503, 504 | Transient failure | Retry after backoff. Increments attempt count. |
| 501 | Permanent failure (not implemented — CRITICAL) | Mark FAILED, no retry. |
| Connection timeout / DNS failure | Transient failure | Retry after backoff |

The 4xx-is-terminal / 5xx-is-transient rule matters. Returning a 5xx for a permanently-bad request causes infinite retry (until the dispatch job's max attempts is reached). Returning a 4xx for a temporarily-bad situation causes the message to be dropped from your receiver's perspective. Use status codes meaningfully.

---

## Idempotency

Webhook delivery is **at-least-once**. Your receiver must be idempotent.

- **The dispatch job ID is stable.** `X-FlowCatalyst-Dispatch-Job-Id` doesn't change across retries. Track which job IDs you've processed; on a repeat, return 200 immediately without re-doing the work.
- **The event ID is stable.** `X-FlowCatalyst-Event-Id` — useful if you key your idempotency table on event rather than dispatch job (one event might fan out to many dispatch jobs, but each dispatch job represents one event-to-this-subscription delivery).

Typical pattern:

```python
def handle_webhook(request):
    if not verify_signature(request):
        return 401

    job_id = request.headers["X-FlowCatalyst-Dispatch-Job-Id"]

    with db.transaction():
        if db.execute(
            "INSERT INTO webhook_idempotency (job_id) VALUES (%s) ON CONFLICT DO NOTHING RETURNING job_id",
            [job_id]
        ).rowcount == 0:
            # Already processed this job ID
            return 200, {"ack": True}

        # Do the actual work
        process_event(request.json())

    return 200, {"ack": True}
```

Without this, every retry processes the event again. The platform doesn't dedupe on your behalf — it's at-least-once by design (the alternative is at-most-once, which is worse).

---

## Retry semantics

When FlowCatalyst gets a transient failure (5xx, network, `ack=false`), it retries.

| Question | Behaviour |
|---|---|
| How long between retries? | Default 30s. Override via `Retry-After` header (429) or `delaySeconds` (ack=false body). |
| How many retries? | Per the dispatch job's configured max attempts (typically dozens). |
| Where does the retry happen? | The same router pool, the same endpoint. |
| What if your endpoint is consistently slow? | The platform's per-endpoint circuit breaker may trip after sustained failures. Breaker recovers automatically. |
| What if your endpoint is consistently down? | The operator should pause the connection. PENDING jobs accumulate; un-pausing resumes delivery. |
| What's the per-message timeout? | The router waits 15 minutes per HTTP request. If your endpoint takes 16 minutes, the request times out, the message gets stale-recovered and retried. |

Backoff is **fixed delay** by default, not exponential. The platform doesn't grow delay on repeated failures of the same message — that's the receiver's job (return `ack=false` with growing `delaySeconds` if you want exponential). The reason: a slow endpoint that fails 5 times shouldn't have its next retry waiting hours.

### Order of retries within a message group

For `BLOCK_ON_ERROR` subscriptions, a failed message blocks subsequent messages in the same group until the failed one succeeds or is operator-resolved. So your retries for message A happen *before* messages B, C, D for the same group are attempted.

For `NEXT_ON_ERROR`, a failed message gets retried independently while B, C, D proceed past it.

For `IMMEDIATE`, no ordering — every retry is independent.

---

## Performance characteristics

| Property | Value |
|---|---|
| Concurrent requests per endpoint | Up to your pool's `concurrency` (default 10) |
| Rate limit | Up to your pool's `rate_limit_per_minute` (default unlimited) |
| Connections used by the router | HTTP/2 multiplexed; typically ~10 idle conns per host |
| Header overhead | Several FC-specific headers, sum ~500 bytes |
| Body | Your event payload as-is, unmodified |
| Timeout | 15 min request timeout |

If you're seeing latency spikes that correlate with the router pool's concurrency cap, raise the pool's concurrency in the admin UI. If you're seeing your endpoint refusing connections, lower it.

---

## What the receiver sees on retries

The platform increments `X-FlowCatalyst-Attempt` on each retry of the same dispatch job. Useful for logging:

```
log.info("Received event {} (attempt {})", event_id, request.headers["X-FlowCatalyst-Attempt"])
```

Attempt 1 is the initial delivery. Attempts > 1 are retries.

---

## Security checklist

| Concern | Mitigation |
|---|---|
| Spoofed requests | HMAC signature validation (require it) |
| Replay attacks | Timestamp window check (5 minutes typical) |
| MITM | TLS only — never accept FC traffic over plain HTTP in prod |
| Tenant confusion (multi-tenant receivers) | Use `X-FlowCatalyst-Client-Id` to disambiguate |
| Resource exhaustion | Rate-limit per source IP at your edge; the router's pool rate-limit also helps |
| Stuck attempts | Make your handler timeout aggressively (5–10s); slow handlers create router back-pressure |

If a receiver is meant for FC only, the cleanest setup is:

- HTTPS only.
- HMAC signing required (rejects unsigned).
- Allowlist FC's egress IP range (publish from `https://platform.example.com/.well-known/egress-ips` or coordinate out-of-band).
- Read-only middleware that validates the FC signature and adds the verified `eventId` / `clientId` to request context for the actual handler.

---

## Quick reference

```
Receive POST + headers + body
        │
        ▼
Verify HMAC signature (constant-time, timestamp ≤ 5 min)
        │
        ▼ if invalid → return 401
        │
Read X-FlowCatalyst-Dispatch-Job-Id
        │
        ▼
Check idempotency table — already processed?
        │
        ▼ yes → return 200 {ack: true}
        │
Process event (your business logic)
        │
        ▼ success      → return 200 {ack: true}
        │
        ▼ transient    → return 500 (FC retries)
                          or 200 {ack: false, delaySeconds: N}
        │
        ▼ permanent    → return 4xx (FC marks FAILED)
```

---

## Code references

- HMAC signing (router side): `crates/fc-router/src/mediator.rs::sign`.
- TypeScript SDK webhook helper: `clients/typescript-sdk/src/webhook.ts`.
- Laravel SDK middleware: `clients/laravel-sdk/src/Http/Middleware/VerifyWebhookSignature.php`.
- Rust SDK webhook helper: `crates/fc-sdk/src/webhook.rs` (feature `webhook`).
- Response classification (router side): `crates/fc-router/src/mediator.rs::classify_response`.

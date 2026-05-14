# Subscriptions and Pools

Wiring events to webhook endpoints. Subscriptions define *what fires to where*; dispatch pools define *how much in parallel and how fast*.

---

## Subscription anatomy

```
Subscription {
    code: "ship-notifications"           # human-readable
    event_type: "orders:fulfillment:shipment:shipped"  # exact or wildcard
    client_id: clt_acme                  # tenant scope
    connection_id: con_acme_shipping     # destination
    dispatch_pool_id: dpl_general        # traffic class
    dispatch_mode: IMMEDIATE             # ordering policy
    active: true                         # disable without delete
    filters: { ... }                     # optional payload-level filters
}
```

Configured via admin UI (`/subscriptions`), API (`POST /api/subscriptions`), or SDK sync (declared in your application's manifest, pushed at boot).

---

## Event-type pattern matching

The `event_type` field on a subscription can be exact or wildcarded. Patterns are 4 colon-separated segments matching the producer's event-type code.

| Pattern | Matches |
|---|---|
| `orders:fulfillment:shipment:shipped` | Exactly that code |
| `orders:fulfillment:shipment:*` | Any shipment action |
| `orders:fulfillment:*:*` | Any fulfillment event |
| `orders:*:*:*` | Any orders event |
| `*:*:shipment:*` | Any shipment event across applications |
| `*:*:*:*` | Every event (use sparingly — likely a debug subscription) |

Match resolution is segment-by-segment. `*` matches any value in that segment, including underscores.

### Practical guidance

- **Prefer specific patterns.** `orders:fulfillment:shipment:shipped` is clearer than `orders:*:*:shipped`.
- **One subscription per logical workflow.** If you have two different downstream actions on the same event, two subscriptions, not one with cleverness.
- **Wildcards for "all of this domain" auditing.** A `*:*:*:*` subscription pointed at an audit-storage endpoint can sit alongside the specific ones.

---

## Filters

Optional. Beyond event-type matching, filters apply at fan-out time. Two kinds:

### Payload filters (JSONPath)

```
filters: {
    "include": [
        { "path": "$.data.amount", "operator": ">=", "value": 1000 },
        { "path": "$.data.currency", "operator": "==", "value": "USD" }
    ]
}
```

A subscription on `orders:billing:invoice:created` could filter for only USD invoices ≥ $1000. Saves the receiver from filtering — and saves the platform from creating dispatch jobs that the receiver would just ignore.

### Source filters

```
filters: {
    "source_prefix": "/orders/fulfillment"
}
```

Only events with `source` starting with `/orders/fulfillment` match. Useful when multiple producers emit the same event type but you only care about one producer.

Filters are evaluated by the fan-out service before creating dispatch jobs. Failed filters → no dispatch job → no delivery → no receiver work.

---

## Dispatch modes

Three policies for how a subscription processes the events that match it:

### IMMEDIATE — fully concurrent

Default. Each event-to-subscription dispatch is independent. No ordering. The router fires them concurrently subject to the pool's concurrency cap.

```
Events:        evt-1, evt-2, evt-3 (same message_group)
Dispatches:    [evt-1, evt-2, evt-3] all in flight at once
```

When to use:

- Default for most subscriptions. Webhook handlers should be idempotent and order-agnostic anyway.
- Maximises throughput.
- Receiver crashes don't pile up backlog — surviving messages keep firing.

When not:

- The receiver fundamentally depends on order (e.g. "update X, then read X").

### BLOCK_ON_ERROR — strict FIFO within group

Within each `message_group`, dispatches happen one at a time. If one fails, the entire group halts until the failed one succeeds or is operator-resolved.

```
Events:    msg_group="ord_1": [evt-1] → [evt-2] → [evt-3]
                  evt-1 fails (5xx)
Result:    evt-1 retries forever; evt-2, evt-3 wait. Order ord_1's queue is blocked.
           Other groups (ord_2, ord_3, ...) continue independently.
```

When to use:

- Strict ordering matters for downstream correctness.
- The downstream can't recover from "missed event in the middle of the sequence".
- Failures are rare and operator-resolvable.

When not:

- Order matters loosely but downstream can recover.
- You have a flapping receiver — blocking forever isn't useful.

### NEXT_ON_ERROR — FIFO with skip-on-failure

Within each `message_group`, dispatches happen one at a time. On failure, the failed message is marked FAILED but subsequent messages proceed.

```
Events:    msg_group="ord_1": [evt-1] → [evt-2] → [evt-3]
                  evt-1 fails (5xx, exhausted retries)
Result:    evt-1 marked FAILED. evt-2, evt-3 proceed in order.
           ord_1 queue is not blocked.
```

When to use:

- Order is preferred but not strict.
- Downstream can recover from gaps.

When not:

- Strict ordering matters (use BLOCK_ON_ERROR).
- No ordering matters (use IMMEDIATE — better throughput).

### Decision tree

```
Does the downstream care about order?
   │
   ├── No  → IMMEDIATE
   │
   └── Yes
        │
        Does the downstream tolerate gaps (skipped events)?
           │
           ├── No  → BLOCK_ON_ERROR
           │
           └── Yes → NEXT_ON_ERROR
```

---

## Pause / resume

Connections have a status: `ACTIVE` or `PAUSED`. Pausing the connection halts dispatch for every subscription that points at it — without touching the subscription's own state.

When to pause:

- Receiver is down — pause, jobs stack as PENDING, un-pause when receiver recovers.
- Misconfiguration — pause while you fix, un-pause to resume.
- Load test — temporarily route production traffic into the bin so the test isn't competing.

Pause is **persistent** (lives in `msg_connections.status`) and **explicit** (an operator action with an audit log). Compare to the router's circuit breakers, which are automatic and transient.

**Pause takes ~60 seconds to apply** (the scheduler's PausedConnectionCache refreshes every 60 s). PENDING jobs already in the SQS queue will deliver in that window — usually fine; if you need immediate cessation, also pause-resume from the receiver's side (return 503 to tell the router to back off).

---

## Pools

Pools group subscriptions by traffic policy:

```
Pool {
    code: "general"
    concurrency: 10            # max in-flight dispatches in this pool
    rate_limit_per_minute: 600 # 10/sec sustained
}
```

Every subscription references exactly one pool. The router uses the pool's settings to throttle deliveries.

### Pool dimensions

| Setting | Default | What it does |
|---|---|---|
| `concurrency` | 10 | Max parallel in-flight HTTP requests across all subscriptions in this pool |
| `rate_limit_per_minute` | unlimited | Token-bucket rate limit, applied per-pool |

If `rate_limit_per_minute = 600`, the pool dispatches at most 600 messages/min (10/sec) sustained, with bursts allowed up to whatever fits inside the concurrency cap.

### `DEFAULT-POOL`

A pool with code `DEFAULT-POOL` always exists. It's the fallback when a subscription doesn't specify a pool. Don't delete it. Typical sizing: concurrency = 5, no rate limit.

---

## Sizing pools

Pool sizing is the main operator lever. Two variables drive it:

### Concurrency

How many in-flight HTTP requests the **endpoint** can handle simultaneously.

- Single API key against a small API: `concurrency = 1` (one request at a time).
- Internal service that can handle parallelism: `concurrency = 10` (default — a reasonable middle).
- High-throughput receiver: `concurrency = 50+`.

Start at 10. Raise if `fc_router_pool_queue_depth` is consistently high but the receiver is happy. Lower if the receiver is dropping requests under load.

### Rate limit

The maximum sustained rate. Set just below the receiver's documented limit.

- Receiver advertises "100 req/sec" → set `rate_limit_per_minute = 5400` (~90/sec, with headroom).
- No documented limit → omit `rate_limit_per_minute`. Concurrency will be the only throttle.

### Pool segregation

Two rules:

1. **Isolate failure-prone endpoints.** If endpoint A flaps a lot and endpoint B is rock-solid, put them in separate pools. Otherwise A's failures (consuming concurrency on retries) starve B.

2. **Isolate high-volume from low-volume.** A pool processing 1000/min plus a pool processing 1/min should not be the same pool — the rare event waits behind the flood unnecessarily.

For homogeneous workloads (similar latency, similar failure profile), one pool per endpoint family is the right granularity.

Anti-pattern: **one pool per subscription**. Lots of bookkeeping for no benefit. Group by behaviour, not by identity.

---

## Common topologies

### Topology A — single pool, single connection

Smallest sensible setup. One general-purpose pool, one webhook endpoint receiving everything.

```
all subscriptions ──▶ Pool "general" ──▶ Connection "acme-webhook"
```

When to use: small applications, single receiver, no scaling concerns.

### Topology B — pool per workload class

The common production shape.

```
shipping events     ──▶ Pool "shipping"      ──▶ Connection "ship-svc"
billing events      ──▶ Pool "billing"       ──▶ Connection "bill-svc"
audit/compliance    ──▶ Pool "audit"         ──▶ Connection "audit-svc"
default fallback    ──▶ Pool "DEFAULT-POOL"  ──▶ ...
```

Each pool sized for its workload. A flapping shipping receiver doesn't starve billing.

### Topology C — multi-receiver per event

A single event fans out to several subscriptions, each potentially with its own pool.

```
                                  ┌─▶ Subscription "ship-email"  ──▶ Pool "email"  ──▶ "send-email-svc"
event: orders:shipment:shipped ──┼─▶ Subscription "ship-update"  ──▶ Pool "internal" ──▶ "internal-svc"
                                  └─▶ Subscription "ship-audit"   ──▶ Pool "audit"   ──▶ "audit-svc"
```

One event in, three dispatches out. Common for fan-out workflows.

### Topology D — tenant isolation

When clients should not affect each other's dispatch:

```
Client A's subscriptions ──▶ Pool "tenant-a" (capped concurrency)
Client B's subscriptions ──▶ Pool "tenant-b" (capped concurrency)
```

Sized to give each tenant a guaranteed minimum throughput. Mostly relevant for multi-tenant platforms where one noisy tenant could otherwise drown others.

---

## Updating pools at runtime

Pools support hot reload. Change `concurrency` or `rate_limit_per_minute` via:

- Admin UI: `/dispatch-pools/{id}/edit`.
- API: `PATCH /api/dispatch-pools/{id}`.
- Router monitoring API (for very immediate change): `PUT /monitoring/pools/{code}` against the router directly.

The router picks up changes on its next config sync (5-minute interval) or instantly via the monitoring API path. In-flight messages aren't affected; they retain their pool permit until completion.

---

## Adaptive concurrency (forthcoming)

A future feature: pools can auto-tune their concurrency cap based on observed RTT (TCP Vegas algorithm). See [../architecture/adaptive-concurrency.md](../architecture/adaptive-concurrency.md) for design.

Until that ships, sizing is manual. The trade-off is acceptable for most workloads — endpoint capacity is usually stable enough that a 1-line operator tweak is fine.

---

## Quick reference

| Question | Answer |
|---|---|
| How do I make a subscription not fire? | Set `active = false` (preserves config) or pause the connection (preserves subscription) |
| How do I have one event fire two webhooks? | Two subscriptions, both matching the event |
| How do I rate-limit a webhook to 60/min? | Pool with `rate_limit_per_minute = 60` |
| How do I prevent one slow webhook from blocking others? | Put it in its own pool with bounded concurrency |
| How do I make the platform retry harder? | The platform's retry policy is per-attempt-count, not per-subscription. Failed jobs retry until `max_attempts` (typically tens) |
| Can I send to AWS SQS / Kafka instead of HTTP? | Today: HTTP only. Other mediation types are in the type system but not wired through. |

---

## Code references

- Subscription aggregate: `crates/fc-platform/src/subscription/`.
- Connection aggregate: `crates/fc-platform/src/connection/`.
- Dispatch pool aggregate: `crates/fc-platform/src/dispatch_pool/`.
- Fan-out (matching logic): `crates/fc-stream/src/event_fan_out.rs::matches_event_type`.
- Pool config consumed by router: `crates/fc-common/src/lib.rs::PoolConfig`, `crates/fc-router/src/pool.rs`.
- Dispatch modes: `crates/fc-common/src/lib.rs::DispatchMode`.

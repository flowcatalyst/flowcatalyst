# Concepts

The mental model behind FlowCatalyst. Read this if you're new to the platform.

For the implementation view see [../architecture/](../architecture/).

---

## What FlowCatalyst is

A multi-tenant event router. Your application publishes domain events. Other applications (or yours, or external services) consume them via webhook subscriptions. The platform handles routing, ordering, retry, rate-limiting, circuit breaking, and audit.

You don't write router code, queue infrastructure, retry logic, or webhook auth. The platform does that. You write your business code and declare what events you publish + what subscriptions consume them.

---

## The seven nouns

Most operational concepts are one of these seven:

| Noun | What it is |
|---|---|
| **Event** | A fact: "this thing happened in your system". CloudEvents-shaped. Immutable. |
| **Event type** | The kind of fact: a schema + a code (`orders.fulfillment.shipment.shipped`). |
| **Subscription** | A binding: "when an event matching this pattern fires, send it to this connection". |
| **Connection** | A webhook endpoint: URL, auth method, signing secret, pause state. |
| **Dispatch job** | A specific delivery attempt that the platform is making (event × subscription → one job). |
| **Dispatch pool** | A traffic class: concurrency + rate limit. Subscriptions opt into a pool. |
| **Scheduled job** | A cron-triggered firing (separate from event-triggered dispatch). |

```
EVENT TYPE   ←──── declares ────  YOUR APP
   ▲
   │ matches
   │
EVENT  ─── insert ───  msg_events  ─── fan-out ───
                                                   ▼
                              SUBSCRIPTION  ──── links to ────  CONNECTION  ── target_url ──▶ webhook endpoint
                                   │                                  ▲
                                   │ via                              │ paused?
                                   ▼                                  │
                              DISPATCH POOL  ──── concurrency, rate limit
                                   │
                                   │ used by
                                   ▼
                              DISPATCH JOB  ── PENDING → QUEUED → PROCESSING → COMPLETED | FAILED
```

---

## Events

Events conform to **CloudEvents 1.0**. Minimum shape:

```json
{
  "type": "orders.fulfillment.shipment.shipped",
  "source": "/orders/fulfillment",
  "id": "auto-generated-if-omitted",
  "data": {
    "shipmentId": "shp_abc123",
    "orderId": "ord_xyz789",
    "trackingNumber": "1Z999AA1"
  }
}
```

Optional but useful:

- `time` — when the event occurred (defaults to ingest time).
- `subject` — the entity the event is about (`shipmentId` if you want to query "show me all events for shipment X").
- `datacontenttype` — defaults to `application/json`.
- `traceparent` / `tracestate` — W3C tracing context, propagated to dispatched webhook headers.

Events are **immutable**. You don't update events; you publish new ones.

Each event is stored in `msg_events` (write model) and projected to `msg_events_read` (denormalised read model). Retention defaults to 90 days; older partitions get dropped.

### Event type codes

Convention: 4 colon-separated segments, lowercase, underscores within a segment:

```
application : subdomain : aggregate : event_name
─────────── : ───────── : ───────── : ──────────
orders      : fulfillment : shipment : shipped
billing     : invoicing : invoice : finalised
inventory   : warehouse : stock_count : reconciled
```

Why 4 segments: it gives wildcard subscriptions a useful spectrum. A subscription that wants every orders event can use `orders:*:*:*`. One that wants every shipment-related event across applications: `*:*:shipment:*`. Most subscriptions match an exact code, but the wildcard option is the flexibility lever.

You declare event types via:

- **SDK definition sync** (recommended). Your application's code declares its event types in a manifest; on deploy, the SDK pushes the definitions to the platform. See [publishing-events.md](publishing-events.md).
- **Admin UI**. Manual one-offs.

Event types can be **versioned**. `event_type_spec_versions` table holds schema versions over time; events reference the type via stable code, but each event's payload validates against a specific version.

---

## Subscriptions

A subscription binds a pattern (event type, possibly wildcarded) to a destination (connection).

```
Subscription "ship-notifications" {
    event_type: "orders:fulfillment:shipment:shipped"
    connection: con_acme_email_notifier  # → https://acme.example.com/webhooks/shipments
    dispatch_pool: dpl_general
    dispatch_mode: IMMEDIATE
}
```

Important behaviours:

- **Active flag.** Subscriptions can be deactivated without deletion — fan-out skips inactive subs.
- **Pattern matching.** Exact code or wildcards (`*`) at any of the four segments.
- **Per-tenant.** A subscription belongs to a `client_id`. Events for client A don't fan out to subscriptions belonging to client B (unless the subscription opts into cross-client matching, rare).
- **Pool reference.** The subscription's `dispatch_pool_id` determines which router pool handles its dispatches.

### Subscription dispatch mode

Three modes, per-subscription:

| Mode | Behaviour | When to use |
|---|---|---|
| `IMMEDIATE` | Fully concurrent. No FIFO ordering. | Most webhooks. Fast, scales horizontally. |
| `BLOCK_ON_ERROR` | Sequential within message_group. On failure, halt the group until the failed job is resolved. | Order-sensitive workflows where downstream cares about strict ordering. |
| `NEXT_ON_ERROR` | Sequential within message_group. On failure, skip the failed message and continue. | Order-sensitive workflows that tolerate gaps. |

The "ordered" modes only matter when events share a `message_group` (see below). Without a group, both ordered modes behave like `IMMEDIATE`.

---

## Connections

A connection is the destination metadata:

```
Connection {
    endpoint_url: "https://acme.example.com/webhooks/shipments"
    auth_method: BEARER | HMAC | NONE
    auth_token: "..." (encrypted)
    signing_secret: "..." (encrypted; for HMAC)
    status: ACTIVE | PAUSED
}
```

Important:

- **Pausing a connection is the operator pause-button.** Paused connections stop receiving dispatches; jobs stack up as PENDING in the database. Un-pausing resumes naturally.
- **Auth.** The platform sends bearer tokens or HMAC signatures (or nothing) to the receiver. HMAC is recommended for unauthenticated receivers that need to verify the sender.
- **Signing secret.** When set, the router adds `X-FLOWCATALYST-SIGNATURE` (HMAC-SHA256 of timestamp + body) and `X-FLOWCATALYST-TIMESTAMP` to every request. The receiver validates by recomputing.

---

## Dispatch jobs

One per (event × matching subscription). Created by the stream processor's fan-out. Lifecycle:

```
       PENDING
          │   ┌───── operator: cancel
          │   ▼
          │   CANCELLED (terminal)
          │
          │  scheduler picks up
          ▼
        QUEUED
          │
          │  router takes from SQS
          ▼
       PROCESSING
          │
          ▼
   ┌──────┴──────┬─────────────┐
   │             │             │
COMPLETED     FAILED         EXPIRED (terminal — TTL exceeded)
(terminal)    │
              │  retry policy → back to PENDING with backoff
              ▼
         (cycle until exhausted)
```

You see jobs in `/dispatch-jobs`. Useful filters: status, connection, subscription, time range.

Each job has an **attempts** sub-resource: every delivery attempt records the HTTP status code, response body excerpt, and error type. Useful for "why did this fail?" debugging.

---

## Dispatch pools

A pool is a traffic class. Defined in the admin UI (`/dispatch-pools`):

```
Pool "general" {
    concurrency: 10           # max parallel in-flight dispatches
    rate_limit_per_minute: 600 # 10/sec sustained
}
```

Subscriptions reference a pool. Sizing rules of thumb in [subscriptions-and-pools.md](subscriptions-and-pools.md#sizing-pools).

**`DEFAULT-POOL` always exists** — it's the fallback when a subscription has no explicit pool. Don't delete it.

---

## Message groups (FIFO ordering)

When a subscription cares about ordering, events tagged with the same `message_group` deliver sequentially within that group. Different groups deliver in parallel.

```
Group "order_abc":  [evt-1] → [evt-2] → [evt-3]   sequential
Group "order_xyz":  [evt-4] → [evt-5]             sequential
Group None:         [evt-6], [evt-7]              parallel

Order_abc and order_xyz process in parallel; jobs within each are FIFO.
```

The producer (your app) chooses the message group at event publication time. Typical choice: the aggregate ID (order ID, customer ID, etc.). Events about the same order get the same group; the order's events deliver in order.

Without a group: every event is independent; dispatches are fully concurrent in `IMMEDIATE` mode.

A `BLOCK_ON_ERROR` subscription with `message_group = order_abc` will halt the entire `order_abc` queue if any event fails. `NEXT_ON_ERROR` will skip the failure. `IMMEDIATE` ignores ordering entirely.

---

## Tenancy

FlowCatalyst is multi-tenant. The model:

- **Client** — a tenant. Most resources are scoped to a client.
- **Principal** — a user (or service account). Belongs to a client (or anchors above clients).
- **Application** — a logical unit of FC integration (e.g. `orders`, `billing`). Owns event types, role definitions.

Principal scope (set at login by email-domain mapping):

| Scope | Access |
|---|---|
| Anchor | Platform admin; sees all clients |
| Partner | Sees specified clients (cross-tenant integration partner) |
| Client | Sees own client only |

Permission resolution happens on top of scope. A client-scoped user can have permissions to read events but not write subscriptions; a partner with two client assignments has the same permissions in both, etc.

For more on identity setup, see [../operations/identity-and-auth.md](../operations/identity-and-auth.md).

---

## Scheduled jobs

Cron-triggered firings, independent of event-driven dispatch:

```
Scheduled job "nightly-rollup" {
    cron: "0 2 * * *"
    timezone: "Australia/Brisbane"
    target: EVENT      # or WEBHOOK
    event_type: "billing.invoicing.rollup.requested"
    payload: { "currency": "AUD" }
}
```

Two modes:

- **EVENT** — when the cron fires, the platform emits a domain event. Existing subscriptions (or new ones for this event type) handle the work. Recommended — keeps "events are the integration mechanism" invariant.
- **WEBHOOK** — when the cron fires, the platform directly POSTs to a configured URL. Simpler but creates a parallel pipeline.

Each firing creates a `scheduled_job_instance` row with status (`PENDING`, `RUNNING`, `COMPLETED`, `FAILED`) and logs. You can fire manually for testing.

---

## TSIDs

Every entity has a typed TSID — a 13-character Crockford-Base32 ID prefixed by a 3-letter type code:

```
clt_0HZXEQ5Y8JY5Z   Client
usr_0HZXEQ6A2B3C4   Principal (user / service)
evt_0HZXEQ7D5E6F7   Event Type
sub_0HZXEQ8G8H9I0   Subscription
djb_0HZXEQ9J1K2L3   Dispatch Job
mev_…               Event
con_…               Connection
dpl_…               Dispatch Pool
```

TSIDs are:

- Time-ordered (the timestamp is encoded into the prefix of the bytes).
- URL-safe (Crockford Base32, no `+/=` like base64).
- Case-insensitive on decode.
- JS-safe (strings, not bigints — no Number precision issues).

You don't generate TSIDs as a developer — the platform issues them at create time. You'll see them everywhere in API responses.

---

## What's where

| Concept | UI page | API base |
|---|---|---|
| Events | `/events` | `/api/events` |
| Event types | `/event-types` | `/api/event-types` |
| Subscriptions | `/subscriptions` | `/api/subscriptions` |
| Connections | `/connections` | `/api/connections` |
| Dispatch jobs | `/dispatch-jobs` | `/api/dispatch-jobs` |
| Dispatch pools | `/dispatch-pools` | `/api/dispatch-pools` |
| Scheduled jobs | `/scheduled-jobs` | `/api/scheduled-jobs` |
| Identities | `/identities` | `/api/principals`, `/api/roles`, `/api/applications` |
| Audit logs | `/audit-logs` | `/api/audit-logs` |
| Dashboard | `/dashboard` | `/bff/dashboard` |
| Debug views | `/debug/events`, `/debug/dispatch-jobs` | `/bff/debug/*` |

---

## What's next

- [publishing-events.md](publishing-events.md) — how to actually emit events from your application.
- [receiving-webhooks.md](receiving-webhooks.md) — the contract on the receiving side.
- [subscriptions-and-pools.md](subscriptions-and-pools.md) — wiring up the routing rules.
- [scheduled-jobs.md](scheduled-jobs.md) — cron-driven flows.
- [debugging.md](debugging.md) — when something doesn't work.

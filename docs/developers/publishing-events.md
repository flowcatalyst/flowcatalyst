# Publishing Events

Three patterns for publishing events to FlowCatalyst. Pick the right one for your situation.

| Pattern | When | Reliability |
|---|---|---|
| **Outbox pattern** (recommended) | Your app writes business state to a database. | Crash-safe; event publication is part of the business transaction. |
| **Batch API POST** | Quick scripts, manual one-offs, low-criticality automation. | At-most-once if the call fails. |
| **SDK definition sync** | Declaring event *types* (schemas), not publishing instances. | Idempotent declarative push. |

---

## Pattern 1 — outbox (production publishers)

The outbox pattern is what every production publishing application should use. The principle: write business state and an "outbox" row in the same database transaction; let a background process deliver the outbox row to FlowCatalyst eventually.

```
┌──────────────────────────────────────────────────┐
│  Your Application                                │
│                                                  │
│  BEGIN;                                          │
│    UPDATE orders SET status='shipped' WHERE...;  │
│    INSERT INTO outbox_messages (type=EVENT,      │
│        payload='{...}', ...);                    │
│  COMMIT;                                         │
│                                                  │
│  ┌──────────────────────────────────┐           │
│  │  fc-outbox-processor (sidecar)   │           │
│  │  reads outbox, POSTs to platform │           │
│  └──────────────────────────────────┘           │
└──────────────────────────────────────────────────┘
              │
              ▼ POST /api/events/batch
   FlowCatalyst Platform
```

What this guarantees:

- **If the business transaction commits**, the outbox row is durable. The event will be delivered eventually (likely within seconds).
- **If the business transaction rolls back**, neither the business change nor the outbox row exists. No spurious event.
- **If the application crashes** between commit and any HTTP attempt, the outbox row survives. The processor picks it up on restart.

Compare to the naive pattern (publish HTTP from your handler before/after the database commit): you get into the "did the DB commit, did the HTTP succeed, partially?" trilemma every time.

### Doing it via the SDKs

#### TypeScript

```ts
import { defineUseCase, OutboxUnitOfWork } from "@flowcatalyst/sdk";

// One-time: install the SDK alongside your app and configure the database.
const uow = new OutboxUnitOfWork({
    db: yourPostgresPool,
    outboxTable: "outbox_messages",
});

// In your handler:
await uow.commit({
    aggregate: order,
    repository: orderRepo,
    event: {
        type: "orders:fulfillment:shipment:shipped",
        source: "/orders/fulfillment",
        data: { shipmentId, orderId, trackingNumber },
        messageGroup: orderId,            // for FIFO ordering per order
    },
    command: { kind: "ShipOrder", orderId, shipmentId },
});

// uow.commit opens a transaction, persists the aggregate via the repository,
// inserts an outbox row, commits. fc-outbox-processor delivers it later.
```

#### Laravel / PHP

```php
use FlowCatalyst\Sdk\OutboxUnitOfWork;

$uow = new OutboxUnitOfWork($db);

$uow->commit(
    aggregate: $order,
    repository: $orderRepo,
    event: new DomainEvent(
        type: 'orders:fulfillment:shipment:shipped',
        source: '/orders/fulfillment',
        data: ['shipmentId' => $shipmentId, 'orderId' => $orderId],
        messageGroup: $orderId,
    ),
    command: new ShipOrderCommand($orderId, $shipmentId),
);
```

#### Rust

```rust
use fc_sdk::{OutboxUnitOfWork, DomainEvent};

let uow = OutboxUnitOfWork::new(pg_pool, "outbox_messages");

uow.commit(&order, &order_repo, ShipmentShipped {
    shipment_id, order_id, tracking_number
}, &ship_order_cmd).await?;
```

In all three: the SDK handles serialisation, message group propagation, dedup-id generation. You write business code; the SDK writes the outbox row.

### Running the outbox processor

Deploy `fc-outbox-processor` alongside your application. It points at your application's database (the same one with the outbox table) and forwards rows to FlowCatalyst's `/api/events/batch`.

```sh
FC_OUTBOX_DB_TYPE=postgres
FC_OUTBOX_DB_URL=postgresql://app-pg.internal/myapp
FC_API_BASE_URL=https://flowcatalyst.example.com
FC_API_TOKEN=<service-account-bearer-token>
FC_STANDBY_ENABLED=true                       # if running 2 replicas
FC_STANDBY_REDIS_URL=redis://app-redis:6379
FC_STANDBY_LOCK_KEY=app-myapp-outbox-leader   # unique per outbox
  fc-outbox-processor
```

Full configuration in [../operations/configuration.md#outbox-processor](../operations/configuration.md#outbox-processor-fc-outbox-processor-standalone-or-fc-server-with-fc_outbox_enabledtrue).

The SDK's outbox helpers generate the right table schema for `fc-outbox-processor` to consume — supported in Postgres, MySQL, SQLite, and MongoDB. See [../architecture/outbox-processor.md](../architecture/outbox-processor.md) for the table schema and processing flow.

### Properties

| Property | Outbox pattern |
|---|---|
| Atomic with business txn | yes |
| Crash-safe | yes |
| At-least-once delivery | yes |
| Latency | seconds (depends on poll cadence) |
| Setup cost | requires deploying a sidecar processor + DB schema |
| Best for | production publishers writing to a database |

---

## Pattern 2 — direct batch POST

When you don't have a database transaction to atomically wrap, just POST to the batch endpoint:

```
POST /api/events/batch
Authorization: Bearer <service-account-token>
Content-Type: application/json

{
  "items": [
    {
      "type": "demo.order.created",
      "source": "/demo/orders",
      "data": { "orderId": "ord_001" }
    },
    {
      "type": "demo.order.created",
      "source": "/demo/orders",
      "data": { "orderId": "ord_002" }
    }
  ]
}
```

Response:

```json
{
  "results": [
    { "id": "mev_0HZXEQ7D5E6F7", "status": "SUCCESS" },
    { "id": "mev_0HZXEQ7E6F7G8", "status": "SUCCESS" }
  ]
}
```

Per-item status; one bad event in a batch of 100 doesn't poison the others (assuming the schema check happens server-side).

**Limits:**

- Batch size: 1000 items per request (or the configured platform max).
- Auth: Bearer token from a service account with `event:*:write` permission.

**When this is right:**

- Scripts / one-off automation.
- Producers that don't persist business state in a database (e.g. a webhook receiver that fans out FC events as a relay).
- Initial seeding / migration.

**When this is wrong:**

- Production publishers. If the POST fails (network blip, platform briefly unavailable), the event is lost. Use the outbox.

---

## Pattern 3 — SDK definition sync

Different concern: declaring **event types** (schemas), not events.

When your application starts, the SDK pushes the application's manifest to the platform — which event types you publish, which roles you define, which subscriptions you want, which dispatch pools.

```ts
import { sync } from "@flowcatalyst/sdk";

const definitions = sync
  .defineApplication("orders")
  .withEventTypes([
    {
      code: "orders:fulfillment:shipment:shipped",
      name: "Shipment shipped",
      version: "1.0.0",
      schema: {
        type: "object",
        required: ["shipmentId", "orderId"],
        properties: {
          shipmentId: { type: "string" },
          orderId: { type: "string" },
          trackingNumber: { type: "string" },
        },
      },
    },
  ])
  .withRoles([
    { code: "orders_admin", name: "Orders Admin" },
  ])
  .withDispatchPools([
    { code: "orders-default", concurrency: 10, rateLimitPerMinute: 600 },
  ])
  .build();

await client.definitions().sync(definitions);
```

On the platform side, the sync endpoint diffs against existing definitions and:

- Creates new event types / roles / pools.
- Updates existing ones (new schema versions added; old kept for backward compatibility).
- Marks anything removed from the manifest as deprecated (does not delete — too dangerous).

Sync is **idempotent** and emits a single `EventTypesSynced` / `RolesSynced` / etc. summary event with a diff payload.

The `clients/typescript-sdk/docs/syncing-definitions.md` doc has the full structure guide.

---

## Schema management

Event types have versioned schemas. Each version is a JSON Schema validated server-side:

```
EventType: orders:fulfillment:shipment:shipped
    version 1.0.0:  required [shipmentId, orderId]
    version 2.0.0:  required [shipmentId, orderId, trackingNumber]  ← added required field
```

When you publish an event, the server validates against the latest active version (unless you explicitly include `spec_version` in the event metadata).

**Backward compatibility:**

- Adding optional fields → minor version bump, no consumer breakage.
- Adding required fields → major version bump. Old producers should not be expected to send the new field; mark it optional in v1.0.0 and required in v2.0.0, and migrate consumers first.
- Renaming or removing fields → major version bump. Old consumers will break; coordinate.

In practice most teams use semver-flavoured versions but treat any change as additive in v1.x (never remove fields, never tighten validation). Breaking changes get a new event type code (`orders:fulfillment:shipment:shipped` v2 → `orders:fulfillment:shipment:shipped_v2` if a wholesale change is needed).

---

## Idempotency

Each event has an optional `id` field. If you include the same id twice, the platform deduplicates and returns the original `SUCCESS` for the second attempt.

```json
{
  "type": "orders.created",
  "id": "ord-12345-created-2026-05-13",  // your own deterministic id
  "data": { ... }
}
```

This is the standard pattern for "retry-safe" publishers — generate a deterministic ID (typically derived from your business key + event nature), and the platform handles dedup.

The outbox pattern usually doesn't need explicit IDs because the outbox row id is used as the event id. The SDK does this for you.

---

## Message group selection

If your subscriptions use ordered dispatch modes (`BLOCK_ON_ERROR`, `NEXT_ON_ERROR`), the publisher must set `messageGroup` on each event. Choose a group that captures the ordering constraint:

| Use case | Message group |
|---|---|
| All events for one order should arrive in order | `orderId` |
| All events for one customer | `customerId` |
| All events globally — strict ordering across all events | `"global"` (terrible for throughput; avoid) |
| No ordering needed | omit / null |

Within a group, events deliver one-at-a-time per subscription. Across groups, parallel. Pick the smallest scope that satisfies your ordering needs.

---

## Auth setup

Production publishers need a service account:

1. Operator creates a service account in the platform admin UI: `/identities/service-accounts` → "New".
2. Operator creates an OAuth client for the service account.
3. Service account credentials (`client_id`, `client_secret`) given to the publisher.
4. Publisher uses these to fetch FC bearer tokens via `client_credentials`:

```
POST /oauth/token
  grant_type=client_credentials
  client_id=svc_orders_publisher
  client_secret=<secret>
```

Returns a bearer token (default 1h TTL). Cache it; refresh before expiry.

The SDKs handle this automatically — you give them client_id + client_secret, they manage tokens.

Permissions needed: at minimum `<app>:event:*:create` for the events you'll publish. Anchor accounts can publish anything; service accounts are typically scoped narrowly.

---

## Quick reference

```
                                                       ┌── fc-outbox-processor
Production app  ──── OutboxUnitOfWork ──── outbox table┘
                                                       └── HTTP POST /api/events/batch
                                                                              │
                                                                              ▼
                                                                      FlowCatalyst
                                                                              ▲
                                                                              │
Scripts / one-offs  ──── direct HTTP POST ────────────────────────────────────┘

App boot                ──── SDK definitions.sync() ──── platform admin endpoints
                                                                              ▲
                                                                              │
Admin UI                ──── manual create event type ─────────────────────────┘
```

---

## What's next

- [receiving-webhooks.md](receiving-webhooks.md) — the other side of the pipe.
- [subscriptions-and-pools.md](subscriptions-and-pools.md) — wire events to webhooks.
- [debugging.md](debugging.md) — when an event doesn't fan out, when it does but the receiver doesn't get it, etc.
- TypeScript SDK reference: [`clients/typescript-sdk/README.md`](../../clients/typescript-sdk/README.md).
- Laravel SDK reference: [`clients/laravel-sdk/README.md`](../../clients/laravel-sdk/README.md).
- Rust SDK: [`crates/fc-sdk/`](../../crates/fc-sdk/), summarised in [../architecture/shared-crates.md#fc-sdk](../architecture/shared-crates.md#fc-sdk---application-sdk).

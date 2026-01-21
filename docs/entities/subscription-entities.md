# Subscription Entities

This document describes the subscription and dispatch pool entities in FlowCatalyst.

## Subscription

**Collection**: `subscriptions`

Defines how events are delivered to webhook endpoints.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `code` | String | Unique code (per client) |
| `name` | String | Display name |
| `description` | String | Subscription description |
| `clientId` | String | Owning client (null = anchor-level) |
| `clientIdentifier` | String | Client identifier (denormalized) |
| `eventTypes` | List\<EventTypeBinding\> | Subscribed event types |
| `target` | String | Webhook URL |
| `queue` | String | Optional queue override |
| `customConfig` | List\<ConfigEntry\> | Custom configuration |
| `source` | SubscriptionSource | How created |
| `status` | SubscriptionStatus | `ACTIVE` or `PAUSED` |
| `maxAgeSeconds` | int | Event expiry time |
| `dispatchPoolId` | String | Rate limiting pool ID |
| `dispatchPoolCode` | String | Pool code (denormalized) |
| `delaySeconds` | int | Delivery delay |
| `sequence` | int | Ordering priority |
| `mode` | DispatchMode | Processing mode |
| `timeoutSeconds` | int | Delivery timeout |
| `dataOnly` | boolean | Raw payload vs envelope |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### SubscriptionStatus Enum

```java
public enum SubscriptionStatus {
    ACTIVE,  // Processing events
    PAUSED   // Temporarily stopped
}
```

### SubscriptionSource Enum

```java
public enum SubscriptionSource {
    API,  // Created via REST API
    UI    // Created via control plane UI
}
```

### DispatchMode Enum

```java
public enum DispatchMode {
    IMMEDIATE,       // Process as fast as possible
    NEXT_ON_ERROR,   // Skip failed, continue with next in group
    BLOCK_ON_ERROR   // Block group until error resolved
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "code": "order-notifications",
  "name": "Order Notifications",
  "description": "Send order events to notification service",
  "clientId": "0HZXEQ5Y8JY00",
  "clientIdentifier": "acme-corp",
  "eventTypes": [
    {
      "eventTypeId": "0HZXEQ5Y8JY11",
      "eventTypeCode": "ecommerce:orders:order:created",
      "specVersion": "1.1"
    },
    {
      "eventTypeId": "0HZXEQ5Y8JY22",
      "eventTypeCode": "ecommerce:orders:order:shipped",
      "specVersion": null
    }
  ],
  "target": "https://notifications.acme.com/webhooks/orders",
  "queue": null,
  "customConfig": [
    {"key": "notification_channel", "value": "email"}
  ],
  "source": "API",
  "status": "ACTIVE",
  "maxAgeSeconds": 86400,
  "dispatchPoolId": "0HZXEQ5Y8JY33",
  "dispatchPoolCode": "acme-default",
  "delaySeconds": 0,
  "sequence": 99,
  "mode": "NEXT_ON_ERROR",
  "timeoutSeconds": 30,
  "dataOnly": true,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z"
}
```

---

## EventTypeBinding (Embedded)

Links subscriptions to event types.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `eventTypeId` | String | Event type ID |
| `eventTypeCode` | String | Event type code (denormalized) |
| `specVersion` | String | Optional version filter |

### Version Filtering

- `specVersion = null` - Receive all versions
- `specVersion = "1.1"` - Only receive version 1.1
- `specVersion = "1.*"` - Receive any 1.x version (future)

---

## ConfigEntry (Embedded)

Custom configuration key-value pairs.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `key` | String | Configuration key |
| `value` | String | Configuration value |

### Usage

Pass custom data to webhook receivers:

```json
{
  "customConfig": [
    {"key": "notification_channel", "value": "slack"},
    {"key": "priority", "value": "high"}
  ]
}
```

---

## DispatchPool

**Collection**: `dispatch_pools`

Rate limiting and concurrency pools for subscriptions.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `code` | String | Unique pool code |
| `name` | String | Display name |
| `description` | String | Pool description |
| `rateLimit` | Integer | Requests per minute (null = unlimited) |
| `concurrency` | Integer | Max concurrent requests |
| `clientId` | String | Owning client (null = anchor-level) |
| `clientIdentifier` | String | Client identifier (denormalized) |
| `status` | DispatchPoolStatus | Pool status |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### DispatchPoolStatus Enum

```java
public enum DispatchPoolStatus {
    ACTIVE,    // Pool is operational
    ARCHIVED   // Pool is disabled
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "code": "acme-default",
  "name": "Acme Default Pool",
  "description": "Default pool for Acme Corp webhooks",
  "rateLimit": 600,
  "concurrency": 10,
  "clientId": "0HZXEQ5Y8JY00",
  "clientIdentifier": "acme-corp",
  "status": "ACTIVE",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Pool Sizing Guidelines

| Scenario | Rate Limit | Concurrency |
|----------|------------|-------------|
| Low volume | null (unlimited) | 5 |
| Standard | 600/min | 10 |
| High volume | 3000/min | 50 |
| Burst-capable | 6000/min | 100 |

---

## Subscription Cache

Active subscriptions are cached to accelerate event-to-subscription matching during event ingestion.

### SubscriptionCache

```java
@ApplicationScoped
public class SubscriptionCache {
    // Caffeine cache with 5-minute TTL
    // Key: "{eventTypeCode}:{clientId|anchor}"
    // Value: List<CachedSubscription>
}
```

### CachedSubscription (Lightweight DTO)

```java
public record CachedSubscription(
    String id,
    String code,
    String clientId,
    String target,
    String queue,
    String dispatchPoolId,
    String dispatchPoolCode,
    String serviceAccountId,
    DispatchMode mode,
    int sequence,
    int delaySeconds,
    int timeoutSeconds,
    int maxRetries,
    int maxAgeSeconds,
    boolean dataOnly,
    List<ConfigEntry> customConfig
) {}
```

### Cache Behavior

- **TTL**: 5 minutes (configurable via `flowcatalyst.subscription-cache.ttl-minutes`)
- **Max Size**: 10,000 entries (configurable via `flowcatalyst.subscription-cache.max-size`)
- **Invalidation**: Automatic when subscriptions are created, updated, or deleted
- **Load on Miss**: Queries database and caches result

### Cache Invalidation

The cache is automatically invalidated when subscription mutations occur:

```java
// In SubscriptionOperations
if (result instanceof Result.Success<SubscriptionCreated> success) {
    for (EventTypeBinding binding : success.value().eventTypes()) {
        subscriptionCache.invalidateByEventTypeCode(binding.eventTypeCode());
    }
}
```

---

## Subscription Behavior

### Anchor-Level Subscriptions

```java
// isAnchorLevel() returns true when clientId is null
if (subscription.clientId == null) {
    // Anchor-level: receives events from ALL clients
}
```

### Active Status Check

```java
// isActive() checks status
if (subscription.status == SubscriptionStatus.ACTIVE) {
    // Process events for this subscription
}
```

### Dispatch Mode Behavior

| Mode | On Success | On Error |
|------|------------|----------|
| `IMMEDIATE` | Continue | Continue |
| `NEXT_ON_ERROR` | Continue | Skip, continue next |
| `BLOCK_ON_ERROR` | Continue | Block message group |

---

## Common Queries

### Find active subscriptions for client

```java
subscriptionRepository.find(
    "(clientId = ?1 or clientId is null) and status = ?2",
    clientId, SubscriptionStatus.ACTIVE
).list();
```

### Find subscriptions for event type

```java
subscriptionRepository.find(
    "eventTypes.eventTypeCode", eventTypeCode
).list();
```

### Find subscription by code and client

```java
subscriptionRepository.find(
    "code = ?1 and clientId = ?2",
    code, clientId
).firstResult();
```

### Find dispatch pool by code

```java
dispatchPoolRepository.find("code", poolCode).firstResult();
```

### Find active pools for client

```java
dispatchPoolRepository.find(
    "(clientId = ?1 or clientId is null) and status = ?2",
    clientId, DispatchPoolStatus.ACTIVE
).list();
```

---

## Best Practices

### Subscription Design

1. **One subscription per use case** - Don't overload with too many event types
2. **Use descriptive codes** - Make codes meaningful (`order-notifications` not `sub-1`)
3. **Set appropriate timeouts** - Balance reliability vs latency
4. **Use pools for rate limiting** - Group related subscriptions

### Pool Design

1. **Separate by target system** - One pool per external API
2. **Match target capacity** - Set limits based on downstream capability
3. **Use anchor pools carefully** - Shared pools affect all clients

### Mode Selection

- **IMMEDIATE** - Default for most cases, highest throughput
- **NEXT_ON_ERROR** - When order matters but failures shouldn't block
- **BLOCK_ON_ERROR** - Only when strict ordering is required

## See Also

- [Event Entities](event-entities.md) - Event system entities
- [Dispatch Entities](dispatch-entities.md) - Dispatch job entities
- [Dispatch Jobs Guide](../guides/dispatch-jobs.md) - Webhook delivery
- [Message Groups Guide](../guides/message-groups.md) - FIFO ordering

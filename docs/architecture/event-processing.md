# Event Processing Architecture

FlowCatalyst implements an event-driven architecture where events flow through subscriptions to create dispatch jobs for webhook delivery. This document describes the event processing pipeline.

## Event Lifecycle

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│   Event     │────►│ Subscription│────►│  Dispatch   │────►│   Webhook   │
│   Created   │     │   Matching  │     │    Jobs     │     │  Delivery   │
└─────────────┘     └─────────────┘     └─────────────┘     └─────────────┘
       │                   │                   │                   │
       ▼                   ▼                   ▼                   ▼
   MongoDB          SubscriptionCache      SQS Queue           Router
   (events)         (5-min TTL)           (FIFO)            (delivery)
```

The pipeline is **synchronous**: when an event is created, dispatch jobs are immediately created and queued within the same API call. This provides:
- **Low latency** - No delay waiting for background processors
- **Reliability** - Safety net polling for queue failures
- **Simplicity** - Fewer moving parts than async processing

## Event Model

Events follow the CloudEvents specification with FlowCatalyst extensions.

### Event Entity

```java
@MongoEntity(collection = "events")
public class Event {
    @BsonId
    public String id;              // TSID (e.g., "0HZXEQ5Y8JY5Z")

    public String specVersion;      // CloudEvents version (e.g., "1.0")
    public String type;             // Event type code (e.g., "order.created")
    public String source;           // Origin system
    public String subject;          // Aggregate reference (e.g., "order:12345")
    public Instant time;            // Event timestamp
    public String data;             // JSON payload

    // Tracing
    public String correlationId;    // Request correlation
    public String causationId;      // Causing event ID
    public String deduplicationId;  // Idempotency key
    public String messageGroup;     // FIFO ordering group

    // Filtering
    public List<ContextData> contextData;  // Searchable key-value pairs
}
```

### Context Data

Events can include searchable context data:

```java
public record ContextData(String key, String value) {}
```

Example usage:
```json
{
  "id": "0HZXEQ5Y8JY5Z",
  "type": "order.created",
  "subject": "order:12345",
  "data": "{\"amount\": 99.99}",
  "contextData": [
    {"key": "customer_id", "value": "cust_789"},
    {"key": "region", "value": "us-west"}
  ]
}
```

## Event Types

Event types define the structure and versioning of events.

### EventType Entity

```java
@MongoEntity(collection = "event_types")
public class EventType {
    @BsonId
    public String id;

    public String code;            // Globally unique (e.g., "myapp:orders:order:created")
    public String name;            // Display name
    public String description;     // Documentation
    public EventTypeStatus status; // CURRENT or ARCHIVE

    public List<SpecVersion> specVersions;  // Version history

    public Instant createdAt;
    public Instant updatedAt;
}
```

### Event Type Code Format

Event type codes follow a hierarchical pattern:
```
{application}:{subdomain}:{aggregate}:{event}
```

Examples:
- `ecommerce:orders:order:created`
- `ecommerce:orders:order:shipped`
- `crm:customers:customer:registered`

### Schema Versioning

Each event type maintains versioned schemas:

```java
public record SpecVersion(
    String version,          // "MAJOR.MINOR" (e.g., "1.0", "1.1", "2.0")
    String mimeType,         // "application/json"
    String schema,           // JSON Schema content
    SchemaType schemaType,   // JSON_SCHEMA, PROTO, XSD
    SpecVersionStatus status // FINALISING, CURRENT, DEPRECATED
) {}
```

**Version Lifecycle**:
- `FINALISING` - Under development, schema may change
- `CURRENT` - Active version, accepting events
- `DEPRECATED` - Still accepted, but consumers should migrate

## Subscriptions

Subscriptions define how events are delivered to webhooks.

### Subscription Entity

```java
@MongoEntity(collection = "subscriptions")
public class Subscription {
    @BsonId
    public String id;

    public String code;            // Unique per client
    public String name;
    public String description;
    public String clientId;        // Owning client (null = anchor-level)

    // Event matching
    public List<EventTypeBinding> eventTypes;  // Which events to subscribe to

    // Delivery target
    public String target;          // Webhook URL
    public String queue;           // Optional queue override

    // Behavior
    public DispatchMode mode;      // IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR
    public String dispatchPoolId;  // Rate limiting pool
    public int timeoutSeconds;     // Delivery timeout
    public int maxAgeSeconds;      // Event expiry
    public int delaySeconds;       // Delivery delay
    public int sequence;           // Ordering priority
    public boolean dataOnly;       // Raw payload vs envelope

    public SubscriptionStatus status;  // ACTIVE, PAUSED
    public List<ConfigEntry> customConfig;

    public Instant createdAt;
    public Instant updatedAt;
}
```

### Event Type Binding

Links subscriptions to specific event type versions:

```java
public record EventTypeBinding(
    String eventTypeId,
    String eventTypeCode,
    String specVersion       // Optional version filter
) {}
```

## Event Processing Pipeline

### 1. Event Ingestion

Events arrive via REST API:

```http
POST /api/events
Content-Type: application/json

{
  "type": "order.created",
  "source": "ecommerce-api",
  "subject": "order:12345",
  "data": {"amount": 99.99, "items": 3}
}
```

The event is stored in MongoDB with a generated TSID.

### 2. Synchronous Subscription Matching

Matching subscriptions are found using a cached lookup:

```java
// Single cache lookup per (eventTypeCode, clientId) combination
List<CachedSubscription> subscriptions = subscriptionCache
    .getByEventTypeCode(event.type, clientId);
```

**Subscription Cache**:
- **Caffeine cache** with 5-minute TTL
- Keyed by `{eventTypeCode}:{clientId|anchor}`
- Automatically invalidated when subscriptions change (create/update/delete)
- Reduces database load for high-volume event ingestion

**Matching Criteria**:
- Event type code matches subscription binding
- Subscription is ACTIVE
- Client scope matches (if applicable)

### 3. Dispatch Job Creation

For each matching subscription, a dispatch job is created with **QUEUED** status:

```java
DispatchJob job = new DispatchJob();
job.id = TsidGenerator.generate();
job.kind = DispatchKind.EVENT;
job.code = event.type;
job.subject = event.subject;
job.eventId = event.id;
job.correlationId = event.correlationId;
job.targetUrl = subscription.target;
job.payload = formatPayload(event, subscription);
job.serviceAccountId = subscription.serviceAccountId;
job.mode = subscription.mode;
job.dispatchPoolId = subscription.dispatchPoolId;
job.messageGroup = computeMessageGroup(subscription.code, event.messageGroup);
job.status = DispatchStatus.QUEUED;  // Created ready for queue
```

Jobs are bulk inserted to MongoDB within the same transaction.

### 4. Queue Submission

Dispatch jobs are immediately sent to the SQS FIFO queue:

```java
// Batch send to queue (max 10 per SQS batch)
Set<String> failedJobIds = sendToQueueBatch(dispatchJobs);

// Handle failures: update failed jobs to PENDING
if (!failedJobIds.isEmpty()) {
    dispatchJobRepository.updateStatusBatch(failedJobIds, DispatchStatus.PENDING);
}
```

**Queue Failure Handling**:
- Jobs that fail to queue are updated to PENDING status
- The safety net poller will pick them up for retry
- This ensures no jobs are lost even if the queue is temporarily unavailable

### 5. Safety Net Polling

Two pollers ensure reliability:

**StaleQueuedJobPoller** (every 60s):
```java
// Find QUEUED jobs older than threshold (default: 15 min)
List<DispatchJob> staleJobs = dispatchJobRepository
    .findStaleQueued(threshold);

// Reset to PENDING for re-processing
dispatchJobRepository.updateStatusBatch(ids, DispatchStatus.PENDING);
```

**PendingJobPoller** (every 5s):
```java
// Find PENDING jobs ready for dispatch
List<DispatchJob> pending = dispatchJobRepository
    .findPendingJobs(batchSize);

// Send to queue and update to QUEUED
for (DispatchJob job : pending) {
    queueClient.send(job.toMessagePointer());
}
```

### 6. Message Routing & Delivery

The **Message Router** processes queued jobs:

1. Consumer receives message from queue
2. Pool applies rate limiting and concurrency
3. HTTP Mediator delivers to webhook
4. Result recorded as dispatch attempt
5. Job status updated (COMPLETED, ERROR, etc.)

## Dispatch Jobs

### DispatchJob Entity

See [Dispatch Entities](../entities/dispatch-entities.md) for full reference.

Key fields:
- `kind` - EVENT (from subscription) or TASK (direct API)
- `code` - Event type or task code
- `subject` - Aggregate reference
- `status` - PENDING, QUEUED, IN_PROGRESS, COMPLETED, ERROR, CANCELLED
- `attempts` - Delivery attempt history

### Dispatch Status Flow

```
                    ┌─────────────────────────────────────────┐
                    │         Synchronous Creation            │
                    │  (Event or DispatchJob API call)        │
                    └─────────────────────────────────────────┘
                                      │
                                      ▼
                    ┌─────────────────────────────────────────┐
                    │  QUEUED (sent to SQS)                   │
                    │  └─► queue failure → PENDING            │
                    └─────────────────────────────────────────┘
                                      │
                    ┌─────────────────┴─────────────────┐
                    ▼                                   ▼
              (normal flow)                    (stale after 15min)
                    │                                   │
                    ▼                                   ▼
              IN_PROGRESS                          PENDING
                    │                                   │
         ┌─────────┴─────────┐                          │
         ▼                   ▼                          │
    COMPLETED              ERROR ◄──────────────────────┘
                             │        (safety net requeue)
                             ▼
                      (retry or CANCELLED)
```

**Key Points**:
- Jobs are created with **QUEUED** status (not PENDING)
- PENDING is only used when queue submission fails
- StaleQueuedJobPoller resets stuck QUEUED jobs to PENDING after 15 minutes
- PendingJobPoller picks up PENDING jobs and sends them to the queue

## Message Groups & FIFO Ordering

Events can be ordered within message groups for strict FIFO delivery.

### Message Group Computation

```
messageGroup = {subscriptionCode}:{eventMessageGroup}
```

Example:
- Event message group: `order:12345`
- Subscription code: `order-processor`
- Dispatch job message group: `order-processor:order:12345`

### Dispatch Modes

| Mode | Behavior |
|------|----------|
| `IMMEDIATE` | Process as fast as possible, no ordering guarantees |
| `NEXT_ON_ERROR` | Skip failed messages, continue with next in group |
| `BLOCK_ON_ERROR` | Block entire group until error resolved |

See [Message Groups Guide](../guides/message-groups.md) for details.

## Schemas

### Schema Entity

```java
@MongoEntity(collection = "schemas")
public class Schema {
    @BsonId
    public String id;

    public String name;
    public String description;
    public String mimeType;
    public SchemaType schemaType;   // JSON_SCHEMA, PROTO, XSD
    public String content;          // Schema definition

    // Optional link to event type
    public String eventTypeId;
    public String version;

    public Instant createdAt;
    public Instant updatedAt;
}
```

**Schema Types**:
- `JSON_SCHEMA` - JSON Schema (draft-07)
- `PROTO` - Protocol Buffers
- `XSD` - XML Schema Definition

## Best Practices

### Event Design

1. **Use descriptive type codes** - Follow `{app}:{domain}:{aggregate}:{event}` pattern
2. **Include subject** - Always identify the affected aggregate
3. **Use correlation IDs** - Enable distributed tracing
4. **Version schemas** - Support backward compatibility

### Subscription Design

1. **One subscription per use case** - Don't overload subscriptions
2. **Use dispatch pools** - Control rate limits per target
3. **Set appropriate timeouts** - Balance reliability vs latency
4. **Consider message groups** - Use for ordering requirements

### Error Handling

1. **Implement idempotency** - Webhooks may receive duplicates
2. **Return appropriate status codes** - 2xx for success, 5xx for retry
3. **Use BLOCK_ON_ERROR sparingly** - Only when strict ordering required

## See Also

- [Entity Overview](../entities/overview.md) - Data model reference
- [Dispatch Jobs Guide](../guides/dispatch-jobs.md) - Webhook delivery
- [Message Groups Guide](../guides/message-groups.md) - FIFO ordering

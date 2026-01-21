# Postbox Module

The postbox module provides reliable message ingestion and batch processing for external systems. It acts as an intermediate store for events and dispatch jobs before they enter the main FlowCatalyst processing pipeline.

## Overview

```
External System
      │
      ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Postbox Module                           │
│                                                                 │
│  ┌─────────────────┐    ┌─────────────────┐                    │
│  │ Ingest Endpoint │───►│   PostgreSQL    │                    │
│  │ POST /ingest    │    │ postbox_messages│                    │
│  └─────────────────┘    └────────┬────────┘                    │
│                                  │                              │
│                         ┌────────┴────────┐                    │
│                         │ Poller Discovery│                    │
│                         │ (per partition) │                    │
│                         └────────┬────────┘                    │
│                                  │                              │
│              ┌───────────────────┼───────────────────┐         │
│              ▼                   ▼                   ▼         │
│  ┌─────────────────┐ ┌─────────────────┐ ┌─────────────────┐  │
│  │ Partition       │ │ Partition       │ │ Partition       │  │
│  │ Poller (T1:P1)  │ │ Poller (T1:P2)  │ │ Poller (T2:P1)  │  │
│  └────────┬────────┘ └────────┬────────┘ └────────┬────────┘  │
│           │                   │                   │            │
│           └───────────────────┼───────────────────┘            │
│                               ▼                                │
│                    ┌─────────────────────┐                     │
│                    │  Message Handlers   │                     │
│                    │ (EVENT/DISPATCH_JOB)│                     │
│                    └─────────────────────┘                     │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
                    FlowCatalyst Platform
                    (Events / Dispatch Jobs)
```

## Use Cases

1. **Outbox Pattern** - External applications write to postbox instead of calling FlowCatalyst directly
2. **Batch Ingestion** - Bulk event imports from external systems
3. **Reliable Delivery** - Guaranteed at-least-once processing with retry support
4. **Partition Isolation** - Process messages per logical partition for ordering

## Core Components

### PostboxIngestResource

**Endpoint**: `POST /api/v1/postbox/ingest`

REST endpoint for message ingestion.

```java
@POST
@Path("/ingest")
public Response ingestMessage(@Valid PostboxPayload payload)
```

**Request Body**:
```json
{
  "id": "msg-12345",
  "tenantId": 1001,
  "partitionId": "orders",
  "type": "EVENT",
  "payload": "{\"orderId\": \"123\", \"status\": \"created\"}",
  "headers": {
    "event_type": "order.created",
    "correlation_id": "abc-123"
  }
}
```

**Response** (201 Created):
```json
{
  "id": "msg-12345",
  "created_at": "2024-01-15T10:30:00Z",
  "payload_size": 48
}
```

---

### PostboxMessage Entity

Messages are stored in PostgreSQL with the following schema:

```java
@Entity
@Table(name = "postbox_messages")
public class PostboxMessage {
    @Id
    public String id;              // Unique message ID (from client)
    public Long tenantId;          // Tenant identifier
    public String partitionId;     // Logical partition for ordering
    public MessageType type;       // EVENT or DISPATCH_JOB
    public String payload;         // JSON payload (TEXT)
    public Long payloadSize;       // Size in bytes
    public MessageStatus status;   // PENDING, PROCESSED, FAILED
    public Instant createdAt;      // Creation timestamp
    public Instant processedAt;    // Processing completion time
    public Integer retryCount;     // Number of delivery attempts
    public String errorReason;     // Last error message
    public Map<String, Object> headers;  // Optional headers (JSONB)
}
```

---

### PollerDiscovery

Automatically discovers and manages partition pollers.

**Behavior**:
1. Scans for active partitions every 5 minutes (configurable)
2. Starts new pollers for partitions with recent messages
3. Stops pollers for inactive partitions (no messages in 3 days by default)

**Discovery Query**:
```sql
SELECT DISTINCT tenant_id, partition_id
FROM postbox_messages
WHERE created_at > :cutoff
ORDER BY tenant_id, partition_id
```

---

### PartitionPoller

Polls and processes messages for a specific tenant/partition combination.

**Processing Flow**:
1. Query pending messages (batch of 100 by default)
2. For each message:
   - Route to appropriate handler based on `type`
   - Update status to PROCESSED on success
   - Increment retry count on failure
   - Mark as FAILED if max retries exceeded

**Message Ordering**:
Messages are processed in order by `payloadSize` then `createdAt`, prioritizing smaller messages.

---

### Message Handlers

#### EventMessageHandler

Handles `EVENT` type messages:
- Extracts event type from headers or payload
- Routes to FlowCatalyst event processing pipeline

#### DispatchJobMessageHandler

Handles `DISPATCH_JOB` type messages:
- Parses dispatch job details from payload
- Routes to message router for webhook delivery

## Message Types

| Type | Description | Use Case |
|------|-------------|----------|
| `EVENT` | CloudEvents-compatible event | Domain events for subscription matching |
| `DISPATCH_JOB` | Webhook dispatch job | Direct webhook delivery without subscriptions |

## Message Status Lifecycle

```
PENDING ────────► PROCESSED
    │
    │ (on failure)
    ▼
retry_count++
    │
    │ (if retry_count >= maxRetries)
    ▼
 FAILED
```

## Configuration

### Application Properties

```properties
# Poller Configuration
postbox.poller.discovery-interval-ms=300000    # 5 minutes
postbox.poller.inactive-window-days=3          # Stop polling after 3 days inactive
postbox.poller.poll-interval-ms=5000           # 5 seconds between polls
postbox.poller.batch-size=100                  # Messages per poll
postbox.poller.max-retries=5                   # Max retry attempts
postbox.poller.request-gzip=true               # Compress payloads

# Payload Limits
postbox.payload.max-size-bytes=10485760        # 10MB max payload
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `POSTBOX_POLLER_DISCOVERY_INTERVAL_MS` | Partition discovery interval | `300000` (5 min) |
| `POSTBOX_POLLER_INACTIVE_WINDOW_DAYS` | Days before poller stops | `3` |
| `POSTBOX_POLLER_POLL_INTERVAL_MS` | Poll frequency per partition | `5000` (5 sec) |
| `POSTBOX_POLLER_BATCH_SIZE` | Messages per poll | `100` |
| `POSTBOX_POLLER_MAX_RETRIES` | Max retries for 4xx errors | `5` |
| `POSTBOX_PAYLOAD_MAX_SIZE_BYTES` | Max payload size | `10485760` (10MB) |

## Database Requirements

The postbox module uses **PostgreSQL** with Hibernate ORM (unlike the rest of FlowCatalyst which uses MongoDB).

```sql
CREATE TABLE postbox_messages (
    id TEXT PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    partition_id VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    payload TEXT NOT NULL,
    payload_size BIGINT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMP,
    retry_count INTEGER DEFAULT 0,
    error_reason TEXT,
    headers JSONB
);

-- Indexes for efficient polling
CREATE INDEX idx_postbox_pending
    ON postbox_messages(tenant_id, partition_id, status, payload_size, created_at);

CREATE INDEX idx_postbox_discovery
    ON postbox_messages(created_at);
```

## Metrics

The postbox module exposes Micrometer metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `postbox.messages.ingested` | Counter | Total messages ingested |
| `postbox.messages.processed` | Counter | Successfully processed messages |
| `postbox.messages.failed` | Counter | Failed messages (max retries exceeded) |
| `postbox.pollers.active` | Gauge | Number of active partition pollers |
| `postbox.processing.time` | Timer | Message processing duration |

## Health Checks

```bash
# Postbox health check
GET /health/ready
```

The health check verifies:
- Database connectivity
- Poller scheduler running
- No critical errors in processing

## Error Handling

### Retry Logic

| HTTP Status | Behavior |
|-------------|----------|
| 2xx | Mark as PROCESSED |
| 4xx | Increment retry_count, fail if >= maxRetries |
| 5xx | Keep retrying indefinitely (transient error) |

### Dead Letter Handling

Messages that exceed `maxRetries` are marked as `FAILED` with:
- `status = FAILED`
- `errorReason` containing the last error message
- `processedAt` set to failure time

Failed messages remain in the table for manual inspection and can be reprocessed by resetting their status to `PENDING`.

## Integration Example

### Publishing Events via Postbox

```bash
# Publish an event
curl -X POST http://localhost:8080/api/v1/postbox/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "id": "evt-001",
    "tenantId": 1001,
    "partitionId": "orders",
    "type": "EVENT",
    "payload": "{\"orderId\": \"ORD-123\", \"status\": \"created\"}",
    "headers": {
      "event_type": "order.created"
    }
  }'
```

### Publishing Dispatch Jobs

```bash
# Publish a dispatch job directly
curl -X POST http://localhost:8080/api/v1/postbox/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "id": "job-001",
    "tenantId": 1001,
    "partitionId": "webhooks",
    "type": "DISPATCH_JOB",
    "payload": "{\"url\": \"https://webhook.example.com\", \"body\": {...}}"
  }'
```

## Best Practices

1. **Idempotent Message IDs** - Use deterministic IDs to prevent duplicates
2. **Partition Strategy** - Group related messages by logical entity (e.g., order ID)
3. **Payload Size** - Keep payloads small; use references for large data
4. **Error Monitoring** - Monitor `FAILED` messages and set up alerts
5. **Cleanup** - Periodically archive or delete old `PROCESSED` messages

## See Also

- [Architecture Overview](overview.md) - System design
- [Message Router](message-router.md) - Webhook delivery
- [Dispatch Jobs Guide](../guides/dispatch-jobs.md) - Dispatch workflow

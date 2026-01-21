# Message Router Architecture

The message router is FlowCatalyst's high-performance message processing engine. It consumes messages from queues, routes them to processing pools, and delivers them to downstream services with concurrency control and rate limiting.

## Overview

```
Queue (SQS/ActiveMQ/Embedded)
        │
        ▼
┌───────────────────────────────────────────────────────────────┐
│                      Message Router                           │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────────┐   │
│  │   Queue     │───►│   Queue     │───►│   Process       │   │
│  │  Consumer   │    │   Manager   │    │   Pools         │   │
│  └─────────────┘    └─────────────┘    └────────┬────────┘   │
│                                                  │            │
│                                                  ▼            │
│                                        ┌─────────────────┐   │
│                                        │  HTTP Mediator  │───┼──► Webhooks
│                                        └─────────────────┘   │
└───────────────────────────────────────────────────────────────┘
```

## Core Components

### Queue Manager

**Class**: `QueueManager`

The central orchestrator that manages queue consumers and processing pools.

**Responsibilities**:
- Fetch configuration from control endpoint on startup
- Create and manage ProcessPools
- Create and manage QueueConsumers
- Route messages from consumers to appropriate pools
- Handle configuration sync (default: every 5 minutes)
- Manage message deduplication via global pipeline map
- Coordinate ack/nack callbacks between pools and consumers

**Pool Management**:
- Maximum pools: 2000 (configurable via `message-router.max-pools`)
- Warning threshold: 1000 pools (configurable)
- Real-time monitoring via `flowcatalyst.queuemanager.pools.active` metric
- Automated leak detection every 30 seconds

**Lifecycle**:
1. **Startup**: Delayed init (2s after HTTP ready) → Fetch config → Start pools → Start consumers
2. **Sync**: Incremental update (no stop-the-world) - only affected resources change
3. **Shutdown**: Stop consumers → Drain pools → Cleanup remaining messages

---

### Queue Consumers

**Base Class**: `AbstractQueueConsumer`
**Implementations**: `SqsQueueConsumer`, `ActiveMqQueueConsumer`, `EmbeddedQueueConsumer`

**Responsibilities**:
- Poll/consume messages from queues
- Parse message bodies to `MessagePointer`
- Set MDC context for structured logging
- Route messages to QueueManager with callback
- Handle queue-specific ack/nack operations

**Configuration**:
- 1 consumer per queue
- N connections per consumer (configurable, default 1)
- Each connection runs on a virtual thread

#### SQS Consumer

- Long polling (20 seconds)
- Max 10 messages per poll
- Ack: Delete message from queue
- Nack: No-op (visibility timeout handles redelivery)
- Graceful shutdown: Completes current poll, processes batch, then exits

#### ActiveMQ Consumer

- INDIVIDUAL_ACKNOWLEDGE mode (prevents head-of-line blocking)
- Ack: `message.acknowledge()` (acknowledges only this message)
- Nack: No-op (message redelivered via RedeliveryPolicy)
- RedeliveryPolicy: 30-second delay, no exponential backoff
- Shared connection with per-thread sessions

---

### Process Pools

**Class**: `ProcessPoolImpl`

Manages concurrent message processing with rate limiting.

**Architecture**:
- One pool per pool code (e.g., "POOL-A", "POOL-B")
- BlockingQueue controls backpressure (capacity: max(concurrency × 10, 500))
- N worker threads (= concurrency setting, uses virtual threads)
- Semaphore controls concurrent processing

**Processing Flow**:
```
1. Poll message from BlockingQueue
2. Check rate limit (BEFORE semaphore - optimization)
3. If rate-limited: nack & continue (no semaphore acquired)
4. Acquire semaphore (concurrency control)
5. Process via Mediator
6. Invoke ack/nack callback
7. Release semaphore (ALWAYS in finally block)
```

**Buffer Sizing**:
- Queue capacity = max(concurrency × 10, 500)
- Examples: 5 workers → 500 buffer, 100 workers → 1000 buffer
- Rejected messages stay in SQS/ActiveMQ with visibility timeout

**Rate Limiting**:
- Pool-level rate limiting (not per-message)
- Optional `rateLimitPerMinute` per pool
- Uses Resilience4j RateLimiter
- Sliding window, 1-minute period
- Checked BEFORE semaphore to avoid wasting concurrency slots
- Messages nacked when rate limited

---

### HTTP Mediator

**Class**: `HttpMediator`

Sends messages to downstream services with fault tolerance.

**Features**:
- Java 11 HTTP/2 client with virtual thread executor
- SmallRye Fault Tolerance integration
- Retry on connection/timeout errors (max 3 attempts)
- Circuit breaker: 50% failure ratio, 10 request threshold
- Maps HTTP status codes to `MediationResult`

**Result Types**:
- `SUCCESS` - Delivery successful (2xx response)
- `ERROR_CONNECTION` - Network/connection failure
- `ERROR_SERVER` - Server error (5xx response)
- `ERROR_PROCESS` - Processing error (4xx response)

---

## Message Flow

```
Queue (SQS/ActiveMQ)
  ↓
QueueConsumer (N connections per queue)
  ↓ parse & set MDC
QueueManager.routeMessage()
  ↓ dedup check & routing
ProcessPool.submit() → BlockingQueue
  ↓ poll from queue
Check Rate Limit (pool-level, BEFORE semaphore)
  ↓ if rate-limited: nack & continue
Acquire Semaphore (concurrency control)
  ↓ record processing started
Mediator.process() → HTTP call to downstream
  ↓ handle result
QueueManager.ack/nack → Consumer callback
  ↓ cleanup (ALWAYS in finally block)
Release Semaphore & remove from pipeline map
```

## Message Schema

**MessagePointer** (parsed from queue message):

```json
{
  "id": "msg-12345",
  "poolCode": "POOL-A",
  "authToken": "Bearer xyz...",
  "mediationType": "HTTP",
  "mediationTarget": "https://api.example.com/process",
  "messageGroupId": "order-12345"
}
```

| Field | Description |
|-------|-------------|
| `id` | Unique message identifier (used for deduplication) |
| `poolCode` | Processing pool identifier (e.g., "POOL-HIGH", "order-service") |
| `authToken` | Authentication token for downstream service calls |
| `mediationType` | Type of mediation to perform (HTTP) |
| `mediationTarget` | Target endpoint URL for mediation |
| `messageGroupId` | Optional group ID for FIFO ordering within business entities |

## Configuration

### Config Endpoint Response

The message router fetches configuration from a REST endpoint:

```json
{
  "queues": [
    {"queueName": "queue-1", "queueUri": null},
    {"queueName": "queue-2", "queueUri": null}
  ],
  "connections": 1,
  "processingPools": [
    {"code": "POOL-A", "concurrency": 5, "rateLimitPerMinute": null},
    {"code": "POOL-B", "concurrency": 10, "rateLimitPerMinute": 600}
  ]
}
```

### Application Properties

```properties
# Queue type selection
message-router.queue-type=SQS  # SQS, ACTIVEMQ, or EMBEDDED

# Config endpoint
message-router.config-url=http://localhost:8080/api/config

# Sync interval
message-router.sync-interval=5m

# Pool limits
message-router.max-pools=2000
message-router.pool-warning-threshold=1000

# SQS Configuration
quarkus.sqs.aws.region=us-east-1
sqs.endpoint-override=http://localhost:4566  # LocalStack

# ActiveMQ Configuration
activemq.broker.url=tcp://localhost:61616
activemq.username=admin
activemq.password=admin
```

## Configuration Sync

**Frequency**: Every 5 minutes (configurable)

**Incremental Sync Process**:

1. Fetch new configuration from control endpoint
2. Compare pools:
   - Unchanged (same concurrency & rate limit): Keep running ✓
   - Changed (concurrency or rate limit changed): Drain and recreate
   - Removed: Drain, remove, cleanup metrics
   - New: Create and start
3. Compare queues:
   - Unchanged: Keep consumers running
   - Removed: Stop consumers
   - New: Start consumers

**Benefits**:
- Zero interruption for unchanged resources
- High availability maintained during sync
- Surgical updates only affect changed components

## Fault Tolerance

### Rate Limiting

- Pool-level rate limiting (optional per pool)
- Resilience4j RateLimiter
- Sliding window, 1-minute period
- Messages nacked when limit exceeded
- Automatic cleanup when pool removed

### Circuit Breaker

- SmallRye implementation on HTTP Mediator
- 50% failure ratio threshold
- Opens after 10 requests
- 5-second delay before half-open
- 3 successful calls to close

### Retry

- Max 3 retries on connection/timeout errors
- 1 second delay with 500ms jitter
- Only retries transient errors

### Backpressure

- BlockingQueue in ProcessPool
- Buffer size scales with concurrency
- Rejected messages rely on queue visibility timeout
- SQS/ActiveMQ acts as overflow buffer

## Deduplication

Global `ConcurrentHashMap<String, MessagePointer>` tracks in-flight messages:

1. Consumer receives message
2. QueueManager checks if `messageId` exists in map
3. If exists: return false, consumer discards
4. If not: add to map, route to pool
5. Pool removes from map in finally block (guaranteed cleanup)

**Resource Cleanup Guarantees**:
- Always in finally block - never leaks resources
- Semaphore permits always released
- Pipeline map always cleaned
- Metrics always balanced (started/finished paired)
- Automated leak detection runs every 30 seconds

## Observability

### Metrics

| Metric | Description |
|--------|-------------|
| `flowcatalyst.queuemanager.pools.active` | Active pool count |
| `flowcatalyst.queuemanager.pipeline.size` | Messages in pipeline |
| `flowcatalyst.broker.connection.attempts` | Broker connection attempts |
| `flowcatalyst.broker.connection.successes` | Successful connections |
| `flowcatalyst.broker.available` | Broker availability (gauge) |

### Health Endpoints

- `GET /health/live` - Liveness probe
- `GET /health/ready` - Readiness probe
- `GET /health/startup` - Startup probe

### Monitoring Dashboard

Real-time dashboard at `/dashboard.html`:
- Queue metrics (received, processed, failed, throughput)
- Pool metrics (concurrency, active workers, success/error rates)
- System warnings and alerts
- Circuit breaker status

### Structured Logging

MDC fields for structured JSON logs:
- `messageId` - Unique message identifier
- `poolCode` - Processing pool code
- `queueName` / `queueUrl` - Source queue
- `targetUri` - Destination endpoint
- `result` - Mediation result
- `durationMs` - Processing duration

## See Also

- [Architecture Overview](overview.md) - System design
- [Message Groups Guide](../guides/message-groups.md) - FIFO ordering
- [Queue Configuration](../guides/queue-configuration.md) - Queue setup

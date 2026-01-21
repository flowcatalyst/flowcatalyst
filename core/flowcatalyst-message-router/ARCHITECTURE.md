# FlowCatalyst Message Router - Architecture Documentation

This document provides complete architecture documentation for the FlowCatalyst Message Router, enabling reimplementation in any language without reading the source code.

## Table of Contents

1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Message Lifecycle](#message-lifecycle)
4. [Component Architecture](#component-architecture)
5. [Queue Consumer Layer](#queue-consumer-layer)
6. [Queue Manager](#queue-manager)
7. [Process Pools](#process-pools)
8. [HTTP Mediator](#http-mediator)
9. [Message Deduplication](#message-deduplication)
10. [FIFO Ordering Guarantees](#fifo-ordering-guarantees)
11. [Rate Limiting](#rate-limiting)
12. [Error Handling and Retry Logic](#error-handling-and-retry-logic)
13. [Health Monitoring](#health-monitoring)
14. [Configuration](#configuration)
15. [API Reference](#api-reference)
16. [Data Structures](#data-structures)
17. [Metrics](#metrics)
18. [Security](#security)

---

## Overview

The FlowCatalyst Message Router is a high-throughput message processing system that:

1. **Consumes messages** from queues (AWS SQS, ActiveMQ, or embedded SQLite)
2. **Routes messages** to processing pools based on pool codes
3. **Processes messages** by making HTTP POST requests to downstream endpoints
4. **Acknowledges or retries** messages based on processing results

### Key Design Goals

- **High throughput**: Processes thousands of messages per second
- **FIFO ordering**: Messages with the same `messageGroupId` are processed sequentially
- **Resilience**: Automatic retries, circuit breakers, rate limiting
- **Scalability**: Virtual threads for efficient resource usage
- **Hot standby**: Optional primary/standby deployment with Redis-based leader election

---

## Core Concepts

### Message Pointer

A `MessagePointer` is the central data structure representing a message to be processed:

```json
{
  "id": "01K97FHM11EKYSXT135MVM6AC7",
  "poolCode": "staging-client-RATE_LIMIT-120",
  "authToken": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "mediationType": "HTTP",
  "mediationTarget": "https://api.example.com/webhook",
  "messageGroupId": "order-12345"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | Unique message identifier (TSID format) |
| `poolCode` | String | Identifier for the processing pool to route to |
| `authToken` | String | Bearer token for downstream HTTP requests |
| `mediationType` | Enum | Currently only `HTTP` is supported |
| `mediationTarget` | String | URL to POST the message to |
| `messageGroupId` | String | Optional group ID for FIFO ordering |

### Processing Pools

A processing pool is a configurable worker pool that processes messages with:
- **Concurrency limit**: Maximum parallel workers
- **Rate limit**: Optional requests per minute cap
- **Queue capacity**: Bounded buffer for incoming messages

### Queue Configuration

```json
{
  "queues": [
    {
      "queueUri": "https://sqs.eu-west-1.amazonaws.com/123456789/my-queue.fifo",
      "queueName": "my-queue.fifo",
      "connections": 2
    }
  ],
  "connections": 1,
  "processingPools": [
    {
      "code": "POOL-HIGH",
      "concurrency": 10,
      "rateLimitPerMinute": 600
    }
  ]
}
```

---

## Message Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MESSAGE LIFECYCLE                                  │
└─────────────────────────────────────────────────────────────────────────────┘

  ┌──────────┐     ┌──────────────┐     ┌──────────────┐     ┌────────────┐
  │  Queue   │────►│Queue Consumer│────►│Queue Manager │────►│Process Pool│
  │(SQS/AMQ) │     │              │     │              │     │            │
  └──────────┘     └──────────────┘     └──────────────┘     └─────┬──────┘
                                                                    │
                                                                    ▼
                                                            ┌──────────────┐
                                                            │ HTTP Mediator│
                                                            └──────┬───────┘
                                                                   │
                                              ┌────────────────────┼───────────────────┐
                                              │                    │                   │
                                              ▼                    ▼                   ▼
                                        ┌──────────┐        ┌──────────┐        ┌──────────┐
                                        │  SUCCESS │        │  RETRY   │        │   DROP   │
                                        │   ACK    │        │   NACK   │        │   ACK    │
                                        └──────────┘        └──────────┘        └──────────┘
```

### Phase 1: Queue Consumption

1. Queue consumer polls for messages (SQS: up to 10 messages, long poll 20s)
2. Each message is parsed from JSON to `MessagePointer`
3. Malformed messages are acknowledged (removed) to prevent infinite retries

### Phase 2: Routing

1. QueueManager receives batch of messages
2. Deduplication check: Already-in-pipeline messages are skipped
3. Messages grouped by `poolCode`
4. Pool capacity check: If pool buffer is full, NACK all messages for that pool
5. Rate limit check: If pool is rate-limited, NACK all messages for that pool
6. Messages added to pool's internal queue

### Phase 3: Processing

1. Pool's virtual thread picks message from queue
2. Rate limiter permit acquired (if configured)
3. Semaphore permit acquired (for concurrency control)
4. HTTP POST sent to `mediationTarget` with message ID
5. Response evaluated to determine ACK or NACK

### Phase 4: Completion

Based on the HTTP response:
- **200 + `ack: true`**: ACK message (delete from queue)
- **200 + `ack: false`**: NACK message (retry after delay)
- **4xx (except 429)**: ACK message (configuration error, don't retry)
- **5xx**: NACK message (transient error, retry later)
- **Timeout/Connection error**: NACK message (transient, retry)

---

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                                 MESSAGE ROUTER                                       │
├─────────────────────────────────────────────────────────────────────────────────────┤
│                                                                                      │
│  ┌─────────────────────┐   ┌─────────────────────┐   ┌─────────────────────┐        │
│  │   SqsQueueConsumer  │   │ ActiveMqQueueConsumer│   │ EmbeddedQueueConsumer│       │
│  └──────────┬──────────┘   └──────────┬──────────┘   └──────────┬──────────┘        │
│             │                         │                          │                   │
│             └─────────────────────────┼──────────────────────────┘                   │
│                                       ▼                                              │
│                           ┌───────────────────────┐                                  │
│                           │     QueueManager      │                                  │
│                           │  - routeMessageBatch()│                                  │
│                           │  - deduplication      │                                  │
│                           │  - pool routing       │                                  │
│                           └───────────┬───────────┘                                  │
│                                       │                                              │
│        ┌──────────────────────────────┼──────────────────────────────────┐           │
│        ▼                              ▼                                  ▼           │
│  ┌──────────────┐            ┌──────────────┐                  ┌──────────────┐      │
│  │ProcessPoolImpl│            │ProcessPoolImpl│                  │ProcessPoolImpl│     │
│  │  POOL-HIGH   │            │  POOL-MEDIUM │                  │  POOL-LOW    │      │
│  │  (10 workers)│            │  (5 workers) │                  │  (2 workers) │      │
│  └──────┬───────┘            └──────┬───────┘                  └──────┬───────┘      │
│         │                           │                                 │              │
│         └───────────────────────────┼─────────────────────────────────┘              │
│                                     ▼                                                │
│                           ┌───────────────────┐                                      │
│                           │    HttpMediator   │                                      │
│                           │  - HTTP POST      │                                      │
│                           │  - Circuit Breaker│                                      │
│                           │  - Retry Logic    │                                      │
│                           └───────────────────┘                                      │
│                                                                                      │
└─────────────────────────────────────────────────────────────────────────────────────┘
```

---

## Queue Consumer Layer

### Consumer Interface

All queue consumers implement:

```
QueueConsumer
├── start()              // Begin consuming
├── stop()               // Stop consuming gracefully
├── isHealthy()          // Return true if actively polling
├── isFullyStopped()     // Return true if all threads terminated
├── getLastPollTime()    // Timestamp of last successful poll
└── getQueueIdentifier() // Queue name/URL
```

### SQS Consumer

**Polling:**
- Long poll with `waitTimeSeconds=20`
- Batch size up to `maxMessagesPerPoll=10`
- Per-request timeout of 25 seconds (20s poll + 5s buffer)

**Visibility Timeout:**
- Default: 120 seconds (configured on queue)
- Messages are invisible while being processed
- If processing takes longer, QueueManager extends visibility every 55 seconds

**ACK/NACK:**
- ACK = `DeleteMessage` API call
- NACK = No action (message becomes visible after timeout)
- Receipt handle can expire if processing takes too long
- Expired receipt handles tracked for deletion on redelivery

**Adaptive Batching:**
- Empty response: 1 second delay before next poll
- Partial batch (1-9 messages): 50ms delay
- Full batch (10 messages): No delay

### ActiveMQ Consumer

**Connection:**
- Uses JMS with `INDIVIDUAL_ACKNOWLEDGE` mode
- Single shared connection with multiple sessions
- Configurable redelivery policy (default 30s delay)

**ACK/NACK:**
- ACK = `message.acknowledge()` (only this message)
- NACK = No action + set `AMQ_SCHEDULED_DELAY` property
- Redelivery handled by broker

### Embedded Queue Consumer

**Purpose:** Local development without external dependencies

**Storage:** SQLite database with table `queue_messages`

**Schema:**
```sql
CREATE TABLE queue_messages (
    id INTEGER PRIMARY KEY,
    message_id TEXT NOT NULL,
    message_group_id TEXT,
    message_json TEXT NOT NULL,
    visible_at INTEGER NOT NULL,
    receipt_handle TEXT,
    receive_count INTEGER DEFAULT 0,
    first_received_at INTEGER
);
```

**Dequeue Algorithm:**
1. Find oldest visible message group
2. Lock the oldest message in that group (set `visible_at` to future)
3. Return message with receipt handle

---

## Queue Manager

### Responsibilities

1. **Batch Routing**: Route batches of messages to pools
2. **Deduplication**: Prevent duplicate processing of same message
3. **Pool Buffer Management**: Check capacity before routing
4. **FIFO Enforcement**: Maintain message group ordering within batches
5. **Callback Management**: Store callbacks for ACK/NACK operations
6. **Visibility Extension**: Extend SQS visibility for long-running tasks

### Deduplication Strategy

Two levels of deduplication:

1. **SQS Message ID**: Physical redelivery detection (visibility timeout expired)
   - If same SQS message ID is in pipeline, NACK the duplicate
   - Update the stored receipt handle with the new one

2. **Application Message ID**: Requeued message detection
   - If same app message ID but different SQS message ID: ACK the new one
   - This handles external processes requeueing stuck messages

### Pipeline Tracking

```
inPipelineMap:         Map<SQS_MessageId, MessagePointer>
inPipelineTimestamps:  Map<SQS_MessageId, Long>  // Start time
inPipelineQueueIds:    Map<SQS_MessageId, String> // Source queue
messageCallbacks:      Map<SQS_MessageId, MessageCallback>
appMessageIdToPipelineKey: Map<AppMessageId, SQS_MessageId>
```

### Batch Routing Algorithm

```
function routeMessageBatch(messages):
    // Phase 1: Filter duplicates
    for message in messages:
        if inPipelineMap.contains(message.sqsMessageId):
            // Visibility timeout redelivery - update receipt handle, NACK
            updateReceiptHandle(message)
            nack(message)
        else if appMessageIdToPipelineKey.contains(message.id):
            // External requeue - ACK to remove duplicate
            ack(message)
        else:
            groupByPool[message.poolCode].add(message)

    // Phase 2: Check pool capacity and rate limits
    for poolCode, poolMessages in groupByPool:
        pool = getOrCreatePool(poolCode)

        if pool.availableCapacity < poolMessages.size:
            nackAll(poolMessages)  // Pool buffer full
        else if pool.isRateLimited:
            nackAll(poolMessages)  // Rate limited
        else:
            toRoute[poolCode] = poolMessages

    // Phase 3: Route with FIFO enforcement
    for poolCode, poolMessages in toRoute:
        groupByMessageGroup = groupMessages(poolMessages)

        for groupId, groupMessages in groupByMessageGroup:
            nackRemaining = false
            for message in groupMessages:
                if nackRemaining:
                    nack(message)
                else:
                    if not pool.submit(message):
                        nack(message)
                        nackRemaining = true  // FIFO: nack all subsequent
                    else:
                        trackInPipeline(message)
```

### Visibility Extension

Every 55 seconds, QueueManager checks all in-pipeline messages:
- If processing time > 50 seconds, extend visibility by 120 seconds
- Uses `ChangeMessageVisibility` API for SQS

---

## Process Pools

### Architecture: Per-Group Virtual Threads

Each message group gets a dedicated Java virtual thread:

```
┌─────────────────────────────────────────────────────────────────┐
│                     PROCESS POOL                                │
│                                                                 │
│  Pool-level Semaphore (concurrency=10)                          │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                                                             ││
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          ││
│  │  │Group: order-│  │Group: order-│  │Group: user- │    ...   ││
│  │  │   12345     │  │   67890     │  │   99999     │          ││
│  │  ├─────────────┤  ├─────────────┤  ├─────────────┤          ││
│  │  │VirtualThread│  │VirtualThread│  │VirtualThread│          ││
│  │  │    ↓        │  │    ↓        │  │    ↓        │          ││
│  │  │[Queue]      │  │[Queue]      │  │[Queue]      │          ││
│  │  │ msg1        │  │ msg1        │  │ msg1        │          ││
│  │  │ msg2        │  │ msg2        │  │ msg2        │          ││
│  │  └─────────────┘  └─────────────┘  └─────────────┘          ││
│  │                                                             ││
│  └─────────────────────────────────────────────────────────────┘│
│                                                                 │
│  Rate Limiter (optional): X requests per minute                 │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Per-Group Threading

```
messageGroupQueues: Map<messageGroupId, BlockingQueue<MessagePointer>>
activeGroupThreads: Map<messageGroupId, Boolean>
```

When a message arrives for a new group:
1. Create `LinkedBlockingQueue` for that group
2. Start virtual thread calling `processMessageGroup(groupId, queue)`
3. Virtual thread blocks on `queue.poll(5 minutes)` when idle
4. After 5 minutes of inactivity, thread exits and group is cleaned up

### Group Thread Algorithm

```
function processMessageGroup(groupId, queue):
    while running:
        message = queue.poll(5 minutes)

        if message == null:
            // Idle timeout - cleanup
            if queue.isEmpty():
                messageGroupQueues.remove(groupId)
                activeGroupThreads.remove(groupId)
                return
            continue

        // Check batch+group FIFO failure
        batchGroupKey = message.batchId + "|" + groupId
        if failedBatchGroups.contains(batchGroupKey):
            nack(message)  // Previous message in batch failed
            continue

        // Rate limit check (before semaphore)
        if rateLimiter != null and not rateLimiter.acquirePermission():
            setFastFailVisibility(message, 10s)
            nack(message)
            continue

        // Acquire concurrency permit
        semaphore.acquire()

        try:
            outcome = mediator.process(message)
            handleOutcome(message, outcome)
        finally:
            semaphore.release()
```

### Concurrency Control

- Pool-level semaphore with `concurrency` permits
- Each message must acquire permit before processing
- Rate limiting checked BEFORE acquiring semaphore (prevents slot waste)

### Buffer Sizing

```
queueCapacity = max(concurrency × 2, 50)
```

Examples:
- 5 workers → 50 buffer
- 100 workers → 200 buffer
- 200 workers → 400 buffer

### In-Place Configuration Updates

Pools support live reconfiguration without draining:

**Concurrency Increase:**
```
semaphore.release(newLimit - currentLimit)
```

**Concurrency Decrease:**
```
if semaphore.tryAcquire(currentLimit - newLimit, timeout):
    concurrency = newLimit
else:
    // Timeout - keep current limit
```

**Rate Limit Update:**
```
rateLimiter = RateLimiter.of(poolCode, newConfig)
```

---

## HTTP Mediator

### Request Format

```http
POST {mediationTarget}
Authorization: Bearer {authToken}
Content-Type: application/json
Accept: application/json

{
  "messageId": "01K97FHM11EKYSXT135MVM6AC7"
}
```

### Response Format

```json
{
  "ack": true,
  "message": "Processed successfully",
  "delaySeconds": null
}
```

| Field | Type | Description |
|-------|------|-------------|
| `ack` | Boolean | `true` = message processed, `false` = retry later |
| `message` | String | Optional description/reason |
| `delaySeconds` | Integer | Optional delay before retry (1-43200 seconds) |

### Response Handling

| HTTP Status | `ack` Value | Action | Retry |
|-------------|-------------|--------|-------|
| 200 | true | ACK | No |
| 200 | false | NACK with delay | Yes |
| 200 | (parse error) | ACK | No (backward compat) |
| 400 | - | ACK + warning | No (config error) |
| 401, 403 | - | ACK + warning | No (auth error) |
| 404 | - | ACK + warning | No (endpoint missing) |
| 429 | - | NACK | Yes (rate limited) |
| 500-599 | - | NACK | Yes (server error) |
| Timeout | - | NACK | Yes (3 retries first) |
| Connection Error | - | NACK | Yes (3 retries first) |

### Retry Logic

For transient errors (timeout, connection error):
1. Retry up to 3 times with backoff: 1s, 2s, 3s
2. If all retries fail, NACK message for queue visibility timeout retry

### Circuit Breaker

Configuration:
```
requestVolumeThreshold = 10
failureRatio = 0.5 (50%)
delay = 5000ms (5 seconds)
successThreshold = 3
failOn = [HttpTimeoutException, IOException]
```

States:
- **CLOSED**: Normal operation
- **OPEN**: Requests fail fast (5s duration)
- **HALF_OPEN**: Test requests to check recovery

### Timeout Configuration

Default: 900,000ms (15 minutes) for long-running endpoint operations.

---

## Message Deduplication

### Problem: Multiple Processing

Without deduplication, messages can be processed multiple times when:
1. SQS visibility timeout expires before processing completes
2. External process requeues a stuck message
3. Network issues cause duplicate deliveries

### Solution: Two-Layer Deduplication

**Layer 1: SQS Message ID (Physical Deduplication)**
```
inPipelineMap[sqsMessageId] → MessagePointer
```
- Same SQS message ID = visibility timeout redelivery
- Action: NACK the duplicate, update receipt handle on original

**Layer 2: Application Message ID (Logical Deduplication)**
```
appMessageIdToPipelineKey[appMessageId] → sqsMessageId
```
- Same app ID, different SQS ID = external requeue
- Action: ACK the new message (original still processing)

### Receipt Handle Update

When SQS redelivers a message (same SQS ID), the receipt handle changes. The router updates the stored callback's receipt handle so the ACK uses the valid handle.

---

## FIFO Ordering Guarantees

### Guarantee Level

**FIFO within message group + batch:**
- Messages with same `messageGroupId` in the same batch process sequentially
- Messages with same `messageGroupId` across batches may interleave

### Batch+Group FIFO Enforcement

When a message fails in a batch:
1. Mark `batchId|messageGroupId` as failed
2. All subsequent messages in that batch+group are automatically NACKed
3. This prevents out-of-order processing

```
failedBatchGroups: Set<"batchId|messageGroupId">
batchGroupMessageCount: Map<"batchId|messageGroupId", AtomicInteger>
```

### Cleanup

When all messages in a batch+group are processed:
```
if batchGroupMessageCount[key].decrementAndGet() == 0:
    batchGroupMessageCount.remove(key)
    failedBatchGroups.remove(key)
```

---

## Rate Limiting

### Per-Pool Rate Limiting

Each pool can have an optional rate limit (requests per minute):

```
rateLimiter = RateLimiter.of("pool-" + poolCode, config)
config.limitRefreshPeriod = 1 minute
config.limitForPeriod = rateLimitPerMinute
config.timeoutDuration = 0 (fail immediately)
```

### Rate Limit Behavior

1. Rate limit checked BEFORE acquiring concurrency semaphore
2. If rate limited:
   - Set 10-second visibility timeout (fast retry)
   - NACK message
   - Don't waste concurrency slot

### Rate Limit Status Check

```
isRateLimited = rateLimiter.getMetrics().getAvailablePermissions() <= 0
```

---

## Error Handling and Retry Logic

### Error Categories

| Category | Examples | Action |
|----------|----------|--------|
| **Config Error** | 400, 401, 403, 404, 501 | ACK (don't retry) |
| **Transient Error** | 5xx, timeout, connection | NACK (retry) |
| **Processing Error** | `ack: false` | NACK with custom delay |
| **Parse Error** | Invalid JSON response | ACK (treat as success) |

### Visibility Timeout Control

For different scenarios:

| Scenario | Visibility Timeout |
|----------|-------------------|
| Rate limited | 10 seconds (fast retry) |
| Batch+group failed | 10 seconds |
| Default retry | 30 seconds |
| Custom delay (from response) | 1-43200 seconds |
| Processing still running | Extended by 120 seconds |

### Graceful Shutdown

1. Stop all queue consumers (stop polling)
2. Wait up to 25 seconds for consumers to finish current polls
3. Set pools to draining mode (stop accepting new work)
4. Wait up to 60 seconds for pools to finish processing
5. NACK any remaining messages in pipeline
6. Shutdown executor services

---

## Health Monitoring

### Infrastructure Health (`/health`)

Returns HTTP 200 if infrastructure is operational, 503 if compromised.

**Checks:**
1. Message router is enabled
2. At least one process pool exists
3. Pools are not stalled (activity within 2 minutes)

**NOT checked:** Downstream service failures, circuit breaker states

### Consumer Health

Each consumer tracks:
- `lastPollTime`: Timestamp of last successful poll
- Unhealthy if no poll in 60 seconds

QueueManager automatically restarts unhealthy consumers.

### Queue Metrics

Polled periodically from SQS/ActiveMQ:
- `ApproximateNumberOfMessages`: Pending in queue
- `ApproximateNumberOfMessagesNotVisible`: Currently being processed

### Pool Metrics

Tracked in real-time:
- `activeWorkers`: Currently processing (concurrency - available permits)
- `queueSize`: Messages waiting in buffer
- `totalProcessed`, `totalSucceeded`, `totalFailed`
- `averageProcessingTimeMs`

---

## Configuration

### Application Properties

```properties
# Core Settings
message-router.enabled=true
message-router.queue-type=SQS  # SQS, ACTIVEMQ, or EMBEDDED
message-router.sync-interval=5m
message-router.max-pools=2000
message-router.pool-warning-threshold=1000

# SQS Settings
message-router.sqs.max-messages-per-poll=10
message-router.sqs.wait-time-seconds=20

# ActiveMQ Settings
activemq.broker.url=tcp://localhost:61616
activemq.username=admin
activemq.password=admin
message-router.activemq.receive-timeout-ms=1000

# Embedded Queue Settings
message-router.embedded.visibility-timeout-seconds=30
message-router.embedded.receive-timeout-ms=1000

# HTTP Mediator Settings
mediator.http.version=HTTP_2  # or HTTP_1_1
mediator.http.timeout.ms=900000  # 15 minutes

# Metrics
message-router.metrics.poll-interval-seconds=300

# Hot Standby (Optional)
standby.enabled=false
standby.instance-id=${HOSTNAME:instance-1}
standby.lock-key=message-router-primary-lock
standby.lock-ttl-seconds=30
```

### Environment Variables

```bash
# Queue Type
MESSAGE_ROUTER_QUEUE_TYPE=SQS

# SQS Configuration
AWS_REGION=eu-west-1
SQS_ENDPOINT_OVERRIDE=  # For LocalStack

# ActiveMQ Configuration
ACTIVEMQ_BROKER_URL=tcp://localhost:61616
ACTIVEMQ_USERNAME=admin
ACTIVEMQ_PASSWORD=admin

# Configuration Client
MESSAGE_ROUTER_CONFIG_URL=http://localhost:8080/api/config

# Redis for Hot Standby
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_USERNAME=
REDIS_PASSWORD=

# Authentication
AUTHENTICATION_ENABLED=false
AUTHENTICATION_MODE=NONE  # NONE, BASIC, or OIDC
AUTH_BASIC_USERNAME=admin
AUTH_BASIC_PASSWORD=secret
```

### Configuration Client

The router fetches its configuration from an external endpoint:

```
GET {MESSAGE_ROUTER_CONFIG_URL}

Response:
{
  "queues": [...],
  "connections": 1,
  "processingPools": [...]
}
```

Configuration is synced every 5 minutes (configurable).

---

## API Reference

### Health Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Infrastructure health check (for load balancers) |
| `GET /health/live` | Kubernetes liveness probe |
| `GET /health/ready` | Kubernetes readiness probe |

### Monitoring Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /monitoring/health` | Detailed system health status |
| `GET /monitoring/queue-stats` | Statistics for all queues |
| `GET /monitoring/pool-stats` | Statistics for all pools |
| `GET /monitoring/warnings` | All system warnings |
| `GET /monitoring/warnings/unacknowledged` | Unacknowledged warnings |
| `POST /monitoring/warnings/{id}/acknowledge` | Acknowledge a warning |
| `DELETE /monitoring/warnings` | Clear all warnings |
| `GET /monitoring/circuit-breakers` | Circuit breaker states |
| `POST /monitoring/circuit-breakers/{name}/reset` | Reset circuit breaker |
| `GET /monitoring/in-flight-messages` | Messages currently processing |
| `GET /monitoring/standby-status` | Hot standby status |
| `GET /monitoring/dashboard` | HTML dashboard UI |

### Message Seeding (Development)

| Endpoint | Description |
|----------|-------------|
| `POST /api/seed/message` | Seed a test message |

---

## Data Structures

### Queue Statistics

```json
{
  "name": "flow-catalyst-high-priority.fifo",
  "totalMessages": 150000,
  "totalConsumed": 149950,
  "totalFailed": 50,
  "successRate": 0.9996666666666667,
  "currentSize": 500,
  "throughput": 25.5,
  "pendingMessages": 500,
  "messagesNotVisible": 10
}
```

### Pool Statistics

```json
{
  "poolCode": "POOL-HIGH",
  "totalProcessed": 128647,
  "totalSucceeded": 128637,
  "totalFailed": 10,
  "totalRateLimited": 0,
  "successRate": 0.9999222679114165,
  "activeWorkers": 10,
  "availablePermits": 0,
  "maxConcurrency": 10,
  "queueSize": 500,
  "maxQueueCapacity": 500,
  "averageProcessingTimeMs": 103.8,
  "messageGroupCount": 45
}
```

### Warning

```json
{
  "id": "abc123",
  "type": "CONFIGURATION",
  "severity": "ERROR",
  "message": "Endpoint not found for message 01K...: HTTP 404",
  "source": "HttpMediator",
  "timestamp": "2024-01-18T10:30:45Z",
  "acknowledged": false
}
```

### Circuit Breaker Stats

```json
{
  "name": "http-mediator",
  "state": "CLOSED",
  "successfulCalls": 10000,
  "failedCalls": 5,
  "rejectedCalls": 0,
  "failureRate": 0.0005,
  "bufferedCalls": 100,
  "bufferSize": 100
}
```

---

## Metrics

### Micrometer Gauges

| Metric | Description |
|--------|-------------|
| `flowcatalyst.queuemanager.pipeline.size` | Messages currently in pipeline |
| `flowcatalyst.queuemanager.callbacks.size` | Callbacks waiting for completion |
| `flowcatalyst.queuemanager.pools.active` | Number of active pools |
| `flowcatalyst.queuemanager.defaultpool.usage` | Counter for default pool fallback |

### Pool Metrics

| Metric | Description |
|--------|-------------|
| `pool.{code}.active_workers` | Currently processing messages |
| `pool.{code}.available_permits` | Available concurrency slots |
| `pool.{code}.queue_size` | Messages waiting in buffer |
| `pool.{code}.message_groups` | Active message groups |
| `pool.{code}.processed` | Total messages processed |
| `pool.{code}.succeeded` | Successful messages |
| `pool.{code}.failed` | Failed messages |
| `pool.{code}.rate_limited` | Rate-limited messages |
| `pool.{code}.processing_time_ms` | Average processing time |

### Queue Metrics

| Metric | Description |
|--------|-------------|
| `queue.{name}.pending` | Messages in queue |
| `queue.{name}.not_visible` | Messages being processed |
| `queue.{name}.received` | Total received |
| `queue.{name}.processed` | Total processed |

---

## Security

### Authentication Modes

1. **NONE**: No authentication (default)
2. **BASIC**: Username/password via HTTP Basic Auth
3. **OIDC**: OpenID Connect (Keycloak, Auth0, etc.)

### Protected Endpoints

When authentication is enabled:
- `/monitoring/*` - Requires authentication
- `/api/*` - Requires authentication

### Always Public

- `/health/*` - Kubernetes probes
- `/q/health/*` - Quarkus health endpoints

### Basic Auth Usage

```bash
curl -H "Authorization: Basic $(echo -n 'admin:password' | base64)" \
  http://localhost:8080/monitoring/health
```

### OIDC Usage

```bash
TOKEN=$(curl -X POST https://keycloak/token -d "grant_type=client_credentials" | jq -r .access_token)
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/monitoring/health
```

---

## Deployment Considerations

### Resource Sizing

| Pools | Workers/Pool | Recommended Memory | CPU |
|-------|--------------|-------------------|-----|
| 50 | 10 | 1GB | 2 cores |
| 200 | 20 | 2GB | 4 cores |
| 500 | 50 | 4GB | 8 cores |
| 1000+ | 100 | 8GB | 16 cores |

### Scaling Strategy

1. **Vertical**: Increase `max-pools` and instance size
2. **Horizontal**: Use hot standby for HA, not parallel processing

### Queue Recommendations

**SQS FIFO Queues:**
- Enable deduplication (5-minute window)
- Set visibility timeout to 120s (longer than typical processing)
- Configure DLQ with maxReceiveCount=5

**ActiveMQ:**
- Enable persistence
- Configure DLQ policies
- Use `INDIVIDUAL_ACKNOWLEDGE` mode

---

## Related Documentation

- [Authentication Guide](AUTHENTICATION.md)
- [Debugging Guide](DEBUGGING_GUIDE.md)
- [Native Build Guide](NATIVE_BUILD_GUIDE.md)
- [Hot Standby Mode](STANDBY.md)

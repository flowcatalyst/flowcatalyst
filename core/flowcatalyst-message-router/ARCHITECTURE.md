# FlowCatalyst Message Router — Architecture & Rules

This document describes the complete rules, flows, and architecture of the Java message router as implemented in source code. It covers every scenario, branch, and decision point across the full processing pipeline.

---

## Table of Contents

1. [Overview](#overview)
2. [Core Data Structures](#core-data-structures)
3. [Startup Sequence](#startup-sequence)
4. [Configuration Sync](#configuration-sync)
5. [Queue Consumer Layer](#queue-consumer-layer)
6. [Routing Algorithm](#routing-algorithm)
7. [In-Flight Message Tracking](#in-flight-message-tracking)
8. [Process Pools](#process-pools)
9. [Per-Group Virtual Threads](#per-group-virtual-threads)
10. [Rate Limiting](#rate-limiting)
11. [FIFO Enforcement (Batch+Group)](#fifo-enforcement-batchgroup)
12. [HTTP Mediator](#http-mediator)
13. [Outcome Handling](#outcome-handling)
14. [Visibility Timeout Control](#visibility-timeout-control)
15. [Health Monitoring & Consumer Restart](#health-monitoring--consumer-restart)
16. [Leak Detection](#leak-detection)
17. [Graceful Shutdown](#graceful-shutdown)
18. [Scheduled Tasks Summary](#scheduled-tasks-summary)
19. [Warning Types](#warning-types)
20. [Configuration Reference](#configuration-reference)

---

## Overview

The message router pulls messages from queues (SQS, ActiveMQ, NATS, or embedded SQLite), routes them to processing pools, and delivers them via HTTP POST to downstream endpoints. The core guarantees are:

- **At-least-once delivery** via queue visibility timeouts
- **FIFO within a message group and batch** — messages with the same `messageGroupId` in the same batch process in arrival order
- **No concurrent processing of the same message** — in-flight tracking prevents duplicate processing
- **Automatic retries** for transient errors via queue visibility timeout
- **Permanent drop** for configuration errors (4xx) to prevent infinite retry loops

---

## Core Data Structures

### MessagePointer

The central message object passed through the entire pipeline:

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | Application message ID (TSID format) |
| `poolCode` | String | Pool to route to; missing pool falls back to `DEFAULT-POOL` |
| `authToken` | String | Bearer token sent in HTTP `Authorization` header |
| `mediationType` | MediationType | Always `HTTP` in current implementation |
| `mediationTarget` | String | URL to POST to |
| `messageGroupId` | String | Group ID for FIFO ordering; null/blank uses `__DEFAULT__` |
| `highPriority` | boolean | Routes to high-priority tier queue within pool |
| `batchId` | String | UUID assigned when entering routing; used for batch+group FIFO tracking |
| `sqsMessageId` | String | Broker-level message ID; used as pipeline key for deduplication |

### MediationOutcome

Returned by the mediator for every processed message:

| Field | Type | Description |
|-------|------|-------------|
| `result` | MediationResult | `SUCCESS`, `ERROR_PROCESS`, `ERROR_CONFIG`, `ERROR_CONNECTION` |
| `delaySeconds` | Integer | Custom retry delay (1–43200s); null uses default 30s |
| `error` | MediationError | Typed error: `Timeout`, `CircuitOpen`, `HttpError`, `NetworkError`, `RateLimited` |

`getEffectiveDelaySeconds()` returns `delaySeconds` clamped to `[1, 43200]`, or `30` if null/0.

---

## Startup Sequence

1. Quarkus starts and CDI injects all dependencies into `QueueManager`.
2. `onStartup()` fires (`@Observes StartupEvent`): logs virtual thread configuration and spawns a test virtual thread to verify the JVM is configured correctly.
3. `scheduledSync()` fires after a **2-second delay** (`delay = 2, delayUnit = SECONDS`). This is the initial configuration sync.
4. If the standby service is present and this instance is **not primary**, `scheduledSync()` sets `initialized = true` and returns immediately. The instance acts as a hot standby and does not start processing.
5. If `message-router.enabled = false`, `scheduledSync()` sets `initialized = true` and returns.
6. Otherwise, `syncConfiguration(isInitialSync=true)` runs. See [Configuration Sync](#configuration-sync).
7. On success, `initialized = true` and processing begins.
8. On failure (all retries exhausted), `Quarkus.asyncExit(1)` is called — the process exits.

---

## Configuration Sync

Configuration is fetched from an external HTTP endpoint (`MESSAGE_ROUTER_CONFIG_URL`) and applied on startup and periodically thereafter.

### Retry Logic

- **12 attempts**, 5 seconds apart (1 minute total)
- On each failure, logs a warning with attempt number
- If **initial sync** fails all retries:
  - Adds a `CONFIG_SYNC_FAILED` **CRITICAL** warning
  - Calls `Quarkus.asyncExit(1)` — application exits
- If a **subsequent sync** fails:
  - Adds a `CONFIG_SYNC_FAILED` **WARN** warning
  - Continues processing with the existing (last successfully synced) configuration

### What Gets Applied

**Pools:**

1. For each pool in the existing `processPools` map:
   - If the pool is **no longer in new config** → call `pool.drain()`, move to `drainingPools`, remove from `processPools`. The draining pool is cleaned up asynchronously by `cleanupDrainingResources()`.
   - If the pool **still exists** in new config but config changed:
     - If concurrency changed → `pool.updateConcurrency(newConcurrency, timeoutSeconds=60)`
     - If rate limit changed → `pool.updateRateLimit(newRateLimitPerMinute)`

2. For each pool in the new config that does **not yet exist**:
   - Check `processPools.size() >= maxPools` (default 10000): if true → add `POOL_LIMIT` **CRITICAL** warning, skip this pool
   - Check `processPools.size() >= poolWarningThreshold` (default 5000): if true → add `POOL_LIMIT` **WARNING**
   - Calculate `queueCapacity = max(effectiveConcurrency × 20, 50)`
   - Create `ProcessPoolImpl`, call `pool.start()`, add to `processPools`

**Queue consumers:**

1. For each existing consumer whose queue is **no longer in new config**: call `consumer.stop()`, move to `drainingConsumers`, remove from `queueConsumers`.

2. Validate all queues via `queueValidationService.validateQueues()` — issues are logged as warnings but processing continues regardless.

3. For each queue in new config that has **no running consumer**: create via `queueConsumerFactory.createConsumer(queueConfig, connections)` (connections default: 1), call `consumer.start()`.

4. Existing consumers for unchanged queues are left running untouched.

### Sync Lock

`syncConfiguration()` acquires a `ReentrantLock syncLock` (not `synchronized`) to prevent concurrent execution while still being safe on virtual threads.

---

## Queue Consumer Layer

All consumers share a common contract:

- `start()` — begin polling
- `stop()` — signal stop, begin graceful shutdown
- `isHealthy()` — returns true if `lastPollTime` is within 60 seconds (or never polled yet)
- `isFullyStopped()` — returns true when all polling threads have exited
- `getLastPollTime()` — timestamp of last successful poll

### SQS Consumer

**Polling:**
- `waitTimeSeconds = 20` (long poll)
- `maxMessages` = configured value (typically 10)
- Per-request API call timeout: **25 seconds** (`apiCallTimeout`)
- One polling thread (virtual) per configured **connection**

**Adaptive delays after each poll:**
- Empty batch (0 messages): no sleep — the long poll already waited up to 20 seconds
- Partial batch (1 to maxMessages-1): sleep **100ms**
- Full batch (maxMessages messages): no sleep

**Message parsing:**
Each SQS message body is JSON-parsed to extract `MessagePointer` fields. If parsing fails, the message is immediately **ACKed** (deleted from the queue) to prevent infinite retry of malformed messages.

**Deferred deletes (`pendingDeleteSqsMessageIds`):**
When ACKing a message fails because the receipt handle has expired (SQS error: `ReceiptHandleIsInvalid` or `receipt handle has expired`), the SQS message ID is added to `pendingDeleteSqsMessageIds`. On the next delivery of the same SQS message ID, the message is deleted immediately before processing.

**Receipt handle expiry:**
SQS receipt handles expire if processing takes longer than the queue's visibility timeout. The router stores the latest receipt handle and updates it when SQS redelivers a message (visibility timeout redelivery). This ensures ACKs use a valid handle.

**ACK/NACK:**
- ACK = `DeleteMessage` API call
- NACK = `ChangeMessageVisibility` API call to set custom timeout
- Default NACK visibility: **30 seconds**
- Fast-fail NACK visibility: **10 seconds** (for rate-limited, batch-group-failed messages)
- Custom delay NACK: 1–43200 seconds (clamped to SQS limits)

### ActiveMQ Consumer

- JMS `INDIVIDUAL_ACKNOWLEDGE` mode — only the specific message is acknowledged
- ACK = `message.acknowledge()`
- NACK = no action + sets `AMQ_SCHEDULED_DELAY` property for redelivery delay
- Configurable redelivery policy (default 30s delay)

### NATS Consumer

- Uses NATS JetStream for durable, at-least-once delivery
- ACK = `message.ack()`
- NACK = `message.nak()`

### Embedded Queue Consumer

For local development only. Uses an SQLite database (table `queue_messages`). Implements the same `QueueConsumer` interface.

---

## Routing Algorithm

`QueueManager.routeMessageBatch()` is called by queue consumers with a list of `BatchMessage` records. Each `BatchMessage` contains the `MessagePointer`, `MessageCallback`, `queueIdentifier`, and `sqsMessageId`.

A **snapshot** of `processPools` is taken at the start (`Map.copyOf(processPools)`) to ensure consistent pool references throughout the entire batch, even if `syncConfiguration()` runs concurrently.

A **UUID `batchId`** is generated for this batch.

### Phase 1: Deduplication

For each message in the batch, derive the pipeline key:
```
pipelineKey = sqsMessageId != null ? sqsMessageId : appMessageId
```

**Check 1 — Physical redelivery (same broker message ID):**
If `inFlightTracker.containsKey(sqsMessageId)` is true:
- The same SQS message was redelivered because the visibility timeout expired while the original is still processing.
- Attempt to update the stored callback's receipt handle with the new one via `updateReceiptHandleIfPossible()`.
- Add to `duplicates` list.

**Check 2 — Logical requeue (same app ID, different broker ID):**
If `inFlightTracker.isInFlight(appMessageId)` is true AND the existing pipeline key differs from the new SQS message ID:
- An external process requeued this message (e.g., a stuck-message cleanup job).
- Add to `requeuedDuplicates` list.

**Check 2 edge case:** If the app message ID is in-flight but the broker ID is the same (or null) — treat as physical redelivery (duplicates).

**After checks:**
- All `duplicates` → **NACK** (let SQS retry after visibility timeout)
- All `requeuedDuplicates` → **ACK** (permanently remove the new duplicate; original is still processing)
- All other messages → grouped by `poolCode` in `messagesByPool`

### Phase 2: Pool Capacity Check

For each pool in `messagesByPool`:

1. Look up pool in the snapshot. If not found:
   - Log warning + increment `defaultPoolUsageCounter` metric
   - Add `ROUTING` **WARN** warning
   - Route to `DEFAULT-POOL` (created on demand with concurrency=20 if it doesn't exist)

2. Calculate `availableCapacity = pool.getQueueCapacity() - pool.getQueueSize()`

3. If `availableCapacity < poolMessages.size()` (pool cannot accept ALL messages for this pool from this batch):
   - Add `QUEUE_FULL` **WARN** warning
   - Add ALL messages for this pool to `toNackPoolFull`

4. Otherwise, add pool's messages to `messagesToRoute`.

After checking all pools:
- All `toNackPoolFull` messages → **NACK**
- Note: **Rate limiting is not checked here** — it is handled inside the pool worker as a blocking wait.

### Phase 3: FIFO Routing with Sequential Nacking

For each pool in `messagesToRoute`, group messages by `messageGroupId` (null/blank → `"__DEFAULT__"`). The grouping uses `LinkedHashMap` to preserve insertion order.

For each group, iterate messages in order:

```
nackRemaining = false
for each message in group:
    if nackRemaining:
        NACK message
        continue

    enrich message with batchId, sqsMessageId
    trackResult = inFlightTracker.track(enrichedMessage, callback, queueIdentifier)

    if trackResult is Duplicate:
        NACK, continue  (unexpected — was checked in Phase 1)

    pipelineKey = trackResult.pipelineKey
    inPipelineMap.put(pipelineKey, enrichedMessage)  // legacy compatibility

    submitted = pool.submit(enrichedMessage)
    if not submitted:
        inFlightTracker.remove(pipelineKey)
        inPipelineMap.remove(pipelineKey)
        NACK
        nackRemaining = true  // FIFO: nack all subsequent in this group
```

This ensures that if message N in a batch+group fails to be submitted to the pool, all messages N+1, N+2, ... in the same batch+group are also NACKed. This preserves ordering — when the messages are redelivered from the queue, they will arrive in the same order.

---

## In-Flight Message Tracking

`InFlightMessageTracker` consolidates five previously separate maps into a single consistent unit with `ReentrantReadWriteLock`.

### Internal State

- `trackedMessages: ConcurrentHashMap<String, TrackedMessage>` — pipelineKey → full tracking record
- `appMessageIdIndex: ConcurrentHashMap<String, String>` — appMessageId → pipelineKey (secondary index for requeue detection)

### Pipeline Key Rule

```
pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id()
```

### TrackedMessage Fields

| Field | Description |
|-------|-------------|
| `pipelineKey` | The key used to track this message |
| `messageId` | Application message ID |
| `brokerMessageId` | SQS/AMQP message ID |
| `queueId` | Source queue identifier |
| `message` | The full MessagePointer |
| `callback` | Ack/nack callback |
| `trackedAt` | When tracking started |

### track() — Adding a Message

Acquires **write lock**, then:

1. If `trackedMessages.containsKey(pipelineKey)` → return `Duplicate(pipelineKey, isRequeue=false)`
2. If `appMessageIdIndex.get(appMessageId)` returns an existing key:
   - If that key is still in `trackedMessages` → return `Duplicate(existingKey, isRequeue=true)`
   - Else (stale index entry) → remove from index, proceed
3. Add to both maps, return `Tracked(pipelineKey)`

### remove() — Removing a Message

Acquires **write lock**, removes from both maps atomically.

### Stale index cleanup

During duplicate detection (step 2), stale entries in `appMessageIdIndex` (where the message was already acked/nacked) are cleaned up automatically.

---

## Process Pools

Each pool (`ProcessPoolImpl`) manages a set of virtual threads that process messages concurrently.

### Key State

| Field | Description |
|-------|-------------|
| `poolCode` | Unique identifier |
| `concurrency` | Maximum concurrent workers (volatile for visibility) |
| `queueCapacity` | `max(concurrency × 20, 50)` — total messages across all group queues |
| `semaphore` | Pool-level `Semaphore(concurrency)` — controls total concurrent processing |
| `rateLimiter` | Resilience4j `RateLimiter` (volatile for atomic replacement); null if not configured |
| `highPriorityGroupQueues` | `ConcurrentHashMap<String, BlockingQueue<MessagePointer>>` |
| `regularGroupQueues` | `ConcurrentHashMap<String, BlockingQueue<MessagePointer>>` |
| `activeGroupThreads` | `ConcurrentHashMap<String, Boolean>` — tracks live virtual threads per group |
| `totalQueuedMessages` | `AtomicInteger` — total across all group queues; compared against `queueCapacity` |
| `failedBatchGroups` | `ConcurrentHashMap<String, Boolean>` — key: `"batchId|groupId"` |
| `batchGroupMessageCount` | `ConcurrentHashMap<String, AtomicInteger>` — count of remaining messages per batch+group |
| `running` | `AtomicBoolean` |
| `configLock` | `ReentrantLock` — protects concurrent configuration updates |

### submit()

Called by `QueueManager.routeMessageBatch()` to submit a message to the pool.

```
if !running → return false (pool is draining)

groupId = message.messageGroupId() == null/blank ? "__DEFAULT__" : message.messageGroupId()

track in batchGroupMessageCount (increment count for batchGroupKey)

targetQueues = message.highPriority() ? highPriorityGroupQueues : regularGroupQueues
groupQueue = targetQueues.computeIfAbsent(groupId, k -> new LinkedBlockingQueue<>())  // unbounded

// Thread start check (atomic)
startedThread = activeGroupThreads.putIfAbsent(groupId, TRUE) == null
if startedThread:
    check if either queue has messages (= thread died and left orphans)
    if orphaned messages → log warning + add GROUP_THREAD_RESTART warning
    executorService.submit(() -> processMessageGroup(groupId))

// Capacity check (atomic CAS loop with Thread.yield())
while true:
    current = totalQueuedMessages.get()
    if current >= queueCapacity:
        clean up batchGroupMessageCount
        return false
    if totalQueuedMessages.compareAndSet(current, current + 1): break
    Thread.onSpinWait(); Thread.yield()

groupQueue.offer(message)  // always succeeds (unbounded queue)
return true
```

### Concurrency Update (`updateConcurrency`)

Protected by `configLock`.

- **Increase**: `semaphore.release(diff)`, `concurrency = newLimit` — immediate
- **Decrease**: `semaphore.tryAcquire(diff, timeoutSeconds=60, SECONDS)`:
  - If acquired: `concurrency = newLimit`
  - If timeout (workers still active): log warning, return false (retain current limit)

### Rate Limit Update (`updateRateLimit`)

Protected by `configLock`. Creates a brand-new `RateLimiter` instance and atomically assigns it to the volatile field. Any worker in `waitForRateLimitPermit()` picks up the new limiter on its next iteration.

To disable rate limiting: set `rateLimiter = null`.

---

## Per-Group Virtual Threads

Each message group gets exactly one dedicated Java virtual thread. Groups are created on demand and cleaned up after idle.

### Thread Lifecycle

```
Message arrives for group "order-12345":
  → activeGroupThreads.putIfAbsent("order-12345", TRUE) returns null → start thread
  → processMessageGroup("order-12345") runs in virtual thread

While thread is running:
  → activeGroupThreads contains "order-12345" = TRUE

Thread exits (idle timeout or interrupted):
  → finally block: activeGroupThreads.remove("order-12345")
```

### `processMessageGroup()` Loop

```
while running:
    highQueue = highPriorityGroupQueues.get(groupId)
    regularQueue = regularGroupQueues.get(groupId)

    // Step 1: Non-blocking poll from high priority queue
    message = highQueue?.poll()  // non-blocking

    // Step 2: If no high-priority message, block on regular queue
    if message == null:
        if regularQueue != null:
            message = regularQueue.poll(5, MINUTES)
        else if highQueue != null:
            message = highQueue.poll(5, MINUTES)
        else:
            Thread.sleep(100); continue

    // Step 3: Idle timeout handling
    if message == null:
        if both queues empty:
            remove both queues from maps
            activeGroupThreads.remove(groupId)  // clean exit
            return
        continue  // queue became non-empty, keep processing

    // Step 4: Got a message
    totalQueuedMessages.decrementAndGet()
    setMDCContext(message)

    // Step 5: Check batch+group FIFO failure
    batchGroupKey = batchId + "|" + effectiveGroupId
    if failedBatchGroups.containsKey(batchGroupKey):
        setFastFailVisibility(message)  // 10s
        nackSafely(message)
        decrementAndCleanupBatchGroup(batchGroupKey)
        continue

    // Step 6: Wait for rate limit permit (blocking)
    waitForRateLimitPermit()

    // Step 7: Acquire pool concurrency permit
    semaphore.acquire()  // blocks until a slot is available

    // Step 8: Process
    outcome = mediator.process(message)

    // Step 9: Handle outcome
    handleMediationOutcome(message, outcome, durationMs)

    // finally: performCleanup() — semaphore.release(), MDC.clear()
```

### Thread Death and Restart

If a virtual thread exits abnormally (not via idle timeout):
- `finally` block removes the group from `activeGroupThreads`
- If the pool is still `running` and messages remain in the queues: adds a `GROUP_THREAD_EXIT_WITH_MESSAGES` **WARN** warning
- On the **next** `submit()` for that group: `putIfAbsent` returns null → detects orphaned messages → adds `GROUP_THREAD_RESTART` **WARN** warning → starts new thread

---

## Rate Limiting

### Pool-Level Rate Limiter

Each pool can have an optional `rateLimitPerMinute`. The Resilience4j `RateLimiter` is configured with:
- `limitRefreshPeriod = 1 minute`
- `limitForPeriod = rateLimitPerMinute`
- `timeoutDuration = Duration.ZERO` — `acquirePermission()` returns false immediately if no permits

### `isRateLimited()` (checked by QueueManager — no longer used for routing)

```java
return rateLimiter != null && rateLimiter.getMetrics().getAvailablePermissions() <= 0
```

Note: As of current code, QueueManager no longer checks `isRateLimited()` before routing. Rate limiting happens entirely inside the pool worker.

### `waitForRateLimitPermit()`

Called inside `processMessageGroup()` after the batch+group check but **before** acquiring the semaphore. This is a **blocking loop** — virtual threads make this cheap (no OS thread is blocked):

```
while running:
    limiter = this.rateLimiter  // read volatile fresh each iteration
    if limiter == null: return  // rate limiting disabled

    if limiter.acquirePermission(): return  // got permit

    record rate limit metric (once per wait period)
    Thread.sleep(100ms)  // wait and retry
```

This means:
- If the rate limiter is **removed** (via `updateRateLimit(null)`) while a worker is waiting, it exits on the next 100ms check.
- If the rate limit is **increased** (new limiter instance), the worker picks up the new limiter on the next iteration.
- Messages **stay in memory** (in the virtual thread) while rate-limited rather than being NACKed back to SQS.

---

## FIFO Enforcement (Batch+Group)

### The Problem

Messages with the same `messageGroupId` in a batch must process in order. If message N fails (NACK), messages N+1, N+2, ... should not be processed before N is retried.

### Two Levels of Enforcement

**Level 1 — At routing time (`routeMessageBatch`):**
If `pool.submit()` returns false for message N in a group (pool full at that exact moment), set `nackRemaining = true` and NACK all subsequent messages in that group within the batch. This is immediate — the messages never enter the pool.

**Level 2 — Inside the pool (`processMessageGroup`):**
If the mediator returns `ERROR_PROCESS` or `ERROR_CONNECTION` for message N:
- Mark `"batchId|groupId"` in `failedBatchGroups`
- When the next message in the same batch+group is dequeued by the group's virtual thread, it checks `failedBatchGroups` and NACKs immediately with 10-second visibility

### Cleanup

```
decrementAndCleanupBatchGroup(batchGroupKey):
    count = batchGroupMessageCount.get(key).decrementAndGet()
    if count <= 0:
        batchGroupMessageCount.remove(key)
        failedBatchGroups.remove(key)
```

Called on every outcome (success, config error, process error, connection error) and on every batch-group-failure NACK. Once all messages in a batch+group have been processed or NACKed, the tracking entries are cleaned up.

---

## HTTP Mediator

### Circuit Breaker

```java
@CircuitBreaker(
    requestVolumeThreshold = 10,
    failureRatio = 0.5,        // 50% failure rate
    delay = 5000,              // 5s in OPEN state
    successThreshold = 3,      // 3 successes to close from HALF_OPEN
    failOn = {HttpTimeoutException.class, IOException.class}
)
@CircuitBreakerName("http-mediator")
```

A single circuit breaker named `"http-mediator"` protects all outbound HTTP calls. When open, calls to `mediator.process()` fail immediately (not retried inside the retry loop).

### Retry Loop

`process()` runs up to **3 attempts** with backoff **1s, 2s, 3s**:

```
for attempt = 1 to 3:
    outcome = attemptProcess(message)
    if SUCCESS or ERROR_CONFIG: return immediately (no retry)
    if outcome.error.isRetryable() and attempt < 3:
        sleep(1000 * attempt)
        continue
    return outcome
```

**Retryable errors:** `Timeout`, `NetworkError`, `HttpError(5xx)`
**Non-retryable errors:** `CircuitOpen`, `HttpError(4xx)`

### Request Format

```http
POST {mediationTarget}
Authorization: Bearer {authToken}
Content-Type: application/json
Accept: application/json

{"messageId": "{message.id()}"}
```

**Connect timeout:** 30 seconds (hardcoded on `HttpClient`)
**Request timeout:** configurable via `mediator.http.timeout.ms` (default 900,000ms = 15 minutes)

### Response Handling

| HTTP Status | Condition | Outcome | Retry? | Warning |
|-------------|-----------|---------|--------|---------|
| 200 | `ack=true` | `SUCCESS` | No | — |
| 200 | `ack=false`, delay provided | `ERROR_PROCESS(delaySeconds)` | Yes | — |
| 200 | `ack=false`, no delay | `ERROR_PROCESS(null)` | Yes | — |
| 200 | JSON parse error | `SUCCESS` | No (backward compat) | — |
| 400 | — | `ERROR_CONFIG` | No | CONFIGURATION/ERROR |
| 401–499 (except 404, 429) | — | `ERROR_CONFIG` | No | CONFIGURATION/ERROR |
| 404 | — | `ERROR_CONFIG` | No | CONFIGURATION/ERROR |
| 429 | `Retry-After` header present (integer) | `ERROR_PROCESS(retryAfterSeconds)` | Yes | — |
| 429 | No `Retry-After` header | `ERROR_PROCESS(30)` | Yes | — |
| 501 | — | `ERROR_CONFIG` | No | CONFIGURATION/**CRITICAL** |
| 5xx (not 501) | — | `ERROR_PROCESS` | Yes (retryable) | — |
| Unexpected (not 4xx/5xx) | — | `ERROR_PROCESS` | Yes | — |
| `HttpConnectTimeoutException` | — | `ERROR_CONNECTION` | Yes (retryable) | — |
| `ConnectException` | — | `ERROR_CONNECTION` | Yes (retryable) | — |
| `UnresolvedAddressException` | — | `ERROR_CONNECTION` | Yes (retryable) | — |
| `HttpTimeoutException` | — | `ERROR_PROCESS(Timeout)` | Yes (retryable) | — |
| `IOException` (general) | — | `ERROR_CONNECTION` | Yes (retryable) | — |
| Any other exception | — | `ERROR_PROCESS(NetworkError)` | Yes (retryable) | — |

**`Retry-After` parsing:** Only integer (delta-seconds) format is supported. HTTP-date format is not parsed — falls back to the default 30s.

---

## Outcome Handling

After `mediator.process()` returns, `handleMediationOutcome()` in `ProcessPoolImpl` dispatches:

### SUCCESS

- `messageCallback.ack(message)` — triggers `QueueManager.ack()` → removes from tracker → calls SQS `DeleteMessage`
- `poolMetrics.recordProcessingSuccess()`
- `decrementAndCleanupBatchGroup()`

### ERROR_CONFIG

- `messageCallback.ack(message)` — **ACKs** (deletes from queue) to prevent infinite retry
- `poolMetrics.recordProcessingFailure()`
- `decrementAndCleanupBatchGroup()`
- Note: `HttpMediator` has already added the warning; no additional warning is added here

### ERROR_PROCESS

- Check `outcome.hasCustomDelay()`:
  - True → `visibilityControl.setVisibilityDelay(message, delaySeconds)` — custom delay
  - False → `visibilityControl.resetVisibilityToDefault(message)` — 30s
- `messageCallback.nack(message)` — message becomes visible after the set delay
- `poolMetrics.recordProcessingTransient()` (not counted as failure)
- `failedBatchGroups.putIfAbsent(batchGroupKey, TRUE)` — marks batch+group as failed
- `decrementAndCleanupBatchGroup()`

### ERROR_CONNECTION

- `visibilityControl.resetVisibilityToDefault(message)` — 30s
- `messageCallback.nack(message)`
- `poolMetrics.recordProcessingFailure()`
- `failedBatchGroups.putIfAbsent(batchGroupKey, TRUE)` — marks batch+group as failed
- `decrementAndCleanupBatchGroup()`

### null / unknown result

- Adds `MEDIATOR_NULL_RESULT` **CRITICAL** warning
- Treated as `ERROR_PROCESS(null)` — same flow as ERROR_PROCESS without custom delay

---

## Visibility Timeout Control

`SqsMessageCallback` implements `MessageVisibilityControl` with these methods:

| Method | Visibility Timeout | When Used |
|--------|--------------------|-----------|
| `nack()` | 30 seconds (default) | Generic NACK |
| `resetVisibilityToDefault(message)` | 30 seconds | ERROR_PROCESS (no custom delay), ERROR_CONNECTION |
| `setFastFailVisibility(message)` | 10 seconds | Batch+group FIFO failure — faster retry |
| `setVisibilityDelay(message, seconds)` | 1–43200 seconds (clamped) | ERROR_PROCESS with custom delay (429 with Retry-After, `ack=false` with delaySeconds) |

All visibility changes use `ChangeMessageVisibility` API. Errors are caught and logged but do not affect the NACK itself.

---

## Health Monitoring & Consumer Restart

### Consumer Health Check — every 60 seconds

`monitorAndRestartUnhealthyConsumers()` iterates all active consumers:

For each consumer:
- `consumer.isHealthy()` checks: `running` is true AND (`lastPollTime == 0` OR `System.currentTimeMillis() - lastPollTime < 60_000`)
- If **unhealthy**:
  1. Add `CONSUMER_RESTART` **WARN** warning
  2. `consumer.stop()` — signal stop
  3. Move to `drainingConsumers`, remove from `queueConsumers`
  4. Look up `queueConfigs.get(queueIdentifier)` — if not found, log error and skip
  5. `queueConsumerFactory.createConsumer(queueConfig, connections)` — creates a brand new consumer
  6. `newConsumer.start()`
  7. Add to `queueConsumers`
  8. If any step throws: add `CONSUMER_RESTART_FAILED` **CRITICAL** warning

### Draining Resource Cleanup — every 10 seconds

`cleanupDrainingResources()` checks `drainingPools` and `drainingConsumers`:

- Pool: `pool.isFullyDrained()` = `semaphore.availablePermits() == concurrency` → if true: `pool.shutdown()`, remove from `drainingPools`, remove pool metrics
- Consumer: `consumer.isFullyStopped()` = `!running && pollingTasks.length == 0` → if true: remove from `drainingConsumers`

---

## Leak Detection

`checkForMapLeaks()` runs every **30 seconds**:

```
pipelineSize = inFlightTracker.size()

totalCapacity = sum(pool.getConcurrency() * 20 for all active pools)
totalCapacity = max(totalCapacity, 50)

if pipelineSize > totalCapacity:
    add PIPELINE_MAP_LEAK WARN warning
    log: "LEAK DETECTION: in-flight tracker size (N) > total capacity (M)"
```

A consistently growing `inFlightTracker` (beyond what active pools can hold) indicates messages are not being removed from tracking after ACK/NACK.

---

## Graceful Shutdown

`onShutdown()` fires on `@Observes ShutdownEvent`:

### Step 1: Pause Scheduled Tasks

`shutdownInProgress = true` — all `@Scheduled` methods check this and return immediately. Wait 500ms for any in-progress scheduled tasks to exit.

### Step 2: Stop All Consumers (`stopAllConsumers`)

1. For each consumer: `consumer.stop()` (signals stop, returns immediately), move to `drainingConsumers`
2. Clear `queueConsumers`
3. Poll `drainingConsumers` every 500ms until all `isFullyStopped()` or **25-second timeout**
4. After timeout: force-clear `drainingConsumers` (warn if any remain)

### Step 3: Drain All Pools (`drainAllPools`)

1. For each pool: `pool.drain()` (see below), move to `drainingPools`
2. Clear `processPools`
3. Poll `drainingPools` every 500ms until all `isFullyDrained()` or **30-second timeout**
4. After timeout: force `pool.shutdown()` on remaining pools

**`pool.drain()` behavior:**
- `running.set(false)` — stop accepting new messages
- `clearAllQueuedMessages()` — immediately clears ALL messages from both priority tier queues, clears `activeGroupThreads` and batch tracking maps. Total discarded count is logged.
- `executorService.shutdownNow()` — interrupts all virtual threads blocked on `queue.poll()`
- Threads that have acquired the semaphore (actively calling HTTP) are **not** interrupted — they complete their current request
- `isFullyDrained()` = `semaphore.availablePermits() == concurrency` — true only when all active HTTP calls have finished

### Step 4: Cleanup Remaining Messages (`cleanupRemainingMessages`)

Iterates `inFlightTracker.clear()` and NACKs every remaining tracked message. This returns any still-tracked messages to their queues via visibility timeout. Errors during NACK are counted and reported as a `SHUTDOWN_CLEANUP_ERRORS` **WARN** warning.

---

## Scheduled Tasks Summary

| Task | Schedule | Notes |
|------|----------|-------|
| `scheduledSync()` | Every 5 min (configurable), initial delay 2s | Config sync, standby check |
| `monitorAndRestartUnhealthyConsumers()` | Every 60s | Restarts stalled consumers |
| `cleanupDrainingResources()` | Every 10s | Removes fully-drained pools and stopped consumers |
| `checkForMapLeaks()` | Every 30s | Detects in-flight tracker growth |

All scheduled tasks check `!initialized || shutdownInProgress` and return immediately if either is true (except `scheduledSync` which checks `shutdownInProgress` only, since it drives initialization).

All scheduled tasks run on virtual threads (`@RunOnVirtualThread`).

---

## Warning Types

Warnings are stored in `WarningService` (in-memory) and auto-expire after **8 hours**.

| Warning Code | Severity | Source | Meaning |
|---|---|---|---|
| `CONFIG_SYNC_FAILED` | CRITICAL | QueueManager | Initial sync failed after all retries — app will exit |
| `CONFIG_SYNC_FAILED` | WARN | QueueManager | Periodic sync failed — continuing with old config |
| `ROUTING` | WARN | QueueManager | No pool found for code, routed to DEFAULT-POOL |
| `QUEUE_FULL` | WARN | QueueManager | Pool buffer full — batch NACKed |
| `POOL_LIMIT` | CRITICAL | QueueManager | max-pools limit reached — pool not created |
| `POOL_LIMIT` | WARNING | QueueManager | Pool count approaching limit |
| `PIPELINE_MAP_LEAK` | WARN | QueueManager | In-flight tracker exceeds total pool capacity |
| `CONSUMER_RESTART` | WARN | QueueManager | Consumer restarted due to health check failure |
| `CONSUMER_RESTART_FAILED` | CRITICAL | QueueManager | Consumer restart threw an exception |
| `SHUTDOWN_CLEANUP_ERRORS` | WARN | QueueManager | Errors NACKing messages during shutdown |
| `GROUP_THREAD_RESTART` | WARN | ProcessPool | Virtual thread for group died and was restarted |
| `GROUP_THREAD_EXIT_WITH_MESSAGES` | WARN | ProcessPool | Thread exited with messages still in its queue |
| `SEMAPHORE_RELEASE_FAILED` | CRITICAL | ProcessPool | Failed to release semaphore (should never happen) |
| `MEDIATOR_NULL_RESULT` | CRITICAL | ProcessPool | Mediator returned null outcome |
| `PROCESSING` | WARN | ProcessPool | Unexpected exception during message processing |
| `CONFIGURATION` | CRITICAL | HttpMediator | HTTP 501 from endpoint |
| `CONFIGURATION` | ERROR | HttpMediator | HTTP 400, 404, or 401–499 from endpoint |

---

## Configuration Reference

### Core Properties

| Property | Default | Description |
|----------|---------|-------------|
| `message-router.enabled` | `true` | Master enable switch |
| `message-router.sync-interval` | `5m` | How often to sync config from platform |
| `message-router.max-pools` | `10000` | Hard cap on process pool count |
| `message-router.pool-warning-threshold` | `5000` | Pool count that triggers a warning |

### SQS

| Property | Default | Description |
|----------|---------|-------------|
| `message-router.sqs.max-messages-per-poll` | 10 | Batch size per poll |
| `message-router.sqs.wait-time-seconds` | 20 | Long-poll duration |
| `SQS_ENDPOINT_OVERRIDE` | (none) | Override for LocalStack/Elasticmq |

### HTTP Mediator

| Property | Default | Description |
|----------|---------|-------------|
| `mediator.http.timeout.ms` | `900000` | Per-request timeout (15 min) |
| `mediator.http.version` | `HTTP_2` | `HTTP_2` or `HTTP_1_1` |

### Internal Constants (source code)

| Constant | Value | Meaning |
|----------|-------|---------|
| `QUEUE_CAPACITY_MULTIPLIER` | `20` | `queueCapacity = max(concurrency × 20, 50)` |
| `MIN_QUEUE_CAPACITY` | `50` | Floor on pool buffer size |
| `DEFAULT_POOL_CODE` | `"DEFAULT-POOL"` | Fallback pool name |
| `DEFAULT_POOL_CONCURRENCY` | `20` | Default pool concurrency |
| `IDLE_TIMEOUT_MINUTES` | `5` | Group thread idle cleanup time |
| `DEFAULT_GROUP` | `"__DEFAULT__"` | Group ID for messages without messageGroupId |
| Connect timeout | `30s` | Hardcoded on `HttpClient` |
| Rate limit wait poll interval | `100ms` | Sleep between rate limit checks |
| Gauge update interval | `500ms` | How often pool metrics gauges refresh |
| NACK default visibility | `30s` | Default retry delay |
| Fast-fail visibility | `10s` | Batch+group failure retry delay |
| Max delay (SQS limit) | `43200s` | Maximum custom visibility timeout |
| Shutdown consumer wait | `25s` | Max wait for consumers to stop |
| Shutdown pool drain wait | `30s` | Max wait for active workers to finish |
| Consumer health timeout | `60s` | Consumer is unhealthy if no poll in this window |
| Warning auto-expire | `8h` | Warnings are removed after this duration |

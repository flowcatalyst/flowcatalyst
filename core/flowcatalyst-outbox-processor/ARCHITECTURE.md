# FlowCatalyst Outbox Processor - Architecture Documentation

This document provides complete architecture documentation for the FlowCatalyst Outbox Processor module, enabling reimplementation in any language without reading the source code.

## Table of Contents

1. [Overview](#overview)
2. [The Outbox Pattern](#the-outbox-pattern)
3. [Data Model](#data-model)
4. [Database Operations](#database-operations)
5. [Threading & Concurrency Model](#threading--concurrency-model)
6. [API Integration](#api-integration)
7. [Configuration](#configuration)
8. [Database Schema](#database-schema)
9. [Execution Flow](#execution-flow)
10. [Implementation Guide](#implementation-guide)

---

## Overview

The **flowcatalyst-outbox-processor** implements the **Outbox Pattern** for reliable message publishing. It polls customer databases for pending outbox items and sends them to FlowCatalyst APIs.

### Supported Databases

- PostgreSQL
- MySQL
- MongoDB

### Key Features

- **Atomic fetch-and-lock** for crash safety
- **FIFO ordering** within message groups
- **Concurrent group processing** with configurable limits
- **Automatic retry** with max attempts
- **Crash recovery** for stuck items

---

## The Outbox Pattern

### Problem Statement

When a service makes a database change AND needs to publish a message:
1. Write to database
2. Publish to message queue/API

If the service crashes between steps 1 and 2, the message is lost.

### Solution

1. Write business data AND message to the **same database** in a **single transaction**
2. A separate outbox processor reads pending messages and sends them externally
3. If the processor crashes, it recovers and retries

### FlowCatalyst Implementation

```
┌──────────────────────────────────────────────────────────────────┐
│                     CUSTOMER APPLICATION                          │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  BEGIN TRANSACTION                                                │
│    INSERT INTO orders (...)                                       │
│    INSERT INTO outbox_events (id, type, payload, status='PENDING')│
│  COMMIT                                                           │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────┐
│                   OUTBOX PROCESSOR                                │
├──────────────────────────────────────────────────────────────────┤
│                                                                   │
│  1. Poll PENDING items from customer's outbox table               │
│  2. Atomically mark as PROCESSING                                 │
│  3. Send to FlowCatalyst API                                      │
│  4. Mark as COMPLETED or schedule retry                           │
│                                                                   │
└──────────────────────────────────────────────────────────────────┘
                                   │
                                   ▼
┌──────────────────────────────────────────────────────────────────┐
│                   FLOWCATALYST API                                │
│  POST /api/events/batch                                           │
│  POST /api/dispatch/jobs/batch                                    │
└──────────────────────────────────────────────────────────────────┘
```

---

## Data Model

### OutboxItem

| Field | Type | Description |
|-------|------|-------------|
| `id` | String | TSID format (13-char Crockford Base32) |
| `type` | OutboxItemType | `EVENT` or `DISPATCH_JOB` |
| `messageGroup` | String | For FIFO ordering, nullable |
| `payload` | String | JSON payload to send |
| `status` | OutboxStatus | Current status |
| `retryCount` | Integer | Number of retry attempts |
| `createdAt` | Instant | Creation timestamp |
| `processedAt` | Instant | When locked for processing |
| `errorMessage` | String | Error message if FAILED |

### OutboxStatus Enum

| Status | Description |
|--------|-------------|
| `PENDING` | Waiting to be processed |
| `PROCESSING` | Locked, being sent to API |
| `COMPLETED` | Successfully delivered |
| `FAILED` | Max retries exceeded, final failure |

### Status Lifecycle

```
┌─────────┐     ┌──────────────┐     ┌───────────┐
│ PENDING │────▶│ PROCESSING   │────▶│ COMPLETED │
└─────────┘     └──────────────┘     └───────────┘
      ▲                │
      │            (error)
      │                │
      └────────────────┼──────────────────┐
                       │                  │
                       ▼                  ▼
                  ┌─────────────┐    ┌──────────┐
                  │  PENDING    │    │  FAILED  │
                  │  (retry)    │    │(max      │
                  └─────────────┘    │retries)  │
                                     └──────────┘
```

### OutboxItemType Enum

| Type | API Endpoint |
|------|-------------|
| `EVENT` | `/api/events/batch` |
| `DISPATCH_JOB` | `/api/dispatch/jobs/batch` |

---

## Database Operations

### OutboxRepository Interface

All implementations must provide these atomic operations:

#### fetchAndLockPending(type, limit) → List\<OutboxItem\>

**Purpose**: Atomically select pending items AND mark them as PROCESSING.

**Critical**: MUST be atomic to prevent multiple processors fetching same items.

**Ordering**: Items ordered by `(messageGroup, createdAt)` for FIFO within groups.

#### PostgreSQL Implementation

```sql
WITH selected AS (
    SELECT id FROM {table}
    WHERE status = 'PENDING'
    ORDER BY message_group, created_at
    LIMIT ?
    FOR UPDATE SKIP LOCKED
)
UPDATE {table} t
SET status = 'PROCESSING', processed_at = NOW()
FROM selected s
WHERE t.id = s.id
RETURNING t.*;
```

- Uses `FOR UPDATE SKIP LOCKED` for non-blocking concurrent access
- CTE + UPDATE...FROM pattern for atomic select-and-update
- `SKIP LOCKED` allows multiple processors without blocking

#### MySQL Implementation

```sql
-- Within same transaction:
1. SELECT id FROM {table}
   WHERE status='PENDING'
   FOR UPDATE SKIP LOCKED
   LIMIT ?

2. UPDATE {table}
   SET status='PROCESSING', processed_at=NOW()
   WHERE id IN (?)

3. SELECT * FROM {table} WHERE id IN (...)
```

- MySQL doesn't support UPDATE...FROM with RETURNING
- Three steps within single transaction for atomicity

#### MongoDB Implementation

```javascript
// Loop up to 'limit' times:
db.collection.findOneAndUpdate(
    { status: 'PENDING' },
    { $set: { status: 'PROCESSING', processedAt: new Date() } },
    {
        sort: { messageGroup: 1, createdAt: 1 },
        returnDocument: 'after'
    }
)
```

- `findOneAndUpdate` is atomic in MongoDB
- Looped to fetch multiple items

### Other Repository Methods

| Method | Description |
|--------|-------------|
| `markCompleted(type, ids)` | Update to COMPLETED, set processedAt |
| `markFailed(type, ids, errorMessage)` | Mark as FAILED with error |
| `scheduleRetry(type, ids)` | Increment retryCount, reset to PENDING |
| `recoverStuckItems(type, timeoutSeconds)` | Reset stuck PROCESSING items to PENDING |

---

## Threading & Concurrency Model

### Architecture Overview

```
┌─────────────────┐
│  OutboxPoller   │ ─── Runs every 1s (configurable)
│   (Scheduled)   │
└────────┬────────┘
         │
         │ fetchAndLockPending()
         ▼
┌─────────────────┐
│  GlobalBuffer   │ ─── ArrayBlockingQueue (capacity: 1000)
│  (Backpressure) │
└────────┬────────┘
         │
         │ distribute()
         ▼
┌─────────────────────────────────────────┐
│          GroupDistributor               │
│  ┌─────────────────────────────────┐    │
│  │ ConcurrentHashMap of Processors │    │
│  │                                 │    │
│  │  EVENT:order-123 → Processor    │    │
│  │  EVENT:order-456 → Processor    │    │
│  │  DISPATCH_JOB:customer-789 →... │    │
│  └─────────────────────────────────┘    │
│                                         │
│  Semaphore: max 10 concurrent groups    │
└─────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────┐
│       MessageGroupProcessor             │
│  (One per type:messageGroup)            │
│                                         │
│  - LinkedBlockingQueue of items         │
│  - Virtual thread for processing        │
│  - Acquires semaphore before API call   │
│  - Batch sends to FlowCatalyst API      │
└─────────────────────────────────────────┘
```

### OutboxPoller

**Purpose**: Main polling loop.

| Property | Description |
|----------|-------------|
| Schedule | Every 1 second (configurable) |
| Execution | SKIP overlapping invocations |
| Primary Only | Via StandbyService leader election |

**Polling Logic**:
```
1. Check if enabled and is primary
2. Acquire atomic lock (prevent overlapping)
3. Poll EVENTS table → add to GlobalBuffer
4. Poll DISPATCH_JOBS table → add to GlobalBuffer
5. Release lock
```

**Crash Recovery**:
- Separate scheduled method runs every 1 minute
- Calls `recoverStuckItems()` for items in PROCESSING > 5 minutes

### GlobalBuffer

**Purpose**: In-memory queue between poller and processors.

| Property | Description |
|----------|-------------|
| Implementation | `ArrayBlockingQueue` |
| Capacity | Configurable (default 1000) |
| Backpressure | Rejected items stay in database |

**Algorithm**:
```
1. Start virtual thread running distributor loop
2. Poll queue every 100ms
3. Call GroupDistributor.distribute() for each item
4. On shutdown, drain remaining items
```

### GroupDistributor

**Purpose**: Route items to appropriate MessageGroupProcessor.

**Components**:
- `ConcurrentHashMap<groupKey, MessageGroupProcessor>`
- `Semaphore` for limiting concurrent groups

**Routing**:
```
groupKey = type + ":" + (messageGroup ?? "default")
processor = processors.computeIfAbsent(groupKey, createProcessor)
processor.enqueue(item)
```

### MessageGroupProcessor

**Purpose**: Process items from single message group in FIFO order.

**Key Design**:
- One virtual thread per processor
- LinkedBlockingQueue for pending items
- Acquires global semaphore before API call
- Batch sends to minimize API calls

**Processing Loop**:
```
while queue not empty:
    semaphore.acquire()
    try:
        batch = queue.drainTo(list, batchSize)
        result = apiClient.sendBatch(batch)
        if success:
            repository.markCompleted(batch.ids)
        else:
            handleFailure(batch)
    finally:
        semaphore.release()
```

**Failure Handling**:
```
for item in failedBatch:
    if item.retryCount < maxRetries:
        repository.scheduleRetry(item.id)
    else:
        repository.markFailed(item.id, errorMessage)
```

---

## API Integration

### FlowCatalystApiClient

| Property | Value |
|----------|-------|
| HTTP Client | Java 21+ HttpClient with virtual threads |
| Connection Timeout | 10 seconds |
| Request Timeout | 30 seconds |
| Authentication | Bearer token (optional) |

### Endpoints

| Type | Endpoint |
|------|----------|
| `EVENT` | `POST /api/events/batch` |
| `DISPATCH_JOB` | `POST /api/dispatch/jobs/batch` |

### Request Format

```json
[
    {
        // Parsed from OutboxItem.payload
        "id": "0HZXEQ5Y8JY5Z",
        "type": "order.created",
        ...
    },
    ...
]
```

### Error Handling

- Status code >= 400 throws `ApiException`
- Triggers failure handling in MessageGroupProcessor
- Items can be retried unless max retries exceeded

---

## Configuration

All configs prefixed with `outbox-processor.`:

| Config Key | Type | Default | Description |
|------------|------|---------|-------------|
| `enabled` | boolean | true | Enable/disable processor |
| `poll-interval` | Duration | 1s | Polling frequency |
| `poll-batch-size` | int | 500 | Max items per poll |
| `api-batch-size` | int | 100 | Max items per API call |
| `max-concurrent-groups` | int | 10 | Max parallel groups |
| `global-buffer-size` | int | 1000 | Buffer queue capacity |
| `database-type` | enum | POSTGRESQL | Database backend |
| `events-table` | string | outbox_events | Events table name |
| `dispatch-jobs-table` | string | outbox_dispatch_jobs | Jobs table name |
| `api-base-url` | string | (required) | FlowCatalyst API URL |
| `api-token` | string | (optional) | Bearer token |
| `max-retries` | int | 3 | Max retry attempts |
| `processing-timeout-seconds` | int | 300 | Stuck item threshold |
| `mongo-database` | string | outbox | MongoDB database name |

---

## Database Schema

### PostgreSQL / MySQL

```sql
CREATE TABLE outbox_events (
    id VARCHAR(13) PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    message_group VARCHAR(255),
    payload TEXT NOT NULL,
    status VARCHAR(20) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    error_message TEXT
);

CREATE INDEX idx_outbox_events_status
    ON outbox_events(status, message_group, created_at);

CREATE TABLE outbox_dispatch_jobs (
    id VARCHAR(13) PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    message_group VARCHAR(255),
    payload TEXT NOT NULL,
    status VARCHAR(20) NOT NULL,
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,
    error_message TEXT
);

CREATE INDEX idx_outbox_dispatch_jobs_status
    ON outbox_dispatch_jobs(status, message_group, created_at);
```

### MongoDB

```javascript
// Collections: outbox_events, outbox_dispatch_jobs

db.outbox_events.createIndex(
    { status: 1, messageGroup: 1, createdAt: 1 }
);

db.outbox_dispatch_jobs.createIndex(
    { status: 1, messageGroup: 1, createdAt: 1 }
);

// Document structure:
{
    "_id": "0HZXEQ5Y8JY5Z",
    "type": "EVENT",
    "messageGroup": "order-123",
    "payload": "{...}",
    "status": "PENDING",
    "retryCount": 0,
    "createdAt": ISODate("2025-01-15T10:30:00Z"),
    "processedAt": null,
    "errorMessage": null
}
```

---

## Execution Flow

```
┌──────────────────┐
│  OutboxPoller    │ (runs every 1s)
└────────┬─────────┘
         │
         ├─ fetchAndLockPending(EVENT, 500)
         │  └─ Items 1-500 now PROCESSING
         │
         ├─ fetchAndLockPending(DISPATCH_JOB, 500)
         │  └─ Items 501-1000 now PROCESSING
         │
         └─ globalBuffer.addAll(all items)
            │
            ├─ Add to ArrayBlockingQueue
            └─ Return rejected count if full
               │
               └─▶ GlobalBuffer.runDistributor()
                  │
                  └─ For each item:
                     │
                     └─ GroupDistributor.distribute(item)
                        │
                        ├─ Find/Create MessageGroupProcessor
                        │
                        └─ processor.enqueue(item)
                           │
                           └─ If not running:
                              startVirtualThread(processLoop)
                              │
                              ├─ semaphore.acquire()
                              │
                              ├─ drainTo(batch, 100)
                              │
                              ├─ POST /api/events/batch
                              │  or POST /api/dispatch/jobs/batch
                              │
                              ├─ markCompleted(ids)
                              │
                              └─ On failure:
                                 ├─ scheduleRetry() if < maxRetries
                                 └─ markFailed() if >= maxRetries

[Separate thread - every 1 minute]
OutboxPoller.recoverStuckItems()
└─ Find PROCESSING items older than 5 minutes
   └─ Reset to PENDING for reprocessing
```

---

## Implementation Guide

### Core Components

1. **Data Model**: OutboxItem, OutboxStatus, OutboxItemType
2. **Repository**: Database-agnostic interface with 3 implementations
3. **OutboxPoller**: Scheduled polling with crash recovery
4. **GlobalBuffer**: Backpressure buffer
5. **GroupDistributor**: Routes items to group processors
6. **MessageGroupProcessor**: Per-group FIFO processing
7. **FlowCatalystApiClient**: HTTP batch endpoint caller

### Key Implementation Details

#### Atomic Fetch-and-Lock

**Critical**: The `fetchAndLockPending` operation MUST be atomic.

- PostgreSQL: CTE with `FOR UPDATE SKIP LOCKED`
- MySQL: Transaction with `FOR UPDATE SKIP LOCKED`
- MongoDB: `findOneAndUpdate` (naturally atomic)

#### FIFO Ordering

Items must be ordered by `(messageGroup, createdAt)`:
- Maintains ordering within groups
- Only one batch per group processed at a time

#### Backpressure

If GlobalBuffer is full:
- Items remain in PROCESSING status in database
- Crash recovery picks them up after timeout
- No lost messages, just delayed

#### Concurrency Limiting

Semaphore-based limiting:
- Max N groups processing simultaneously
- Prevents resource exhaustion
- Each group acquires semaphore before API call

### Design Principles

1. **Crash Safety**: Items only move to PROCESSING when locked atomically
2. **FIFO**: Ordered by messageGroup + createdAt
3. **Concurrency**: Multiple groups processed in parallel
4. **Backpressure**: Buffer prevents unbounded memory growth
5. **Multi-Database**: Single codebase, runtime selection

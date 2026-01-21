# FlowCatalyst Dispatch Scheduler - Architecture Documentation

This document provides complete architecture documentation for the FlowCatalyst Dispatch Scheduler module, enabling reimplementation in any language without reading the source code.

## Table of Contents

1. [Overview](#overview)
2. [Job Scheduling and Dispatch Flow](#job-scheduling-and-dispatch-flow)
3. [Message Group Concept](#message-group-concept)
4. [Dispatch Modes](#dispatch-modes)
5. [Component Architecture](#component-architecture)
6. [Concurrency Model](#concurrency-model)
7. [Stale Job Recovery](#stale-job-recovery)
8. [Configuration](#configuration)
9. [Integration Points](#integration-points)
10. [Implementation Guide](#implementation-guide)

---

## Overview

The **flowcatalyst-dispatch-scheduler** is a job scheduling and dispatch service that converts pending dispatch jobs from the database into messages queued for external processing.

### Primary Role

Bridge between the data store (jobs in PENDING status) and the external queue system (delivering jobs for processing via the message router).

### Key Features

- **Periodic polling** for pending jobs
- **FIFO ordering** within message groups
- **Error-based flow control** (block-on-error, next-on-error)
- **Rate limiting** via dispatch pools
- **Crash recovery** for stale queued jobs
- **Hot standby support** (only primary instance runs)

---

## Job Scheduling and Dispatch Flow

### Overall Pipeline

```
Database (PENDING jobs)
    ↓
PendingJobPoller (periodic polling every 5s)
    ↓
Group jobs by messageGroup
    ↓
Filter by DispatchMode rules (check for ERROR jobs)
    ↓
MessageGroupDispatcher (in-memory queues per group)
    ↓
MessageGroupQueue (enforces 1 job in-flight per group)
    ↓
JobDispatcher (sends to external queue)
    ↓
Update status to QUEUED
```

### Flow Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    DISPATCH SCHEDULER                            │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────────┐                                            │
│  │ PendingJobPoller │ ─── Runs every 5 seconds (configurable)   │
│  │   (Scheduled)    │                                            │
│  └────────┬─────────┘                                            │
│           │                                                      │
│           │ 1. Query PENDING jobs (batch of 20)                  │
│           │ 2. Group by messageGroup                             │
│           │ 3. Check for ERROR jobs in each group                │
│           │ 4. Filter based on DispatchMode                      │
│           ▼                                                      │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │             MessageGroupDispatcher                           ││
│  │  ┌─────────────────────────────────────────────────────────┐││
│  │  │ ConcurrentHashMap<groupKey, MessageGroupQueue>          │││
│  │  │                                                         │││
│  │  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  │││
│  │  │  │ Group: A     │  │ Group: B     │  │ Group: C     │  │││
│  │  │  │ Queue: [...] │  │ Queue: [...] │  │ Queue: [...] │  │││
│  │  │  │ InFlight: 1  │  │ InFlight: 0  │  │ InFlight: 1  │  │││
│  │  │  └──────────────┘  └──────────────┘  └──────────────┘  │││
│  │  └─────────────────────────────────────────────────────────┘││
│  │                                                              ││
│  │  Semaphore: max 10 concurrent groups                         ││
│  └──────────────────────────────────────────────────────────────┘│
│                              │                                   │
│                              ▼                                   │
│                    ┌─────────────────┐                           │
│                    │  JobDispatcher  │                           │
│                    │  (Virtual Thread)│                          │
│                    └────────┬────────┘                           │
│                             │                                    │
│                             │ Create MessagePointer              │
│                             │ Publish to queue                   │
│                             │ Update status → QUEUED             │
│                             ▼                                    │
│                    ┌─────────────────┐                           │
│                    │  External Queue │                           │
│                    │ (SQS/Embedded)  │                           │
│                    └─────────────────┘                           │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Message Group Concept

### What is a Message Group?

A **message group** is a logical grouping of dispatch jobs that must be processed sequentially (FIFO order) to maintain ordering guarantees.

### Purpose

Ensure that jobs for the same business entity are delivered in order:
- All orders for customer X
- All payment events for account Y
- All shipment updates for tracking number Z

### Implementation

| Field | Type | Description |
|-------|------|-------------|
| `messageGroup` | String | Group identifier, nullable |

**If null**: Defaults to "default" message group.

**Example**: `subscriptionName:eventMessageGroup` → `"order-service:customer-12345"`

### Ordering Guarantees

Within a message group, jobs are sorted by:
1. `sequence` number (lower values first, default 99)
2. `createdAt` timestamp (older jobs first)

**Critical**: Only ONE job per message group is in-flight at any time.

---

## Dispatch Modes

The `DispatchMode` enum controls how errors affect subsequent job processing:

### IMMEDIATE

| Behavior | Description |
|----------|-------------|
| Ordering | None - jobs dispatch immediately |
| On Error | Continue dispatching other jobs |
| Use Case | Fast, unordered processing |

### NEXT_ON_ERROR

| Behavior | Description |
|----------|-------------|
| Ordering | FIFO within message groups |
| On Error | Skip failed messages, continue with next |
| Use Case | Loose ordering (FIFO but skip failures) |

### BLOCK_ON_ERROR

| Behavior | Description |
|----------|-------------|
| Ordering | FIFO within message groups |
| On Error | Block entire group until error resolved |
| Use Case | Strict ordering with halt on problems |

### Error Blocking Logic

```
Before dispatching jobs from pending queue:
1. Query count of ERROR status jobs per message group
2. If any ERROR jobs exist in a group → mark group as BLOCKED
3. Skip all pending jobs in blocked groups
4. Only unblocked groups are dispatched
```

---

## Component Architecture

### PendingJobPoller

**Purpose**: Scheduled service that polls for pending jobs.

| Property | Description |
|----------|-------------|
| Schedule | Configurable interval (default 5 seconds) |
| Instance | Only runs on primary instance (leader election) |
| Batch Size | Configurable (default 20 jobs per poll) |

**Algorithm**:
```
1. Check if enabled and is primary instance
2. Acquire atomic lock (prevent overlapping polls)
3. Query PENDING jobs with limit
4. Group by messageGroup (null → "default")
5. Get blocked groups from BlockOnErrorChecker
6. For each group:
   - If blocked, skip all jobs
   - Filter jobs based on DispatchMode
   - Submit remaining jobs to MessageGroupDispatcher
7. Release lock
```

### BlockOnErrorChecker

**Purpose**: Check for ERROR jobs that should block message groups.

```
countByMessageGroupAndStatus(group, ERROR): int

Returns count of ERROR status jobs for the given message group.
If count > 0, the group should be blocked.
```

### MessageGroupDispatcher

**Purpose**: Orchestrates dispatch across multiple message groups.

| Component | Description |
|-----------|-------------|
| Group Queues | `ConcurrentHashMap<groupKey, MessageGroupQueue>` |
| Semaphore | Limits concurrent group dispatches (default 10) |
| Cleanup | Periodically removes empty queues |

**Group Key Format**: `{type}:{messageGroup}` (e.g., `"DISPATCH_JOB:customer-12345"`)

### MessageGroupQueue

**Purpose**: Per-group queue enforcing FIFO with single in-flight constraint.

| Component | Description |
|-----------|-------------|
| Pending Queue | `ConcurrentLinkedQueue<DispatchJob>` |
| In-Flight Flag | `AtomicBoolean` - true if a job is being dispatched |
| Pending Count | `AtomicInteger` - number of jobs waiting |

**Algorithm**:
```
tryDispatchNext():
    if compareAndSet(inFlight, false, true):
        job = pendingQueue.poll()
        if job != null:
            startVirtualThread(dispatchJob, job)
        else:
            inFlight = false
```

### JobDispatcher

**Purpose**: Actually sends jobs to the external queue.

**Responsibilities**:
1. Generate auth token via DispatchAuthService
2. Create MessagePointer from DispatchJob
3. Publish to queue (SQS FIFO, embedded, or ActiveMQ)
4. Update job status to QUEUED on success
5. Handle deduplication (same job already sent = success)

---

## Concurrency Model

### Thread Safety

| Component | Mechanism |
|-----------|-----------|
| Group Queues | `ConcurrentHashMap` |
| In-Flight Flag | `AtomicBoolean` with `compareAndSet` |
| Pending Count | `AtomicInteger` |
| Concurrent Groups | `Semaphore` |
| Dispatch Execution | Virtual Threads |

### Synchronization Flow

```
1. MessageGroupQueue.tryDispatchNext()
   └─ compareAndSet(inFlight, false, true)
      └─ Only one thread wins

2. MessageGroupDispatcher.dispatchJob()
   └─ semaphore.acquire()
      └─ Blocks if max concurrent groups reached
   └─ finally: semaphore.release()

3. ConcurrentLinkedQueue.poll()
   └─ Atomic poll operation
```

### Virtual Thread Benefits

- Lightweight (minimal memory per thread)
- Efficient for I/O-bound dispatch operations
- High concurrency without thread pool overhead

---

## Stale Job Recovery

### Problem

Jobs can get stuck in QUEUED status if:
- Queue send fails immediately after status update
- Processor crashes mid-dispatch
- Network issues prevent queue delivery

### Solution: StaleQueuedJobPoller

**Purpose**: Safety net for jobs stuck in QUEUED status.

| Property | Description |
|----------|-------------|
| Schedule | Separate interval (default 60 seconds) |
| Threshold | Jobs in QUEUED longer than 15 minutes |
| Batch Size | Up to 100 stale jobs per poll |
| Action | Reset status from QUEUED → PENDING |

**Algorithm**:
```
threshold = now() - 15 minutes
staleJobs = findQueuedOlderThan(threshold, limit=100)
updateStatusBatch(staleJobs.ids, PENDING)

# Next PendingJobPoller cycle will re-dispatch them
```

---

## Configuration

All configuration via application properties:

```properties
# Core Control
dispatch-scheduler.enabled=true
dispatch-scheduler.poll-interval=5s
dispatch-scheduler.batch-size=20

# Concurrency
dispatch-scheduler.max-concurrent-groups=10

# Queue Configuration
dispatch-scheduler.queue-type=EMBEDDED  # EMBEDDED | SQS | ACTIVEMQ
dispatch-scheduler.queue-url=<URL>
dispatch-scheduler.embedded-db-path=./dispatch-queue.db

# Processing Endpoints
dispatch-scheduler.processing-endpoint=http://localhost:8080/api/dispatch/process
dispatch-scheduler.default-dispatch-pool-code=DISPATCH-POOL

# Stale Job Recovery
dispatch-scheduler.stale-queued-threshold-minutes=15
dispatch-scheduler.stale-queued-poll-interval=60s
```

---

## Integration Points

### Upstream Dependencies

| Component | Purpose |
|-----------|---------|
| DispatchJobRepository | Query PENDING jobs, update statuses |
| BlockOnErrorChecker | Check for ERROR jobs in groups |
| StandbyService | Leader election (only primary runs) |
| DispatchSchedulerConfig | Configuration management |

### Downstream Dependencies

| Component | Purpose |
|-----------|---------|
| JobDispatcher | Send jobs to queue |
| QueuePublisherFactory | Get queue-specific publisher |
| DispatchAuthService | Generate auth tokens |

### Queue Integration

| Queue Type | Features |
|------------|----------|
| SQS FIFO | Uses message group ID for ordering, deduplication |
| Embedded | SQLite-backed queue for development |
| ActiveMQ | Apache message queue backend |

### MessagePointer Format

```json
{
  "id": "01K97FHM11EKYSXT135MVM6AC7",
  "poolCode": "DISPATCH-POOL",
  "authToken": "eyJhbGciOiJIUzI1NiIs...",
  "mediationType": "HTTP",
  "mediationTarget": "http://localhost:8080/api/dispatch/process",
  "messageGroupId": "customer-12345"
}
```

---

## Implementation Guide

### Core Components to Implement

1. **PendingJobPoller**: Periodic task that queries PENDING jobs
2. **MessageGroupDispatcher**: Manages per-group queues
3. **MessageGroupQueue**: FIFO queue with single in-flight constraint
4. **JobDispatcher**: Sends jobs to external queue
5. **BlockOnErrorChecker**: Queries for ERROR status jobs
6. **StaleQueuedJobPoller**: Recovers stuck QUEUED jobs

### Key Algorithms

**Pending Job Poll**:
```
jobs = database.findPending(limit=20)
grouped = groupBy(jobs, messageGroup)
blockedGroups = findGroupsWithErrors(grouped.keys())

for group, jobs in grouped:
    if group in blockedGroups:
        continue
    dispatchable = filterByMode(jobs, blockedGroups)
    dispatcher.submitJobs(group, dispatchable)
```

**Dispatch Single Job**:
```
semaphore.acquire()
try:
    pointer = createMessagePointer(job)
    result = queuePublisher.publish(pointer)
    if result.success():
        database.updateStatus(job.id, QUEUED)
finally:
    semaphore.release()
    triggerNextDispatch(group)
```

**Message Group Dispatch**:
```
if compareAndSet(inFlight, false, true):
    job = pendingQueue.poll()
    if job:
        launchVirtualThread(dispatchFunction, job)
    else:
        inFlight = false
```

### Critical Implementation Details

- Use atomic operations for in-flight flag (no race conditions)
- Process one job per message group at a time
- Use semaphore/lock for limiting concurrent groups
- Handle deduplication from queue (same message sent twice)
- Catch and retry on dispatch failures
- Implement leadership election for multi-instance deployments

### Status Lifecycle

```
Job Created (status = PENDING)
    ↓
PendingJobPoller.doPoll()
    ├─ Checks for ERROR jobs in message group
    ├─ Filters by DispatchMode
    └─ Submits to MessageGroupDispatcher
        ↓
    MessageGroupQueue.addJobs()
        ├─ Sorts by sequence + createdAt
        └─ Calls tryDispatchNext()
            ↓
        JobDispatcher.dispatch(job)
            ├─ Generates auth token
            ├─ Creates MessagePointer
            ├─ Publishes to queue
            └─ Updates job status = QUEUED

Stale QUEUED Job (older than 15 min)
    ↓
StaleQueuedJobPoller.doPoll()
    └─ Resets status back to PENDING
```

### Error Handling

| Scenario | Action |
|----------|--------|
| Dispatch Failure | Return false, job stays PENDING |
| Queue Publisher Init | Log warning, continue running |
| Polling Exceptions | Catch and log, don't stop scheduler |
| Virtual Thread Failure | Catch, log, release in-flight flag |
| Deduplication | Treat as success, mark QUEUED |

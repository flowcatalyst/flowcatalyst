# Architecture Overview

FlowCatalyst is a high-performance, event-driven platform built with Quarkus and Java 21 virtual threads. This document provides a comprehensive overview of the system architecture.

## System Design

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           FlowCatalyst Platform                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────────┐ │
│  │  Event Ingest   │───►│ Event Processor │───►│  Dispatch Scheduler     │ │
│  │  (REST API)     │    │ (Change Streams)│    │  (Job Queuing)          │ │
│  └─────────────────┘    └─────────────────┘    └───────────┬─────────────┘ │
│                                                             │               │
│  ┌─────────────────┐    ┌─────────────────┐                │               │
│  │  Platform API   │    │  Subscriptions  │◄───────────────┘               │
│  │  (Management)   │    │  (Matching)     │                                │
│  └─────────────────┘    └─────────────────┘                                │
│                                                                             │
│                         ┌─────────────────────────────────────────────────┐ │
│                         │              Message Router                      │ │
│                         │  ┌─────────┐  ┌─────────┐  ┌─────────────────┐  │ │
│                         │  │ Queue   │  │ Process │  │    HTTP         │  │ │
│                         │  │ Manager │─►│ Pools   │─►│    Mediator     │──┼─┼──► Webhooks
│                         │  └─────────┘  └─────────┘  └─────────────────┘  │ │
│                         └─────────────────────────────────────────────────┘ │
│                                          │                                  │
└──────────────────────────────────────────┼──────────────────────────────────┘
                                           │
                              ┌────────────┼────────────┐
                              ▼            ▼            ▼
                         ┌────────┐  ┌──────────┐  ┌───────────┐
                         │  SQS   │  │ ActiveMQ │  │ Embedded  │
                         └────────┘  └──────────┘  │ (SQLite)  │
                                                   └───────────┘
```

## Core Components

### 1. Event Processing Pipeline

**Event Ingest** → **Event Processor** → **Dispatch Scheduler** → **Message Router** → **Webhook Delivery**

1. **Events** arrive via REST API and are stored in MongoDB
2. **Event Processor** watches MongoDB change streams for new events
3. **Subscriptions** are matched against incoming events
4. **Dispatch Jobs** are created for matching subscriptions
5. **Dispatch Scheduler** queues jobs to the message router
6. **Message Router** processes jobs through pools with rate limiting
7. **HTTP Mediator** delivers webhooks with signing and retries

### 2. Message Router

The message router is a stateless, high-throughput component responsible for:

- Consuming messages from queues (SQS, ActiveMQ, Embedded SQLite)
- Managing processing pools with configurable concurrency
- Applying rate limits per pool
- Delivering messages via HTTP with circuit breakers and retries

Key characteristics:
- Uses Java 21 virtual threads for efficient I/O
- Horizontally scalable (stateless design)
- Supports hot standby via Redis leader election

### 3. Platform Services

Core platform functionality:

- **Authentication** - OIDC and internal auth providers
- **Authorization** - Role-based access control with permissions
- **Multi-Tenancy** - Client isolation with ANCHOR/PARTNER/CLIENT scopes
- **Service Accounts** - Machine-to-machine authentication with webhook credentials

## Data Flow

### Event to Webhook Flow

```
1. Event Created
   └──► MongoDB (events collection)
         └──► Change Stream (Event Processor)
               └──► Match Subscriptions
                     └──► Create Dispatch Jobs
                           └──► Queue Message (SQS/ActiveMQ)
                                 └──► Message Router
                                       └──► Process Pool
                                             └──► HTTP Mediator
                                                   └──► Webhook Endpoint
```

### Message Processing Flow

```
Queue (SQS/ActiveMQ/Embedded)
  │
  ▼
QueueConsumer (N connections per queue)
  │ parse & set MDC
  ▼
QueueManager.routeMessage()
  │ dedup check & routing
  ▼
ProcessPool.submit() → BlockingQueue
  │ poll from queue
  ▼
Check Rate Limit (pool-level)
  │ if rate-limited: nack & continue
  ▼
Acquire Semaphore (concurrency control)
  │ record processing started
  ▼
Mediator.process() → HTTP call
  │ handle result
  ▼
QueueManager.ack/nack → Consumer callback
  │ cleanup
  ▼
Release Semaphore & remove from pipeline
```

## Technology Stack

| Layer | Technology |
|-------|------------|
| **Runtime** | Java 21 (virtual threads) |
| **Framework** | Quarkus 3.28.x |
| **Database** | MongoDB |
| **Message Queues** | AWS SQS, Apache ActiveMQ, Embedded SQLite |
| **Caching/Coordination** | Redis (Redisson) |
| **Observability** | Micrometer, Prometheus |
| **Fault Tolerance** | SmallRye Fault Tolerance, Resilience4j |

## Design Principles

### 1. Virtual Thread Architecture

All I/O operations run on Java 21 virtual threads:
- Queue consumer connections
- ProcessPool workers
- HTTP client executor

Benefits:
- Lightweight concurrency (thousands of threads)
- Blocking I/O without platform thread overhead
- Simplified programming model (no reactive complexity)

### 2. Stateless Components

The message router is fully stateless:
- Configuration fetched from control endpoint
- No local state between restarts
- Horizontal scaling by adding instances

### 3. Event Sourcing Compatible

Events are stored as immutable records:
- CloudEvents-compatible structure
- Full audit trail via dispatch attempts
- Correlation and causation tracking

### 4. Multi-Tenant by Design

All data is tenant-aware:
- Client isolation at data layer
- Scoped access control (ANCHOR/PARTNER/CLIENT)
- Per-client configuration overrides

## Module Dependencies

```
flowcatalyst-platform (Core Library)
└── flowcatalyst-queue-client (Queue Abstraction)

flowcatalyst-message-router (Routing Engine)
└── flowcatalyst-standby (Optional HA)

flowcatalyst-event-processor (Stream Processing)
├── flowcatalyst-queue-client
└── flowcatalyst-standby

flowcatalyst-dispatch-scheduler (Job Scheduling)
├── flowcatalyst-platform
├── flowcatalyst-queue-client
└── flowcatalyst-standby
```

## Deployment Options

### Option 1: Full-Stack (flowcatalyst-app)

All components in single deployment:
- Platform services
- Message router
- Event processor
- Dispatch scheduler

Best for: Development, small deployments

### Option 2: Microservices

Separate deployments:
- **Platform API** - REST endpoints and management
- **Message Router** - Scalable message processing
- **Event Processor** - Change stream processing
- **Dispatch Scheduler** - Job scheduling

Best for: Production, high-volume deployments

### Option 3: Router-Only (flowcatalyst-router-app)

Lightweight message router only:
- No database required
- Configuration via REST endpoint
- Stateless and scalable

Best for: Edge deployments, dedicated routing

## Configuration Sync

The message router syncs configuration every 5 minutes (configurable):

1. Fetch new configuration from control endpoint
2. Compare pools - unchanged keep running, changed recreate
3. Compare queues - unchanged keep consumers, removed stop
4. Apply changes incrementally (no stop-the-world)

Benefits:
- Zero interruption for unchanged resources
- High availability during sync
- Surgical updates only affect changed components

## Security Model

### Authentication

- **OIDC** - External identity providers (Keycloak, Entra ID)
- **Internal** - Username/password with password hashing
- **Service Accounts** - API key + signing secret

### Authorization

- **Role-Based** - Roles assigned to principals
- **Permission-Based** - Fine-grained permissions on roles
- **Scope-Based** - ANCHOR/PARTNER/CLIENT access levels

### Webhook Security

- **HMAC Signing** - SHA-256 signature on payload
- **Bearer Tokens** - Optional auth header
- **Secret Management** - AWS Secrets Manager, Vault, GCP

## Observability

### Metrics (Prometheus)

- Queue statistics (received, processed, failed)
- Pool metrics (concurrency, active, success/error rates)
- Broker connectivity
- Circuit breaker states

### Health Checks

- **Liveness** (`/health/live`) - Application running
- **Readiness** (`/health/ready`) - Ready for traffic
- **Startup** (`/health/startup`) - Initialization complete

### Structured Logging

- JSON format in production
- MDC fields: messageId, poolCode, queueName, targetUri, result, durationMs

## See Also

- [Modules](modules.md) - Detailed module descriptions
- [Message Router](message-router.md) - Routing engine deep-dive
- [Postbox](postbox.md) - Message ingestion module
- [Event Processing](event-processing.md) - Event lifecycle
- [Multi-Tenancy](multi-tenancy.md) - Tenant model

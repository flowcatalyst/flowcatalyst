# Platform Modules

FlowCatalyst is organized as a monorepo with multiple Gradle modules. This document describes each module's purpose, dependencies, and key features.

## Module Overview

```
core/
├── flowcatalyst-platform          # Core library (auth, clients, events)
├── flowcatalyst-message-router    # Message routing engine
├── flowcatalyst-event-processor   # MongoDB change stream processor
├── flowcatalyst-dispatch-scheduler # Job scheduling service
├── flowcatalyst-queue-client      # Queue abstraction layer
├── flowcatalyst-standby           # Redis leader election
├── flowcatalyst-sdk               # Java SDK for integrations
├── flowcatalyst-outbox-processor  # Customer outbox polling
├── flowcatalyst-benchmark         # Performance benchmarks
├── flowcatalyst-app               # Full-stack deployment
├── flowcatalyst-router-app        # Router-only deployment
├── flowcatalyst-bffe              # Control plane BFFE
└── flowcatalyst-postbox           # Message ingestion store
```

## Core Libraries

### flowcatalyst-platform

**Purpose**: Core platform services providing authentication, authorization, multi-tenancy, and domain entities.

**Type**: Java Library (`java-library` plugin)

**Key Features**:
- Multi-tenant client management
- Principal/user management with OIDC support
- Role-based access control
- Event type and subscription management
- Dispatch job system
- Service account management
- Audit logging

**Package Structure**:
```
tech.flowcatalyst.platform/
├── principal/          # User/service account management
├── client/             # Multi-tenant client management
├── authentication/     # OIDC, OAuth, internal auth
├── authorization/      # Roles, permissions, RBAC
├── application/        # Application/integration management
├── security/secrets/   # Secret provider integrations
├── audit/              # Audit logging
└── common/             # Shared utilities (Result, DomainEvent)

tech.flowcatalyst/
├── eventtype/          # Event type definitions
├── subscription/       # Subscription management
├── dispatchjob/        # Dispatch job entities
├── dispatchpool/       # Rate limiting pools
├── serviceaccount/     # Service accounts
├── event/              # Event operations
└── schema/             # Schema definitions
```

**Dependencies**:
- Quarkus MongoDB Panache
- Quarkus OIDC
- AWS Secrets Manager, GCP Secret Manager, HashiCorp Vault
- Resilience4j, Micrometer

---

### flowcatalyst-message-router

**Purpose**: High-throughput message routing engine that consumes from queues and delivers to webhooks.

**Type**: Quarkus Application Library

**Key Features**:
- Virtual thread-based message processing
- Multi-queue support (SQS, ActiveMQ, Embedded SQLite)
- Configurable processing pools with concurrency control
- Pool-level rate limiting
- Circuit breaker and retry patterns
- Hot standby support (optional)
- Real-time monitoring dashboard

**Package Structure**:
```
tech.flowcatalyst.messagerouter/
├── endpoint/       # REST API endpoints
├── consumer/       # Queue consumers (SQS, ActiveMQ)
├── manager/        # QueueManager orchestration
├── pool/           # ProcessPool implementation
├── mediator/       # HTTP delivery
├── callback/       # Ack/nack callbacks
├── config/         # Configuration management
├── security/       # Authentication
├── traffic/        # Traffic management (ALB)
├── metrics/        # Observability
├── health/         # Health checks
├── standby/        # Hot standby coordination
├── diagnostics/    # Debugging tools
└── factory/        # Factory patterns
```

**Key Classes**:
- `QueueManager` - Central orchestrator
- `ProcessPoolImpl` - Worker pool with rate limiting
- `HttpMediator` - HTTP delivery with fault tolerance
- `SqsQueueConsumer` / `ActiveMqQueueConsumer` - Queue consumers

---

### flowcatalyst-queue-client

**Purpose**: Queue abstraction layer supporting multiple queue backends.

**Type**: Java Library

**Supported Backends**:
- AWS SQS
- Apache ActiveMQ Artemis
- Embedded SQLite (development)

**Key Features**:
- Unified API for all queue types
- Backend-specific optimizations
- Message acknowledgment handling

---

### flowcatalyst-standby

**Purpose**: Redis-based leader election for hot standby capability.

**Type**: Java Library

**Key Features**:
- Leader election via Redisson
- Automatic failover
- Health check integration

**Dependencies**:
- Redisson 3.40.2

---

### flowcatalyst-event-processor

**Purpose**: MongoDB change stream processor that watches for new events and triggers subscriptions.

**Type**: Quarkus Application

**Key Features**:
- Change stream monitoring
- Subscription matching
- Dispatch job creation
- Checkpointing via Redis

**Dependencies**:
- flowcatalyst-queue-client
- flowcatalyst-standby

---

### flowcatalyst-dispatch-scheduler

**Purpose**: Polls pending dispatch jobs from database and queues them for processing.

**Type**: Java Library with Quarkus

**Key Features**:
- Job polling and scheduling
- Dispatch mode handling (IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR)
- Queue integration

**Dependencies**:
- flowcatalyst-platform
- flowcatalyst-queue-client
- flowcatalyst-standby

---

### flowcatalyst-sdk

**Purpose**: Java SDK for applications integrating with FlowCatalyst.

**Type**: Java Library

**Key Features**:
- Postbox/outbox pattern support
- Event publishing helpers
- Role definition annotations

**Dependencies**:
- Quarkus Hibernate ORM Panache
- Jackson, Validation

---

### flowcatalyst-outbox-processor

**Purpose**: Polls customer outbox tables and sends events to FlowCatalyst.

**Type**: Quarkus Application

**Key Features**:
- Outbox pattern implementation
- Multiple database support
- Event forwarding

---

### flowcatalyst-postbox

**Purpose**: Reliable message ingestion store for external systems using the outbox pattern.

**Type**: Quarkus Application Library

**Key Features**:
- REST endpoint for message ingestion
- PostgreSQL-based message storage
- Dynamic partition poller discovery
- Automatic retry with configurable limits
- Support for EVENT and DISPATCH_JOB message types

**Package Structure**:
```
tech.flowcatalyst.postbox/
├── endpoint/       # REST API (ingest)
├── entity/         # PostboxMessage JPA entity
├── repository/     # Panache repository
├── service/        # PostboxService, PollerDiscovery, PollerScheduler
├── handler/        # Message handlers (Event, DispatchJob)
├── model/          # MessageStatus, MessageType enums
├── config/         # Configuration
├── health/         # Health checks
└── metrics/        # Micrometer metrics
```

**Dependencies**:
- Quarkus Hibernate ORM Panache
- PostgreSQL JDBC Driver

See [Postbox Architecture](postbox.md) for detailed documentation.

## Application Modules

### flowcatalyst-app

**Purpose**: Full-stack deployment combining all components.

**Includes**:
- flowcatalyst-platform
- flowcatalyst-message-router
- flowcatalyst-event-processor
- flowcatalyst-dispatch-scheduler

**Use Case**: Development, small deployments, single-instance scenarios

---

### flowcatalyst-router-app

**Purpose**: Lightweight router-only deployment.

**Includes**:
- flowcatalyst-message-router

**Use Case**: Edge deployments, dedicated routing, stateless scaling

---

### flowcatalyst-bffe

**Purpose**: Control plane Backend-For-Frontend.

**Key Features**:
- Session-based authentication
- Platform management REST API
- Quarkus Web Bundler for Vue.js integration

**Dependencies**:
- flowcatalyst-platform
- Quarkus OIDC, Web Bundler

## Module Dependency Graph

```
                    ┌─────────────────────┐
                    │  flowcatalyst-app   │
                    │   (Full-Stack)      │
                    └─────────┬───────────┘
                              │
        ┌─────────────────────┼─────────────────────┐
        │                     │                     │
        ▼                     ▼                     ▼
┌───────────────┐   ┌─────────────────┐   ┌─────────────────┐
│   platform    │   │ message-router  │   │ event-processor │
│               │   │                 │   │                 │
└───────┬───────┘   └────────┬────────┘   └────────┬────────┘
        │                    │                     │
        │                    │                     │
        ▼                    ▼                     ▼
┌───────────────┐   ┌─────────────────┐   ┌─────────────────┐
│ queue-client  │   │    standby      │   │  queue-client   │
│               │   │   (optional)    │   │    standby      │
└───────────────┘   └─────────────────┘   └─────────────────┘
```

## Configuration by Module

### flowcatalyst-platform

```properties
# MongoDB connection
quarkus.mongodb.connection-string=mongodb://localhost:27017
quarkus.mongodb.database=flowcatalyst

# OIDC (optional)
quarkus.oidc.auth-server-url=https://auth.example.com
quarkus.oidc.client-id=flowcatalyst
```

### flowcatalyst-message-router

```properties
# Queue type
message-router.queue-type=SQS  # or ACTIVEMQ, EMBEDDED

# Config endpoint
message-router.config-url=http://localhost:8080/api/config

# Pool limits
message-router.max-pools=2000
message-router.pool-warning-threshold=1000

# Sync interval
message-router.sync-interval=5m
```

### flowcatalyst-standby

```properties
# Redis for leader election
flowcatalyst.standby.enabled=true
flowcatalyst.standby.redis-url=redis://localhost:6379
```

## Development Commands

```bash
# Run specific module in dev mode
./gradlew :core:flowcatalyst-message-router:quarkusDev
./gradlew :core:flowcatalyst-platform:quarkusDev
./gradlew :core:flowcatalyst-app:quarkusDev

# Build modules
./gradlew :core:flowcatalyst-message-router:build
./gradlew :core:flowcatalyst-app:build

# Run tests
./gradlew :core:flowcatalyst-message-router:test
./gradlew :core:flowcatalyst-platform:test
```

## See Also

- [Architecture Overview](overview.md) - System design
- [Message Router](message-router.md) - Router deep-dive
- [Postbox](postbox.md) - Message ingestion module
- [Development Setup](../development/setup.md) - Getting started

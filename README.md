# FlowCatalyst

**High-Performance Event-Driven Platform**

FlowCatalyst is a message routing and webhook dispatch platform built with Quarkus and Java 21 virtual threads. It provides reliable message processing, webhook delivery, and multi-tenant platform management.

## Key Features

- **High Throughput** - Java 21 virtual threads for efficient I/O
- **FIFO Ordering** - Message groups ensure ordered processing per business entity
- **Reliable Delivery** - Retries, circuit breakers, and audit trails
- **Multi-Tenant** - Client isolation with flexible access scopes
- **Flexible Queues** - SQS, ActiveMQ, or embedded for development

## Core Modules

| Module | Description |
|--------|-------------|
| `flowcatalyst-platform` | Core entities, auth, and business logic |
| `flowcatalyst-message-router` | Stateless message routing engine |
| `flowcatalyst-event-processor` | MongoDB change stream processing |
| `flowcatalyst-dispatch-scheduler` | Job scheduling and dispatch |
| `flowcatalyst-app` | Full-stack deployment |
| `flowcatalyst-router-app` | Router-only deployment |

## Repository Structure

```
flowcatalyst/
├── core/                              # Java/Quarkus modules
│   ├── flowcatalyst-platform/         # Core platform library
│   ├── flowcatalyst-message-router/   # Message routing engine
│   ├── flowcatalyst-event-processor/  # Event processing
│   ├── flowcatalyst-dispatch-scheduler/ # Job scheduling
│   ├── flowcatalyst-app/              # Full-stack deployment
│   └── flowcatalyst-router-app/       # Router-only deployment
│
├── packages/                          # Frontend packages
│   └── platform-ui-vue/               # Vue 3 platform UI
│
└── docs/                              # Documentation
    ├── architecture/                  # System architecture
    ├── entities/                      # Entity reference
    ├── guides/                        # How-to guides
    ├── operations/                    # Deployment & operations
    ├── development/                   # Development guides
    └── reference/                     # Configuration & enums
```

## Quick Start

### Prerequisites

- Java 21+
- MongoDB (with replica set for change streams)
- Docker (for local services)

### Development Mode

```bash
# Start MongoDB replica set
docker-compose -f docker-compose.dev.yml up -d

# Message Router (most common)
./gradlew :core:flowcatalyst-message-router:quarkusDev

# Full-stack application
./gradlew :core:flowcatalyst-app:quarkusDev

# With embedded queue (no external broker)
./gradlew :core:flowcatalyst-message-router:quarkusDev -Dquarkus.profile=chronicle-dev
```

### Access Points

| Endpoint | URL |
|----------|-----|
| API | http://localhost:8080 |
| Health | http://localhost:8080/health |
| Metrics | http://localhost:8080/q/metrics |
| Dev UI | http://localhost:8080/q/dev/ |

## Documentation

Full documentation is available in the [docs/](docs/) directory:

### Architecture
- [System Overview](docs/architecture/overview.md) - Components and design
- [Module Reference](docs/architecture/modules.md) - All modules explained
- [Message Router](docs/architecture/message-router.md) - Routing deep-dive
- [Multi-Tenancy](docs/architecture/multi-tenancy.md) - Client isolation model

### Entity Reference
- [Entity Overview](docs/entities/overview.md) - ER diagram and collections
- [Auth Entities](docs/entities/auth-entities.md) - Principal, roles, permissions
- [Client Entities](docs/entities/client-entities.md) - Tenants and access
- [Dispatch Entities](docs/entities/dispatch-entities.md) - Jobs and delivery

### Guides
- [Authentication](docs/guides/authentication.md) - Auth system setup
- [Authorization](docs/guides/authorization.md) - RBAC and permissions
- [Dispatch Jobs](docs/guides/dispatch-jobs.md) - Webhook delivery
- [Message Groups](docs/guides/message-groups.md) - FIFO ordering
- [Queue Configuration](docs/guides/queue-configuration.md) - SQS/ActiveMQ setup

### Operations
- [Deployment](docs/operations/deployment.md) - Docker and Kubernetes
- [Monitoring](docs/operations/monitoring.md) - Metrics and health
- [Scaling](docs/operations/scaling.md) - Horizontal scaling

### Development
- [Setup Guide](docs/development/setup.md) - Environment setup
- [Build Reference](docs/development/build-reference.md) - Build commands
- [Testing Guide](docs/development/testing.md) - Running tests
- [Coding Standards](docs/development/coding-standards.md) - TSID and patterns

### Reference
- [Configuration](docs/reference/configuration.md) - All config options
- [Enums](docs/reference/enums.md) - Domain enumerations
- [Glossary](docs/reference/glossary.md) - Terminology

## Technology Stack

| Component | Technology |
|-----------|------------|
| Runtime | Java 21 with virtual threads |
| Framework | Quarkus 3.x |
| Database | MongoDB |
| Queues | AWS SQS, ActiveMQ, or embedded |
| Build | Gradle |
| Frontend | Vue 3, TypeScript, Tailwind CSS |

## License

Apache-2.0 - See LICENSE file for details

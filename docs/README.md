# FlowCatalyst Documentation

Welcome to the FlowCatalyst platform documentation. FlowCatalyst is a high-performance, event-driven platform for building domain-oriented applications with reliable message processing and webhook delivery.

## Quick Navigation

| Section | Description |
|---------|-------------|
| [Getting Started](getting-started/) | Installation, quickstart, and first project tutorial |
| [Architecture](architecture/) | System design, modules, and technical decisions |
| [Entities](entities/) | Complete reference for all data models |
| [Guides](guides/) | How-to guides for common tasks |
| [Operations](operations/) | Deployment, monitoring, and troubleshooting |
| [Development](development/) | Developer setup, testing, and contribution |
| [Reference](reference/) | Configuration, enums, and glossary |

## Platform Overview

FlowCatalyst provides:

- **High-Performance Message Routing** - Java 21 virtual threads for efficient I/O
- **Reliable Webhook Delivery** - HMAC signing, retries, and full audit trail
- **Multi-Tenant Architecture** - Client isolation with flexible access scopes
- **Event-Driven Design** - CloudEvents-compatible event processing
- **Flexible Queue Support** - SQS, ActiveMQ, or embedded SQLite queue

## Architecture at a Glance

```
                    ┌─────────────────────────────────────────────────┐
                    │              FlowCatalyst Platform              │
                    ├─────────────────────────────────────────────────┤
    Events In ────► │  Event Processor  ─►  Dispatch Scheduler       │
                    │        │                     │                  │
                    │        ▼                     ▼                  │
                    │   Subscriptions    ───►  Dispatch Jobs ◄────── │ ◄── API (Direct Creation)
                    │                              │                  │
                    │                              ▼                  │
                    │                      Message Router             │ ────► Webhooks Out
                    └─────────────────────────────────────────────────┘
                                          │
                              ┌───────────┴───────────┐
                              ▼           ▼           ▼
                            SQS      ActiveMQ    Embedded
                                                 (SQLite)
```

**Note**: Dispatch Jobs can be created via:
- **Subscriptions** - Automatically when events match subscription filters
- **API** - Directly for tasks, scheduled work, or custom integrations

## Core Modules

| Module | Purpose |
|--------|---------|
| **flowcatalyst-platform** | Core services: auth, clients, events, subscriptions |
| **flowcatalyst-message-router** | Stateless message routing engine |
| **flowcatalyst-event-processor** | MongoDB change stream processing |
| **flowcatalyst-dispatch-scheduler** | Job scheduling and queue management |
| **flowcatalyst-queue-client** | Queue abstraction (SQS, ActiveMQ, Embedded SQLite) |
| **flowcatalyst-standby** | Redis-based leader election for HA |

## Getting Started

**New to FlowCatalyst?** Start here:

1. [Quickstart](getting-started/quickstart.md) - Get running in 5 minutes
2. [Installation](getting-started/installation.md) - Detailed setup guide
3. [First Project](getting-started/first-project.md) - Build your first integration

**Understanding the Platform:**

1. [Architecture Overview](architecture/overview.md) - How it all fits together
2. [Module Guide](architecture/modules.md) - What each module does
3. [Multi-Tenancy](architecture/multi-tenancy.md) - Client and access model

**Working with Data:**

1. [Entity Overview](entities/overview.md) - Data model reference
2. [Dispatch Jobs](guides/dispatch-jobs.md) - Webhook delivery system
3. [Authentication](guides/authentication.md) - Security and auth

## Technology Stack

- **Runtime**: Java 21 with virtual threads
- **Framework**: Quarkus 3.28.x
- **Database**: MongoDB
- **Queues**: AWS SQS, Apache ActiveMQ, Embedded SQLite
- **Caching**: Redis with Redisson
- **IDs**: TSID (Time-Sorted IDs) as Crockford Base32 strings

## Key Concepts

### TSID Identifiers

All entity IDs use TSID format - 13-character Crockford Base32 strings that are:
- Lexicographically sortable (newer IDs sort after older)
- URL-safe and case-insensitive
- Safe from JavaScript number precision issues

Example: `0HZXEQ5Y8JY5Z`

### Multi-Tenant Scopes

Users have one of three access scopes:
- **ANCHOR** - Platform admins with access to all clients
- **PARTNER** - Partner users with access to assigned clients
- **CLIENT** - Users bound to a single client

### Dispatch Modes

Control how message groups are processed:
- **IMMEDIATE** - Process messages as fast as possible
- **NEXT_ON_ERROR** - Skip failed messages, continue with next
- **BLOCK_ON_ERROR** - Block message group until error resolved

## Documentation Conventions

- **Code examples** use Java unless otherwise noted
- **Entity references** link to the Entities section
- **Configuration** uses Quarkus property format
- **API examples** use curl commands

## Contributing

See [Development](development/) for:
- [Setup](development/setup.md) - Developer environment
- [Testing](development/testing.md) - Running and writing tests
- [Coding Standards](development/coding-standards.md) - Code conventions

## Support

- **GitHub Issues** - Bug reports and feature requests
- **Discussions** - Questions and community support

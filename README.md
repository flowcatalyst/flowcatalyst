# FlowCatalyst Rust

High-performance event routing and webhook delivery platform written in Rust.

## Architecture Overview

FlowCatalyst is a distributed event processing system composed of specialized services that work together to receive events, match them to subscriptions, and reliably deliver them to webhook endpoints.

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Application   │────▶│  Outbox Table    │────▶│    Outbox       │
│   (Publisher)   │     │  (App Database)  │     │   Processor     │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
                                                          │
                                                          ▼
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Platform      │◀────│   PostgreSQL     │◀────│     Stream      │
│   APIs          │────▶│   (Events DB)    │────▶│    Processor    │
└─────────────────┘     └──────────────────┘     └────────┬────────┘
                                                          │
                                                          ▼
                        ┌──────────────────┐     ┌─────────────────┐
                        │   Dispatch Jobs  │◀────│    Scheduler    │
                        │                  │────▶│                 │
                        └────────┬─────────┘     └─────────────────┘
                                 │
                                 ▼
                        ┌──────────────────┐     ┌─────────────────┐
                        │   Message Queue  │────▶│    Message      │
                        │  (SQS/SQLite)    │     │    Router       │
                        └──────────────────┘     └────────┬────────┘
                                                          │
                                                          ▼
                                                 ┌─────────────────┐
                                                 │    Webhook      │
                                                 │   Endpoints     │
                                                 └─────────────────┘
```

## Services

| Service | Description | Documentation |
|---------|-------------|---------------|
| **Message Router** | Consumes queued messages and delivers to webhooks with retry, circuit breakers, and rate limiting | [docs/message-router.md](docs/message-router.md) |
| **Platform** | REST APIs for events, subscriptions, clients, principals, and administration | [docs/platform.md](docs/platform.md) |
| **Outbox Processor** | Reads outbox tables from application databases and publishes events | [docs/outbox-processor.md](docs/outbox-processor.md) |
| **Stream Processor** | Watches for new events and creates dispatch jobs from subscriptions | [docs/stream-processor.md](docs/stream-processor.md) |
| **Scheduler** | Polls pending dispatch jobs and queues them for delivery | [docs/scheduler.md](docs/scheduler.md) |
| **Shared Crates** | Common libraries used across services | [docs/shared-crates.md](docs/shared-crates.md) |
| **Partitioning** | Monthly RANGE partitioning for the high-volume messaging tables; in-Rust manager (no Postgres extensions) | [docs/partitioning.md](docs/partitioning.md) |

## Binaries

| Binary | Package | Purpose |
|--------|---------|---------|
| `fc-dev` | `bin/fc-dev` | All-in-one development monolith with embedded SQLite queue |
| `fc-router` | `bin/fc-router` | Production message router (SQS consumer) |
| `fc-platform-server` | `bin/fc-platform-server` | Production platform API server |
| `fc-outbox-processor` | `bin/fc-outbox-processor` | Production outbox processor |
| `fc-stream-processor` | `bin/fc-stream-processor` | Production stream processor |
| `fc-scheduler-server` | `bin/fc-scheduler-server` | Production dispatch scheduler |

## Quick Start

### Development (All-in-One)

```bash
# Start PostgreSQL
docker compose -f docker-compose.dev.yml up -d

# Run development server
cargo run -p fc-dev

# Or use the dev script
./dev.sh
```

Development URLs:
- Platform API: http://localhost:8080
- Router API: http://localhost:8081
- Metrics: http://localhost:9090/metrics

### Installing fc-dev from a release

Prebuilt binaries are published on every release for macOS (Apple Silicon),
Linux (x86_64 and ARM64), and Windows (x86_64). One-liner installers fetch
the latest, verify SHA256, and put `fc-dev` on your `PATH`:

```sh
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.sh | sh
```

```powershell
# Windows (PowerShell 5.1+)
irm https://raw.githubusercontent.com/flowcatalyst/flowcatalyst/main/install.ps1 | iex
```

Full per-platform instructions, manual install, cosign verification, and
troubleshooting are in [`INSTALL.md`](INSTALL.md).

Once installed, the binary keeps itself up to date:

```sh
fc-dev upgrade           # download & replace if a newer release exists
fc-dev upgrade --check   # just check
```

On startup fc-dev also does a best-effort check against GitHub (24h
cached; disable with `FC_DEV_UPDATE_CHECK=false`) and prints a hint when
a newer release is available.

### Releasing fc-dev

1. Bump `version` in [`bin/fc-dev/Cargo.toml`](bin/fc-dev/Cargo.toml).
2. Commit, then tag: `git tag fc-dev/v0.2.0 && git push --tags`.
3. The [`Release fc-dev`](.github/workflows/release-fc-dev.yml) workflow
   builds binaries for all four target triples in parallel and publishes
   a GitHub Release with the archives + SHA256 sidecars.
4. fc-dev's tag is independent of the workspace version and other crates.

### Production

Each service runs independently:

```bash
# Build all binaries
cargo build --release

# Run individual services
./target/release/fc-platform-server
./target/release/fc-stream-processor
./target/release/fc-scheduler-server
./target/release/fc-router
./target/release/fc-outbox-processor
```

#### Postgres requirements

Vanilla PostgreSQL — **no extensions required**. The high-volume messaging
tables are partitioned via native `PARTITION BY RANGE (created_at)` and
maintained at runtime by an in-Rust partition manager that runs in every
environment (dev and prod alike). Details in
[docs/partitioning.md](docs/partitioning.md).

## Project Structure

```
flowcatalyst-rust/
├── bin/                          # Production binaries
│   ├── fc-dev/                   # Development monolith
│   ├── fc-router/                # Message router
│   ├── fc-platform-server/       # Platform APIs
│   ├── fc-outbox-processor/      # Outbox processor
│   ├── fc-stream-processor/      # Stream processor
│   └── fc-scheduler-server/      # Dispatch scheduler
├── crates/                       # Shared libraries
│   ├── fc-common/                # Core types and models
│   ├── fc-config/                # Configuration system
│   ├── fc-queue/                 # Queue abstraction (SQS, SQLite, ActiveMQ)
│   ├── fc-router/                # Routing engine
│   ├── fc-api/                   # HTTP API layer
│   ├── fc-platform/              # Platform domain and services
│   ├── fc-outbox/                # Outbox pattern implementation
│   ├── fc-stream/                # Change stream processing
│   ├── fc-scheduler/             # Job scheduling engine
│   ├── fc-standby/               # Leader election (HA)
│   └── fc-secrets/               # Secrets management
├── docs/                         # Component documentation
├── Cargo.toml                    # Workspace configuration
├── Makefile                      # Build commands
└── docker-compose.dev.yml        # Development infrastructure
```

## Technology Stack

- **Runtime**: Tokio (async)
- **Web Framework**: Axum
- **Databases**: PostgreSQL, SQLite
- **Queue**: AWS SQS, SQLite (dev), ActiveMQ
- **Serialization**: Serde
- **Authentication**: JWT with RSA keys
- **Observability**: Tracing, Prometheus metrics
- **API Docs**: OpenAPI/Swagger (utoipa)

## Configuration

All services are configured via environment variables. See individual component documentation for specific variables.

Common variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `RUST_LOG` | `info` | Log level |

## Health Checks

All services expose health endpoints:

| Endpoint | Port | Description |
|----------|------|-------------|
| `/q/live` | 9090 | Kubernetes liveness probe |
| `/q/ready` | 9090 | Kubernetes readiness probe |
| `/health` | 9090 | Basic health status |
| `/metrics` | 9090 | Prometheus metrics |

## License

Proprietary - FlowCatalyst

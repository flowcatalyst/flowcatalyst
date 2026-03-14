# Builds

FlowCatalyst produces 7 binaries. For production, use **fc-server** (unified) or the individual service binaries. For local development, use **fc-dev**.

## Quick Reference

| Binary | Purpose | Build | Run |
|--------|---------|-------|-----|
| `fc-server` | Unified production server (all subsystems) | `just build-server` | `just run-server` |
| `fc-dev` | Local development monolith | `just build-dev` | `just dev` |
| `fc-platform-server` | Platform REST API only | `just build-platform` | `cargo run --bin fc-platform-server` |
| `fc-router-bin` | SQS message router only | `just build-router` | `cargo run --bin fc-router-bin` |
| `fc-scheduler-server` | Dispatch scheduler only | `cargo build --bin fc-scheduler-server` | `cargo run --bin fc-scheduler-server` |
| `fc-stream-processor` | CQRS stream projections only | `just build-stream` | `cargo run --bin fc-stream-processor` |
| `fc-outbox-processor` | Outbox relay only | `just build-outbox` | `cargo run --bin fc-outbox-processor` |

Build all at once:

```sh
just build          # debug
just release        # optimized
```

---

## fc-server (Unified Production Binary)

Single binary combining all subsystems. Each subsystem is toggled via environment variables. Background processors can run in standby mode with Redis leader election.

### Build

```sh
just build-server

# With ALB support (registers with AWS ALB on leadership):
cargo build --bin fc-server --features alb
```

### Run

```sh
just run-server

# Platform API only (default):
FC_DATABASE_URL=postgresql://localhost:5432/flowcatalyst \
  cargo run --bin fc-server

# Platform + Router + Scheduler with standby:
FC_DATABASE_URL=postgresql://localhost:5432/flowcatalyst \
FC_ROUTER_ENABLED=true \
FC_SCHEDULER_ENABLED=true \
FC_STANDBY_ENABLED=true \
FC_STANDBY_REDIS_URL=redis://localhost:6379 \
  cargo run --bin fc-server

# All subsystems, no standby:
FC_PLATFORM_ENABLED=true \
FC_ROUTER_ENABLED=true \
FC_SCHEDULER_ENABLED=true \
FC_STREAM_PROCESSOR_ENABLED=true \
FC_OUTBOX_ENABLED=true \
FC_DATABASE_URL=postgresql://localhost:5432/flowcatalyst \
FLOWCATALYST_CONFIG_URL=http://config-service/api/router/config \
FC_OUTBOX_DB_URL=postgresql://localhost:5432/flowcatalyst \
  cargo run --bin fc-server
```

### Environment Variables

#### Core

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_API_PORT` | `3000` | HTTP API port |
| `FC_METRICS_PORT` | `9090` | Metrics/health port |
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `FC_DEV_MODE` | `false` | Enable dev data seeding |
| `RUST_LOG` | `info` | Log level filter |

#### Subsystem Toggles

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_PLATFORM_ENABLED` | `true` | Run the platform REST API |
| `FC_ROUTER_ENABLED` | `false` | Run the SQS message router |
| `FC_SCHEDULER_ENABLED` | `false` | Run the dispatch scheduler |
| `FC_STREAM_PROCESSOR_ENABLED` | `false` | Run the CQRS stream processor |
| `FC_OUTBOX_ENABLED` | `false` | Run the outbox relay |

#### Standby / HA

When enabled, all background processors (router, scheduler, stream, outbox) share a single Redis leader lock. Only the leader instance runs them. The platform API always runs regardless of leadership.

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_STANDBY_ENABLED` | `false` | Enable Redis leader election |
| `FC_STANDBY_REDIS_URL` | `redis://127.0.0.1:6379` | Redis URL for leader election |
| `FC_STANDBY_LOCK_KEY` | `fc:server:leader` | Redis lock key |

#### ALB Integration (requires `alb` feature)

When the router has leadership, registers with the ALB target group. Deregisters on leadership loss.

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_ALB_ENABLED` | `false` | Register with ALB when leader |
| `FC_ALB_TARGET_GROUP_ARN` | - | ALB target group ARN (required if enabled) |
| `FC_ALB_TARGET_ID` | - | Target ID: instance ID or IP (required if enabled) |
| `FC_ALB_TARGET_PORT` | `8080` | Port for ALB health checks |

#### Auth / JWT

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_JWT_PRIVATE_KEY_PATH` | - | Path to RSA private key PEM file |
| `FC_JWT_PUBLIC_KEY_PATH` | - | Path to RSA public key PEM file |
| `FLOWCATALYST_JWT_PRIVATE_KEY` | - | RSA private key PEM (inline env) |
| `FLOWCATALYST_JWT_PUBLIC_KEY` | - | RSA public key PEM (inline env) |
| `FC_JWT_ISSUER` | `flowcatalyst` | JWT issuer claim |
| `FC_ACCESS_TOKEN_EXPIRY_SECS` | `3600` | Access token TTL |
| `FC_SESSION_TOKEN_EXPIRY_SECS` | `28800` | Session token TTL (8h) |
| `FC_REFRESH_TOKEN_EXPIRY_SECS` | `2592000` | Refresh token TTL (30d) |
| `FC_JWT_PUBLIC_KEY_PATH_PREVIOUS` | - | Previous public key for key rotation |
| `FLOWCATALYST_APP_KEY` | - | AES key for encrypting OIDC client secrets |
| `FC_EXTERNAL_BASE_URL` | `http://localhost:{port}` | External URL for OIDC redirects |

#### Router-specific (when `FC_ROUTER_ENABLED=true`)

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_CONFIG_URL` | - | Router config URL (required unless dev mode) |
| `FLOWCATALYST_DEV_MODE` | `false` | Use built-in LocalStack config |
| `LOCALSTACK_ENDPOINT` | `http://localhost:4566` | LocalStack endpoint (dev mode) |
| `LOCALSTACK_SQS_HOST` | `http://sqs.eu-west-1.localhost.localstack.cloud:4566` | SQS host (dev mode) |

#### Scheduler-specific (when `FC_SCHEDULER_ENABLED=true`)

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_SCHEDULER_MAX_CONCURRENT_GROUPS` | `10` | Max concurrent message groups |
| `FC_SCHEDULER_DEFAULT_POOL_CODE` | `DISPATCH-POOL` | Default dispatch pool |
| `FC_SCHEDULER_PROCESSING_ENDPOINT` | `http://localhost:8080/api/dispatch/process` | Processing endpoint URL |

#### Stream Processor-specific (when `FC_STREAM_PROCESSOR_ENABLED=true`)

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_STREAM_EVENTS_ENABLED` | `true` | Enable event projection |
| `FC_STREAM_EVENTS_BATCH_SIZE` | `100` | Event projection batch size |
| `FC_STREAM_DISPATCH_JOBS_ENABLED` | `true` | Enable dispatch job projection |
| `FC_STREAM_DISPATCH_JOBS_BATCH_SIZE` | `100` | Dispatch job projection batch size |

#### Outbox-specific (when `FC_OUTBOX_ENABLED=true`)

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_OUTBOX_DB_TYPE` | `postgres` | Database type: `sqlite` or `postgres` |
| `FC_OUTBOX_DB_URL` | - | Database connection URL (required) |
| `FC_OUTBOX_EVENTS_TABLE` | `outbox_messages` | Events outbox table name |
| `FC_OUTBOX_DISPATCH_JOBS_TABLE` | `outbox_messages` | Dispatch jobs outbox table name |
| `FC_OUTBOX_AUDIT_LOGS_TABLE` | `outbox_messages` | Audit logs outbox table name |
| `FC_OUTBOX_POLL_INTERVAL_MS` | `1000` | Poll interval in ms |
| `FC_OUTBOX_BATCH_SIZE` | `500` | Max items per poll |
| `FC_API_BASE_URL` | `http://localhost:8080` | FlowCatalyst API URL |
| `FC_API_TOKEN` | - | API bearer token |
| `FC_API_BATCH_SIZE` | `100` | Items per API call |
| `FC_MAX_IN_FLIGHT` | `5000` | Max concurrent items |
| `FC_GLOBAL_BUFFER_SIZE` | `1000` | Buffer capacity |
| `FC_MAX_CONCURRENT_GROUPS` | `10` | Max concurrent message groups |

#### Frontend Static Serving

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_STATIC_DIR` | - | Path to built frontend (serves SPA with fallback) |

### Health Endpoint

`GET /health` on the metrics port returns:

```json
{
  "status": "UP",
  "leader": true,
  "version": "0.1.0",
  "components": {
    "platform": "UP",
    "router": "UP|STANDBY|DISABLED",
    "scheduler": "UP|STANDBY|DISABLED",
    "stream_processor": "UP|STANDBY|DISABLED",
    "outbox": "UP|STANDBY|DISABLED"
  }
}
```

---

## fc-dev (Development Monolith)

All-in-one binary for local development. Uses an embedded SQLite queue instead of SQS. No standby mode, no ALB. Includes the message router and platform APIs.

### Build & Run

```sh
# With hot-reload:
just dev

# Once:
just run

# With frontend:
just dev-full

# With built frontend:
just dev-static
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_API_PORT` | `8080` | HTTP API port |
| `FC_METRICS_PORT` | `9090` | Metrics/health port |
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `FC_STATIC_DIR` | - | Path to built frontend |
| `RUST_LOG` | `info` | Log level filter |

Plus all Auth / JWT variables listed under fc-server.

---

## fc-platform-server

Standalone platform REST API server. No message router, scheduler, or background processors.

### Build & Run

```sh
just build-platform
cargo run --bin fc-platform-server
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_API_PORT` | `3000` | HTTP API port |
| `FC_METRICS_PORT` | `9090` | Metrics/health port |
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `FC_DEV_MODE` | `false` | Enable dev data seeding |
| `FC_STATIC_DIR` | - | Path to built frontend |
| `RUST_LOG` | `info` | Log level filter |

Plus all Auth / JWT variables listed under fc-server.

---

## fc-router-bin (Message Router)

Consumes messages from SQS and routes them through processing pools. Supports active/standby HA with Redis leader election and optional ALB integration.

### Build & Run

```sh
just build-router

# Production (config from URL):
FLOWCATALYST_CONFIG_URL=http://config-service/api/router/config \
  cargo run --bin fc-router-bin

# Dev mode (LocalStack):
FLOWCATALYST_DEV_MODE=true cargo run --bin fc-router-bin

# With ALB:
cargo build --bin fc-router-bin --features alb
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `8080` | HTTP API port |
| `FLOWCATALYST_CONFIG_URL` | - | Router configuration URL (required in production) |
| `FLOWCATALYST_DEV_MODE` | `false` | Use built-in LocalStack config |
| `LOCALSTACK_ENDPOINT` | `http://localhost:4566` | LocalStack SQS endpoint (dev mode) |
| `LOCALSTACK_SQS_HOST` | `http://sqs.eu-west-1.localhost.localstack.cloud:4566` | SQS host override (dev) |
| `FLOWCATALYST_CONFIG_INTERVAL` | `300` | Config sync interval (seconds) |
| `RUST_LOG` | `info` | Log level filter |

#### Standby / HA

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_STANDBY_ENABLED` | `false` | Enable Redis leader election |
| `FLOWCATALYST_STANDBY_REDIS_URL` | `redis://127.0.0.1:6379` | Redis URL |
| `FLOWCATALYST_STANDBY_LOCK_KEY` | `fc:router:leader` | Redis lock key |
| `FLOWCATALYST_STANDBY_LOCK_TTL` | `30` | Lock TTL (seconds) |
| `FLOWCATALYST_STANDBY_HEARTBEAT_INTERVAL` | `10` | Heartbeat interval (seconds) |
| `FLOWCATALYST_INSTANCE_ID` | hostname | Instance identifier |

#### Notifications

| Variable | Default | Description |
|----------|---------|-------------|
| `NOTIFICATION_TEAMS_ENABLED` | `false` | Enable Teams webhook notifications |
| `NOTIFICATION_TEAMS_WEBHOOK_URL` | - | Teams webhook URL |
| `NOTIFICATION_MIN_SEVERITY` | `WARN` | Minimum severity: INFO, WARN, ERROR, CRITICAL |
| `NOTIFICATION_BATCH_INTERVAL` | `300` | Batch interval (seconds) |

---

## fc-scheduler-server (Dispatch Scheduler)

Polls for pending dispatch jobs and publishes them to processing queues.

### Build & Run

```sh
cargo build --bin fc-scheduler-server
cargo run --bin fc-scheduler-server
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `FC_SCHEDULER_MAX_CONCURRENT_GROUPS` | `10` | Max concurrent message groups |
| `FC_SCHEDULER_DEFAULT_POOL_CODE` | `DISPATCH-POOL` | Default dispatch pool code |
| `FC_SCHEDULER_PROCESSING_ENDPOINT` | `http://localhost:{http_port}/api/dispatch/process` | Processing endpoint URL |
| `RUST_LOG` | `info` | Log level filter |

Also reads `config.toml` / `config.yaml` via `fc-config::AppConfig` for `scheduler.enabled`, `scheduler.poll_interval_ms`, `scheduler.batch_size`, etc.

---

## fc-stream-processor (CQRS Projections)

Polls PostgreSQL projection feed tables and projects rows into read-model tables.

### Build & Run

```sh
just build-stream
cargo run --bin fc-stream-processor
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_DATABASE_URL` | `postgresql://localhost:5432/flowcatalyst` | PostgreSQL connection URL |
| `FC_STREAM_EVENTS_ENABLED` | `true` | Enable event projection |
| `FC_STREAM_EVENTS_BATCH_SIZE` | `100` | Event projection batch size |
| `FC_STREAM_DISPATCH_JOBS_ENABLED` | `true` | Enable dispatch job projection |
| `FC_STREAM_DISPATCH_JOBS_BATCH_SIZE` | `100` | Dispatch job projection batch size |
| `FC_METRICS_PORT` | `9090` | Metrics/health port |
| `RUST_LOG` | `info` | Log level filter |

---

## fc-outbox-processor (Outbox Relay)

Reads messages from application database outbox tables and dispatches them to the FlowCatalyst HTTP API with message group ordering. Supports SQLite, PostgreSQL, and MongoDB backends.

### Build & Run

```sh
just build-outbox

# PostgreSQL outbox:
FC_OUTBOX_DB_TYPE=postgres \
FC_OUTBOX_DB_URL=postgresql://localhost:5432/myapp \
FC_API_BASE_URL=http://localhost:8080 \
  cargo run --bin fc-outbox-processor

# SQLite outbox:
FC_OUTBOX_DB_TYPE=sqlite \
FC_OUTBOX_DB_URL=sqlite:./outbox.db \
  cargo run --bin fc-outbox-processor
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `FC_OUTBOX_DB_TYPE` | `postgres` | Database type: `sqlite`, `postgres`, `mongo` |
| `FC_OUTBOX_DB_URL` | - | Database connection URL (required) |
| `FC_OUTBOX_MONGO_DB` | `flowcatalyst` | MongoDB database name (mongo only) |
| `FC_OUTBOX_EVENTS_TABLE` | `outbox_messages` | Events outbox table name |
| `FC_OUTBOX_DISPATCH_JOBS_TABLE` | `outbox_messages` | Dispatch jobs outbox table name |
| `FC_OUTBOX_AUDIT_LOGS_TABLE` | `outbox_messages` | Audit logs outbox table name |
| `FC_OUTBOX_POLL_INTERVAL_MS` | `1000` | Poll interval in ms |
| `FC_OUTBOX_BATCH_SIZE` | `500` | Max items per poll |
| `FC_API_BASE_URL` | `http://localhost:8080` | FlowCatalyst API URL |
| `FC_API_TOKEN` | - | API bearer token |
| `FC_API_BATCH_SIZE` | `100` | Items per API call |
| `FC_MAX_IN_FLIGHT` | `5000` | Max concurrent items |
| `FC_GLOBAL_BUFFER_SIZE` | `1000` | Buffer capacity |
| `FC_MAX_CONCURRENT_GROUPS` | `10` | Max concurrent message groups |
| `FC_METRICS_PORT` | `9090` | Metrics/health port |
| `RUST_LOG` | `info` | Log level filter |

---

## Deployment Patterns

### Single Instance (simplest)

Run everything in one process:

```sh
FC_PLATFORM_ENABLED=true \
FC_ROUTER_ENABLED=true \
FC_SCHEDULER_ENABLED=true \
FC_STREAM_PROCESSOR_ENABLED=true \
  cargo run --bin fc-server
```

### Active/Standby HA (two instances)

Both instances run the same command. Only the leader processes background work:

```sh
FC_PLATFORM_ENABLED=true \
FC_ROUTER_ENABLED=true \
FC_SCHEDULER_ENABLED=true \
FC_STREAM_PROCESSOR_ENABLED=true \
FC_STANDBY_ENABLED=true \
FC_STANDBY_REDIS_URL=redis://redis:6379 \
  cargo run --bin fc-server
```

### Split Services (scale independently)

```sh
# API tier (multiple instances behind load balancer):
FC_PLATFORM_ENABLED=true cargo run --bin fc-server

# Background processor (single active instance):
FC_PLATFORM_ENABLED=false \
FC_ROUTER_ENABLED=true \
FC_SCHEDULER_ENABLED=true \
FC_STREAM_PROCESSOR_ENABLED=true \
FC_STANDBY_ENABLED=true \
FC_STANDBY_REDIS_URL=redis://redis:6379 \
  cargo run --bin fc-server
```

### Individual Binaries (maximum separation)

Use the standalone binaries when each service needs its own deployment, scaling, or resource limits:

```sh
cargo run --bin fc-platform-server   # API
cargo run --bin fc-router-bin        # Router
cargo run --bin fc-scheduler-server  # Scheduler
cargo run --bin fc-stream-processor  # Stream
cargo run --bin fc-outbox-processor  # Outbox
```

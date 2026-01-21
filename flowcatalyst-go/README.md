# FlowCatalyst Go

A high-performance event-driven message routing platform, rewritten in Go for improved developer experience and cross-platform deployment.

## Features

- **Single Binary Deployment**: Cross-platform builds for macOS, Linux, and Windows (amd64/arm64)
- **Embedded NATS JetStream**: Zero-configuration dev mode with persistent message queue
- **MongoDB Change Streams**: Real-time event projections for efficient querying
- **Per-Group Message Ordering**: FIFO guarantees within message groups
- **Circuit Breaker**: Resilient HTTP webhook delivery with automatic recovery
- **Rate Limiting**: Per-pool rate limits to protect downstream systems
- **Multi-Tenant**: Full client isolation with RBAC

## Quick Start

### Prerequisites

- Go 1.23+
- MongoDB 6.0+ (replica set required for change streams)
- Docker (optional, for MongoDB)

### Development Mode

```bash
# Start MongoDB replica set
docker run -d --name mongo -p 27017:27017 mongo:7 --replSet rs0
docker exec mongo mongosh --eval "rs.initiate()"

# Run FlowCatalyst in dev mode
make dev
```

The server starts at `http://localhost:8080` with:
- Embedded NATS JetStream on port 4222
- Data persisted to `./data/`
- Console logging enabled

### Build

```bash
# Build development binary (all-in-one)
make build

# Build production binaries (separate microservices)
make build-production

# Build for all platforms
make build-all

# View binary sizes
make sizes
```

### Production Binaries

For production deployments, use separate binaries for independent scaling:

| Binary | Purpose | Port |
|--------|---------|------|
| `fc-platform` | Platform API, Auth, Admin | 8080 |
| `fc-router` | Message Router (NATS/SQS → HTTP) | 8083 |
| `fc-stream` | Stream Processor (Change Streams) | 8082 |
| `fc-outbox` | Outbox Processor (DB → API) | 8084 |

```bash
# Build all production binaries
make build-production

# Or build individually
make build-platform
make build-router
make build-stream
make build-outbox
```

### Development with UI

From the `packages/platform-ui-vue` directory:

```bash
# Start everything (DB + Go backend + Stream + UI)
npm run dev:go:full

# Or just the Go services (if DB already running)
npm run dev:go
```

## Configuration

All configuration is via environment variables.

> **Full documentation**: See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for complete configuration reference for all binaries.

### HTTP Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | HTTP server port |
| `CORS_ORIGINS` | `http://localhost:4200` | Allowed CORS origins (comma-separated) |

### MongoDB

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGODB_URI` | `mongodb://localhost:27017/?replicaSet=rs0` | MongoDB connection URI |
| `MONGODB_DATABASE` | `flowcatalyst` | Database name |

### Queue

| Variable | Default | Description |
|----------|---------|-------------|
| `QUEUE_TYPE` | `embedded` | Queue type: `embedded`, `nats`, or `sqs` |
| `NATS_URL` | `nats://localhost:4222` | NATS server URL (for `nats` type) |
| `NATS_DATA_DIR` | `./data/nats` | Data directory for embedded NATS |
| `SQS_QUEUE_URL` | | AWS SQS queue URL (for `sqs` type) |
| `AWS_REGION` | `us-east-1` | AWS region for SQS |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_MODE` | `embedded` | Auth mode: `embedded` or `remote` |
| `AUTH_EXTERNAL_BASE_URL` | `http://localhost:4200` | External base URL for OAuth callbacks |
| `JWT_ISSUER` | `flowcatalyst` | JWT issuer claim |
| `SESSION_COOKIE_NAME` | `FLOWCATALYST_SESSION` | Session cookie name |
| `SESSION_SECURE` | `true` | Use secure cookies |

## API Endpoints

### Health

- `GET /q/health` - Combined health status
- `GET /q/health/live` - Liveness probe
- `GET /q/health/ready` - Readiness probe (checks MongoDB, NATS)

### Events

```bash
# Create event
POST /api/events
{
  "type": "order.created",
  "source": "order-service",
  "data": "{\"orderId\": \"123\"}"
}

# Get event
GET /api/events/{id}

# Batch create (max 100)
POST /api/events/batch
{
  "events": [...]
}
```

### Subscriptions

```bash
# List subscriptions
GET /api/subscriptions

# Create subscription
POST /api/subscriptions
{
  "name": "Order Notifications",
  "eventTypes": ["order.created", "order.updated"],
  "targetUrl": "https://webhook.example.com/orders",
  "dispatchPoolId": "default"
}

# Pause/Resume
POST /api/subscriptions/{id}/pause
POST /api/subscriptions/{id}/resume
```

### Dispatch Jobs

```bash
# Create dispatch job
POST /api/dispatch/jobs
{
  "eventId": "...",
  "subscriptionId": "...",
  "targetUrl": "https://webhook.example.com",
  "payload": "{...}"
}

# Get job status
GET /api/dispatch/jobs/{id}

# Get delivery attempts
GET /api/dispatch/jobs/{id}/attempts
```

### BFF (Backend for Frontend)

Optimized read endpoints using MongoDB projections:

```bash
# Search events with filters
GET /api/bff/events?clientId=...&type=order.created&page=0&size=20

# Get filter options (for cascading dropdowns)
GET /api/bff/events/filter-options

# Search dispatch jobs
GET /api/bff/dispatch-jobs?status=COMPLETED&page=0&size=20
```

### Admin APIs

Admin endpoints under `/api/admin/platform/`:

- `/clients` - Client management
- `/principals` - User management
- `/roles` - Role management
- `/applications` - Application registry
- `/service-accounts` - Service account management

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         HTTP Server                              │
│  (Chi Router, JWT Auth, CORS, Rate Limiting)                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │   Platform   │  │   Stream     │  │   Dispatch           │   │
│  │   APIs       │  │   Processor  │  │   Scheduler          │   │
│  │              │  │              │  │                      │   │
│  │  - Events    │  │  - Change    │  │  - Job Polling       │   │
│  │  - Subs      │  │    Streams   │  │  - Stale Recovery    │   │
│  │  - Jobs      │  │  - Projec-   │  │  - Queue Dispatch    │   │
│  │  - Admin     │  │    tions     │  │                      │   │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────┘   │
│         │                  │                      │              │
│         ▼                  ▼                      ▼              │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                      MongoDB                             │    │
│  │  (Events, Dispatch Jobs, Subscriptions, Auth)           │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│                           │                                      │
│                           ▼                                      │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                   Message Router                         │    │
│  │                                                          │    │
│  │  ┌─────────────────────────────────────────────────┐    │    │
│  │  │              Queue Manager                       │    │    │
│  │  │  - Deduplication  - Pool Routing                │    │    │
│  │  └─────────────────────────────────────────────────┘    │    │
│  │                         │                                │    │
│  │  ┌──────────┐  ┌──────────┐  ┌──────────┐              │    │
│  │  │  Pool A  │  │  Pool B  │  │  Pool C  │  ...         │    │
│  │  │          │  │          │  │          │              │    │
│  │  │ group-1  │  │ group-1  │  │ group-1  │              │    │
│  │  │ group-2  │  │ group-2  │  │ group-2  │              │    │
│  │  │ group-3  │  │ group-3  │  │ group-3  │              │    │
│  │  └──────────┘  └──────────┘  └──────────┘              │    │
│  │                         │                                │    │
│  │  ┌─────────────────────────────────────────────────┐    │    │
│  │  │              HTTP Mediator                       │    │    │
│  │  │  - Retries  - Circuit Breaker  - Timeouts       │    │    │
│  │  └─────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
│                           │                                      │
│                           ▼                                      │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │           NATS JetStream / AWS SQS                       │    │
│  │           (Message Queue)                                │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

## Message Router Semantics

The message router implements per-group FIFO processing:

1. **One goroutine per message group**: Messages within the same group are processed sequentially
2. **Pool-level concurrency**: Semaphore limits total concurrent HTTP calls per pool
3. **Rate limiting**: Checked before semaphore acquisition
4. **Batch+Group FIFO**: If a message fails, all subsequent messages in the same batch+group are auto-nacked

### Retry Logic

- Max 3 attempts
- Backoff: `attempt × 1 second`
- Retryable: Connection errors, 5xx responses, timeouts
- Non-retryable: 4xx responses (except 429)

### Response Handling

| Status | `ack` field | Action |
|--------|-------------|--------|
| 2xx | `true` or missing | ACK, success |
| 2xx | `false` | NACK with delay |
| 4xx | - | ACK, config error |
| 429 | - | NACK with delay |
| 5xx | - | NACK, retry |

## Development

```bash
# Run tests
make test

# Run with coverage
make test-coverage

# Lint
make lint

# Format code
make fmt
```

## Project Structure

```
flowcatalyst-go/
├── cmd/flowcatalyst/       # Main entry point
├── internal/
│   ├── common/             # Shared utilities
│   │   ├── health/         # Health checks
│   │   └── tsid/           # TSID generator
│   ├── config/             # Configuration
│   ├── platform/           # Platform service
│   │   ├── api/            # HTTP handlers
│   │   ├── auth/           # Authentication
│   │   ├── client/         # Client management
│   │   ├── event/          # Events
│   │   ├── principal/      # Users
│   │   └── ...             # Other entities
│   ├── queue/              # Queue abstraction
│   │   ├── nats/           # NATS implementation
│   │   └── sqs/            # SQS implementation
│   ├── router/             # Message router
│   │   ├── manager/        # Queue manager
│   │   ├── mediator/       # HTTP mediator
│   │   └── pool/           # Processing pools
│   ├── scheduler/          # Dispatch scheduler
│   └── stream/             # Stream processor
├── Makefile
└── go.mod
```

## License

Proprietary - FlowCatalyst

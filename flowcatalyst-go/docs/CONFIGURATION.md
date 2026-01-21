# FlowCatalyst Go - Configuration Guide

This document describes all binaries and their configuration options.

## Binaries Overview

| Binary | Purpose | Use Case |
|--------|---------|----------|
| `flowcatalyst` | All-in-one development binary | Local development with embedded NATS |
| `fc-platform` | Platform API | Production control plane |
| `fc-router` | Message Router | Production message processing |
| `fc-stream` | Stream Processor | Production change stream processing |
| `fc-outbox` | Outbox Processor | Production outbox pattern processing |

---

## Development Binary: `flowcatalyst`

**Purpose**: Combined binary for local development. Includes embedded NATS, all APIs, stream processor, and scheduler.

**Build**: `make build-dev`

**Run**: `make dev` or `./bin/flowcatalyst`

### Environment Variables

All environment variables from all components apply. Key ones:

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_DEV` | `false` | Enable console logging |
| `QUEUE_TYPE` | `embedded` | Use `embedded` for built-in NATS |

---

## Production Binary: `fc-platform`

**Purpose**: Control plane API server. Handles all REST APIs, authentication, admin functions.

**Build**: `make build-platform`

**Run**: `./bin/fc-platform`

### Environment Variables

#### HTTP Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | HTTP server port |
| `CORS_ORIGINS` | `http://localhost:4200` | Comma-separated allowed origins |

#### MongoDB

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGODB_URI` | `mongodb://localhost:27017/?replicaSet=rs0&directConnection=true` | MongoDB connection string |
| `MONGODB_DATABASE` | `flowcatalyst` | Database name |

#### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_MODE` | `embedded` | `embedded` or `remote` |
| `AUTH_EXTERNAL_BASE_URL` | `http://localhost:4200` | External URL for OAuth callbacks |
| `JWT_ISSUER` | `flowcatalyst` | JWT issuer claim |
| `JWT_PRIVATE_KEY_PATH` | `` | Path to RSA private key (auto-generated if empty) |
| `JWT_PUBLIC_KEY_PATH` | `` | Path to RSA public key |
| `JWT_ACCESS_TOKEN_EXPIRY` | `1h` | Access token lifetime |
| `JWT_SESSION_TOKEN_EXPIRY` | `8h` | Session token lifetime |
| `JWT_REFRESH_TOKEN_EXPIRY` | `720h` | Refresh token lifetime (30 days) |
| `JWT_AUTHORIZATION_CODE_EXPIRY` | `10m` | OAuth authorization code lifetime |

#### Session

| Variable | Default | Description |
|----------|---------|-------------|
| `SESSION_COOKIE_NAME` | `FLOWCATALYST_SESSION` | Session cookie name |
| `SESSION_SECURE` | `true` | Require HTTPS for cookies |
| `SESSION_SAME_SITE` | `Strict` | SameSite cookie policy |

#### PKCE

| Variable | Default | Description |
|----------|---------|-------------|
| `PKCE_REQUIRED` | `true` | Require PKCE for OAuth flows |

#### Remote Auth (when AUTH_MODE=remote)

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_REMOTE_JWKS_URL` | `` | JWKS endpoint URL |
| `AUTH_REMOTE_ISSUER` | `` | Expected issuer |

#### General

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_DIR` | `./data` | Data directory for keys, etc. |
| `FLOWCATALYST_DEV` | `false` | Enable console logging |

### Endpoints

- `GET /q/health` - Health check
- `GET /q/health/live` - Liveness probe
- `GET /q/health/ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /swagger/*` - Swagger UI
- `GET /.well-known/openid-configuration` - OIDC discovery
- `GET /.well-known/jwks.json` - JWKS endpoint

---

## Production Binary: `fc-router`

**Purpose**: Message router. Consumes from queue (NATS/SQS), delivers via HTTP mediation to webhooks.

**Build**: `make build-router`

**Run**: `./bin/fc-router`

### Environment Variables

#### HTTP Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | HTTP server port (health/metrics only) |

#### Queue (choose one type)

| Variable | Default | Description |
|----------|---------|-------------|
| `QUEUE_TYPE` | `embedded` | `nats` or `sqs` (embedded not supported) |

##### NATS Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `NATS_URL` | `nats://localhost:4222` | NATS server URL |

##### SQS Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `SQS_QUEUE_URL` | `` | SQS queue URL (required) |
| `AWS_REGION` | `us-east-1` | AWS region |
| `SQS_WAIT_TIME_SECONDS` | `20` | Long polling wait time |
| `SQS_VISIBILITY_TIMEOUT` | `120` | Message visibility timeout |

#### Leader Election (for HA)

| Variable | Default | Description |
|----------|---------|-------------|
| `LEADER_ELECTION_ENABLED` | `false` | Enable Redis-based leader election |
| `HOSTNAME` | `` | Instance ID (defaults to hostname) |
| `LEADER_TTL` | `30s` | Lock TTL |
| `LEADER_REFRESH_INTERVAL` | `10s` | Lock refresh interval |

#### General

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_DEV` | `false` | Enable console logging |

### Endpoints

- `GET /q/health` - Health check
- `GET /q/health/live` - Liveness probe
- `GET /q/health/ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /router/status` - Router status (role, instance ID)

---

## Production Binary: `fc-stream`

**Purpose**: Stream processor. Watches MongoDB change streams, builds read-model projections.

**Build**: `make build-stream`

**Run**: `./bin/fc-stream`

### Environment Variables

#### HTTP Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | HTTP server port (health/metrics only) |

#### MongoDB

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGODB_URI` | `mongodb://localhost:27017/?replicaSet=rs0&directConnection=true` | MongoDB connection string (must be replica set) |
| `MONGODB_DATABASE` | `flowcatalyst` | Database name |

#### General

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_DEV` | `false` | Enable console logging |

### Endpoints

- `GET /q/health` - Health check
- `GET /q/health/live` - Liveness probe
- `GET /q/health/ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /stream/status` - Stream processor status

### Requirements

- MongoDB must be configured as a replica set (required for change streams)

---

## Production Binary: `fc-outbox`

**Purpose**: Outbox processor. Polls outbox tables in MongoDB, sends batches to Platform API.

**Build**: `make build-outbox`

**Run**: `./bin/fc-outbox`

### Environment Variables

#### HTTP Server

| Variable | Default | Description |
|----------|---------|-------------|
| `HTTP_PORT` | `8080` | HTTP server port (health/metrics only) |

#### MongoDB

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGODB_URI` | `mongodb://localhost:27017/?replicaSet=rs0&directConnection=true` | MongoDB connection string |
| `MONGODB_DATABASE` | `flowcatalyst` | Database name |

#### Outbox API Client

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTBOX_API_BASE_URL` | `http://localhost:8080` | Platform API base URL |
| `OUTBOX_API_AUTH_TOKEN` | `` | Bearer token for API auth |

#### Processor Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `OUTBOX_POLL_INTERVAL` | `1s` | How often to poll for items |
| `OUTBOX_POLL_BATCH_SIZE` | `500` | Max items per poll |
| `OUTBOX_MAX_RETRIES` | `3` | Max retries before marking failed |

#### Leader Election (for HA)

| Variable | Default | Description |
|----------|---------|-------------|
| `LEADER_ELECTION_ENABLED` | `false` | Enable leader election |
| `HOSTNAME` | `` | Instance ID |
| `LEADER_TTL` | `30s` | Lock TTL |
| `LEADER_REFRESH_INTERVAL` | `10s` | Lock refresh interval |

#### General

| Variable | Default | Description |
|----------|---------|-------------|
| `FLOWCATALYST_DEV` | `false` | Enable console logging |

### Endpoints

- `GET /q/health` - Health check
- `GET /q/health/live` - Liveness probe
- `GET /q/health/ready` - Readiness probe
- `GET /metrics` - Prometheus metrics
- `GET /outbox/status` - Outbox processor status

---

## Deployment Examples

### Local Development

```bash
# Start MongoDB
make dev-db

# Run all-in-one binary
make dev

# Or run separate services
make dev-platform  # Terminal 1
make dev-stream    # Terminal 2
```

### Production (Docker Compose)

```yaml
version: '3.8'
services:
  platform:
    image: flowcatalyst/platform:latest
    environment:
      - HTTP_PORT=8080
      - MONGODB_URI=mongodb://mongo:27017/?replicaSet=rs0
      - MONGODB_DATABASE=flowcatalyst
    ports:
      - "8080:8080"

  stream:
    image: flowcatalyst/stream:latest
    environment:
      - HTTP_PORT=8082
      - MONGODB_URI=mongodb://mongo:27017/?replicaSet=rs0
      - MONGODB_DATABASE=flowcatalyst

  router:
    image: flowcatalyst/router:latest
    environment:
      - HTTP_PORT=8083
      - QUEUE_TYPE=nats
      - NATS_URL=nats://nats:4222

  outbox:
    image: flowcatalyst/outbox:latest
    environment:
      - HTTP_PORT=8084
      - MONGODB_URI=mongodb://mongo:27017/?replicaSet=rs0
      - OUTBOX_API_BASE_URL=http://platform:8080
```

### Production (Kubernetes)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fc-platform
spec:
  replicas: 2
  template:
    spec:
      containers:
      - name: platform
        image: flowcatalyst/platform:latest
        env:
        - name: HTTP_PORT
          value: "8080"
        - name: MONGODB_URI
          valueFrom:
            secretKeyRef:
              name: mongodb
              key: uri
        ports:
        - containerPort: 8080
        livenessProbe:
          httpGet:
            path: /q/health/live
            port: 8080
        readinessProbe:
          httpGet:
            path: /q/health/ready
            port: 8080
```

### High Availability (with Leader Election)

For `fc-router` and `fc-outbox`, enable leader election to run multiple replicas:

```yaml
environment:
  - LEADER_ELECTION_ENABLED=true
  - REDIS_URL=redis://redis:6379
```

Only the primary instance will process messages; standbys are ready to take over.

---

## Health Endpoints

All binaries expose Quarkus-compatible health endpoints:

| Endpoint | Purpose | Use Case |
|----------|---------|----------|
| `/q/health` | Full health status | General health check |
| `/q/health/live` | Liveness | Kubernetes liveness probe |
| `/q/health/ready` | Readiness | Kubernetes readiness probe |
| `/metrics` | Prometheus metrics | Monitoring |

# Enums Reference

Complete reference for all domain enumerations in FlowCatalyst.

## Authentication & Authorization

### AuthMode

Auth deployment mode for the platform.

| Value | Description |
|-------|-------------|
| `EMBEDDED` | Full IdP mode - handles token issuance, user auth, OAuth2/OIDC endpoints |
| `REMOTE` | Token validation only - validates tokens using remote JWKS |

**Location**: `tech.flowcatalyst.platform.authentication.AuthMode`

### AuthProvider

Authentication provider types for email domains.

| Value | Description |
|-------|-------------|
| `INTERNAL` | Internal authentication using username/password stored in FlowCatalyst |
| `OIDC` | External OIDC authentication via external Identity Provider |

**Location**: `tech.flowcatalyst.platform.authentication.AuthProvider`

### IdpType

Type of Identity Provider used for authentication.

| Value | Description |
|-------|-------------|
| `INTERNAL` | Internal username/password authentication |
| `OIDC` | OIDC authentication (e.g., Keycloak, Okta) |

**Location**: `tech.flowcatalyst.platform.authentication.IdpType`

### AuthConfigType

Type of authentication configuration, determining user scope.

| Value | Description | User Scope |
|-------|-------------|------------|
| `ANCHOR` | Platform-wide configuration | `ANCHOR` (all clients) |
| `PARTNER` | Partner configuration with granted clients | `PARTNER` |
| `CLIENT` | Client-specific configuration | `CLIENT` (single client) |

**Location**: `tech.flowcatalyst.platform.client.AuthConfigType`

## Principal & User

### PrincipalType

Type of principal in the system.

| Value | Description |
|-------|-------------|
| `USER` | Human user account |
| `SERVICE` | Service account for machine-to-machine authentication |

**Location**: `tech.flowcatalyst.platform.principal.PrincipalType`

### UserScope

Access scope for a user principal.

| Value | Description | Client Access |
|-------|-------------|---------------|
| `ANCHOR` | Platform admin users | All clients |
| `PARTNER` | Partner users | Multiple assigned clients |
| `CLIENT` | Client-bound users | Single client only |

**Location**: `tech.flowcatalyst.platform.principal.UserScope`

## Client

### ClientStatus

Status of a client organization.

| Value | Description |
|-------|-------------|
| `ACTIVE` | Client is active and operational |
| `INACTIVE` | Client is inactive (see statusReason) |
| `SUSPENDED` | Client is temporarily disabled |

**Location**: `tech.flowcatalyst.platform.client.ClientStatus`

## Event Types

### EventTypeStatus

Status of an EventType.

| Value | Description |
|-------|-------------|
| `CURRENT` | EventType is active, new events can be created |
| `ARCHIVE` | EventType is archived, no new events allowed |

**Location**: `tech.flowcatalyst.eventtype.EventTypeStatus`

### SpecVersionStatus

Status of a schema spec version.

| Value | Description |
|-------|-------------|
| `FINALISING` | Schema being finalized, can be modified |
| `CURRENT` | Active version for its major version line |
| `DEPRECATED` | Superseded by newer version, still valid for reading |

**Location**: `tech.flowcatalyst.eventtype.SpecVersionStatus`

### SchemaType

Type of schema definition.

| Value | Description |
|-------|-------------|
| `JSON_SCHEMA` | JSON Schema (draft-07 or later) |
| `PROTO` | Protocol Buffers schema |
| `XSD` | XML Schema Definition |

**Location**: `tech.flowcatalyst.eventtype.SchemaType`

## Subscription

### SubscriptionStatus

Status of a subscription.

| Value | Description |
|-------|-------------|
| `ACTIVE` | Subscription creates dispatch jobs for matching events |
| `PAUSED` | Subscription does not create dispatch jobs |

**Location**: `tech.flowcatalyst.subscription.SubscriptionStatus`

### SubscriptionSource

Source of a subscription - how it was created.

| Value | Description |
|-------|-------------|
| `API` | Created or synced via SDK/API |
| `UI` | Created via the user interface |

**Location**: `tech.flowcatalyst.subscription.SubscriptionSource`

## Dispatch Pool

### DispatchPoolStatus

Status of a dispatch pool.

| Value | Description |
|-------|-------------|
| `ACTIVE` | Pool is active and can process dispatch jobs |
| `SUSPENDED` | Pool is temporarily suspended |
| `ARCHIVED` | Pool is archived (soft-deleted) |

**Location**: `tech.flowcatalyst.dispatchpool.DispatchPoolStatus`

## Dispatch Job

### DispatchKind

Category of a dispatch job.

| Value | Description | Code Contains |
|-------|-------------|---------------|
| `EVENT` | Event dispatch to subscribers | Event type (e.g., `order.created`) |
| `TASK` | Asynchronous task execution | Task code (e.g., `send-welcome-email`) |

**Location**: `tech.flowcatalyst.dispatchjob.model.DispatchKind`

### DispatchStatus

Status of a dispatch job through its lifecycle.

| Value | Description |
|-------|-------------|
| `PENDING` | Created, waiting to be queued |
| `QUEUED` | Message pointer is on the queue |
| `IN_PROGRESS` | Being processed by endpoint |
| `COMPLETED` | Successfully delivered |
| `ERROR` | Retries exhausted or permanent failure |
| `CANCELLED` | Manually cancelled |

**Lifecycle**:
```
PENDING → QUEUED → IN_PROGRESS → COMPLETED
                 ↓             ↓
             (retry)      ERROR (retries exceeded)
                 ↓
              QUEUED

Any state → CANCELLED (manual intervention)
```

**Location**: `tech.flowcatalyst.dispatchjob.model.DispatchStatus`

### DispatchMode

Processing mode for dispatch jobs.

| Value | Description | Message Group Behavior |
|-------|-------------|------------------------|
| `IMMEDIATE` | No ordering guarantees | Ignored |
| `NEXT_ON_ERROR` | Order with error skip | Continue on error |
| `BLOCK_ON_ERROR` | Order with error blocking | Stop group on error |

**Location**: `tech.flowcatalyst.dispatch.DispatchMode`

### DispatchProtocol

Delivery protocol for dispatch jobs.

| Value | Description |
|-------|-------------|
| `HTTP_WEBHOOK` | HTTP webhook delivery |
| `GRPC` | gRPC delivery |
| `AWS_SQS` | AWS SQS queue |
| `AWS_SNS` | AWS SNS topic |
| `KAFKA` | Apache Kafka topic |
| `RABBITMQ` | RabbitMQ queue |

**Location**: `tech.flowcatalyst.dispatchjob.model.DispatchProtocol`

### DispatchAttemptStatus

Status of a dispatch attempt.

| Value | Description |
|-------|-------------|
| `SUCCESS` | Webhook delivered successfully |
| `FAILURE` | Webhook delivery failed |
| `TIMEOUT` | Target did not respond within timeout |
| `CIRCUIT_OPEN` | Circuit breaker prevented attempt |

**Location**: `tech.flowcatalyst.dispatchjob.model.DispatchAttemptStatus`

### ErrorType

Classification of errors for dispatch job processing.

| Value | Description | Retry Behavior |
|-------|-------------|----------------|
| `TRANSIENT` | Temporary failure | Retry appropriate |
| `NOT_TRANSIENT` | Permanent failure | Do not retry |
| `UNKNOWN` | Cannot determine | Default to retry |

**Location**: `tech.flowcatalyst.dispatchjob.model.ErrorType`

### MediationType

Supported mediation types for message delivery.

| Value | Description |
|-------|-------------|
| `HTTP` | HTTP-based mediation to external webhooks |

**Location**: `tech.flowcatalyst.dispatchjob.model.MediationType`

### MediationResult

Result of a mediation attempt.

| Value | Description | Action |
|-------|-------------|--------|
| `SUCCESS` | 200 OK - processed successfully | ACK |
| `ERROR_CONNECTION` | Connection timeout/refused | NACK (retry) |
| `ERROR_PROCESS` | 400, 502-599 errors | NACK (retry) |
| `ERROR_CONFIG` | 401, 403, 404, 501 errors | ACK (no retry) |

**Location**: `tech.flowcatalyst.dispatchjob.model.MediationResult`

## Service Account

### WebhookAuthType

Authentication type for webhook credentials.

| Value | Description | Header Format |
|-------|-------------|---------------|
| `BEARER` | Bearer token authentication | `Authorization: Bearer {token}` |
| `BASIC` | Basic authentication | `Authorization: Basic {base64}` |

**Location**: `tech.flowcatalyst.serviceaccount.entity.WebhookAuthType`

## See Also

- [Entity Documentation](../entities/overview.md) - Entity reference
- [Dispatch Jobs Guide](../guides/dispatch-jobs.md) - Dispatch workflow
- [Authentication Guide](../guides/authentication.md) - Auth system

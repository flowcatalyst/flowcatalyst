# Dispatch Entities

This document describes the dispatch job and service account entities that power webhook delivery.

## DispatchJob

**Collection**: `dispatch_jobs`

Webhook delivery jobs with full attempt tracking.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `externalId` | String | External reference ID |
| **Classification** | | |
| `source` | String | Origin system |
| `kind` | DispatchKind | `EVENT` or `TASK` |
| `code` | String | Event type or task code |
| `subject` | String | Aggregate reference |
| `eventId` | String | Source event ID |
| `correlationId` | String | Request correlation |
| `metadata` | List\<DispatchJobMetadata\> | Key-value metadata |
| **Target** | | |
| `targetUrl` | String | Webhook URL |
| `protocol` | DispatchProtocol | `HTTP_WEBHOOK` |
| `headers` | Map\<String, String\> | Custom headers |
| **Payload** | | |
| `payload` | String | Delivery content |
| `payloadContentType` | String | Content type |
| `dataOnly` | boolean | Raw payload vs envelope |
| **Credentials** | | |
| `serviceAccountId` | String | Service account for auth |
| **Context** | | |
| `clientId` | String | Owning client |
| `subscriptionId` | String | Source subscription |
| **Behavior** | | |
| `mode` | DispatchMode | Processing mode |
| `dispatchPoolId` | String | Rate limiting pool |
| `messageGroup` | String | FIFO ordering group |
| `sequence` | int | Order within group |
| `timeoutSeconds` | int | Delivery timeout |
| `schemaId` | String | Optional payload schema |
| **Execution** | | |
| `status` | DispatchStatus | Job status |
| `maxRetries` | Integer | Max retry attempts |
| `retryStrategy` | String | Retry algorithm |
| `scheduledFor` | Instant | Scheduled execution time |
| `expiresAt` | Instant | Expiration time |
| **Tracking** | | |
| `attemptCount` | Integer | Number of attempts |
| `lastAttemptAt` | Instant | Last attempt time |
| `completedAt` | Instant | Completion time |
| `durationMillis` | Long | Total duration |
| `lastError` | String | Last error message |
| `idempotencyKey` | String | Deduplication key |
| `attempts` | List\<DispatchAttempt\> | Attempt history |
| **Timestamps** | | |
| `createdAt` | Instant | Creation time |
| `updatedAt` | Instant | Last update time |

### DispatchKind Enum

```java
public enum DispatchKind {
    EVENT,  // Created from subscription match
    TASK    // Created directly via API
}
```

### DispatchStatus Enum

```java
public enum DispatchStatus {
    QUEUED,       // Sent to message queue (initial state for synchronous creation)
    PENDING,      // Waiting to be queued (fallback when queue send fails)
    IN_PROGRESS,  // Currently being processed
    COMPLETED,    // Successfully delivered
    ERROR,        // Failed (may retry)
    CANCELLED     // Manually cancelled
}
```

**Note**: Jobs are created with `QUEUED` status during synchronous event/job creation.
The `PENDING` status is only used when the initial queue submission fails, allowing
the safety net poller to recover the job.

### DispatchProtocol Enum

```java
public enum DispatchProtocol {
    HTTP_WEBHOOK  // HTTP POST delivery
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "externalId": null,
  "source": "ecommerce-api",
  "kind": "EVENT",
  "code": "ecommerce:orders:order:created",
  "subject": "order:12345",
  "eventId": "0HZXEQ5Y8JY00",
  "correlationId": "req-abc-123",
  "metadata": [
    {"id": "0HZXEQ5Y8JY11", "key": "customer_id", "value": "cust_789"}
  ],
  "targetUrl": "https://notifications.acme.com/webhooks/orders",
  "protocol": "HTTP_WEBHOOK",
  "headers": {
    "X-Custom-Header": "value"
  },
  "payload": "{\"orderId\": \"12345\", \"amount\": 99.99}",
  "payloadContentType": "application/json",
  "dataOnly": true,
  "serviceAccountId": "0HZXEQ5Y8JY22",
  "clientId": "0HZXEQ5Y8JY33",
  "subscriptionId": "0HZXEQ5Y8JY44",
  "mode": "NEXT_ON_ERROR",
  "dispatchPoolId": "0HZXEQ5Y8JY55",
  "messageGroup": "order-notifications:order:12345",
  "sequence": 99,
  "timeoutSeconds": 30,
  "schemaId": null,
  "status": "COMPLETED",
  "maxRetries": 3,
  "retryStrategy": "exponential",
  "scheduledFor": null,
  "expiresAt": "2024-01-16T10:30:00Z",
  "attemptCount": 1,
  "lastAttemptAt": "2024-01-15T10:30:05Z",
  "completedAt": "2024-01-15T10:30:05Z",
  "durationMillis": 150,
  "lastError": null,
  "idempotencyKey": "order-created-12345-sub-44",
  "attempts": [
    {
      "id": "0HZXEQ5Y8JY66",
      "attemptNumber": 1,
      "attemptedAt": "2024-01-15T10:30:05Z",
      "completedAt": "2024-01-15T10:30:05Z",
      "durationMillis": 150,
      "status": "SUCCESS",
      "responseCode": 200,
      "responseBody": "{\"received\": true}",
      "errorMessage": null,
      "errorStackTrace": null,
      "errorType": null,
      "createdAt": "2024-01-15T10:30:05Z"
    }
  ],
  "createdAt": "2024-01-15T10:30:00Z",
  "updatedAt": "2024-01-15T10:30:05Z"
}
```

---

## DispatchAttempt (Embedded)

Individual delivery attempt records.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Attempt ID |
| `attemptNumber` | int | Attempt sequence (1-based) |
| `attemptedAt` | Instant | When attempt started |
| `completedAt` | Instant | When attempt finished |
| `durationMillis` | Long | Attempt duration |
| `status` | DispatchAttemptStatus | Attempt result |
| `responseCode` | Integer | HTTP response code |
| `responseBody` | String | Response body (truncated) |
| `errorMessage` | String | Error message if failed |
| `errorStackTrace` | String | Stack trace if available |
| `errorType` | ErrorType | Error classification |
| `createdAt` | Instant | Record creation time |

### DispatchAttemptStatus Enum

```java
public enum DispatchAttemptStatus {
    SUCCESS,        // Delivery successful (2xx)
    CLIENT_ERROR,   // Client error (4xx, no retry)
    SERVER_ERROR,   // Server error (5xx, retryable)
    TIMEOUT,        // Request timed out
    CONNECTION_ERROR // Connection failed
}
```

### ErrorType Enum

```java
public enum ErrorType {
    TRANSIENT,    // Temporary, should retry
    PERMANENT,    // Won't succeed on retry
    UNKNOWN       // Unclassified
}
```

---

## DispatchJobMetadata (Embedded)

Key-value metadata for search and filtering.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Metadata entry ID |
| `key` | String | Metadata key |
| `value` | String | Metadata value |

### Usage

Index-friendly metadata for efficient queries:

```java
// Find jobs by customer
dispatchJobRepository.find(
    "metadata.key = ?1 and metadata.value = ?2",
    "customer_id", customerId
).list();
```

---

## ServiceAccount

**Collection**: `service_accounts`

Machine-to-machine credentials for webhook authentication.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `code` | String | Unique identifier |
| `name` | String | Display name |
| `description` | String | Account description |
| `clientIds` | List\<String\> | Accessible clients |
| `applicationId` | String | Associated application |
| `active` | boolean | Whether account is active |
| `webhookCredentials` | WebhookCredentials | Auth/signing credentials |
| `roles` | List\<RoleAssignment\> | Assigned roles |
| `lastUsedAt` | Instant | Last usage time |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "code": "acme-notifications",
  "name": "Acme Notifications Service",
  "description": "Service account for notification webhooks",
  "clientIds": ["0HZXEQ5Y8JY00"],
  "applicationId": null,
  "active": true,
  "webhookCredentials": {
    "authType": "BEARER",
    "authTokenRef": "aws-secretsmanager://acme-webhook-token",
    "signingSecretRef": "aws-secretsmanager://acme-signing-secret",
    "signingAlgorithm": "HMAC_SHA256",
    "createdAt": "2024-01-01T00:00:00Z",
    "regeneratedAt": null
  },
  "roles": [
    {
      "roleName": "webhook-sender",
      "assignmentSource": "MANUAL",
      "assignedAt": "2024-01-01T00:00:00Z"
    }
  ],
  "lastUsedAt": "2024-01-15T10:30:00Z",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## WebhookCredentials (Embedded)

Authentication and signing credentials for webhooks.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `authType` | WebhookAuthType | Auth method |
| `authTokenRef` | String | Secret reference for token |
| `signingSecretRef` | String | Secret reference for signing |
| `signingAlgorithm` | SignatureAlgorithm | Signing algorithm |
| `createdAt` | Instant | When credentials created |
| `regeneratedAt` | Instant | When last regenerated |

### WebhookAuthType Enum

```java
public enum WebhookAuthType {
    BEARER,  // Authorization: Bearer {token}
    BASIC    // Authorization: Basic {base64}
}
```

### SignatureAlgorithm Enum

```java
public enum SignatureAlgorithm {
    HMAC_SHA256  // HMAC-SHA256 signature
}
```

### Secret Reference Format

Secrets are stored externally and referenced by URI:

- `aws-secretsmanager://secret-name` - AWS Secrets Manager
- `aws-ssm://parameter-name` - AWS SSM Parameter Store
- `gcp-secretmanager://projects/x/secrets/y` - GCP Secret Manager
- `vault://path/to/secret` - HashiCorp Vault
- `encrypted://base64data` - Encrypted at rest

---

## Status Flow

```
              ┌─────────────────────────────────────┐
              │       Synchronous Creation          │
              │  (createEvent / createDispatchJob)  │
              └─────────────────────────────────────┘
                                │
                                ▼
              ┌─────────────────────────────────────┐
              │  QUEUED (normal path)               │
              │  └─► queue send fails → PENDING     │
              └─────────────────────────────────────┘
                                │
        ┌───────────────────────┴───────────────────┐
        ▼                                           ▼
  (message router                          (stale > 15 min)
   picks up)                                        │
        │                                           ▼
        ▼                                       PENDING
  IN_PROGRESS                                       │
        │                        (PendingJobPoller) │
  ┌─────┴─────┐                                     │
  ▼           ▼                                     ▼
COMPLETED   ERROR ◄─────────────────────────────────┘
              │              (safety net requeue)
              ▼
       (retry or CANCELLED)
```

### Status Transitions

| From | To | Trigger |
|------|-----|---------|
| (create) | QUEUED | Synchronous job creation with successful queue send |
| (create) | PENDING | Synchronous job creation when queue send fails |
| QUEUED | PENDING | StaleQueuedJobPoller resets jobs older than 15 min |
| PENDING | QUEUED | PendingJobPoller queues pending jobs |
| QUEUED | IN_PROGRESS | Message router starts processing |
| IN_PROGRESS | COMPLETED | Successful delivery |
| IN_PROGRESS | ERROR | Failed delivery |
| ERROR | IN_PROGRESS | Retry attempt |
| * | CANCELLED | Manual cancellation |

### Safety Net Pollers

| Poller | Interval | Purpose |
|--------|----------|---------|
| **StaleQueuedJobPoller** | 60s | Resets QUEUED jobs older than 15 min to PENDING |
| **PendingJobPoller** | 5s | Picks up PENDING jobs and sends to queue |

This ensures jobs are never lost, even if the SQS queue is temporarily unavailable.

---

## Common Queries

### Find pending jobs

```java
dispatchJobRepository.find(
    "status = ?1 and (scheduledFor is null or scheduledFor <= ?2)",
    DispatchStatus.PENDING, Instant.now()
).list();
```

### Find jobs by event

```java
dispatchJobRepository.find("eventId", eventId).list();
```

### Find jobs by subscription

```java
dispatchJobRepository.find("subscriptionId", subscriptionId).list();
```

### Find failed jobs for retry

```java
dispatchJobRepository.find(
    "status = ?1 and attemptCount < maxRetries",
    DispatchStatus.ERROR
).list();
```

### Find service account by code

```java
serviceAccountRepository.find("code", code).firstResult();
```

## See Also

- [Subscription Entities](subscription-entities.md) - Subscription entities
- [Event Entities](event-entities.md) - Event entities
- [Dispatch Jobs Guide](../guides/dispatch-jobs.md) - Webhook delivery guide
- [Message Groups Guide](../guides/message-groups.md) - FIFO ordering

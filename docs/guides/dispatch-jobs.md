# Dispatch Jobs Guide

Dispatch jobs are the core mechanism for delivering webhooks in FlowCatalyst. This guide covers job creation, delivery, retries, and monitoring.

## Overview

```
Event ──► Subscription Match ──► Dispatch Job ──► SQS Queue ──► Webhook Delivery
               (cached)              (QUEUED)                         │
                                                                      │
                                      ├── Attempt 1 ──► Success ──► Complete
                                      │
                                      └── Attempt 1 ──► Fail ──► Retry
                                                                   │
                                                                   └── Attempt 2...
```

Dispatch jobs are created **synchronously** during event ingestion. This ensures:
- **Low latency** - Jobs are queued immediately, no background processing delay
- **Reliability** - Safety net polling catches queue failures
- **Visibility** - API response includes dispatch job count

## Job Creation

### From Subscriptions (EVENT kind)

When an event is created, matching subscriptions are looked up via cache and dispatch jobs are created immediately:

```
POST /api/events
  │
  ├── Store event in MongoDB
  │
  ├── Lookup subscriptions (SubscriptionCache, 5-min TTL)
  │     └── Event: order.created
  │           └── Matches subscription "order-notifications"
  │
  ├── Create DispatchJob(s) with status QUEUED:
  │         kind: EVENT
  │         code: order.created
  │         subject: order:12345
  │         eventId: {event-id}
  │         status: QUEUED
  │
  ├── Batch send to SQS queue
  │     └── On failure: update job status to PENDING
  │
  └── Return response with dispatchJobCount
```

### Direct API (TASK kind)

Create jobs directly for async tasks:

```http
POST /api/dispatch-jobs
Content-Type: application/json
Authorization: Bearer {token}

{
  "kind": "TASK",
  "code": "send-welcome-email",
  "subject": "user:789",
  "targetUrl": "https://email-service.example.com/send",
  "payload": "{\"userId\": \"789\", \"template\": \"welcome\"}",
  "serviceAccountId": "0HZXEQ5Y8JY5Z"
}
```

## Webhook Delivery

### Request Format

```http
POST https://target-webhook.example.com/endpoint
Content-Type: application/json
Authorization: Bearer {service-account-token}
X-FlowCatalyst-ID: 0HZXEQ5Y8JY5Z
X-FlowCatalyst-Signature: sha256=abc123...
X-FlowCatalyst-Timestamp: 1705312200
X-FlowCatalyst-Event-Type: order.created

{payload}
```

### Headers

| Header | Description |
|--------|-------------|
| `Authorization` | Service account bearer token |
| `X-FlowCatalyst-ID` | Dispatch job ID |
| `X-FlowCatalyst-Signature` | HMAC-SHA256 signature |
| `X-FlowCatalyst-Timestamp` | Request timestamp (Unix) |
| `X-FlowCatalyst-Event-Type` | Event type code |

### Payload Modes

**dataOnly = true** (default):
```json
{"orderId": "12345", "amount": 99.99}
```

**dataOnly = false** (envelope):
```json
{
  "id": "0HZXEQ5Y8JY5Z",
  "kind": "EVENT",
  "code": "order.created",
  "subject": "order:12345",
  "eventId": "0HZXEQ5Y8JY00",
  "timestamp": "2024-01-15T10:30:00Z",
  "data": {"orderId": "12345", "amount": 99.99}
}
```

## Signature Verification

### Signature Format

```
X-FlowCatalyst-Signature: sha256=<hex-encoded-hmac>
```

### Verification Code

```python
import hmac
import hashlib

def verify_webhook(request, signing_secret):
    signature = request.headers.get('X-FlowCatalyst-Signature')
    timestamp = request.headers.get('X-FlowCatalyst-Timestamp')
    payload = request.body

    # Build message
    message = f"{timestamp}.{payload}"

    # Compute expected signature
    expected = hmac.new(
        signing_secret.encode('utf-8'),
        message.encode('utf-8'),
        hashlib.sha256
    ).hexdigest()

    # Compare (constant-time)
    return hmac.compare_digest(f"sha256={expected}", signature)
```

```javascript
const crypto = require('crypto');

function verifyWebhook(req, signingSecret) {
  const signature = req.headers['x-flowcatalyst-signature'];
  const timestamp = req.headers['x-flowcatalyst-timestamp'];
  const payload = req.rawBody;

  const message = `${timestamp}.${payload}`;
  const expected = crypto
    .createHmac('sha256', signingSecret)
    .update(message)
    .digest('hex');

  return signature === `sha256=${expected}`;
}
```

## Retry Logic

### Retry Strategy

Default: Exponential backoff with jitter

| Attempt | Delay |
|---------|-------|
| 1 | Immediate |
| 2 | ~1 minute |
| 3 | ~5 minutes |
| 4 | ~15 minutes |
| (max 3 retries) | |

### Retryable Errors

| Error Type | Retry? | Description |
|------------|--------|-------------|
| Connection timeout | Yes | Network issues |
| 5xx response | Yes | Server errors |
| 429 Too Many Requests | Yes | Rate limited |
| 4xx response | No | Client errors (permanent) |

### Configuration

```json
{
  "maxRetries": 5,
  "retryStrategy": "exponential",
  "timeoutSeconds": 30
}
```

## Job Status

### Status Values

| Status | Description |
|--------|-------------|
| `QUEUED` | Created and sent to message queue (initial state) |
| `PENDING` | Waiting to be queued (fallback for queue failures) |
| `IN_PROGRESS` | Currently being delivered |
| `COMPLETED` | Successfully delivered |
| `ERROR` | Failed (may retry) |
| `CANCELLED` | Manually cancelled |

### Status Flow

```
                   ┌─────────────────────────────────┐
                   │     Synchronous Creation        │
                   │  (createEvent / createDispatchJob)
                   └─────────────────────────────────┘
                                   │
                                   ▼
                   ┌─────────────────────────────────┐
                   │  QUEUED (normal path)           │
                   │  └─► queue fails → PENDING      │
                   └─────────────────────────────────┘
                                   │
           ┌───────────────────────┴───────────────────────┐
           ▼                                               ▼
     (message router                              (stale > 15 min)
      picks up)                                           │
           │                                               ▼
           ▼                                          PENDING
     IN_PROGRESS                                          │
           │                                               │
    ┌──────┴──────┐                    (PendingJobPoller) │
    ▼             ▼                                       │
COMPLETED      ERROR ◄────────────────────────────────────┘
                 │
                 ▼
          (retry or CANCELLED)
```

**Key Points**:
- Jobs are created with **QUEUED** status by default
- PENDING is only used as a fallback when SQS queue send fails
- **StaleQueuedJobPoller** (every 60s) resets QUEUED jobs older than 15 minutes to PENDING
- **PendingJobPoller** (every 5s) picks up PENDING jobs and re-queues them

## Monitoring

### Query Jobs

```http
GET /api/dispatch-jobs?status=ERROR&clientId=xxx
Authorization: Bearer {token}
```

### Job Details

```http
GET /api/dispatch-jobs/{jobId}
Authorization: Bearer {token}
```

Response includes attempt history:

```json
{
  "id": "0HZXEQ5Y8JY5Z",
  "status": "COMPLETED",
  "attemptCount": 2,
  "attempts": [
    {
      "attemptNumber": 1,
      "status": "SERVER_ERROR",
      "responseCode": 503,
      "errorMessage": "Service Unavailable"
    },
    {
      "attemptNumber": 2,
      "status": "SUCCESS",
      "responseCode": 200,
      "durationMillis": 150
    }
  ]
}
```

### Metrics

| Metric | Description |
|--------|-------------|
| `dispatch.jobs.created` | Jobs created (counter) |
| `dispatch.jobs.completed` | Successful deliveries |
| `dispatch.jobs.failed` | Failed deliveries |
| `dispatch.jobs.duration` | Delivery duration (histogram) |

## Best Practices

### Webhook Endpoints

1. **Return quickly** - Respond within timeout (default 30s)
2. **Implement idempotency** - Jobs may retry
3. **Verify signatures** - Validate HMAC signature
4. **Return appropriate codes**:
   - `2xx` - Success
   - `4xx` - Permanent failure (no retry)
   - `5xx` - Temporary failure (will retry)

### Job Configuration

1. **Set appropriate timeouts** - Match endpoint response time
2. **Use dispatch pools** - Control rate limits
3. **Configure retries** - Balance reliability vs latency
4. **Use metadata** - Add searchable context

### Error Handling

1. **Monitor ERROR status** - Set up alerts
2. **Review failed jobs** - Check attempt details
3. **Manual retry** - Re-queue jobs if needed
4. **Dead letter handling** - Process permanently failed jobs

## Configuration Reference

```properties
# Default timeout
flowcatalyst.dispatch.default-timeout-seconds=30

# Default retries
flowcatalyst.dispatch.default-max-retries=3

# Retry strategy
flowcatalyst.dispatch.retry-strategy=exponential
```

## See Also

- [Dispatch Entities](../entities/dispatch-entities.md) - Entity reference
- [Message Groups Guide](message-groups.md) - FIFO ordering
- [Message Router](../architecture/message-router.md) - Router architecture

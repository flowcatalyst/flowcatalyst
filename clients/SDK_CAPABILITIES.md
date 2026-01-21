# FlowCatalyst SDK Capabilities

This document defines the required capabilities for all FlowCatalyst client SDKs. Each SDK should implement these features to ensure consistent functionality across languages.

## 1. Authentication

### 1.1 OIDC Client Credentials Flow
- **Required**: OAuth2 client credentials grant flow
- Token endpoint: `{base_url}/oauth/token`
- Grant type: `client_credentials`
- Credentials: `client_id` and `client_secret`

### 1.2 Token Management
- **Automatic token refresh** before expiration (with 60-second buffer)
- **Token caching** (in-memory + persistent cache option)
- **Retry on 401** with token refresh

### 1.3 Configuration
| Setting | Description | Default |
|---------|-------------|---------|
| `base_url` | FlowCatalyst API base URL | Required |
| `client_id` | OIDC client ID | Required |
| `client_secret` | OIDC client secret | Required |
| `token_url` | OAuth token endpoint | `{base_url}/oauth/token` |

---

## 2. HTTP Client

### 2.1 Core Features
- Async/non-blocking HTTP requests
- Configurable timeout (default: 30 seconds)
- Automatic retry with exponential backoff
- Configurable retry attempts (default: 3)
- JSON request/response handling

### 2.2 Error Handling
SDKs must throw/return specific error types:

| HTTP Status | Error Type | Retry |
|-------------|------------|-------|
| 401 | `AuthenticationException` | No (refresh token, then retry once) |
| 403 | `ForbiddenException` | No |
| 404 | `NotFoundException` | No |
| 422 | `ValidationException` | No |
| 429 | `RateLimitException` | Yes (with backoff) |
| 5xx | `ServerException` | Yes |

### 2.3 Configuration
| Setting | Description | Default |
|---------|-------------|---------|
| `timeout` | Request timeout in seconds | 30 |
| `retry_attempts` | Max retry attempts | 3 |
| `retry_delay` | Initial retry delay in ms | 100 |

---

## 3. Control Plane API Resources

### 3.1 Event Types
Endpoint base: `/api/event-types`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list(filters?)` | GET `/api/event-types` | List event types with optional filters |
| `get(id)` | GET `/api/event-types/{id}` | Get event type by ID |
| `create(data)` | POST `/api/event-types` | Create new event type |
| `update(id, data)` | PATCH `/api/event-types/{id}` | Update event type |
| `delete(id)` | DELETE `/api/event-types/{id}` | Delete event type |
| `archive(id)` | POST `/api/event-types/{id}/archive` | Archive event type |
| `addSchema(id, schema)` | POST `/api/event-types/{id}/schemas` | Add schema version |
| `finaliseSchema(id, version)` | POST `/api/event-types/{id}/schemas/{version}/finalise` | Finalise schema |
| `deprecateSchema(id, version)` | POST `/api/event-types/{id}/schemas/{version}/deprecate` | Deprecate schema |
| `filterApplications()` | GET `/api/event-types/filters/applications` | Get distinct applications |
| `filterSubdomains(app?)` | GET `/api/event-types/filters/subdomains` | Get distinct subdomains |
| `filterAggregates(app?, sub?)` | GET `/api/event-types/filters/aggregates` | Get distinct aggregates |

**Create Event Type Request:**
```json
{
  "code": "app:domain:aggregate:event",
  "name": "Human Readable Name",
  "description": "Optional description",
  "category": "optional-category"
}
```

**Add Schema Request:**
```json
{
  "version": "1.0",
  "mimeType": "application/json",
  "schema": "{\"type\": \"object\", ...}",
  "schemaType": "JSON_SCHEMA"
}
```

### 3.2 Subscriptions
Endpoint base: `/api/admin/platform/subscriptions`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list(filters?)` | GET | List subscriptions |
| `get(id)` | GET `/{id}` | Get subscription by ID |
| `create(data)` | POST | Create subscription |
| `update(id, data)` | PUT `/{id}` | Update subscription |
| `delete(id)` | DELETE `/{id}` | Delete subscription |
| `pause(id)` | POST `/{id}/pause` | Pause subscription |
| `resume(id)` | POST `/{id}/resume` | Resume subscription |

**Create Subscription Request:**
```json
{
  "code": "unique-code",
  "name": "Subscription Name",
  "description": "Optional description",
  "eventTypes": [
    {"eventTypeCode": "app:domain:aggregate:event", "specVersion": "1.0"}
  ],
  "target": "https://webhook.example.com/endpoint",
  "queue": "default",
  "dispatchPoolId": "POOL_TSID",
  "mode": "IMMEDIATE",
  "timeoutSeconds": 30,
  "maxRetries": 5,
  "delaySeconds": 0,
  "dataOnly": false,
  "customConfig": [{"key": "header-name", "value": "header-value"}]
}
```

**Dispatch Modes:**
- `IMMEDIATE` - No ordering guarantee, parallel dispatch
- `NEXT_ON_ERROR` - Skip failed, process next in group
- `BLOCK_ON_ERROR` - Block group on failure until resolved

### 3.3 Dispatch Pools
Endpoint base: `/api/admin/platform/dispatch-pools`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list(filters?)` | GET | List dispatch pools |
| `get(id)` | GET `/{id}` | Get pool by ID |
| `create(data)` | POST | Create pool |
| `update(id, data)` | PUT `/{id}` | Update pool |
| `delete(id)` | DELETE `/{id}` | Archive pool |
| `suspend(id)` | POST `/{id}/suspend` | Suspend pool |
| `activate(id)` | POST `/{id}/activate` | Activate pool |

**Create Dispatch Pool Request:**
```json
{
  "code": "unique-pool-code",
  "name": "Pool Name",
  "description": "Optional description",
  "rateLimit": 100,
  "concurrency": 10
}
```

**Pool Statuses:**
- `ACTIVE` - Pool is operational
- `SUSPENDED` - Pool is paused, no dispatches
- `ARCHIVED` - Pool is soft-deleted

### 3.4 Roles
Endpoint base: `/api/admin/platform/roles`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list()` | GET | List all roles |
| `get(name)` | GET `/{name}` | Get role by name |
| `create(data)` | POST | Create role |
| `update(name, data)` | PUT `/{name}` | Update role |
| `delete(name)` | DELETE `/{name}` | Delete role |
| `sync(appCode, roles, removeUnlisted?)` | POST `/api/applications/{code}/roles/sync` | Sync SDK-managed roles |

**Create Role Request:**
```json
{
  "name": "role-name",
  "displayName": "Display Name",
  "description": "Role description",
  "permissions": ["app:resource:action"],
  "scope": "CLIENT"
}
```

**Role Scopes:**
- `ANCHOR` - Platform-level roles
- `PARTNER` - Partner-level roles
- `CLIENT` - Client-level roles

### 3.5 Permissions
Endpoint base: `/api/admin/platform/permissions`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list()` | GET | List all permissions |

### 3.6 Applications
Endpoint base: `/api/admin/platform/applications`

| Method | Operation | Description |
|--------|-----------|-------------|
| `list()` | GET | List all applications |
| `get(id)` | GET `/{id}` | Get application by ID |
| `getByCode(code)` | GET `/by-code/{code}` | Get application by code |
| `create(data)` | POST | Create application |
| `update(id, data)` | PUT `/{id}` | Update application |
| `delete(id)` | DELETE `/{id}` | Delete application |

---

## 4. Outbox (Transactional Outbox Pattern)

The outbox allows applications to write events atomically with their business transactions. Events are stored locally and later processed by FlowCatalyst.

### 4.1 Outbox Drivers
SDKs should support multiple storage backends:
- **Database** (SQL) - PostgreSQL, MySQL
- **MongoDB** - Document storage

### 4.2 Message Types
- `EVENT` - Domain events for subscription matching
- `DISPATCH_JOB` - Direct webhook dispatch without subscription matching

### 4.3 Create Event
```
createEvent(event: CreateEventDto) -> string (message ID)
createEvents(events: CreateEventDto[]) -> string[] (message IDs)
```

**CreateEventDto Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Event type code |
| `data` | object | Yes | Event payload |
| `partitionId` | string | Yes | Partition for ordering |
| `source` | string | No | Event source identifier |
| `subject` | string | No | Event subject |
| `correlationId` | string | No | Correlation ID for tracing |
| `causationId` | string | No | Causation ID for event chains |
| `deduplicationId` | string | No | Idempotency key |
| `messageGroup` | string | No | Message ordering group |
| `contextData` | array | No | Additional key-value context |
| `headers` | object | No | Custom headers |

### 4.4 Create Dispatch Job
```
createDispatchJob(job: CreateDispatchJobDto) -> string (message ID)
createDispatchJobs(jobs: CreateDispatchJobDto[]) -> string[] (message IDs)
```

**CreateDispatchJobDto Fields:**
| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `source` | string | Yes | Job source identifier |
| `code` | string | Yes | Unique job code |
| `targetUrl` | string | Yes | Webhook URL |
| `payload` | object | Yes | Request body |
| `dispatchPoolId` | string | Yes | Pool for rate limiting |
| `partitionId` | string | Yes | Partition for ordering |
| `correlationId` | string | No | Correlation ID |
| `headers` | object | No | Custom HTTP headers |
| `mode` | string | No | Dispatch mode |
| `maxRetries` | int | No | Max retry attempts |
| `timeoutSeconds` | int | No | Request timeout |

### 4.5 Outbox Table Schema
```sql
CREATE TABLE outbox_messages (
    id VARCHAR(13) PRIMARY KEY,  -- TSID Crockford Base32
    tenant_id VARCHAR(13) NOT NULL,
    partition_id VARCHAR(255) NOT NULL,
    type VARCHAR(20) NOT NULL,   -- EVENT or DISPATCH_JOB
    payload TEXT NOT NULL,
    payload_size INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    headers JSON,
    created_at TIMESTAMP NOT NULL,
    processed_at TIMESTAMP,

    INDEX idx_outbox_pending (tenant_id, partition_id, status, created_at)
);
```

### 4.6 Configuration
| Setting | Description | Default |
|---------|-------------|---------|
| `enabled` | Enable/disable outbox | true |
| `driver` | Storage driver | "database" |
| `connection` | Database connection | default |
| `table` | Table/collection name | "outbox_messages" |
| `tenant_id` | FlowCatalyst tenant ID | Required |
| `default_partition` | Default partition ID | "default" |

---

## 5. Webhook Validation

SDKs must validate incoming webhooks using HMAC-SHA256 signatures.

### 5.1 Signature Scheme
- Algorithm: HMAC-SHA256
- Message: `{timestamp}{body}`
- Headers:
  - `X-FlowCatalyst-Signature` - HMAC signature (hex-encoded)
  - `X-FlowCatalyst-Timestamp` - Unix timestamp (seconds)

### 5.2 Validation Steps
1. Extract signature and timestamp from headers
2. Validate timestamp is within tolerance (default: 300 seconds)
3. Compute expected signature: `HMAC-SHA256(timestamp + body, secret)`
4. Compare signatures using constant-time comparison
5. Reject if timestamp is in the future (>60 second grace period)

### 5.3 Validation Errors
| Error | Description |
|-------|-------------|
| `MissingSignature` | X-FlowCatalyst-Signature header missing |
| `MissingTimestamp` | X-FlowCatalyst-Timestamp header missing |
| `InvalidSignature` | Signature verification failed |
| `TimestampExpired` | Webhook too old |
| `TimestampInFuture` | Webhook timestamp in future |

### 5.4 Configuration
| Setting | Description |
|---------|-------------|
| `signing_secret` | HMAC signing secret from service account |

---

## 6. TSID Support

All entity IDs use TSIDs (Time-Sorted IDs) in Crockford Base32 format.

### 6.1 TSID Properties
- Length: 13 characters
- Format: Crockford Base32 (0-9, A-Z excluding I, L, O, U)
- Case-insensitive
- Lexicographically sortable (time-ordered)
- Example: `0HZXEQ5Y8JY5Z`

### 6.2 TSID Generation
SDKs should provide a TSID generator:
```
TsidGenerator.generate() -> string  // e.g., "0HZXEQ5Y8JY5Z"
```

Optional conversion utilities:
```
TsidGenerator.toLong(tsid: string) -> int64
TsidGenerator.toString(value: int64) -> string
```

---

## 7. Enums

### 7.1 Dispatch Mode
```
IMMEDIATE      // No ordering, parallel dispatch
NEXT_ON_ERROR  // Skip failed, continue with next
BLOCK_ON_ERROR // Block group until failure resolved
```

### 7.2 Subscription Status
```
ACTIVE  // Subscription is active
PAUSED  // Subscription is paused
```

### 7.3 Dispatch Pool Status
```
ACTIVE    // Pool is operational
SUSPENDED // Pool is paused
ARCHIVED  // Pool is soft-deleted
```

### 7.4 Event Type Status
```
CURRENT   // Event type is active
ARCHIVED  // Event type is archived
```

### 7.5 Spec Version Status
```
FINALISING // Schema in draft state
CURRENT    // Schema is active
DEPRECATED // Schema is deprecated
```

### 7.6 Schema Type
```
JSON_SCHEMA // JSON Schema
PROTO       // Protocol Buffers
XSD         // XML Schema
```

### 7.7 Message Type (Outbox)
```
EVENT        // Domain event
DISPATCH_JOB // Direct dispatch job
```

### 7.8 Subscription Source
```
API // Created via API/SDK
UI  // Created via UI
```

---

## 8. SDK Structure Recommendations

### 8.1 Module Organization
```
flowcatalyst-sdk/
├── client/           # HTTP client and authentication
│   ├── auth/         # Token management
│   └── resources/    # API resource classes
├── outbox/           # Outbox implementation
│   ├── drivers/      # Storage drivers
│   └── dtos/         # Data transfer objects
├── webhook/          # Webhook validation
├── types/            # DTOs and models
├── enums/            # Enumeration types
├── errors/           # Exception types
└── support/          # Utilities (TSID, etc.)
```

### 8.2 Client Interface Pattern
```
client = FlowCatalystClient(config)

// Resource access
client.eventTypes().list()
client.subscriptions().create(data)
client.dispatchPools().suspend(id)
client.roles().sync(appCode, roles)
```

### 8.3 Builder Pattern for DTOs
```
event = CreateEventDto.create("order.created", data, "orders")
    .withCorrelationId("corr-123")
    .withSource("order-service")
    .withHeaders({"X-Priority": "high"})
```

---

## 9. Testing Support

SDKs should provide testing utilities:

### 9.1 Mock Client
Provide a mockable client interface for unit testing.

### 9.2 Webhook Signature Generation
Utility to generate valid signatures for testing:
```
WebhookTestHelper.sign(payload, secret, timestamp?) -> {signature, timestamp}
```

---

## 10. Version Compatibility

| SDK Version | API Version | Breaking Changes |
|-------------|-------------|------------------|
| 1.x | v1 | Initial release |

SDKs should include version information in the User-Agent header:
```
User-Agent: FlowCatalyst-{Lang}-SDK/{version}
```

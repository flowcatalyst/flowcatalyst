# Event System Entities

This document describes the entities that power FlowCatalyst's event system.

## Event

**Collection**: `events`

CloudEvents-compatible event records.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `specVersion` | String | CloudEvents version (e.g., "1.0") |
| `type` | String | Event type code |
| `source` | String | Origin system identifier |
| `subject` | String | Aggregate reference (e.g., "order:12345") |
| `time` | Instant | Event timestamp |
| `data` | String | JSON payload |
| `correlationId` | String | Request correlation ID |
| `causationId` | String | Causing event ID |
| `deduplicationId` | String | Idempotency key |
| `messageGroup` | String | FIFO ordering group |
| `contextData` | List\<ContextData\> | Searchable key-value pairs |

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "specVersion": "1.0",
  "type": "ecommerce:orders:order:created",
  "source": "ecommerce-api",
  "subject": "order:12345",
  "time": "2024-01-15T10:30:00Z",
  "data": "{\"orderId\": \"12345\", \"amount\": 99.99, \"items\": 3}",
  "correlationId": "req-abc-123",
  "causationId": null,
  "deduplicationId": "order-created-12345",
  "messageGroup": "order:12345",
  "contextData": [
    {"key": "customer_id", "value": "cust_789"},
    {"key": "region", "value": "us-west"}
  ]
}
```

---

## ContextData (Embedded)

Searchable key-value pairs embedded in Event.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `key` | String | Context key |
| `value` | String | Context value |

### Usage

Context data enables efficient filtering without parsing JSON payload:

```java
// Find events for specific customer
eventRepository.find("contextData.key = ?1 and contextData.value = ?2",
                     "customer_id", "cust_789").list();
```

---

## EventType

**Collection**: `event_types`

Event type definitions with versioned schemas.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `code` | String | Globally unique code |
| `name` | String | Display name |
| `description` | String | Event type description |
| `specVersions` | List\<SpecVersion\> | Version history |
| `status` | EventTypeStatus | `CURRENT` or `ARCHIVE` |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### Event Type Code Format

```
{application}:{subdomain}:{aggregate}:{event}
```

Examples:
- `ecommerce:orders:order:created`
- `ecommerce:orders:order:shipped`
- `crm:customers:customer:registered`
- `billing:payments:payment:failed`

### EventTypeStatus Enum

```java
public enum EventTypeStatus {
    CURRENT,  // Active event type
    ARCHIVE   // Deprecated, no new events
}
```

### Example Document

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "code": "ecommerce:orders:order:created",
  "name": "Order Created",
  "description": "Fired when a new order is placed",
  "specVersions": [
    {
      "version": "1.0",
      "mimeType": "application/json",
      "schema": "{\"type\": \"object\", \"properties\": {...}}",
      "schemaType": "JSON_SCHEMA",
      "status": "DEPRECATED"
    },
    {
      "version": "1.1",
      "mimeType": "application/json",
      "schema": "{\"type\": \"object\", \"properties\": {...}}",
      "schemaType": "JSON_SCHEMA",
      "status": "CURRENT"
    }
  ],
  "status": "CURRENT",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-15T10:30:00Z"
}
```

---

## SpecVersion (Embedded)

Schema version information embedded in EventType.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `version` | String | Version string (MAJOR.MINOR) |
| `mimeType` | String | Content type (e.g., "application/json") |
| `schema` | String | Schema content |
| `schemaType` | SchemaType | Schema format |
| `status` | SpecVersionStatus | Version status |

### Version Format

Versions follow `MAJOR.MINOR` format:
- `1.0`, `1.1`, `1.2` - Minor versions (backward compatible)
- `2.0` - Major version (breaking change)

### SchemaType Enum

```java
public enum SchemaType {
    JSON_SCHEMA,  // JSON Schema (draft-07)
    PROTO,        // Protocol Buffers
    XSD           // XML Schema Definition
}
```

### SpecVersionStatus Enum

```java
public enum SpecVersionStatus {
    FINALISING,   // Under development, may change
    CURRENT,      // Active version
    DEPRECATED    // Still accepted, migration recommended
}
```

### Methods

```java
// Get major version number
int major = specVersion.majorVersion();  // "1.2" → 1

// Get minor version number
int minor = specVersion.minorVersion();  // "1.2" → 2

// Create with new status
SpecVersion updated = specVersion.withStatus(SpecVersionStatus.DEPRECATED);
```

---

## Schema

**Collection**: `schemas`

Standalone or event-type-linked schema definitions.

### Fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | String (TSID) | Primary key |
| `name` | String | Schema name |
| `description` | String | Schema description |
| `mimeType` | String | Content type |
| `schemaType` | SchemaType | Schema format |
| `content` | String | Schema definition |
| `eventTypeId` | String | Linked event type (optional) |
| `version` | String | Version if linked to event type |
| `createdAt` | Instant | Creation timestamp |
| `updatedAt` | Instant | Last update timestamp |

### Standalone vs Linked

**Standalone** (eventTypeId = null):
- Independent schema for validation
- Can be referenced by dispatch jobs
- Useful for shared data structures

**Linked** (eventTypeId + version):
- Associated with specific event type version
- Provides detailed schema separate from SpecVersion
- Allows larger schema storage

### Example Document (Standalone)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "name": "Address",
  "description": "Standard address structure",
  "mimeType": "application/json",
  "schemaType": "JSON_SCHEMA",
  "content": "{\"type\": \"object\", \"properties\": {\"street\": {\"type\": \"string\"}, \"city\": {\"type\": \"string\"}}}",
  "eventTypeId": null,
  "version": null,
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

### Example Document (Linked)

```json
{
  "_id": "0HZXEQ5Y8JY5Z",
  "name": "Order Created Schema v1.1",
  "description": "Full schema for order.created event v1.1",
  "mimeType": "application/json",
  "schemaType": "JSON_SCHEMA",
  "content": "{\"$schema\": \"http://json-schema.org/draft-07/schema#\", ...}",
  "eventTypeId": "0HZXEQ5Y8JY00",
  "version": "1.1",
  "createdAt": "2024-01-01T00:00:00Z",
  "updatedAt": "2024-01-01T00:00:00Z"
}
```

---

## Common Queries

### Find event by ID

```java
eventRepository.findById(eventId);
```

### Find events by type

```java
eventRepository.find("type", eventTypeCode).list();
```

### Find events by subject

```java
eventRepository.find("subject", subject).list();
```

### Find events with context filter

```java
eventRepository.find(
    "contextData.key = ?1 and contextData.value = ?2",
    "customer_id", customerId
).list();
```

### Find event type by code

```java
eventTypeRepository.find("code", code).firstResult();
```

### Find current event types

```java
eventTypeRepository.find("status", EventTypeStatus.CURRENT).list();
```

### Find schema for event type version

```java
schemaRepository.find(
    "eventTypeId = ?1 and version = ?2",
    eventTypeId, version
).firstResult();
```

## See Also

- [Subscription Entities](subscription-entities.md) - Subscription entities
- [Dispatch Entities](dispatch-entities.md) - Dispatch job entities
- [Event Processing](../architecture/event-processing.md) - Event architecture

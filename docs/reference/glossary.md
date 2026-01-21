# Glossary

Terminology and definitions used in FlowCatalyst.

## A

### ACK (Acknowledge)
Message acknowledgement signal sent to the queue backend indicating successful processing. The message is removed from the queue.

### Anchor Domain
The primary administrative domain for the FlowCatalyst platform. Users authenticating from anchor domains receive `ANCHOR` scope with access to all clients.

### AuthMode
Deployment mode for authentication: `EMBEDDED` (full IdP) or `REMOTE` (token validation only).

## B

### Bearer Token
Authentication token included in HTTP headers as `Authorization: Bearer {token}`. Used for API authentication.

## C

### Change Stream
MongoDB feature that provides real-time notifications of data changes. Used by the Event Processor to detect new events.

### Embedded Queue
SQLite-based queue implementation for local development. Provides full SQS FIFO semantics (message groups, visibility timeout, deduplication) without external dependencies. Database stored in a single file.

### Circuit Breaker
Pattern that prevents repeated calls to a failing service. When open, requests fail immediately without attempting delivery.

### Client
A tenant organization in FlowCatalyst. Each client has isolated data and configuration. See [Client Entities](../entities/client-entities.md).

### CloudEvents
CNCF specification for describing event data in a common way. FlowCatalyst events follow CloudEvents conventions.

### Crockford Base32
Encoding scheme used for TSID strings. Uses 32 alphanumeric characters (0-9, A-Z excluding I, L, O, U) for URL-safe, case-insensitive IDs.

## D

### Dispatch Attempt
A single attempt to deliver a dispatch job to its target endpoint. Records status, response code, and timing.

### Dispatch Job
A unit of work representing event delivery to a subscriber. Contains target URL, payload, and delivery status.

### Dispatch Mode
Controls message ordering and error handling: `IMMEDIATE`, `NEXT_ON_ERROR`, or `BLOCK_ON_ERROR`.

### Dispatch Pool
Configuration for processing groups. Defines concurrency limits, rate limiting, and buffer sizes.

## E

### Embedded Queue
SQLite-based queue implementation for development and testing. No external broker required.

### Event
An occurrence captured by the system, following CloudEvents format. Contains type, source, subject, and data.

### Event Processor
Service that watches for new events via MongoDB change streams and creates dispatch jobs for matching subscriptions.

### Event Type
Definition of an event category with schema versioning. Events are validated against their type's schema.

## F

### FIFO (First In, First Out)
Message ordering guarantee where messages in the same group are processed in order. Enabled via message groups.

## H

### Hot Standby
Deployment pattern where multiple instances run but only one (primary) actively processes messages. Uses Redis for leader election.

## I

### IdP (Identity Provider)
System that manages user identities and authentication. FlowCatalyst can operate as an embedded IdP or integrate with external providers.

## J

### JWKS (JSON Web Key Set)
Standard format for publishing public keys used to verify JWT signatures. Available at `/.well-known/jwks.json`.

### JWT (JSON Web Token)
Compact, URL-safe token format for representing claims. Used for session and access tokens.

## L

### Leader Election
Process where multiple instances coordinate to select one primary instance. Implemented with Redisson distributed locks.

### Long Polling
Technique where the consumer holds a connection open until messages are available. Used with SQS (20-second wait).

## M

### Mediation
The process of delivering a dispatch job to its target endpoint. Handles HTTP calls, retries, and response processing.

### Mediation Result
Outcome of a delivery attempt: `SUCCESS`, `ERROR_CONNECTION`, `ERROR_PROCESS`, or `ERROR_CONFIG`.

### Message Group
Identifier for ordering related messages. Messages with the same group ID are processed in FIFO order.

### Message Pointer
Lightweight reference placed on the queue containing job ID, pool code, and routing information. The actual payload is fetched from MongoDB.

### Message Router
Core service that consumes queue messages and routes them to target endpoints via HTTP mediation.

## N

### NACK (Negative Acknowledge)
Signal indicating failed processing. The message remains on the queue for retry after visibility timeout.

## O

### OAuth2
Authorization framework enabling third-party applications to access resources. FlowCatalyst implements OAuth2 authorization code flow.

### OIDC (OpenID Connect)
Identity layer on top of OAuth2. Provides authentication and user info via ID tokens.

## P

### PKCE (Proof Key for Code Exchange)
Security extension to OAuth2 that protects against authorization code interception. Required by default.

### Principal
Unified identity entity representing a user or service account. Contains authentication credentials and role assignments.

### Processing Pool
See [Dispatch Pool](#dispatch-pool).

## Q

### Queue Backend
The underlying message queue technology: `SQS`, `ACTIVEMQ`, `EMBEDDED`, or `CHRONICLE`.

## R

### Rate Limiting
Mechanism to control request throughput. Configured per pool as requests per minute.

### Redisson
Redis client library used for distributed locking in hot standby mode.

### Refresh Token
Long-lived token used to obtain new access tokens without re-authentication.

### Replica Set
MongoDB deployment with data replication. Required for change streams functionality.

## S

### Schema
Definition of event data structure. Supports JSON Schema, Protocol Buffers, or XSD.

### Service Account
Non-human identity for machine-to-machine authentication. Uses client credentials flow.

### Spec Version
Version of an event type schema. Follows semantic versioning with status lifecycle.

### Subscription
Configuration that connects event types to webhook endpoints. Defines filtering, target URL, and delivery settings.

## T

### TSID (Time-Sorted ID)
Unique identifier format that is chronologically sortable. FlowCatalyst uses 13-character Crockford Base32 strings.

**Example**: `0HZXEQ5Y8JY5Z`

**Properties**:
- Lexicographically sortable (newer IDs sort after older)
- URL-safe and case-insensitive
- JavaScript-safe (no precision loss)
- 13 characters long

## U

### UserScope
Access level for user principals: `ANCHOR` (all clients), `PARTNER` (assigned clients), or `CLIENT` (single client).

## V

### Virtual Thread
Java 21 feature enabling lightweight threads. FlowCatalyst uses virtual threads for efficient I/O operations.

### Visibility Timeout
Time period during which a message is invisible to other consumers after being received. If not acknowledged, message becomes visible again.

## W

### Webhook
HTTP endpoint that receives event notifications. FlowCatalyst delivers events via POST requests to configured webhooks.

### Watchdog
Automatic lock renewal mechanism in Redisson that refreshes distributed locks before they expire.

## See Also

- [Architecture Overview](../architecture/overview.md) - System design
- [Entity Reference](../entities/overview.md) - Data model
- [Enums Reference](enums.md) - Domain enumerations

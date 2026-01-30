# FlowCatalyst Project Context

## Java Style Guidelines

### Local Variable Type Inference
Prefer `var` for local variables when the type is obvious from the right-hand side:

```java
// Preferred
var context = ExecutionContext.from(tracingContext, principalId);
var result = roleOperations.createRole(command, context);
var app = applicationRepository.findByCode(code).orElse(null);
var roles = roleService.getAllRoles();

// Explicit type when not obvious or when interface type is preferred
List<String> ids = new ArrayList<>();  // ArrayList -> List
UseCaseError error = f.error();        // Type not obvious from method name
```

## ID Handling - TSID as Crockford Base32 Strings

**IMPORTANT**: All entity IDs in this project use TSIDs (Time-Sorted IDs) stored and transmitted as Crockford Base32 strings.

### TSID Format
- **Library**: `tsid-creator` (com.github.f4b6a3.tsid)
- **String Format**: 13-character Crockford Base32 (e.g., `0HZXEQ5Y8JY5Z`)
- **Properties**:
  - Lexicographically sortable (newer IDs sort after older ones)
  - URL-safe and case-insensitive
  - Shorter than numeric strings (13 vs ~19 chars)
  - Safe from JavaScript number precision issues

### Rules:
1. **Entities**: Use `String id` for all `@BsonId` fields
2. **Repositories**: Extend `PanacheMongoRepositoryBase<Entity, String>`
3. **DTOs**: All ID fields must be `String` type
4. **Services/Commands**: All ID parameters must be `String` type
5. **Frontend**: IDs are always strings (no parsing needed)

### TsidGenerator Usage:
```java
import tech.flowcatalyst.platform.shared.TsidGenerator;

// Generate a new TSID string
String id = TsidGenerator.generate();  // e.g., "0HZXEQ5Y8JY5Z"

// Convert between formats (for migration/compatibility)
Long longId = TsidGenerator.toLong("0HZXEQ5Y8JY5Z");
String strId = TsidGenerator.toString(786259737685263979L);
```

### Entity Pattern:
```java
@MongoEntity(collection = "my_entities")
public class MyEntity extends PanacheMongoEntityBase {
    @BsonId
    public String id;  // TSID Crockford Base32

    public String relatedEntityId;  // Foreign key as String
}

// Repository
public class MyEntityRepository implements PanacheMongoRepositoryBase<MyEntity, String> {}
```

### Why Not Long?
JavaScript loses precision for integers > 2^53. A TSID like `786259737685263979` becomes `786259737685264000` when parsed as a JavaScript number, causing 404 errors.

## Multi-Tenant Architecture

### UserScope Enum
Users have explicit access scopes:
- `ANCHOR` - Platform admin users, access to all clients
- `PARTNER` - Partner users, access to multiple assigned clients
- `CLIENT` - Users bound to a single client

### Email Domain Configuration
- Anchor domains: Users get `ANCHOR` scope automatically
- Client-bound domains: Users get `CLIENT` scope, constrained to that client
- Unconfigured domains: Default to internal auth

## Authentication

### External Base URL
For OAuth/OIDC callbacks behind a proxy (e.g., Vite dev server), configure:
```properties
flowcatalyst.auth.external-base-url=http://localhost:4200
```

### Token Claims
Session tokens include a `clients` claim:
- `["*"]` for ANCHOR users (access all)
- `["123", "456"]` for specific client IDs

## Database Operations - CRITICAL RULES

### No Foreign Keys
**IMPORTANT**: Never use foreign key constraints in this project. Use indexes on join columns instead.

- **No `REFERENCES` clauses** - don't create FK constraints
- **Always add indexes** on columns used for joins (e.g., `CREATE INDEX idx_xxx ON table(parent_id)`)
- **Application enforces integrity** - referential integrity is handled in code, not database

This allows for:
- Easier data migrations and schema changes
- Better performance (no FK constraint checks)
- More flexible data cleanup strategies
- Simpler cross-service data management

### NEVER Drop Collections or Databases Without Permission
**IMPORTANT**: NEVER drop MongoDB collections or databases without explicit user permission. Dropping data is destructive and irreversible.

### Handling Data Type Mismatches
When encountering MongoDB decode errors (e.g., "expected 'DATE_TIME' BsonType but got 'STRING'"), the proper fix is to **migrate the data**, not drop it:

```javascript
// Example: Fix Instant fields stored as STRING instead of DATE_TIME
db.collection.find({ createdAt: { $type: "string" } }).forEach(function(doc) {
  db.collection.updateOne(
    { _id: doc._id },
    { $set: { createdAt: new Date(doc.createdAt) } }
  );
});
```

### Migration vs Dropping
- **Preferred**: Write a migration script to convert incorrect field types
- **Alternative**: Ask user if they want to drop the affected collection
- **Never**: Silently drop collections/databases as a "quick fix"

## TypeScript Error Handling - neverthrow

**IMPORTANT**: Use `neverthrow` for typed error handling in TypeScript code. Do not use try/catch with untyped exceptions for business logic.

### Why neverthrow
- TypeScript exceptions are untyped - `catch (e)` gives you `unknown`
- Result types make error paths explicit in function signatures
- Forces callers to handle errors - can't accidentally ignore them
- Composable with `map`, `mapErr`, `andThen` for clean pipelines

### Basic Patterns
```typescript
import { ok, err, Result, ResultAsync } from 'neverthrow';

// Define typed errors
type ValidationError = { type: 'validation'; field: string; message: string };
type NotFoundError = { type: 'not_found'; id: string };
type NetworkError = { type: 'network'; cause: Error };

// Synchronous functions return Result<T, E>
function parseConfig(json: string): Result<Config, ValidationError> {
  const parsed = JSON.parse(json);
  if (!parsed.name) {
    return err({ type: 'validation', field: 'name', message: 'required' });
  }
  return ok(parsed as Config);
}

// Async functions return ResultAsync<T, E>
function fetchUser(id: string): ResultAsync<User, NotFoundError | NetworkError> {
  return ResultAsync.fromPromise(
    fetch(`/users/${id}`).then(r => r.json()),
    (e) => ({ type: 'network', cause: e as Error })
  ).andThen((data) =>
    data ? ok(data) : err({ type: 'not_found', id })
  );
}

// Handling results
const result = await fetchUser('123');

// Pattern 1: match
result.match(
  (user) => console.log(user.name),
  (error) => {
    if (error.type === 'not_found') console.log('User not found');
    else console.log('Network error', error.cause);
  }
);

// Pattern 2: isOk/isErr guards
if (result.isOk()) {
  console.log(result.value.name);  // TypeScript knows it's User
}

// Pattern 3: unwrapOr for defaults
const user = result.unwrapOr(defaultUser);
```

### Wrapping External Libraries
```typescript
// Wrap throwing functions with fromThrowable
import { fromThrowable } from 'neverthrow';

const safeJsonParse = fromThrowable(
  JSON.parse,
  (e) => ({ type: 'parse_error' as const, cause: e })
);

// Wrap promises with ResultAsync.fromPromise
function safeFetch(url: string): ResultAsync<Response, NetworkError> {
  return ResultAsync.fromPromise(
    fetch(url),
    (e) => ({ type: 'network', cause: e as Error })
  );
}
```

### Combining Results
```typescript
import { Result, ResultAsync, combine, combineWithAllErrors } from 'neverthrow';

// combine - fails fast on first error
const results: Result<number, string>[] = [ok(1), ok(2), ok(3)];
const combined = combine(results);  // Result<number[], string>

// combineWithAllErrors - collects all errors
const allResults = combineWithAllErrors(results);  // Result<number[], string[]>

// Chaining with andThen
fetchUser(id)
  .andThen((user) => fetchOrders(user.id))
  .andThen((orders) => calculateTotal(orders))
  .mapErr((e) => ({ ...e, context: 'order_total' }));
```

### Message Router Error Types
For the message-router, use these standard error types:
```typescript
// Domain errors
type MediationError =
  | { type: 'circuit_open'; name: string }
  | { type: 'timeout'; durationMs: number }
  | { type: 'http_error'; status: number; body?: string }
  | { type: 'network'; cause: Error };

type ProcessingError =
  | { type: 'parse_error'; message: string }
  | { type: 'validation'; field: string }
  | { type: 'rate_limited'; retryAfterMs: number }
  | { type: 'pool_full'; poolCode: string };

type HealthCheckError =
  | { type: 'broker_unreachable'; broker: string; cause: Error }
  | { type: 'queue_not_found'; queueUrl: string }
  | { type: 'auth_failed'; broker: string };
```

### Rules
1. **Business logic**: Always use Result/ResultAsync
2. **Infrastructure boundaries**: Wrap with fromPromise/fromThrowable
3. **Error types**: Define discriminated unions with `type` field
4. **Never throw**: Convert exceptions at boundaries, propagate as Result
5. **Logging**: Log errors at handling site, not at creation site

## Java Error Handling - Result Pattern

**IMPORTANT**: Use the `Result<T>` sealed interface for typed error handling in Java use cases. Do not use exceptions for business logic errors.

### Why Result
- Exceptions are untyped in Java signatures - callers don't know what can fail
- Result types make error paths explicit in method signatures
- Sealed interfaces enable exhaustive pattern matching
- `UnitOfWork.commit()` is the only way to create success - guarantees domain events are always emitted

### Core Types

```java
// Result sealed interface with two variants
public sealed interface Result<T> permits Result.Success, Result.Failure {
    record Success<T>(T value) implements Result<T> {}
    record Failure<T>(UseCaseError error) implements Result<T> {}

    static <T> Result<T> failure(UseCaseError error);  // public
    // success() is package-private - only UnitOfWork can create Success
}

// Error hierarchy with HTTP status mapping
public sealed interface UseCaseError {
    String code();
    String message();
    Map<String, Object> details();

    record ValidationError(...)     implements UseCaseError {}  // → 400
    record BusinessRuleViolation(...) implements UseCaseError {} // → 409
    record NotFoundError(...)       implements UseCaseError {}  // → 404
    record ConcurrencyError(...)    implements UseCaseError {}  // → 409
}
```

### Usage in Use Cases

```java
public Result<EventTypeCreated> execute(CreateEventTypeCommand cmd, ExecutionContext ctx) {
    // Validation - can return failure directly
    if (cmd.code() == null || cmd.code().isBlank()) {
        return Result.failure(new ValidationError(
            "INVALID_CODE",
            "Code is required",
            Map.of("field", "code")
        ));
    }

    // Business rule check
    if (repository.existsByCode(cmd.code())) {
        return Result.failure(new BusinessRuleViolation(
            "CODE_EXISTS",
            "Event type code already exists",
            Map.of("code", cmd.code())
        ));
    }

    // Create aggregate and event
    EventType eventType = new EventType(...);
    EventTypeCreated event = EventTypeCreated.builder().from(ctx)...build();

    // Only way to return success - guarantees event emission
    return unitOfWork.commit(eventType, event, cmd);
}
```

### Pattern Matching in API Layer

```java
@POST
public Response create(CreateRequest request) {
    Result<EventTypeCreated> result = operations.createEventType(command, context);

    return switch (result) {
        case Result.Success<EventTypeCreated> s -> {
            EventType entity = operations.findById(s.value().eventTypeId()).orElseThrow();
            yield Response.status(201).entity(EventTypeResponse.from(entity)).build();
        }
        case Result.Failure<EventTypeCreated> f -> mapErrorToResponse(f.error());
    };
}

private Response mapErrorToResponse(UseCaseError error) {
    Response.Status status = switch (error) {
        case UseCaseError.ValidationError v -> Response.Status.BAD_REQUEST;        // 400
        case UseCaseError.NotFoundError n -> Response.Status.NOT_FOUND;            // 404
        case UseCaseError.BusinessRuleViolation b -> Response.Status.CONFLICT;     // 409
        case UseCaseError.ConcurrencyError c -> Response.Status.CONFLICT;          // 409
    };
    return Response.status(status)
        .entity(new ErrorResponse(error.code(), error.message(), error.details()))
        .build();
}
```

### Rules
1. **Use cases**: Always return `Result<T>` (never throw for business logic)
2. **Validation errors**: Return `Result.failure(new ValidationError(...))` for input issues
3. **Business rules**: Return `Result.failure(new BusinessRuleViolation(...))` for constraint violations
4. **Not found**: Return `Result.failure(new NotFoundError(...))` when entities don't exist
5. **Success**: Only through `unitOfWork.commit()` - ensures events are emitted
6. **API layer**: Pattern match on Result and map to appropriate HTTP status

## Authorization Model - Two-Level Access Control

FlowCatalyst uses a two-level authorization model:

1. **Action-level**: "Can this principal perform this action?" (e.g., `event:publish`)
2. **Resource-level**: "Can this principal perform this action on THIS specific resource?"

### Why Two Levels?

A service account for Application A with `event:publish` permission should NOT be able to publish events for Application B. Action-level permissions alone don't capture resource ownership.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         API Controller                          │
│                   (Action-level permission check)               │
└─────────────────────────────────────────────────────────────────┘
                                │
                ┌───────────────┴───────────────┐
                ▼                               ▼
┌───────────────────────────┐   ┌───────────────────────────────┐
│        Use Case           │   │       Query Service           │
│  (Resource-level guard)   │   │   (Resource-level filtering)  │
│      FOR WRITES           │   │        FOR READS              │
└───────────────────────────┘   └───────────────────────────────┘
                │                               │
                └───────────────┬───────────────┘
                                ▼
┌─────────────────────────────────────────────────────────────────┐
│                         Repository                              │
│                    (Pure data access)                           │
└─────────────────────────────────────────────────────────────────┘
```

### Use Cases - Write Operations with Guard Methods

Every use case MUST implement `authorizeResource()`. Return `true` if no resource-level restriction is needed.

```java
public abstract class UseCase<C extends Command, R> {

    /**
     * Template method - executes authorization then business logic.
     */
    public final Result<R> execute(C command, ExecutionContext ctx) {
        // Resource-level authorization check
        if (!authorizeResource(command, ctx)) {
            return Result.failure(new AuthorizationError(
                "RESOURCE_ACCESS_DENIED",
                "Not authorized to access this resource"
            ));
        }

        // Business logic
        return doExecute(command, ctx);
    }

    /**
     * Resource-level authorization. MUST be implemented by every use case.
     * Return true if no resource-level restriction is needed.
     */
    protected abstract boolean authorizeResource(C command, ExecutionContext ctx);

    /**
     * Business logic implementation.
     */
    protected abstract Result<R> doExecute(C command, ExecutionContext ctx);
}
```

#### Example: Use Case WITH Resource Restriction

```java
public class PublishEventUseCase extends UseCase<PublishEventCommand, EventPublished> {

    @Inject EventTypeRepository eventTypeRepo;

    @Override
    protected boolean authorizeResource(PublishEventCommand cmd, ExecutionContext ctx) {
        // Check if principal can access the event type's application
        EventType eventType = eventTypeRepo.findById(cmd.eventTypeId()).orElse(null);
        if (eventType == null) {
            return false; // Will fail in doExecute with proper error
        }
        return ctx.canAccessApplication(eventType.applicationId);
    }

    @Override
    protected Result<EventPublished> doExecute(PublishEventCommand cmd, ExecutionContext ctx) {
        // Business logic here...
    }
}
```

#### Example: Use Case WITHOUT Resource Restriction

```java
public class ListApplicationsUseCase extends UseCase<ListApplicationsCommand, List<Application>> {

    @Override
    protected boolean authorizeResource(ListApplicationsCommand cmd, ExecutionContext ctx) {
        return true; // No resource-level restriction - filtering done in query
    }

    @Override
    protected Result<List<Application>> doExecute(ListApplicationsCommand cmd, ExecutionContext ctx) {
        // Returns only applications the principal can see (handled by query service)
    }
}
```

### Query Services - Read Operations with Filtering

Query services sit between controllers and repositories, handling resource-level authorization for reads.

```java
@ApplicationScoped
public class EventTypeQueryService {

    @Inject EventTypeRepository repo;

    /**
     * Get single resource by ID.
     * Returns empty if not found OR not authorized (don't leak existence).
     */
    public Optional<EventType> findById(String id, ExecutionContext ctx) {
        return repo.findById(id)
            .filter(et -> canAccessResource(et, ctx));
    }

    /**
     * List resources - filters at query level for efficiency.
     */
    public List<EventType> findAll(ExecutionContext ctx) {
        Set<String> allowedAppIds = getAccessibleApplicationIds(ctx);
        if (allowedAppIds == null) {
            return repo.findAll(); // No restriction
        }
        return repo.findByApplicationIds(allowedAppIds);
    }

    /**
     * Search with criteria - injects scope filter into query.
     */
    public Page<EventType> search(EventTypeSearchCriteria criteria, ExecutionContext ctx) {
        Set<String> allowedAppIds = getAccessibleApplicationIds(ctx);
        return repo.search(criteria.withApplicationIds(allowedAppIds));
    }

    // ========== Private Helpers ==========

    private boolean canAccessResource(EventType et, ExecutionContext ctx) {
        return ctx.canAccessApplication(et.applicationId);
    }

    /**
     * Returns null if principal can access all applications (no filtering needed).
     */
    private Set<String> getAccessibleApplicationIds(ExecutionContext ctx) {
        if (ctx.canAccessAllApplications()) {
            return null;
        }
        return ctx.getAllowedApplicationIds();
    }
}
```

### ExecutionContext - Principal Access Information

The `ExecutionContext` provides information about what the current principal can access:

```java
public interface ExecutionContext {

    /** The authenticated principal's ID */
    String getPrincipalId();

    /** Check if principal can access a specific application */
    boolean canAccessApplication(String applicationId);

    /** Check if principal can access all applications (platform admins) */
    boolean canAccessAllApplications();

    /** Get the set of application IDs this principal can access */
    Set<String> getAllowedApplicationIds();

    /** Check if principal can access a specific client */
    boolean canAccessClient(String clientId);

    /** Check if principal is a platform admin */
    boolean isPlatformAdmin();

    /** Get home client ID (for CLIENT-scoped users) */
    Optional<String> getHomeClientId();
}
```

### Key Principles

1. **Controllers**: Check action-level permissions (e.g., `@RolesAllowed`, permission annotations)
2. **Use Cases**: MUST implement `authorizeResource()` - return `true` if no restriction needed
3. **Query Services**: Filter results based on principal's accessible resources
4. **Repositories**: Stay pure - no authorization logic, just data access
5. **Single-resource reads**: Fetch then filter, return empty for "not found" AND "not authorized"
6. **List/search reads**: Inject scope filter INTO the query (don't fetch then filter)
7. **Don't leak existence**: Return same response for "not found" and "not authorized"

### Service Account Authorization Flow

When a service account makes a request:

1. Service account authenticates via OAuth client credentials
2. OAuth client is linked to application(s)
3. `ExecutionContext.getAllowedApplicationIds()` returns those application IDs
4. Use case guard and query service filters restrict access to those applications only

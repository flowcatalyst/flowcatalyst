# Java Architecture Patterns

Reference guide for FlowCatalyst's Java platform architecture patterns: sealed event hierarchies, the `UseCase` interface, and the `Either` type.

## Sealed Event Hierarchies

Each domain groups its events under a sealed interface that extends `DomainEvent`. This gives compiler-enforced exhaustive handling via pattern matching.

### Structure

```
DomainEvent (interface)
├── EventTypeEvent (sealed)
│   ├── EventTypeCreated
│   ├── EventTypeUpdated
│   ├── EventTypeArchived
│   ├── EventTypeDeleted
│   ├── SchemaAdded
│   ├── SchemaDeprecated
│   ├── SchemaFinalised
│   └── EventTypesSynced
├── SubscriptionEvent (sealed)
│   ├── SubscriptionCreated
│   ├── SubscriptionUpdated
│   ├── SubscriptionDeleted
│   └── SubscriptionsSynced
├── DispatchPoolEvent (sealed)
│   ├── DispatchPoolCreated
│   ├── DispatchPoolUpdated
│   ├── DispatchPoolDeleted
│   └── DispatchPoolsSynced
├── ApplicationEvent (sealed)
│   ├── ApplicationCreated
│   ├── ApplicationUpdated
│   ├── ApplicationActivated
│   ├── ApplicationDeactivated
│   ├── ApplicationDeleted
│   ├── ServiceAccountProvisioned
│   ├── ApplicationEnabledForClient
│   └── ApplicationDisabledForClient
├── PrincipalEvent (sealed)
│   ├── UserCreated
│   ├── UserUpdated
│   ├── UserDeleted
│   ├── UserActivated
│   ├── UserDeactivated
│   ├── RolesAssigned
│   ├── ClientAccessGranted
│   ├── ClientAccessRevoked
│   └── ApplicationAccessAssigned
├── AuthorizationEvent (sealed)
│   ├── RoleCreated
│   ├── RoleUpdated
│   ├── RoleDeleted
│   └── RolesSynced
├── IdentityProviderEvent (sealed)
│   ├── IdentityProviderCreated
│   ├── IdentityProviderUpdated
│   └── IdentityProviderDeleted
├── EmailDomainMappingEvent (sealed)
│   ├── EmailDomainMappingCreated
│   ├── EmailDomainMappingUpdated
│   └── EmailDomainMappingDeleted
├── CorsEvent (sealed)
│   ├── CorsOriginAdded
│   └── CorsOriginDeleted
├── ClientEvent (sealed)
│   ├── AuthConfigAdditionalClientsUpdated
│   └── AuthConfigGrantedClientsUpdated
└── ServiceAccountEvent (non-sealed marker)
    ├── ServiceAccountCreated
    ├── ServiceAccountDeleted
    ├── ServiceAccountUpdated
    ├── RolesAssigned
    ├── AuthTokenRegenerated
    └── SigningSecretRegenerated
```

### Defining a Sealed Event Interface

```java
// In the domain's events package
public sealed interface EventTypeEvent extends DomainEvent
    permits EventTypeCreated, EventTypeUpdated, EventTypeArchived,
            EventTypeDeleted, SchemaAdded, SchemaDeprecated,
            SchemaFinalised, EventTypesSynced {}
```

Each event record implements the domain-specific interface:

```java
@Builder
public record EventTypeCreated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String eventTypeId,
    String code,
    String name,
    String description
) implements EventTypeEvent {
    // ... DomainEvent method implementations
}
```

### Exhaustive Pattern Matching

The sealed hierarchy enables the compiler to verify all cases are handled:

```java
// Compiler enforces all 8 cases
String description = switch (event) {
    case EventTypeCreated e   -> "Created " + e.code();
    case EventTypeUpdated e   -> "Updated " + e.eventTypeId();
    case EventTypeArchived e  -> "Archived " + e.eventTypeId();
    case EventTypeDeleted e   -> "Deleted " + e.eventTypeId();
    case SchemaAdded e        -> "Schema added to " + e.eventTypeId();
    case SchemaDeprecated e   -> "Schema deprecated on " + e.eventTypeId();
    case SchemaFinalised e    -> "Schema finalised on " + e.eventTypeId();
    case EventTypesSynced e   -> "Synced " + e.count() + " event types";
};
```

### UnitOfWork Compatibility

All sealed interfaces extend `DomainEvent`, so `UnitOfWork.commit()` works unchanged:

```java
// UnitOfWork signature: <T extends DomainEvent> Result<T> commit(...)
// EventTypeCreated implements EventTypeEvent extends DomainEvent — OK
return unitOfWork.commit(eventType, event, command);
```

### ServiceAccount: Non-Sealed Exception

The `ServiceAccountEvent` interface is non-sealed because its event records are scattered across multiple operation packages. Java sealed types require permitted subtypes to be in the same package for unnamed modules. If these events are later consolidated into a single `events` package, the interface can become sealed.

---

## UseCase Interface

All standard use cases implement `UseCase<C, E>`, which provides two-level authorization via a template method.

### The Interface

```java
public interface UseCase<C, E extends DomainEvent> {

    // Template method — checks auth, then delegates
    default Result<E> execute(C command, ExecutionContext context) {
        if (!authorizeResource(command, context)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "RESOURCE_ACCESS_DENIED",
                "Not authorized to access this resource",
                Map.of()
            ));
        }
        return doExecute(command, context);
    }

    // Resource-level authorization guard
    boolean authorizeResource(C command, ExecutionContext context);

    // Business logic
    Result<E> doExecute(C command, ExecutionContext context);
}
```

### Two-Level Authorization Model

```
┌─────────────────────────────────────────────┐
│              API Controller                  │
│       (Action-level: @RolesAllowed)          │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│      UseCase.execute() [default method]      │
│  ┌─────────────────────────────────────┐     │
│  │ authorizeResource(command, context) │     │
│  │   "Can this principal access THIS   │     │
│  │    specific resource?"              │     │
│  └──────────────┬──────────────────────┘     │
│                 │ true                        │
│                 ▼                             │
│  ┌─────────────────────────────────────┐     │
│  │ doExecute(command, context)         │     │
│  │   Validation → Business rules →     │     │
│  │   Create aggregate → Emit event →   │     │
│  │   UnitOfWork.commit()               │     │
│  └─────────────────────────────────────┘     │
└─────────────────────────────────────────────┘
```

### Implementation Examples

**No resource restriction** (most use cases today):

```java
@ApplicationScoped
public class CreateRoleUseCase implements UseCase<CreateRoleCommand, RoleCreated> {

    @Override
    public boolean authorizeResource(CreateRoleCommand command, ExecutionContext context) {
        return true; // No resource-level restriction
    }

    @Override
    public Result<RoleCreated> doExecute(CreateRoleCommand command, ExecutionContext context) {
        // validation, business rules, commit...
    }
}
```

**With resource restriction** (future pattern):

```java
@ApplicationScoped
public class UpdateEventTypeUseCase implements UseCase<UpdateEventTypeCommand, EventTypeUpdated> {

    @Inject EventTypeRepository repo;

    @Override
    public boolean authorizeResource(UpdateEventTypeCommand command, ExecutionContext context) {
        var authz = context.authz();
        if (authz == null) return true;

        // Check if principal can access the event type's application
        var eventType = repo.findById(command.eventTypeId());
        return eventType != null && authz.canAccessResourceWithPrefix(eventType.code());
    }

    @Override
    public Result<EventTypeUpdated> doExecute(UpdateEventTypeCommand command, ExecutionContext context) {
        // business logic...
    }
}
```

### Non-Standard Use Cases

Four use cases return custom wrapper types instead of `Result<E>` and are excluded from the `UseCase` interface:

| Use Case | Return Type | Reason |
|----------|------------|--------|
| `CreateServiceAccountUseCase` | `CreateServiceAccountResult` | Returns credentials shown only once |
| `ProvisionServiceAccountUseCase` | `ProvisionResult` | Composite provisioning result |
| `RegenerateAuthTokenUseCase` | `RegenerateAuthTokenResult` | Returns token shown only once |
| `RegenerateSigningSecretUseCase` | `RegenerateSigningSecretResult` | Returns secret shown only once |

These can be migrated if their return types are refactored to use `Result<E>` with a separate credentials envelope.

### Operations Classes

Operations classes are unchanged — they call `useCase.execute(command, context)`, which now routes through the default method's authorization check:

```java
@ApplicationScoped
public class EventTypeOperations {

    @Inject CreateEventTypeUseCase createUseCase;

    public Result<EventTypeCreated> createEventType(
            CreateEventTypeCommand command, ExecutionContext context) {
        return createUseCase.execute(command, context); // routes through default method
    }
}
```

---

## Either Type

`Either<L, R>` is a general-purpose disjoint union. By convention, `Left` is the error case and `Right` is the success case.

### The Type

```java
public sealed interface Either<L, R> permits Either.Left, Either.Right {
    record Left<L, R>(L value) implements Either<L, R> {}
    record Right<L, R>(R value) implements Either<L, R> {}

    static <L, R> Either<L, R> left(L value);
    static <L, R> Either<L, R> right(R value);

    <T> Either<L, T> map(Function<R, T> fn);
    <T> Either<L, T> flatMap(Function<R, Either<L, T>> fn);
    <T> T fold(Function<L, T> onLeft, Function<R, T> onRight);
    Result<R> toResult(Function<L, UseCaseError> errorMapper);
}
```

### When to Use Either vs Result

| Type | Use For |
|------|---------|
| `Result<T>` | Use case return values (ties to UnitOfWork guarantee) |
| `Either<L, R>` | Validation pipelines, parsing, internal composition |

### Validation Pipeline Example

```java
// Validate multiple fields, collecting typed errors
Either<List<String>, ValidatedInput> validateInput(RawInput input) {
    var errors = new ArrayList<String>();

    if (input.name() == null || input.name().isBlank()) {
        errors.add("Name is required");
    }
    if (input.code() != null && !CODE_PATTERN.matcher(input.code()).matches()) {
        errors.add("Code format is invalid");
    }

    if (!errors.isEmpty()) {
        return Either.left(errors);
    }
    return Either.right(new ValidatedInput(input.name().trim(), input.code()));
}
```

### Chaining with flatMap

```java
Either<String, Config> loadConfig(String path) {
    return readFile(path)                          // Either<String, String>
        .flatMap(content -> parseJson(content))    // Either<String, JsonNode>
        .flatMap(json -> extractConfig(json));     // Either<String, Config>
}
```

### Bridge to Result

```java
// Convert Either to Result for use case return
Either<List<String>, ValidatedInput> validated = validateInput(raw);

return validated.toResult(errors ->
    new UseCaseError.ValidationError(
        "VALIDATION_FAILED",
        String.join("; ", errors),
        Map.of("errors", errors)
    )
);
```

### Folding

```java
// Reduce to a single value
String message = result.fold(
    errors -> "Failed: " + String.join(", ", errors),
    value -> "Success: " + value.name()
);
```

---

## How the Pieces Fit Together

```
API Controller
  │
  │  @RolesAllowed("event-type:create")   ← action-level auth
  │
  ▼
Operations.createEventType(command, context)
  │
  ▼
UseCase.execute(command, context)           ← default method
  │
  ├── authorizeResource(command, context)   ← resource-level auth
  │     Returns true/false
  │
  └── doExecute(command, context)           ← business logic
        │
        ├── Either for validation pipelines
        │     validated.toResult(errorMapper)
        │
        ├── Result.failure() for business rule violations
        │
        └── UnitOfWork.commit(aggregate, event, command)
              │
              ├── Persists aggregate
              ├── Stores EventTypeCreated (implements EventTypeEvent)
              ├── Creates audit log
              └── Returns Result.Success<EventTypeCreated>
```

### Error Hierarchy

```
UseCaseError (sealed)
├── ValidationError        → HTTP 400
├── NotFoundError          → HTTP 404
├── BusinessRuleViolation  → HTTP 409
├── ConcurrencyError       → HTTP 409
└── AuthorizationError     → HTTP 403
```

Pattern matching in the API layer:

```java
return switch (result) {
    case Result.Success<EventTypeCreated> s -> {
        var entity = operations.findById(s.value().eventTypeId()).orElseThrow();
        yield Response.status(201).entity(EventTypeResponse.from(entity)).build();
    }
    case Result.Failure<EventTypeCreated> f -> switch (f.error()) {
        case UseCaseError.ValidationError e      -> Response.status(400).entity(errorBody(e)).build();
        case UseCaseError.NotFoundError e        -> Response.status(404).entity(errorBody(e)).build();
        case UseCaseError.BusinessRuleViolation e -> Response.status(409).entity(errorBody(e)).build();
        case UseCaseError.ConcurrencyError e     -> Response.status(409).entity(errorBody(e)).build();
        case UseCaseError.AuthorizationError e   -> Response.status(403).entity(errorBody(e)).build();
    };
};
```

# Coding Standards

Code conventions and patterns used in FlowCatalyst.

## ID Handling - TSID

**All entity IDs use TSID (Time-Sorted ID) as Crockford Base32 strings.**

### Format

- **Length**: 13 characters
- **Example**: `0HZXEQ5Y8JY5Z`
- **Properties**:
  - Lexicographically sortable
  - URL-safe and case-insensitive
  - JavaScript-safe (no precision loss)

### Usage

```java
import tech.flowcatalyst.platform.shared.TsidGenerator;

// Generate new ID
String id = TsidGenerator.generate();  // "0HZXEQ5Y8JY5Z"

// Convert between formats
Long longId = TsidGenerator.toLong("0HZXEQ5Y8JY5Z");
String strId = TsidGenerator.toString(786259737685263979L);
```

### Entity Pattern

```java
@MongoEntity(collection = "my_entities")
public class MyEntity extends PanacheMongoEntityBase {
    @BsonId
    public String id;  // TSID Crockford Base32

    public String relatedEntityId;  // Foreign keys are also String
}

// Repository
public class MyEntityRepository
    implements PanacheMongoRepositoryBase<MyEntity, String> {}
```

## Domain-Driven Design Patterns

### Result Type

Use `Result<T>` for operations that can fail:

```java
public record Result<T>(T value, UseCaseError error) {

    public static <T> Result<T> success(T value) {
        return new Result<>(value, null);
    }

    public static <T> Result<T> failure(UseCaseError error) {
        return new Result<>(null, error);
    }

    public boolean isSuccess() {
        return error == null;
    }

    public boolean isFailure() {
        return error != null;
    }
}
```

Usage:

```java
public Result<User> createUser(CreateUserCommand cmd) {
    if (userExists(cmd.email())) {
        return Result.failure(new UserAlreadyExistsError(cmd.email()));
    }
    User user = // create user
    return Result.success(user);
}
```

### Use Cases

Encapsulate business logic in use cases:

```java
@ApplicationScoped
public class CreateUserUseCase {

    @Inject
    PrincipalRepository repository;

    public Result<Principal> execute(CreateUserCommand cmd) {
        // Validation
        if (cmd.email() == null) {
            return Result.failure(new ValidationError("email required"));
        }

        // Business logic
        Principal principal = new Principal();
        principal.id = TsidGenerator.generate();
        principal.userIdentity = new UserIdentity(cmd.email(), ...);

        // Persistence
        repository.persist(principal);

        return Result.success(principal);
    }
}
```

### Commands

Use records for commands:

```java
public record CreateUserCommand(
    String email,
    String name,
    String clientId
) {}
```

### Domain Events

Emit events for significant state changes:

```java
public record UserCreated(
    String userId,
    String email,
    Instant timestamp
) implements DomainEvent {}

// In use case
eventPublisher.publish(new UserCreated(user.id, user.email, Instant.now()));
```

## Package Structure

```
tech.flowcatalyst.platform/
├── {domain}/
│   ├── entity/           # Entities and value objects
│   ├── operations/       # Use cases organized by operation
│   │   ├── create/
│   │   ├── update/
│   │   └── delete/
│   ├── events/           # Domain events
│   └── {Domain}Repository.java
```

Example:

```
tech.flowcatalyst.platform.principal/
├── Principal.java
├── UserIdentity.java
├── PrincipalType.java
├── UserScope.java
├── PrincipalRepository.java
├── operations/
│   ├── createuser/
│   │   ├── CreateUserCommand.java
│   │   └── CreateUserUseCase.java
│   └── deactivateuser/
│       ├── DeactivateUserCommand.java
│       └── DeactivateUserUseCase.java
└── events/
    ├── UserCreated.java
    └── UserDeactivated.java
```

## Naming Conventions

### Classes

| Type | Pattern | Example |
|------|---------|---------|
| Entity | `{Name}` | `Principal`, `DispatchJob` |
| Repository | `{Entity}Repository` | `PrincipalRepository` |
| Use Case | `{Action}{Entity}UseCase` | `CreateUserUseCase` |
| Command | `{Action}{Entity}Command` | `CreateUserCommand` |
| Event | `{Entity}{Action}` | `UserCreated`, `JobCompleted` |

### Methods

```java
// Queries - start with find/get/list
findById(id)
findByEmail(email)
listActive()
getConfig()

// Commands - start with action verb
create(command)
update(command)
delete(id)
activate(id)
deactivate(id)
```

### Variables

```java
// Use descriptive names
Principal principal;        // Not: p, user1
DispatchJob dispatchJob;    // Not: dj, job
List<Client> clients;       // Not: list, c

// ID variables
String principalId;         // Not: id (when multiple IDs in scope)
String clientId;
String eventId;
```

## Error Handling

### Use Case Errors

```java
public sealed interface UseCaseError
    permits ValidationError, NotFoundError, ConflictError, AuthorizationError {

    String code();
    String message();
}

public record ValidationError(String field, String message)
    implements UseCaseError {
    @Override public String code() { return "VALIDATION_ERROR"; }
}

public record NotFoundError(String entityType, String id)
    implements UseCaseError {
    @Override public String code() { return "NOT_FOUND"; }
    @Override public String message() {
        return entityType + " not found: " + id;
    }
}
```

### Endpoint Error Mapping

```java
@POST
public Response create(CreateUserCommand cmd) {
    Result<User> result = useCase.execute(cmd);

    if (result.isFailure()) {
        return switch (result.error()) {
            case ValidationError e -> Response.status(400).entity(e).build();
            case NotFoundError e -> Response.status(404).entity(e).build();
            case ConflictError e -> Response.status(409).entity(e).build();
            default -> Response.status(500).entity(result.error()).build();
        };
    }

    return Response.status(201).entity(result.value()).build();
}
```

## Testing Standards

### Test Naming

```java
// Pattern: should{ExpectedBehavior}When{Condition}
void shouldCreateUserWhenEmailIsValid()
void shouldReturnErrorWhenEmailExists()
void shouldRetryWhenConnectionFails()
```

### Test Structure

```java
@Test
void shouldCreateUser() {
    // Given (Arrange)
    CreateUserCommand cmd = new CreateUserCommand("test@example.com", "Test");

    // When (Act)
    Result<User> result = useCase.execute(cmd);

    // Then (Assert)
    assertTrue(result.isSuccess());
    assertEquals("test@example.com", result.value().email());
}
```

## Documentation

### Javadoc

Document public APIs:

```java
/**
 * Creates a new user principal.
 *
 * @param cmd the creation command
 * @return Result containing the created principal or error
 * @throws IllegalArgumentException if cmd is null
 */
public Result<Principal> execute(CreateUserCommand cmd) {
```

### Comments

- Use comments for "why", not "what"
- Avoid obvious comments
- Keep comments up-to-date

```java
// Good: Explains why
// Use exponential backoff to avoid overwhelming the service during recovery
retryDelay = Math.min(retryDelay * 2, MAX_RETRY_DELAY);

// Bad: States the obvious
// Increment counter
counter++;
```

## Code Quality

### Avoid

- `null` where Optional is appropriate
- Mutable state where immutable works
- Large methods (prefer < 20 lines)
- Deep nesting (prefer early returns)
- Magic numbers (use constants)

### Prefer

- Records for value objects
- Sealed interfaces for error types
- Pattern matching (Java 21)
- Stream operations for collections
- Constructor injection

## See Also

- [Entity Overview](../entities/overview.md) - Entity patterns
- [Testing Guide](testing.md) - Test patterns
- [Architecture Overview](../architecture/overview.md) - System design

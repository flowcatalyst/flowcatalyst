# Java 21 Sealed Classes vs TypeScript Discriminated Unions for DDD

A comparison for building business domain-oriented applications that need to be enjoyable to work with, easy to reason about, maintainable, and safe to evolve without regressions.

---

## Executive Summary

**Recommendation: Java 21 with sealed classes** provides stronger compile-time guarantees for enterprise domain applications that need to evolve safely over years.

The key differentiator: **true compiler-enforced exhaustiveness checking**. When you add a new domain error or event type, Java's compiler immediately flags every location that needs updating. TypeScript relies on conventions and linter rules that can fail silently.

---

## Compile-Time Guarantees Comparison

| Aspect | Java 21 Sealed | TypeScript Discriminated Unions |
|--------|----------------|--------------------------------|
| Exhaustiveness checking | Compiler-enforced | Convention + ESLint rules |
| Add new variant to union | Compiler errors at every switch | May silently fall through |
| Pattern matching | Native with deconstruction | `_tag` string comparison |
| Type discrimination | Compile-time, type system | Runtime `_tag` field |
| Illegal state prevention | Type system enforced | Convention-based |
| Refactoring safety | Excellent IDE support | Good but can miss edge cases |

---

## Pattern Matching Comparison

### Java 21

```java
sealed interface DomainError permits ValidationError, NotFoundError, BusinessRuleViolation {}

record ValidationError(String field, String message) implements DomainError {}
record NotFoundError(String entity, String id) implements DomainError {}
record BusinessRuleViolation(String rule, String message) implements DomainError {}

// Pattern matching with deconstruction - compiler enforces exhaustiveness
String handle(DomainError error) {
    return switch (error) {
        case ValidationError(var field, var msg) -> "Invalid " + field + ": " + msg;
        case NotFoundError(var entity, var id) -> entity + " not found: " + id;
        case BusinessRuleViolation(var rule, var msg) -> "Rule " + rule + " violated: " + msg;
        // Compiler error if you miss a case!
    };
}
```

### TypeScript

```typescript
type DomainError = ValidationError | NotFoundError | BusinessRuleViolation;

interface ValidationError {
  readonly _tag: 'ValidationError';
  readonly field: string;
  readonly message: string;
}

interface NotFoundError {
  readonly _tag: 'NotFoundError';
  readonly entity: string;
  readonly id: string;
}

interface BusinessRuleViolation {
  readonly _tag: 'BusinessRuleViolation';
  readonly rule: string;
  readonly message: string;
}

// Must use _tag for discrimination - convention, not enforced
function handle(error: DomainError): string {
  switch (error._tag) {
    case 'ValidationError':
      return `Invalid ${error.field}: ${error.message}`;
    case 'NotFoundError':
      return `${error.entity} not found: ${error.id}`;
    case 'BusinessRuleViolation':
      return `Rule ${error.rule} violated: ${error.message}`;
    // No compiler error if you miss a case (unless using never trick + strict config)
  }
}
```

---

## The `_tag` Convention Problem

TypeScript's discriminated unions rely on a **convention** (the `_tag` field), not a compiler-enforced constraint:

```typescript
// Nothing stops this at compile time:
const fake: ValidationError = {
  _tag: 'ValidationError',
  field: 123 as any,  // Should be string - passes at compile time
  message: null as any  // Should be string - passes at compile time
};

// Or worse - typo in _tag:
const typo = {
  _tag: 'ValidatonError',  // Typo! Won't match any case
  field: 'email',
  message: 'invalid'
};
```

### Java Equivalent - Compiler Rejects This

```java
// Cannot create a ValidationError without exact signature
var error = new ValidationError(123, null);  // Compile error!

// Cannot create fake implementations of sealed interface
class FakeError implements DomainError {}  // Compile error - not in permits list
```

---

## Adding a New Domain Error Type

### Java - Compiler Catches Everything

```java
// Step 1: Add new error type to sealed interface
sealed interface DomainError permits
    ValidationError,
    NotFoundError,
    BusinessRuleViolation,
    RateLimitError {}  // <-- New!

record RateLimitError(String resource, int retryAfterSeconds) implements DomainError {}

// Step 2: Compiler IMMEDIATELY shows errors at every switch statement
// that doesn't handle RateLimitError

// In OrderController.java - COMPILE ERROR:
return switch (error) {
    case ValidationError e -> Response.status(400).entity(e).build();
    case NotFoundError e -> Response.status(404).entity(e).build();
    case BusinessRuleViolation e -> Response.status(422).entity(e).build();
    // ERROR: switch expression does not cover all possible input values
};

// In ErrorLogger.java - COMPILE ERROR:
// In MetricsService.java - COMPILE ERROR:
// etc.
```

### TypeScript - Relies on Conventions

```typescript
// Step 1: Add new error type to union
type DomainError =
  | ValidationError
  | NotFoundError
  | BusinessRuleViolation
  | RateLimitError;  // <-- New!

interface RateLimitError {
  readonly _tag: 'RateLimitError';
  readonly resource: string;
  readonly retryAfterSeconds: number;
}

// Step 2: NO automatic compiler errors unless:
// - You've added the "never" exhaustiveness trick to EVERY switch
// - ESLint is configured with the right rules
// - Your CI catches it

// This switch silently ignores the new error type:
switch (error._tag) {
  case 'ValidationError': return handleValidation(error);
  case 'NotFoundError': return handleNotFound(error);
  case 'BusinessRuleViolation': return handleBusinessRule(error);
  // RateLimitError silently falls through - no error!
}

// Only safe if you've added the exhaustiveness trick:
switch (error._tag) {
  case 'ValidationError': return handleValidation(error);
  case 'NotFoundError': return handleNotFound(error);
  case 'BusinessRuleViolation': return handleBusinessRule(error);
  case 'RateLimitError': return handleRateLimit(error);
  default: {
    const _exhaustive: never = error;  // NOW this errors
    throw new Error(`Unhandled error: ${_exhaustive}`);
  }
}
```

---

## Refactoring Safety

### Java (IntelliJ)

| Operation | Safety Level |
|-----------|--------------|
| Rename sealed interface | All implementations auto-update |
| Rename record field | All usages auto-update |
| Extract method from switch | Type inference preserved |
| Find all usages | Truly exhaustive |
| Add parameter to record | Compile errors at all call sites |

### TypeScript (VS Code)

| Operation | Safety Level |
|-----------|--------------|
| Rename interface | Usually works, can miss string literals |
| Rename field | Works within typed code, misses `any` |
| Extract method | Type inference sometimes lost |
| Find all usages | Can miss dynamic access patterns |
| Add field to interface | No errors for missing optional fields |

---

## Where TypeScript Wins

| Aspect | Advantage |
|--------|-----------|
| **Null safety** | Strict mode handles `undefined` better than Java's Optional |
| **Dev cycle** | Faster edit-refresh, no compile step |
| **Conciseness** | Slightly less boilerplate for simple types |
| **Full-stack sharing** | Same types for frontend and backend |
| **Ecosystem** | npm has packages for everything |
| **Onboarding** | More developers know JavaScript/TypeScript |

---

## Where Java 21 Wins

| Aspect | Advantage |
|--------|-----------|
| **Compile-time safety** | True exhaustiveness checking |
| **Refactoring confidence** | IDE support is more mature |
| **Runtime guarantees** | Sealed classes enforced at runtime too |
| **Performance** | JVM optimizations, no runtime type checks |
| **Enterprise ecosystem** | Spring/Quarkus, proven at scale |
| **Long-term maintenance** | Compiler catches more regressions |
| **Pattern matching** | Native deconstruction, not string comparison |

---

## Code Evolution Scenario

### Scenario: Add retry logic to all error handlers

**Java approach:**
1. Add `boolean isRetryable()` method to sealed interface
2. Compiler forces implementation in all records (or provide default)
3. All switch expressions continue to work
4. New method available everywhere immediately

```java
sealed interface DomainError permits ValidationError, NotFoundError, BusinessRuleViolation {
    default boolean isRetryable() { return false; }
}

record ValidationError(String field, String message) implements DomainError {
    // Uses default - not retryable
}

record NotFoundError(String entity, String id) implements DomainError {
    @Override
    public boolean isRetryable() { return true; }  // Network issues may resolve
}
```

**TypeScript approach:**
1. Add `isRetryable` to each interface
2. No compiler enforcement that all variants have it
3. Must remember to add to all existing interfaces
4. Runtime errors if you forget one

```typescript
interface ValidationError {
  readonly _tag: 'ValidationError';
  readonly field: string;
  readonly message: string;
  readonly isRetryable: false;  // Must add manually
}

interface NotFoundError {
  readonly _tag: 'NotFoundError';
  readonly entity: string;
  readonly id: string;
  readonly isRetryable: boolean;  // Must add manually - easy to forget!
}

// If you forget to add isRetryable to one interface:
function shouldRetry(error: DomainError): boolean {
  return error.isRetryable;  // Runtime error if field doesn't exist!
}
```

---

## Recommendation for FlowCatalyst

For an **enterprise platform** that needs to:

- ✅ Evolve safely over years
- ✅ Be maintained by different developers over time
- ✅ Have confidence in refactoring
- ✅ Prevent regressions when adding new domain concepts
- ✅ Catch errors at compile time, not runtime

**Java 21 with sealed classes + records** provides stronger guarantees.

The TypeScript approach works - many teams use it successfully - but it requires:
- More discipline in applying conventions consistently
- ESLint rules configured correctly
- The `never` exhaustiveness trick in every switch
- Vigilance during code review

Java's sealed classes make these guarantees **automatic and compiler-enforced**.

---

## Summary Table

| Criteria | Java 21 Sealed | TypeScript |
|----------|----------------|------------|
| Enjoyable to work with | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Easy to reason about | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Maintainable | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Safe evolution | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Regression prevention | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Compile-time safety | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Dev cycle speed | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Ecosystem breadth | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

**Bottom line**: Both approaches are valid. TypeScript is faster for initial development. Java 21 is safer for long-term evolution of business-critical domain code.

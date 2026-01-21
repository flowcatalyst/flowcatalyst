# Testing Guide

FlowCatalyst uses JUnit 5 with Quarkus testing extensions. This guide covers running tests, writing tests, and testing patterns.

## Running Tests

### All Tests

```bash
# All modules
./gradlew test

# Specific module
./gradlew :core:flowcatalyst-message-router:test
./gradlew :core:flowcatalyst-platform:test
```

### Unit Tests Only

```bash
# Exclude integration tests
./gradlew test -x integrationTest
```

### Integration Tests

```bash
# Run integration tests
./gradlew :core:flowcatalyst-message-router:integrationTest
```

### Single Test

```bash
# Run specific test class
./gradlew :core:flowcatalyst-message-router:test \
  --tests "QueueManagerTest"

# Run specific test method
./gradlew :core:flowcatalyst-message-router:test \
  --tests "QueueManagerTest.testMessageRouting"
```

## Test Categories

### Unit Tests

Fast, isolated tests without external dependencies.

```java
@QuarkusTest
class ProcessPoolTest {

    @Inject
    ProcessPool pool;

    @Test
    void shouldProcessMessage() {
        // Given
        MessagePointer message = createTestMessage();

        // When
        boolean result = pool.submit(message, callback);

        // Then
        assertTrue(result);
    }
}
```

### Integration Tests

Tests with real dependencies (MongoDB, queues).

```java
@QuarkusIntegrationTest
class QueueManagerIntegrationTest {

    @Test
    void shouldRouteMessageToPool() {
        // Test with actual queue consumer
    }
}
```

### Component Tests

Tests for specific components in isolation.

```java
@QuarkusTest
class HttpMediatorTest {

    @Inject
    HttpMediator mediator;

    @Test
    void shouldRetryOnConnectionError() {
        // Test retry logic
    }
}
```

## Test Utilities

### Test Fixtures

```java
public class TestFixtures {

    public static MessagePointer createTestMessage() {
        return new MessagePointer(
            TsidGenerator.generate(),
            "TEST-POOL",
            "Bearer test-token",
            "HTTP",
            "http://localhost:8080/test"
        );
    }

    public static DispatchJob createTestJob() {
        DispatchJob job = new DispatchJob();
        job.id = TsidGenerator.generate();
        job.kind = DispatchKind.EVENT;
        job.code = "test.event";
        job.status = DispatchStatus.PENDING;
        return job;
    }
}
```

### Mock Services

```java
@Mock
@RestClient
ConfigService mockConfigService;

@BeforeEach
void setup() {
    when(mockConfigService.getConfig())
        .thenReturn(createTestConfig());
}
```

### Test Containers

```java
@QuarkusTest
@TestProfile(MongoTestProfile.class)
class RepositoryTest {

    @Container
    static MongoDBContainer mongo = new MongoDBContainer("mongo:7");

    @Test
    void shouldPersistEntity() {
        // Test with real MongoDB
    }
}
```

## Writing Tests

### Test Structure

Follow Arrange-Act-Assert pattern:

```java
@Test
void shouldHandleErrorGracefully() {
    // Arrange (Given)
    MessagePointer message = createTestMessage();
    when(mediator.process(any())).thenReturn(MediationResult.ERROR_CONNECTION);

    // Act (When)
    boolean result = pool.submit(message, callback);

    // Assert (Then)
    assertTrue(result);
    verify(callback).nack(message);
}
```

### Naming Conventions

```java
// Use descriptive method names
void shouldRouteMessageToCorrectPool()
void shouldRetryOnTransientError()
void shouldNotRetryOnPermanentError()
void shouldRateLimitWhenConfigured()
```

### Testing Async Code

```java
@Test
void shouldProcessAsynchronously() throws Exception {
    // Use Awaitility for async assertions
    await()
        .atMost(5, TimeUnit.SECONDS)
        .until(() -> messageProcessed.get());

    assertEquals(1, processedCount.get());
}
```

## Mocking

### Mockito Integration

```java
@QuarkusTest
class ServiceTest {

    @InjectMock
    ExternalService externalService;

    @Test
    void shouldHandleExternalFailure() {
        when(externalService.call(any()))
            .thenThrow(new RuntimeException("Service unavailable"));

        // Test error handling
    }
}
```

### WireMock for HTTP

```java
@QuarkusTest
@WithWireMock
class HttpMediatorTest {

    @WireMockServer
    WireMockServer wireMock;

    @Test
    void shouldRetryOn503() {
        wireMock.stubFor(post("/webhook")
            .willReturn(aResponse().withStatus(503)));

        // Test retry behavior
    }
}
```

## Test Configuration

### Test Properties

```properties
# src/test/resources/application.properties

# Use in-memory database for tests
quarkus.mongodb.connection-string=mongodb://localhost:27017
quarkus.mongodb.database=flowcatalyst-test

# Disable external services
message-router.enabled=false
```

### Test Profiles

```java
public class TestProfile implements QuarkusTestProfile {
    @Override
    public Map<String, String> getConfigOverrides() {
        return Map.of(
            "message-router.queue-type", "CHRONICLE",
            "quarkus.mongodb.database", "test-db"
        );
    }
}
```

## Test Patterns

### Repository Tests

```java
@QuarkusTest
class DispatchJobRepositoryTest {

    @Inject
    DispatchJobRepository repository;

    @BeforeEach
    void cleanup() {
        repository.deleteAll();
    }

    @Test
    void shouldFindByStatus() {
        // Given
        DispatchJob job = createTestJob();
        job.status = DispatchStatus.PENDING;
        repository.persist(job);

        // When
        List<DispatchJob> pending = repository
            .find("status", DispatchStatus.PENDING)
            .list();

        // Then
        assertEquals(1, pending.size());
    }
}
```

### Service Tests

```java
@QuarkusTest
class DispatchServiceTest {

    @Inject
    DispatchService service;

    @InjectMock
    QueueClient queueClient;

    @Test
    void shouldQueueJob() {
        // Given
        DispatchJob job = createTestJob();

        // When
        service.queue(job);

        // Then
        verify(queueClient).send(any());
        assertEquals(DispatchStatus.QUEUED, job.status);
    }
}
```

### Endpoint Tests

```java
@QuarkusTest
class DispatchJobResourceTest {

    @Test
    void shouldCreateJob() {
        given()
            .contentType(ContentType.JSON)
            .body(createJobRequest())
        .when()
            .post("/api/dispatch-jobs")
        .then()
            .statusCode(201)
            .body("id", notNullValue());
    }
}
```

## Coverage

### Running with Coverage

```bash
./gradlew test jacocoTestReport
```

### Coverage Report

Report generated at `build/reports/jacoco/test/html/index.html`

## See Also

- [Development Setup](setup.md) - Environment setup
- [Build Reference](build-reference.md) - Build commands
- [Coding Standards](coding-standards.md) - Code conventions

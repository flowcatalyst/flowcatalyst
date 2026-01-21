# Testing Guide - Simplified Unit Tests

## Overview

We've simplified our unit tests to make them **faster, more readable, and easier to maintain**.

### Key Changes:

1. ✅ **Removed `@QuarkusTest` from unit tests** - Faster execution, no container startup
2. ✅ **Constructor injection for testability** - No reflection hacks needed
3. ✅ **Test utilities** - Reduced boilerplate
4. ✅ **Pure unit tests** - All dependencies mocked

## Before vs After

### Before (Complex):
```java
@QuarkusTest  // ❌ Slow: starts Quarkus container
class QueueManagerTest {
    @Inject
    QueueManager queueManager;  // ❌ CDI complexity

    @InjectMock
    Mediator mediator;  // ❌ Special mock annotations

    @BeforeEach
    void setUp() throws Exception {
        // ❌ Reflection hack to access dependencies
        Field field = queueManager.getClass().getDeclaredField("someField");
        field.setAccessible(true);
        field.set(queueManager, mockObject);
    }
}
```

### After (Simple):
```java
// ✅ No @QuarkusTest - just plain JUnit
class QueueManagerTest {
    private QueueManager queueManager;
    private Mediator mockMediator;

    @BeforeEach
    void setUp() {
        // ✅ Simple mocking
        mockMediator = mock(Mediator.class);

        // ✅ Constructor injection (no reflection!)
        queueManager = new QueueManager(
            mockConfigClient,
            mockQueueConsumerFactory,
            mockMediatorFactory,
            mockQueueValidationService,
            mockPoolMetrics,
            mockWarningService,
            mockMeterRegistry,
            true,   // messageRouterEnabled
            2000,   // maxPools
            1000    // poolWarningThreshold
        );
    }
}
```

## When to Use What

### Use Plain JUnit (No `@QuarkusTest`) For:
- ✅ **Unit tests** - Testing a single class in isolation
- ✅ **Fast tests** - No need for Quarkus startup
- ✅ **Pure logic** - Business logic, algorithms, utilities
- ✅ **Mocked dependencies** - All collaborators are mocks

### Use `@QuarkusTest` For:
- ✅ **Integration tests** - Testing multiple components together
- ✅ **Database tests** - Need real database interaction
- ✅ **HTTP tests** - Testing REST endpoints with real HTTP server
- ✅ **Container tests** - Need TestContainers (Keycloak, LocalStack, etc.)

## Writing a New Unit Test

### Step 1: No Annotations Needed!
```java
class MyServiceTest {  // ✅ Just a plain class
    private MyService service;
    private Dependency mockDependency;
```

### Step 2: Create Mocks in `@BeforeEach`
```java
@BeforeEach
void setUp() {
    mockDependency = mock(Dependency.class);

    // Use constructor or package-private constructor
    service = new MyService(mockDependency);
}
```

### Step 3: Write Tests Using Given-When-Then
```java
@Test
void shouldProcessMessageSuccessfully() {
    // Given
    Message message = TestUtils.createMessage("msg-1", "TEST-POOL");
    when(mockDependency.process(message)).thenReturn(SUCCESS);

    // When
    boolean result = service.handle(message);

    // Then
    assertTrue(result);
    verify(mockDependency).process(message);
}
```

### Step 4: Use TestUtils for Common Operations
```java
// Create test messages easily
MessagePointer msg = TestUtils.createMessage("id", "pool");

// Access private fields for verification (only if needed)
Map<String, Object> internalMap = TestUtils.getPrivateField(service, "internalMap");

// Sleep without checked exception
TestUtils.sleep(100);
```

## Testing Async Code

### Use Awaitility (Already in Classpath)
```java
@Test
void shouldProcessAsync() {
    // Given
    service.submitAsync(message);

    // When - wait for async processing
    await().untilAsserted(() -> {
        verify(mockDependency).process(message);
        assertTrue(service.isCompleted());
    });
}
```

## Common Patterns

### Testing Concurrency
```java
@Test
void shouldRespectConcurrencyLimit() {
    // Given pool with concurrency = 2
    when(mockMediator.process(any())).thenAnswer(invocation -> {
        Thread.sleep(100);  // Simulate slow processing
        return SUCCESS;
    });

    // When - submit 5 messages
    for (int i = 0; i < 5; i++) {
        pool.submit(TestUtils.createMessage("msg-" + i, "POOL"));
    }

    // Then - verify concurrency limit
    await().untilAsserted(() -> {
        int active = pool.getActiveWorkers();
        assertThat(active).isLessThanOrEqual(2);
    });
}
```

### Testing Error Handling
```java
@Test
void shouldHandleErrorGracefully() {
    // Given
    when(mockMediator.process(any())).thenThrow(new RuntimeException("Test error"));

    // When
    boolean result = service.process(message);

    // Then
    assertFalse(result);
    verify(mockWarningService).addWarning(
        eq("MEDIATION"),
        eq("ERROR"),
        contains("Test error"),
        any()
    );
}
```

## Performance Comparison

### Before (with @QuarkusTest):
```
ProcessPoolImplTest: 12.5 seconds
QueueManagerTest: 15.3 seconds
Total: 27.8 seconds
```

### After (pure unit tests):
```
ProcessPoolImplTest: 0.8 seconds  (15x faster!)
QueueManagerTest: 1.2 seconds     (12x faster!)
Total: 2.0 seconds                (14x faster!)
```

## Making Your Class Testable

If your class uses CDI (`@ApplicationScoped`, `@Inject`), add a package-private test constructor:

```java
@ApplicationScoped
public class MyService {
    @Inject
    Dependency dependency;

    // Default constructor for CDI
    public MyService() {
    }

    // Test-friendly constructor (package-private)
    MyService(Dependency dependency) {
        this.dependency = dependency;
    }
}
```

## Troubleshooting

### Problem: "Cannot mock final class"
**Solution**: Use Mockito inline (already configured in build.gradle.kts)

### Problem: "Test is slow"
**Solution**: Remove `@QuarkusTest` if it's a unit test

### Problem: "Need to access private field"
**Solution 1 (preferred)**: Add package-private getter/constructor
**Solution 2**: Use `TestUtils.getPrivateField()` (sparingly)

### Problem: "Async test is flaky"
**Solution**: Use Awaitility with appropriate timeout:
```java
await()
    .atMost(Duration.ofSeconds(5))
    .pollInterval(Duration.ofMillis(100))
    .untilAsserted(() -> {
        // your assertion
    });
```

## Best Practices

1. ✅ **One assertion concept per test** - Test one thing at a time
2. ✅ **Given-When-Then structure** - Makes tests readable
3. ✅ **Descriptive test names** - Use `shouldDoSomethingWhenCondition()`
4. ✅ **Minimal setup** - Only mock what's needed
5. ✅ **Clean up** - Use `@AfterEach` to clean up resources
6. ✅ **Fast tests** - Unit tests should run in milliseconds
7. ✅ **Isolated tests** - No shared state between tests

## Integration & E2E Testing

### When to Use Integration Tests

Use `@QuarkusTest` with test resources for:
- ✅ Testing message flow through multiple components
- ✅ Validating queue behavior (SQS, ActiveMQ)
- ✅ Testing FIFO ordering with real queues
- ✅ Validating resilience patterns (timeouts, retries)
- ✅ Testing complete end-to-end message processing

### Test Infrastructure

All integration tests use **local containerized infrastructure** via TestContainers:

| Resource | Purpose | Duration |
|----------|---------|----------|
| LocalStack | Local AWS SQS (including FIFO queues) | ~5s startup |
| ActiveMQ | Local JMS broker | ~3s startup |
| WireMock | Mock HTTP endpoints | <1s startup |

**No cloud services required** - tests run completely offline!

### Integration Test Pattern

```java
@QuarkusTest
@QuarkusTestResource(LocalStackTestResource.class)
@QuarkusTestResource(WireMockTestResource.class)
@Tag("integration")
class MyIntegrationTest {
    @Inject
    SqsClient sqsClient;  // Auto-configured to use LocalStack

    @Test
    void shouldProcessMessageEndToEnd() {
        // Given: Real infrastructure
        String queueUrl = createTestQueue();
        stubFor(post("/webhook").willReturn(ok()));

        // When: Send real message
        sqsClient.sendMessage(SendMessageRequest.builder()
            .queueUrl(queueUrl)
            .messageBody(messageJson)
            .build());

        // Then: Verify with Awaitility
        await().atMost(5, SECONDS).untilAsserted(() -> {
            verify(postRequestedFor(urlEqualTo("/webhook")));
        });
    }
}
```

### FIFO Integration Testing

Test FIFO ordering with real SQS FIFO queues in LocalStack:

```java
@Test
void shouldMaintainFifoOrdering() {
    // Given: Create SQS FIFO queue
    String queueUrl = sqsClient.createQueue(CreateQueueRequest.builder()
        .queueName("test-" + UUID.randomUUID() + ".fifo")
        .attributes(Map.of(
            QueueAttributeName.FIFO_QUEUE, "true",
            QueueAttributeName.CONTENT_BASED_DEDUPLICATION, "true"
        ))
        .build()).queueUrl();

    // When: Send messages with messageGroupId
    for (int i = 1; i <= 5; i++) {
        sqsClient.sendMessage(SendMessageRequest.builder()
            .queueUrl(queueUrl)
            .messageBody("{\"id\":\"msg-" + i + "\"}")
            .messageGroupId("order-12345")  // Same group = FIFO ordering
            .messageDeduplicationId(UUID.randomUUID().toString())
            .build());
    }

    // Then: Verify FIFO ordering maintained
    await().atMost(10, SECONDS).untilAsserted(() -> {
        assertEquals(Arrays.asList("msg-1", "msg-2", "msg-3", "msg-4", "msg-5"),
            processedMessages);
    });
}
```

**Key Points:**
- Queue name must end with `.fifo`
- Set `FIFO_QUEUE` attribute to `"true"`
- Use `messageGroupId` to enforce ordering
- Use `messageDeduplicationId` for idempotency

### Batch+Group FIFO Testing

Test cascading nacks when messages fail within a batch+group:

```java
@Test
void shouldCascadeNacksInFailedBatchGroup() {
    // Given: FIFO queue with mixed message groups
    String queueUrl = createFifoQueue();

    // Stub: First message fails, rest succeed
    stubFor(post("/webhook/order-12345")
        .inScenario("cascade")
        .whenScenarioStateIs(STARTED)
        .willReturn(serverError())  // First call fails
        .willSetStateTo("failed"));

    stubFor(post("/webhook/order-12345")
        .inScenario("cascade")
        .whenScenarioStateIs("failed")
        .willReturn(ok()));  // Subsequent calls succeed (but should be NACKed)

    // When: Send 3 messages in same batch+group
    String batchId = UUID.randomUUID().toString();
    for (int i = 1; i <= 3; i++) {
        sendMessage(queueUrl, "msg-" + i, "order-12345");
    }

    // Then: First message fails, subsequent messages cascade NACK
    await().atMost(10, SECONDS).untilAsserted(() -> {
        assertTrue(nackedMessages.contains("msg-1"), "First message should be NACKed");
        assertTrue(nackedMessages.contains("msg-2"), "Second message should cascade NACK");
        assertTrue(nackedMessages.contains("msg-3"), "Third message should cascade NACK");

        // Only first message should reach endpoint
        verify(1, postRequestedFor(urlEqualTo("/webhook/order-12345")));
    });
}
```

**Key Concept**: When a message fails in a batch+group, all subsequent messages in that batch+group are automatically NACKed to preserve FIFO ordering.

### Resilience Testing

Test timeout handling, error responses, and recovery:

```java
@Test
void shouldHandleTimeoutGracefully() {
    // Given: Slow endpoint (15s delay > 10s timeout)
    stubFor(post("/webhook/slow")
        .willReturn(aResponse()
            .withStatus(200)
            .withFixedDelay(15000)));  // Exceeds 10s test timeout

    // When: Process message
    MessagePointer message = new MessagePointer(
        "msg-timeout",
        "TEST-POOL",
        "token",
        MediationType.HTTP,
        webhookBaseUrl + "/webhook/slow",
        "timeout-group",
        UUID.randomUUID().toString()
    );
    processPool.submit(message);

    // Then: Should timeout and NACK
    await().atMost(20, SECONDS).untilAsserted(() -> {
        assertTrue(nackedMessages.contains("msg-timeout"));
        // Verify HTTP call was made (proves timeout, not connection failure)
        verify(postRequestedFor(urlEqualTo("/webhook/slow")));
    });
}

@Test
void shouldHandleErrorResponsesCorrectly() {
    // Given: Error endpoints
    stubFor(post("/webhook/500").willReturn(serverError()));       // NACK + retry
    stubFor(post("/webhook/404").willReturn(notFound()));          // ACK (config error)
    stubFor(post("/webhook/400").willReturn(badRequest()));        // NACK + retry

    // When/Then: Verify error handling
    submitAndVerify("msg-500", "/webhook/500", true);   // NACKed
    submitAndVerify("msg-404", "/webhook/404", false);  // ACKed (don't retry config errors)
    submitAndVerify("msg-400", "/webhook/400", true);   // NACKed
}
```

### Rate Limiting Testing

Test rate limiter enforcement with token bucket algorithm:

```java
@Test
void shouldEnforceRateLimiting() {
    // Given: Pool with 300 requests/minute rate limit
    ProcessPool rateLimitedPool = new ProcessPoolImpl(
        "RATE-LIMITED",
        10,    // High concurrency
        500,   // Large queue
        300,   // 300 requests/minute (token bucket grants all upfront)
        mediator,
        callback,
        inPipelineMap,
        poolMetrics,
        warningService
    );

    // When: Submit 350 messages (exceeds 300 permit limit)
    for (int i = 0; i < 350; i++) {
        MessagePointer msg = createMessage("msg-" + i, "RATE-LIMITED");
        rateLimitedPool.submit(msg);
    }

    // Then: First 300 succeed, remaining 50 are rate-limited
    await().atMost(30, SECONDS).untilAsserted(() -> {
        assertTrue(ackedMessages.size() <= 300,
            "At most 300 should succeed (rate limit)");
        assertTrue(nackedMessages.size() >= 50,
            "At least 50 should be rate-limited");
        assertEquals(350, ackedMessages.size() + nackedMessages.size(),
            "All messages should be processed");
    });
}
```

**Note**: Resilience4j RateLimiter uses token bucket algorithm - all 300 permits are granted upfront at the start of each 1-minute period, allowing bursts up to the limit.

### WireMock Patterns

Useful WireMock patterns for integration tests:

```java
// Basic stub
stubFor(post("/webhook").willReturn(ok()));

// With delay
stubFor(post("/webhook").willReturn(aResponse().withStatus(200).withFixedDelay(1000)));

// With body (MediationResponse format)
stubFor(post("/webhook").willReturn(okJson("{\"ack\":true}")));

// Scenario-based (stateful)
stubFor(post("/webhook")
    .inScenario("retry")
    .whenScenarioStateIs(STARTED)
    .willReturn(serverError())
    .willSetStateTo("failed"));

stubFor(post("/webhook")
    .inScenario("retry")
    .whenScenarioStateIs("failed")
    .willReturn(ok()));

// Verify calls
verify(postRequestedFor(urlEqualTo("/webhook")));
verify(exactly(3), postRequestedFor(urlEqualTo("/webhook")));
```

### Test Configuration

Integration tests use test-specific configuration in `src/test/resources/application.properties`:

```properties
# HTTP mediator timeout - 10 seconds for tests (default is 15 minutes for production)
%test.mediator.http.timeout.ms=10000

# Use LocalStack for SQS
%test.quarkus.sqs.endpoint-override=http://localhost:4566
%test.quarkus.sqs.aws.region=eu-west-1

# Disable schedulers to avoid test flakiness
%test.quarkus.scheduler.enabled=false
%test.queue.health.monitor.enabled=false
```

## Test Suite Organization

### Unit Tests (124 tests) - ~2 seconds
Fast, isolated, mocked dependencies. Run on every commit.

```bash
./gradlew :core:flowcatalyst-message-router:test
```

### Integration Tests (61 tests) - ~2 minutes
Real infrastructure, containerized services. Run before merge.

```bash
./gradlew :core:flowcatalyst-message-router:integrationTest
```

### Specific Test Class

```bash
./gradlew :core:flowcatalyst-message-router:integrationTest --tests '*ResilienceIntegrationTest*'
```

### All Tests

```bash
./gradlew :core:flowcatalyst-message-router:test :core:flowcatalyst-message-router:integrationTest
```

## Complete Test Reference

### Unit Test Classes
- `ProcessPoolImplTest` - Pool processing, concurrency, rate limiting
- `QueueManagerTest` - Queue routing, pool management
- `HttpMediatorTest` - HTTP client behavior, circuit breakers
- `MicrometerQueueMetricsServiceTest` - Metrics collection
- `InfrastructureHealthServiceTest` - Health checks

### Integration Test Classes
- `EmbeddedQueueBehaviorTest` (8 tests) - SQLite queue behavior
- `ActiveMqClassicIntegrationTest` (9 tests) - ActiveMQ integration
- `BatchGroupFifoIntegrationTest` (3 tests) - FIFO ordering, batch+group cascading
- `CompleteEndToEndTest` (4 tests) - End-to-end message flow
- `HealthCheckIntegrationTest` (6 tests) - Health endpoints
- `RateLimiterIntegrationTest` (6 tests) - Rate limiting
- `ResilienceIntegrationTest` (6 tests) - Timeouts, errors, recovery
- `SqsLocalStackIntegrationTest` (6 tests) - SQS integration
- `StalledPoolDetectionTest` (8 tests) - Stalled pool detection

## Examples

See these tests for reference:
- `ProcessPoolImplTest` - Pure unit test with mocked dependencies
- `QueueManagerTest` - Constructor injection without reflection
- `CompleteEndToEndTest` - E2E test with WireMock and LocalStack
- `BatchGroupFifoIntegrationTest` - FIFO ordering with real SQS FIFO queues
- `ResilienceIntegrationTest` - Comprehensive resilience testing

## Further Reading

- **TEST_PLAN_SUMMARY.md** - Complete test suite overview with all test details
- **MESSAGE_GROUP_FIFO.md** - Architecture documentation including batch+group FIFO
- **INTEGRATION_E2E_TEST_PLAN.md** - Original integration test planning document

## Questions?

If you're unsure whether to use `@QuarkusTest` or not, ask yourself:
- Am I testing a single class? → **No `@QuarkusTest`**
- Do I need a real database/HTTP server? → **Yes `@QuarkusTest`**
- Can I mock all dependencies? → **No `@QuarkusTest`**
- Does the test start containers? → **Yes `@QuarkusTest`**

**When in doubt, start without `@QuarkusTest`. Add it only if you truly need the Quarkus context.**

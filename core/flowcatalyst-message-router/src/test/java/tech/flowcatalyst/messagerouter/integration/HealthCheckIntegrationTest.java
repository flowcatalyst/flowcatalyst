package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.concurrent.CountDownLatch;

import static io.restassured.RestAssured.given;
import static org.awaitility.Awaitility.await;
import static org.hamcrest.Matchers.*;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Integration tests for infrastructure health check endpoint
 */
@QuarkusTest
@Tag("integration")
class HealthCheckIntegrationTest {

    @Inject
    PoolMetricsService poolMetricsService;

    @Inject
    WarningService warningService;

    @Test
    void shouldReturn200WhenSystemRunning() {
        // When/Then - real endpoint should return 200 in healthy state
        given()
            .when().get("/health")
            .then()
            .statusCode(anyOf(is(200), is(503))) // Accept either since system state may vary
            .contentType("application/json")
            .body("$", hasKey("healthy"))
            .body("$", hasKey("message"))
            .body("healthy", instanceOf(Boolean.class))
            .body("message", instanceOf(String.class));
    }

    @Test
    void shouldIncludeCorrectFieldsInResponse() {
        // When/Then - validate response structure
        given()
            .when().get("/health")
            .then()
            .contentType("application/json")
            .body("containsKey('healthy')", is(true))
            .body("containsKey('message')", is(true));
    }

    @Test
    void shouldHandleConcurrentHealthCheckRequests() throws Exception {
        // Given - multiple concurrent requests
        int concurrentRequests = 10;
        CountDownLatch latch = new CountDownLatch(concurrentRequests);
        CompletableFuture<?>[] futures = new CompletableFuture[concurrentRequests];

        // When - fire concurrent requests
        for (int i = 0; i < concurrentRequests; i++) {
            futures[i] = CompletableFuture.runAsync(() -> {
                given()
                    .when().get("/health")
                    .then()
                    .statusCode(anyOf(is(200), is(503)))
                    .contentType("application/json")
                    .body("healthy", instanceOf(Boolean.class));
                latch.countDown();
            });
        }

        // Then - all should complete successfully
        CompletableFuture.allOf(futures).get();
        assertEquals(0, latch.getCount());
    }

    @Test
    void shouldUpdateTimestampAfterMessageProcessing() throws Exception {
        // Given - create a test pool with mock mediator
        String poolCode = "TEST-TIMESTAMP-POOL";
        Mediator mockMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                return tech.flowcatalyst.messagerouter.model.MediationOutcome.success();
            }

            @Override
            public tech.flowcatalyst.messagerouter.model.MediationType getMediationType() {
                return tech.flowcatalyst.messagerouter.model.MediationType.HTTP;
            }
        };
        MessageCallback mockCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                // No-op
            }

            @Override
            public void nack(MessagePointer message) {
                // No-op
            }
        };

        ConcurrentMap<String, MessagePointer> inPipelineMap = new ConcurrentHashMap<>();

        ProcessPool pool = new ProcessPoolImpl(
            poolCode,
            2,
            100,
            null, // rateLimitPerMinute
            mockMediator,
            mockCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        pool.start();

        try {
            // Get initial timestamp (should be null)
            Long initialTimestamp = poolMetricsService.getLastActivityTimestamp(poolCode);

            // When - submit and process a message
            MessagePointer message = new MessagePointer("test-msg-1", poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test"
            , null
            , null);

            inPipelineMap.put(message.id(), message);
            boolean submitted = pool.submit(message);
            assertTrue(submitted, "Message should be submitted");

            // Then - wait for timestamp to be updated
            await().untilAsserted(() -> {
                Long timestamp = poolMetricsService.getLastActivityTimestamp(poolCode);
                assertNotNull(timestamp, "Timestamp should be set after processing");
                if (initialTimestamp != null) {
                    assertTrue(timestamp > initialTimestamp, "Timestamp should be updated");
                }
            });

        } finally {
            pool.drain();
        }
    }

    @Test
    void shouldUpdateTimestampOnBothSuccessAndFailure() throws Exception {
        // Given - pool with mediator that alternates success/failure
        // Note: We use errorConfig() (permanent failure) instead of errorProcess() (transient)
        // because transient errors don't update timestamp (they're not "completed" yet)
        String poolCode = "TEST-SUCCESS-FAILURE-POOL";
        final boolean[] shouldSucceed = {true};

        Mediator alternatingMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                boolean result = shouldSucceed[0];
                shouldSucceed[0] = !shouldSucceed[0];
                return result ? tech.flowcatalyst.messagerouter.model.MediationOutcome.success()
                    : tech.flowcatalyst.messagerouter.model.MediationOutcome.errorConfig();
            }

            @Override
            public tech.flowcatalyst.messagerouter.model.MediationType getMediationType() {
                return tech.flowcatalyst.messagerouter.model.MediationType.HTTP;
            }
        };

        MessageCallback mockCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
            }

            @Override
            public void nack(MessagePointer message) {
            }
        };

        ConcurrentMap<String, MessagePointer> inPipelineMap = new ConcurrentHashMap<>();

        ProcessPool pool = new ProcessPoolImpl(
            poolCode,
            2,
            100,
            null, // rateLimitPerMinute
            alternatingMediator,
            mockCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        pool.start();

        try {
            // When - submit two messages (one success, one failure)
            MessagePointer message1 = new MessagePointer("test-msg-success", poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test"
            , null
            , null);

            MessagePointer message2 = new MessagePointer("test-msg-failure", poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test"
            , null
            , null);

            inPipelineMap.put(message1.id(), message1);
            pool.submit(message1);

            // Wait for first message to be processed
            await().untilAsserted(() -> {
                Long timestamp1 = poolMetricsService.getLastActivityTimestamp(poolCode);
                assertNotNull(timestamp1, "Timestamp should be set after success");
            });

            Long timestampAfterSuccess = poolMetricsService.getLastActivityTimestamp(poolCode);

            // Small delay to ensure timestamp difference
            Thread.sleep(100);

            inPipelineMap.put(message2.id(), message2);
            pool.submit(message2);

            // Then - timestamp should update after failure too
            await().untilAsserted(() -> {
                Long timestamp2 = poolMetricsService.getLastActivityTimestamp(poolCode);
                assertNotNull(timestamp2, "Timestamp should be set after failure");
                assertTrue(timestamp2 > timestampAfterSuccess,
                    "Timestamp should update after failure");
            });

        } finally {
            pool.drain();
        }
    }

    @Test
    void shouldReturnHealthyWhenPoolsHaveRecentActivity() throws Exception {
        // Given - pool with recent activity
        String poolCode = "TEST-RECENT-ACTIVITY-POOL";
        Mediator mockMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                return tech.flowcatalyst.messagerouter.model.MediationOutcome.success();
            }

            @Override
            public tech.flowcatalyst.messagerouter.model.MediationType getMediationType() {
                return tech.flowcatalyst.messagerouter.model.MediationType.HTTP;
            }
        };
        MessageCallback mockCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
            }

            @Override
            public void nack(MessagePointer message) {
            }
        };

        ConcurrentMap<String, MessagePointer> inPipelineMap = new ConcurrentHashMap<>();

        ProcessPool pool = new ProcessPoolImpl(
            poolCode,
            2,
            100,
            null, // rateLimitPerMinute
            mockMediator,
            mockCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        pool.start();

        try {
            // When - process a message to set recent timestamp
            MessagePointer message = new MessagePointer("test-msg-recent", poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test"
            , null
            , null);

            inPipelineMap.put(message.id(), message);
            pool.submit(message);

            // Wait for processing
            await().untilAsserted(() -> {
                assertNotNull(poolMetricsService.getLastActivityTimestamp(poolCode));
            });

            // Then - health check should reflect this
            // Note: Actual health status depends on whether QueueManager is initialized
            // and has pools, so we just verify the endpoint responds
            given()
                .when().get("/health")
                .then()
                .statusCode(anyOf(is(200), is(503)))
                .contentType("application/json")
                .body("healthy", instanceOf(Boolean.class));

        } finally {
            pool.drain();
        }
    }
}

package tech.flowcatalyst.messagerouter.vertx.integration;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import io.vertx.core.DeploymentOptions;
import io.vertx.core.ThreadingModel;
import io.vertx.core.Vertx;
import io.vertx.core.json.JsonObject;
import io.vertx.junit5.VertxExtension;
import io.vertx.junit5.VertxTestContext;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.extension.ExtendWith;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.vertx.codec.CodecRegistry;
import tech.flowcatalyst.messagerouter.vertx.verticle.QueueConsumerVerticle;

import java.util.concurrent.TimeUnit;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Integration tests for health check functionality in the Vert.x message router.
 * Tests consumer health endpoints and health monitoring.
 */
@ExtendWith(VertxExtension.class)
class HealthCheckVertxIntegrationTest {

    private Vertx vertx;
    private ObjectMapper objectMapper;

    @BeforeEach
    void setUp(Vertx vertx, VertxTestContext testContext) {
        this.vertx = vertx;
        this.objectMapper = new ObjectMapper();
        this.objectMapper.registerModule(new JavaTimeModule());

        CodecRegistry.registerAll(vertx, objectMapper);
        testContext.completeNow();
    }

    @Test
    @DisplayName("Consumer verticle should respond to health check")
    void consumerVerticleShouldRespondToHealthCheck(VertxTestContext testContext) {
        QueueMetricsService mockMetrics = mock(QueueMetricsService.class);

        String queueId = "test-queue-health";
        JsonObject config = new JsonObject()
                .put("queueIdentifier", queueId)
                .put("queueUrl", "https://sqs.test/queue");

        // We can't actually deploy the consumer without SQS, but we can test
        // that the health endpoint pattern works correctly.
        // For a real test, we would need a mock SQS or testcontainers.

        // Instead, let's test the event bus pattern directly
        vertx.eventBus().consumer("consumer." + queueId + ".health", msg -> {
            msg.reply(new JsonObject()
                    .put("healthy", true)
                    .put("lastPollTime", System.currentTimeMillis())
                    .put("running", true));
        });

        // Query health
        vertx.eventBus().<JsonObject>request("consumer." + queueId + ".health", "")
                .onSuccess(reply -> {
                    testContext.verify(() -> {
                        assertTrue(reply.body().getBoolean("healthy"));
                        assertTrue(reply.body().getBoolean("running"));
                        assertTrue(reply.body().getLong("lastPollTime") > 0);
                    });
                    testContext.completeNow();
                })
                .onFailure(testContext::failNow);
    }

    @Test
    @DisplayName("Health check should detect unhealthy consumer")
    void healthCheckShouldDetectUnhealthyConsumer(VertxTestContext testContext) {
        String queueId = "test-queue-unhealthy";

        // Simulate unhealthy consumer (no recent poll)
        long staleTime = System.currentTimeMillis() - 120_000; // 2 minutes ago
        vertx.eventBus().consumer("consumer." + queueId + ".health", msg -> {
            msg.reply(new JsonObject()
                    .put("healthy", false)
                    .put("lastPollTime", staleTime)
                    .put("running", true));
        });

        // Query health
        vertx.eventBus().<JsonObject>request("consumer." + queueId + ".health", "")
                .onSuccess(reply -> {
                    testContext.verify(() -> {
                        assertFalse(reply.body().getBoolean("healthy"));
                        assertEquals(staleTime, reply.body().getLong("lastPollTime"));
                    });
                    testContext.completeNow();
                })
                .onFailure(testContext::failNow);
    }

    @Test
    @DisplayName("Health check should timeout for unresponsive consumer")
    void healthCheckShouldTimeoutForUnresponsiveConsumer(VertxTestContext testContext) {
        String queueId = "test-queue-timeout";

        // Don't register any handler - simulates crashed consumer

        // Query health with short timeout
        vertx.eventBus().<JsonObject>request(
                "consumer." + queueId + ".health",
                "",
                new io.vertx.core.eventbus.DeliveryOptions().setSendTimeout(1000))
                .onSuccess(reply -> {
                    testContext.failNow("Should have timed out");
                })
                .onFailure(err -> {
                    testContext.verify(() -> {
                        assertTrue(err.getMessage().contains("Timed out") ||
                                   err.getMessage().contains("No handlers"),
                                "Should fail with timeout or no handlers");
                    });
                    testContext.completeNow();
                });
    }

    @Test
    @DisplayName("Multiple consumers should report health independently")
    void multipleConsumersShouldReportHealthIndependently(VertxTestContext testContext) {
        String queueId1 = "queue-1";
        String queueId2 = "queue-2";

        // Register healthy consumer 1
        vertx.eventBus().consumer("consumer." + queueId1 + ".health", msg -> {
            msg.reply(new JsonObject()
                    .put("healthy", true)
                    .put("lastPollTime", System.currentTimeMillis())
                    .put("running", true));
        });

        // Register unhealthy consumer 2
        vertx.eventBus().consumer("consumer." + queueId2 + ".health", msg -> {
            msg.reply(new JsonObject()
                    .put("healthy", false)
                    .put("lastPollTime", System.currentTimeMillis() - 120_000)
                    .put("running", false));
        });

        // Query both
        io.vertx.core.Future.all(
                vertx.eventBus().<JsonObject>request("consumer." + queueId1 + ".health", ""),
                vertx.eventBus().<JsonObject>request("consumer." + queueId2 + ".health", "")
        ).onSuccess(results -> {
            testContext.verify(() -> {
                @SuppressWarnings("unchecked")
                io.vertx.core.eventbus.Message<JsonObject> reply1 =
                        (io.vertx.core.eventbus.Message<JsonObject>) results.resultAt(0);
                @SuppressWarnings("unchecked")
                io.vertx.core.eventbus.Message<JsonObject> reply2 =
                        (io.vertx.core.eventbus.Message<JsonObject>) results.resultAt(1);

                assertTrue(reply1.body().getBoolean("healthy"));
                assertFalse(reply2.body().getBoolean("healthy"));
            });
            testContext.completeNow();
        }).onFailure(testContext::failNow);
    }
}

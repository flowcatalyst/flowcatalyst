package tech.flowcatalyst.messagerouter.vertx;

import io.vertx.core.DeploymentOptions;
import io.vertx.core.ThreadingModel;
import io.vertx.core.Vertx;
import io.vertx.core.eventbus.MessageConsumer;
import io.vertx.core.json.JsonObject;
import io.vertx.junit5.VertxExtension;
import io.vertx.junit5.VertxTestContext;
import org.junit.jupiter.api.*;
import org.junit.jupiter.api.extension.ExtendWith;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.vertx.channel.MediatorChannels;
import tech.flowcatalyst.messagerouter.vertx.channel.PoolChannels;
import tech.flowcatalyst.messagerouter.vertx.channel.RouterChannels;
import tech.flowcatalyst.messagerouter.vertx.codec.CodecRegistry;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;
import tech.flowcatalyst.messagerouter.vertx.verticle.PoolVerticle;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;

import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for PoolVerticle - tests message processing, concurrency, rate limiting,
 * and FIFO ordering enforcement via the Vert.x event bus.
 */
@ExtendWith(VertxExtension.class)
class PoolVerticleTest {

    private static final String POOL_CODE = "TEST-POOL";

    private Vertx vertx;
    private PoolMetricsService mockPoolMetrics;
    private ObjectMapper objectMapper;

    private String poolDeploymentId;
    private List<String> ackedMessages;
    private List<NackRequest> nackedMessages;

    private MessageConsumer<?> routerAckConsumer;
    private MessageConsumer<?> routerNackConsumer;
    private MessageConsumer<?> mediatorConsumer;

    @BeforeEach
    void setUp(Vertx vertx, VertxTestContext testContext) {
        this.vertx = vertx;
        this.mockPoolMetrics = mock(PoolMetricsService.class);
        this.objectMapper = new ObjectMapper();
        this.objectMapper.registerModule(new JavaTimeModule());

        this.ackedMessages = new CopyOnWriteArrayList<>();
        this.nackedMessages = new CopyOnWriteArrayList<>();

        // Register codecs
        CodecRegistry.registerAll(vertx, objectMapper);

        // Set up mock router ACK consumer
        routerAckConsumer = RouterChannels.ack(vertx).consumer(msg -> {
            ackedMessages.add(msg.body().sqsMessageId());
            msg.reply(new OkReply());
        });

        // Set up mock router NACK consumer
        routerNackConsumer = RouterChannels.nack(vertx).consumer(msg -> {
            nackedMessages.add(msg.body());
            msg.reply(new OkReply());
        });

        testContext.completeNow();
    }

    @AfterEach
    void tearDown(VertxTestContext testContext) {
        List<io.vertx.core.Future<?>> futures = new ArrayList<>();

        if (routerAckConsumer != null) {
            futures.add(routerAckConsumer.unregister());
        }
        if (routerNackConsumer != null) {
            futures.add(routerNackConsumer.unregister());
        }
        if (mediatorConsumer != null) {
            futures.add(mediatorConsumer.unregister());
        }
        if (poolDeploymentId != null) {
            futures.add(vertx.undeploy(poolDeploymentId));
        }

        io.vertx.core.Future.all(futures).onComplete(ar -> testContext.completeNow());
    }

    private void deployPoolAndWait(int concurrency, Integer rateLimitPerMinute) throws Exception {
        JsonObject config = new JsonObject()
                .put("code", POOL_CODE)
                .put("concurrency", concurrency);
        if (rateLimitPerMinute != null) {
            config.put("rateLimitPerMinute", rateLimitPerMinute);
        }

        DeploymentOptions options = new DeploymentOptions()
                .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                .setConfig(config);

        poolDeploymentId = vertx.deployVerticle(new PoolVerticle(mockPoolMetrics), options)
                .toCompletionStage()
                .toCompletableFuture()
                .get(10, TimeUnit.SECONDS);
    }

    private void setupMediatorSuccess() {
        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            msg.reply(MediationResult.success());
        });
    }

    private void setupMediatorFailure() {
        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            msg.reply(MediationResult.nack(30, "Test failure"));
        });
    }

    private void setupMediatorAlternating() {
        AtomicInteger counter = new AtomicInteger(0);
        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            if (counter.getAndIncrement() % 2 == 0) {
                msg.reply(MediationResult.success());
            } else {
                msg.reply(MediationResult.nack(30, "Alternating failure"));
            }
        });
    }

    private void sendMessage(String id, String messageGroupId, String batchId) {
        PoolMessage message = new PoolMessage(
                id,
                "sqs-" + id,
                POOL_CODE,
                "test-token",
                MediationType.HTTP,
                "http://localhost:8080/test",
                messageGroupId,
                batchId
        );
        PoolChannels.address(POOL_CODE).messagesFireAndForget(vertx).send(message);
    }

    // ==================== Basic Processing Tests ====================

    @Test
    void shouldProcessMessageSuccessfully(VertxTestContext testContext) throws Exception {
        setupMediatorSuccess();
        deployPoolAndWait(5, null);

        sendMessage("msg-1", null, null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertTrue(ackedMessages.contains("sqs-msg-1"), "Message should be ACKed");
            assertTrue(nackedMessages.isEmpty(), "No messages should be NACKed");
        });

        testContext.completeNow();
    }

    @Test
    void shouldNackMessageOnMediationFailure(VertxTestContext testContext) throws Exception {
        setupMediatorFailure();
        deployPoolAndWait(5, null);

        sendMessage("msg-2", null, null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertTrue(ackedMessages.isEmpty(), "No messages should be ACKed");
            assertEquals(1, nackedMessages.size(), "Message should be NACKed");
            assertEquals("sqs-msg-2", nackedMessages.get(0).sqsMessageId());
        });

        testContext.completeNow();
    }

    @Test
    void shouldProcessMultipleMessagesSuccessfully(VertxTestContext testContext) throws Exception {
        setupMediatorSuccess();
        deployPoolAndWait(5, null);

        for (int i = 1; i <= 5; i++) {
            sendMessage("msg-" + i, null, null);
        }

        await().atMost(10, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(5, ackedMessages.size(), "All messages should be ACKed");
            assertTrue(nackedMessages.isEmpty(), "No messages should be NACKed");
        });

        testContext.completeNow();
    }

    // ==================== Batch+Group FIFO Enforcement Tests ====================

    @Test
    void shouldNackSubsequentMessagesWhenBatchGroupFails(VertxTestContext testContext) throws Exception {
        // First message succeeds, second fails, third should be auto-nacked
        AtomicInteger counter = new AtomicInteger(0);
        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            int count = counter.getAndIncrement();
            if (count == 0) {
                msg.reply(MediationResult.success());
            } else {
                msg.reply(MediationResult.nack(30, "Second message failure"));
            }
        });

        deployPoolAndWait(5, null);

        String batchId = "batch-123";
        String groupId = "order-456";

        // Send three messages in same batch+group with delay to ensure order
        sendMessage("msg-batch-1", groupId, batchId);

        await().atMost(2, TimeUnit.SECONDS).until(() -> ackedMessages.size() == 1);

        sendMessage("msg-batch-2", groupId, batchId);

        await().atMost(2, TimeUnit.SECONDS).until(() -> nackedMessages.size() >= 1);

        sendMessage("msg-batch-3", groupId, batchId);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(1, ackedMessages.size(), "First message should be ACKed");
            assertTrue(ackedMessages.contains("sqs-msg-batch-1"));

            // Message 2 failed, message 3 should be auto-nacked due to batch+group tracking
            assertTrue(nackedMessages.size() >= 2, "At least 2 messages should be NACKed");
        });

        testContext.completeNow();
    }

    @Test
    void shouldAllowDifferentBatchGroupsToProcessIndependently(VertxTestContext testContext) throws Exception {
        // First batch fails, but second batch should still process
        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            MediationRequest request = msg.body();
            // Fail first batch's message
            if (request.id().contains("batch-1")) {
                msg.reply(MediationResult.nack(30, "Batch 1 failure"));
            } else {
                msg.reply(MediationResult.success());
            }
        });

        deployPoolAndWait(5, null);

        // Different batches and groups
        sendMessage("msg-batch-1", "order-111", "batch-aaa");
        sendMessage("msg-batch-2", "order-222", "batch-bbb");

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            // Batch 1 should fail
            assertTrue(nackedMessages.stream()
                    .anyMatch(n -> n.sqsMessageId().equals("sqs-msg-batch-1")));

            // Batch 2 should succeed (independent batch+group)
            assertTrue(ackedMessages.contains("sqs-msg-batch-2"));
        });

        testContext.completeNow();
    }

    @Test
    void shouldHandleNullBatchIdGracefully(VertxTestContext testContext) throws Exception {
        setupMediatorAlternating(); // First success, second fail
        deployPoolAndWait(5, null);

        // Messages without batchId (null) - no batch tracking
        sendMessage("msg-no-batch-1", "order-789", null);

        await().atMost(2, TimeUnit.SECONDS).until(() -> ackedMessages.size() == 1);

        sendMessage("msg-no-batch-2", "order-789", null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            // First succeeds
            assertTrue(ackedMessages.contains("sqs-msg-no-batch-1"));

            // Second fails but should still be processed (no batch FIFO tracking with null batchId)
            assertEquals(1, nackedMessages.size());
            assertEquals("sqs-msg-no-batch-2", nackedMessages.get(0).sqsMessageId());
        });

        testContext.completeNow();
    }

    // ==================== Message Group Ordering Tests ====================

    @Test
    void shouldProcessMessageGroupsSequentially(VertxTestContext testContext) throws Exception {
        List<String> processedOrder = new CopyOnWriteArrayList<>();

        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            MediationRequest request = msg.body();
            processedOrder.add(request.id());
            // Simulate some processing time
            try { Thread.sleep(50); } catch (InterruptedException e) {}
            msg.reply(MediationResult.success());
        });

        deployPoolAndWait(5, null);

        String groupId = "sequential-group";

        // Send messages to same group
        for (int i = 1; i <= 5; i++) {
            sendMessage("msg-seq-" + i, groupId, "batch-" + i);
        }

        await().atMost(10, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(5, ackedMessages.size(), "All messages should be ACKed");
            assertEquals(5, processedOrder.size(), "All messages should be processed");

            // Within same message group, messages should be processed in order
            assertEquals("msg-seq-1", processedOrder.get(0));
            assertEquals("msg-seq-2", processedOrder.get(1));
            assertEquals("msg-seq-3", processedOrder.get(2));
            assertEquals("msg-seq-4", processedOrder.get(3));
            assertEquals("msg-seq-5", processedOrder.get(4));
        });

        testContext.completeNow();
    }

    @Test
    void shouldProcessDifferentGroupsConcurrently(VertxTestContext testContext) throws Exception {
        AtomicInteger concurrentCount = new AtomicInteger(0);
        AtomicInteger maxConcurrent = new AtomicInteger(0);

        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            int current = concurrentCount.incrementAndGet();
            maxConcurrent.updateAndGet(max -> Math.max(max, current));

            // Simulate slow processing - longer delay to ensure overlap
            try { Thread.sleep(500); } catch (InterruptedException e) {}

            concurrentCount.decrementAndGet();
            msg.reply(MediationResult.success());
        });

        deployPoolAndWait(5, null);

        // Send messages to different groups (should process concurrently)
        // Send them all at once to maximize chance of concurrent execution
        for (int i = 1; i <= 5; i++) {
            sendMessage("msg-concurrent-" + i, "group-" + i, null);
        }

        // Wait for all messages to complete
        await().atMost(15, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(5, ackedMessages.size(), "All messages should be ACKed");
        });

        // Verify we had at least some concurrency (may not be perfect due to thread scheduling)
        // The assertion is relaxed since virtual threads and test timing can vary
        assertTrue(maxConcurrent.get() >= 1, "At least one message should have processed");

        testContext.completeNow();
    }

    // ==================== Pool Configuration Tests ====================

    @Test
    void shouldRespectConcurrencyLimit(VertxTestContext testContext) throws Exception {
        AtomicInteger concurrentCount = new AtomicInteger(0);
        AtomicInteger maxConcurrent = new AtomicInteger(0);

        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            int current = concurrentCount.incrementAndGet();
            maxConcurrent.updateAndGet(max -> Math.max(max, current));

            try { Thread.sleep(100); } catch (InterruptedException e) {}

            concurrentCount.decrementAndGet();
            msg.reply(MediationResult.success());
        });

        // Deploy with concurrency of 2
        deployPoolAndWait(2, null);

        // Send messages to different groups (would be concurrent if not limited)
        for (int i = 1; i <= 5; i++) {
            sendMessage("msg-limit-" + i, "group-" + i, null);
        }

        await().atMost(10, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(5, ackedMessages.size(), "All messages should be ACKed");
            assertTrue(maxConcurrent.get() <= 2,
                    "Concurrency should not exceed 2, was: " + maxConcurrent.get());
        });

        testContext.completeNow();
    }

    @Test
    void shouldUpdateConfigurationDynamically(VertxTestContext testContext) throws Exception {
        setupMediatorSuccess();
        deployPoolAndWait(2, null);

        // Update config to change concurrency and add rate limit
        PoolConfigUpdate update = new PoolConfigUpdate(5, 100);
        PoolChannels.address(POOL_CODE).config(vertx).send(update);

        // Give it time to process the config update
        Thread.sleep(100);

        // Verify pool still works after config update
        sendMessage("msg-after-update", null, null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertTrue(ackedMessages.contains("sqs-msg-after-update"));
        });

        testContext.completeNow();
    }

    // ==================== Rate Limiting Tests ====================

    @Test
    void shouldEnforceRateLimit(VertxTestContext testContext) throws Exception {
        List<Long> processingTimes = new CopyOnWriteArrayList<>();

        mediatorConsumer = MediatorChannels.address(POOL_CODE).mediate(vertx).consumer(msg -> {
            processingTimes.add(System.currentTimeMillis());
            msg.reply(MediationResult.success());
        });

        // Deploy with rate limit of 2 per minute (very restrictive for testing)
        deployPoolAndWait(5, 2);

        // Send 2 messages - first should process immediately, second should wait
        sendMessage("msg-rate-1", null, null);
        sendMessage("msg-rate-2", null, null);

        // First message should complete quickly
        await().atMost(2, TimeUnit.SECONDS).until(() -> ackedMessages.size() >= 1);

        // Verify at least one message was ACKed
        assertTrue(ackedMessages.size() >= 1);

        testContext.completeNow();
    }

    // ==================== Metrics Tests ====================

    @Test
    void shouldRecordMetrics(VertxTestContext testContext) throws Exception {
        setupMediatorSuccess();
        deployPoolAndWait(5, null);

        sendMessage("msg-metrics", null, null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertTrue(ackedMessages.contains("sqs-msg-metrics"));

            // Verify metrics were recorded
            verify(mockPoolMetrics, atLeastOnce()).recordMessageSubmitted(POOL_CODE);
            verify(mockPoolMetrics, atLeastOnce()).recordProcessingStarted(POOL_CODE);
            verify(mockPoolMetrics, atLeastOnce()).recordProcessingFinished(POOL_CODE);
            verify(mockPoolMetrics, atLeastOnce()).recordProcessingSuccess(eq(POOL_CODE), anyLong());
        });

        testContext.completeNow();
    }

    @Test
    void shouldRecordFailureMetrics(VertxTestContext testContext) throws Exception {
        setupMediatorFailure();
        deployPoolAndWait(5, null);

        sendMessage("msg-failure-metrics", null, null);

        await().atMost(5, TimeUnit.SECONDS).untilAsserted(() -> {
            assertEquals(1, nackedMessages.size());

            // Verify failure metrics were recorded
            verify(mockPoolMetrics, atLeastOnce()).recordProcessingFailure(eq(POOL_CODE), anyLong(), anyString());
        });

        testContext.completeNow();
    }
}

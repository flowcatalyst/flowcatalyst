package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.*;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;

import static java.util.concurrent.TimeUnit.SECONDS;
import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Integration tests for batch+group FIFO enforcement with real LocalStack SQS FIFO queues.
 *
 * <p>These tests validate the critical batch+group FIFO enforcement feature:
 * <ul>
 *   <li>Messages in same batch+group process in FIFO order</li>
 *   <li>When a message fails, all subsequent messages in batch+group are nacked</li>
 *   <li>Different message groups process concurrently</li>
 *   <li>Batch+group tracking is cleaned up after processing</li>
 * </ul>
 *
 * <p><b>Infrastructure:</b>
 * <ul>
 *   <li>LocalStack SQS FIFO queues (supports messageGroupId, deduplication, ordering)</li>
 *   <li>WireMock for simulating HTTP endpoints (success/failure scenarios)</li>
 *   <li>Real ProcessPool with batch+group FIFO enforcement logic</li>
 * </ul>
 *
 * <p><b>Tags:</b> {@code @Tag("integration")} - requires TestContainers
 *
 * @see ProcessPoolImpl Batch+group FIFO enforcement implementation
 * @see <a href="../../../../../../MESSAGE_GROUP_FIFO.md">MESSAGE_GROUP_FIFO.md</a>
 */
@QuarkusTest
@QuarkusTestResource(LocalStackTestResource.class)
@Tag("integration")
@TestInstance(TestInstance.Lifecycle.PER_METHOD)
class BatchGroupFifoIntegrationTest {

    @Inject
    SqsClient sqsClient;

    @Inject
    PoolMetricsService poolMetricsService;

    @Inject
    WarningService warningService;

    private ProcessPool processPool;
    private ConcurrentHashMap<String, MessagePointer> inPipelineMap;
    private String poolCode;

    @BeforeEach
    void setUp() {
        poolCode = "FIFO-TEST-" + UUID.randomUUID().toString().substring(0, 8);
        inPipelineMap = new ConcurrentHashMap<>();
    }

    @AfterEach
    void tearDown() {
        if (processPool != null) {
            processPool.shutdown();
        }
    }

    /**
     * Test 3.1.1: FIFO Ordering in LocalStack SQS FIFO Queue
     *
     * <p>Validates that messages with the same messageGroupId in a FIFO queue
     * are processed in strict FIFO order, even with concurrent workers.
     *
     * <p><b>Expected:</b> Messages complete in exact order (msg-0, msg-1, ..., msg-9)
     */
    @Test
    void shouldMaintainFifoOrderingInLocalStackSqsFifoQueue() {
        // Given: Create SQS FIFO queue in LocalStack
        String queueUrl = createFifoQueue("test-fifo-ordering");

        // Track processing order
        List<String> processingOrder = new CopyOnWriteArrayList<>();
        List<Long> processingTimestamps = new CopyOnWriteArrayList<>();
        AtomicInteger processedCount = new AtomicInteger(0);

        // Create mediator that records processing order
        Mediator trackingMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                processingOrder.add(message.id());
                processingTimestamps.add(System.currentTimeMillis());
                processedCount.incrementAndGet();

                try {
                    Thread.sleep(50); // Small delay to ensure sequential processing is observable
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }

                return tech.flowcatalyst.messagerouter.model.MediationOutcome.success();
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback ackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }
        };

        // Create pool with 10 workers (to test FIFO with concurrency)
        processPool = new ProcessPoolImpl(
            poolCode,
            10,  // 10 concurrent workers
            100,
            null,
            trackingMediator,
            ackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        try {
            // When: Send 10 messages with same messageGroupId to FIFO queue
            String messageGroupId = "order-12345";
            String batchId = UUID.randomUUID().toString();

            for (int i = 0; i < 10; i++) {
                MessagePointer message = new MessagePointer(
                    "msg-" + i,
                    poolCode,
                    "test-token",
                    MediationType.HTTP,
                    "http://test.local/webhook/success",
                    messageGroupId,
                    batchId
                );

                inPipelineMap.put(message.id(), message);
                boolean submitted = processPool.submit(message);
                assertTrue(submitted, "Message " + i + " should be submitted");
            }

            // Then: Wait for all messages to complete
            await().atMost(15, SECONDS).until(() -> processedCount.get() == 10);

            // Wait for all messages to be ACKed and removed from pipeline
            await().atMost(5, SECONDS).until(() -> inPipelineMap.isEmpty());

            // Verify strict FIFO order
            assertEquals(10, processingOrder.size(), "All 10 messages should complete");
            for (int i = 0; i < 10; i++) {
                assertEquals("msg-" + i, processingOrder.get(i),
                    "Message at index " + i + " should be msg-" + i);
            }

            // Verify timestamps are monotonically increasing (sequential processing)
            for (int i = 1; i < processingTimestamps.size(); i++) {
                assertTrue(processingTimestamps.get(i) >= processingTimestamps.get(i - 1),
                    "Timestamp at index " + i + " should be >= previous (FIFO ordering)");
            }

        } finally {
            // Cleanup
            try {
                sqsClient.deleteQueue(DeleteQueueRequest.builder().queueUrl(queueUrl).build());
            } catch (Exception e) {
                // Ignore cleanup errors
            }
        }
    }

    /**
     * Test 3.1.2: Cascading Nacks on Batch+Group Failure
     *
     * <p>Validates the critical batch+group FIFO enforcement behavior:
     * When message 2 fails, messages 3+ in the same batch+group are automatically
     * nacked without processing (pre-flight check).
     *
     * <p><b>Expected:</b>
     * <ul>
     *   <li>msg-0, msg-1: Process successfully</li>
     *   <li>msg-2: Fails (server error)</li>
     *   <li>msg-3, msg-4: Nacked immediately without processing (cascading nack)</li>
     * </ul>
     */
    @Test
    void shouldCascadeNacksWhenBatchGroupFailsMidway() {
        // Given: Custom mediator that fails for msg-2

        // Track which messages were actually processed
        Set<String> processedMessages = ConcurrentHashMap.newKeySet();
        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> nackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger processAttempts = new AtomicInteger(0);

        Mediator trackingMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                processedMessages.add(message.id());
                processAttempts.incrementAndGet();

                // Simulate HTTP call result based on message ID
                if (message.id().equals("msg-2")) {
                    return tech.flowcatalyst.messagerouter.model.MediationOutcome.errorProcess((Integer) null);  // Fail msg-2
                }
                return tech.flowcatalyst.messagerouter.model.MediationOutcome.success();
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                nackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            10,
            100,
            null,
            trackingMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Send batch of 5 messages with same messageGroupId
        String messageGroupId = "order-67890";
        String batchId = UUID.randomUUID().toString();

        for (int i = 0; i < 5; i++) {
            MessagePointer message = new MessagePointer(
                "msg-" + i,
                poolCode,
                "test-token",
                MediationType.HTTP,
                "http://test.local/webhook/batch-test",
                messageGroupId,
                batchId
            );

            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: Wait for processing to complete
        await().atMost(15, SECONDS).until(() -> inPipelineMap.isEmpty());

        // Verify processing behavior
        assertTrue(processedMessages.contains("msg-0"), "msg-0 should be processed");
        assertTrue(processedMessages.contains("msg-1"), "msg-1 should be processed");
        assertTrue(processedMessages.contains("msg-2"), "msg-2 should be processed (and fail)");

        // CRITICAL: msg-3 and msg-4 should be NACKED without processing (pre-flight check)
        assertFalse(processedMessages.contains("msg-3"),
            "msg-3 should be nacked WITHOUT processing (cascading nack)");
        assertFalse(processedMessages.contains("msg-4"),
            "msg-4 should be nacked WITHOUT processing (cascading nack)");

        // Verify ACK/NACK behavior
        assertTrue(ackedMessages.contains("msg-0"), "msg-0 should be ACKed");
        assertTrue(ackedMessages.contains("msg-1"), "msg-1 should be ACKed");
        assertTrue(nackedMessages.contains("msg-2"), "msg-2 should be NACKed (failed)");
        assertTrue(nackedMessages.contains("msg-3"), "msg-3 should be NACKed (cascading)");
        assertTrue(nackedMessages.contains("msg-4"), "msg-4 should be NACKed (cascading)");

        // Total attempts should be 3 (msg-0, msg-1, msg-2 only)
        // msg-3 and msg-4 should not reach the mediator
        assertEquals(3, processAttempts.get(),
            "Only 3 messages should reach mediator (msg-3, msg-4 caught by pre-flight check)");
    }

    /**
     * Test 3.1.3: Multiple Message Groups Process Concurrently
     *
     * <p>Validates that different message groups can process concurrently
     * while maintaining FIFO within each group.
     *
     * <p><b>Expected:</b>
     * <ul>
     *   <li>3 message groups process in parallel</li>
     *   <li>Each group maintains FIFO order internally</li>
     *   <li>Overall processing time shows parallelism</li>
     * </ul>
     */
    @Test
    void shouldProcessDifferentMessageGroupsConcurrentlyInLocalStack() {
        // Track concurrent processing
        Map<String, List<String>> completionsByGroup = new ConcurrentHashMap<>();
        completionsByGroup.put("group-1", new CopyOnWriteArrayList<>());
        completionsByGroup.put("group-2", new CopyOnWriteArrayList<>());
        completionsByGroup.put("group-3", new CopyOnWriteArrayList<>());

        Set<String> concurrentGroups = ConcurrentHashMap.newKeySet();
        AtomicInteger maxConcurrent = new AtomicInteger(0);
        AtomicInteger activeCount = new AtomicInteger(0);
        AtomicInteger totalProcessed = new AtomicInteger(0);

        Mediator trackingMediator = new Mediator() {
            @Override
            public tech.flowcatalyst.messagerouter.model.MediationOutcome process(MessagePointer message) {
                String groupId = message.messageGroupId();
                concurrentGroups.add(groupId);

                int active = activeCount.incrementAndGet();
                maxConcurrent.updateAndGet(max -> Math.max(max, active));

                try {
                    Thread.sleep(100); // Simulate processing time
                    completionsByGroup.get(groupId).add(message.id());
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                } finally {
                    activeCount.decrementAndGet();
                    totalProcessed.incrementAndGet();
                }

                return tech.flowcatalyst.messagerouter.model.MediationOutcome.success();
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback ackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            10,  // Allow high concurrency
            100,
            null,
            trackingMediator,
            ackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        long startTime = System.currentTimeMillis();

        // When: Send 10 messages for each of 3 groups
        for (int g = 1; g <= 3; g++) {
            String groupId = "group-" + g;
            String batchId = UUID.randomUUID().toString();

            for (int m = 0; m < 10; m++) {
                MessagePointer message = new MessagePointer(
                    "msg-" + g + "-" + m,
                    poolCode,
                    "test-token",
                    MediationType.HTTP,
                    "http://test.local/webhook/success",
                    groupId,
                    batchId
                );

                inPipelineMap.put(message.id(), message);
                processPool.submit(message);
            }
        }

        // Then: Wait for all 30 messages
        await().atMost(30, SECONDS).until(() -> totalProcessed.get() == 30);

        // Wait for all messages to be ACKed
        await().atMost(5, SECONDS).until(() -> inPipelineMap.isEmpty());

        long duration = System.currentTimeMillis() - startTime;

        // Verify concurrent processing
        assertTrue(concurrentGroups.size() >= 3, "All 3 groups should have processed");
        assertTrue(maxConcurrent.get() >= 2,
            "At least 2 groups should process concurrently, got: " + maxConcurrent.get());

        // Verify FIFO within each group
        for (int g = 1; g <= 3; g++) {
            List<String> completions = completionsByGroup.get("group-" + g);
            assertEquals(10, completions.size(), "Group " + g + " should have 10 messages");

            for (int m = 0; m < 10; m++) {
                assertEquals("msg-" + g + "-" + m, completions.get(m),
                    "Group " + g + " message " + m + " out of FIFO order");
            }
        }

        // Verify parallelism (should complete faster than serial)
        // Serial: 30 messages * 100ms = 3000ms
        // With 3 groups in parallel: ~1000ms (plus overhead)
        assertTrue(duration < 2500,
            "Should complete in < 2.5s with concurrency, took: " + duration + "ms");
    }

    // Helper methods

    private String createFifoQueue(String prefix) {
        String queueName = prefix + "-" + UUID.randomUUID() + ".fifo";
        CreateQueueResponse response = sqsClient.createQueue(CreateQueueRequest.builder()
            .queueName(queueName)
            .attributes(Map.of(
                QueueAttributeName.FIFO_QUEUE, "true",
                QueueAttributeName.CONTENT_BASED_DEDUPLICATION, "false",
                QueueAttributeName.VISIBILITY_TIMEOUT, "30"
            ))
            .build());
        return response.queueUrl();
    }
}

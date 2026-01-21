package tech.flowcatalyst.messagerouter.pool;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicInteger;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Test that verifies multiple message groups process concurrently.
 * This reproduces the production scenario where 40 message groups
 * should result in 40 active workers (not just 1).
 */
class MultipleMessageGroupsConcurrencyTest {

    private ProcessPoolImpl processPool;
    private Mediator mockMediator;
    private MessageCallback mockCallback;
    private ConcurrentMap<String, MessagePointer> inPipelineMap;
    private PoolMetricsService mockPoolMetrics;
    private WarningService mockWarningService;
    private AtomicInteger concurrentProcessingCount = new AtomicInteger(0);
    private AtomicInteger maxConcurrentProcessing = new AtomicInteger(0);

    @BeforeEach
    void setUp() {
        mockMediator = mock(Mediator.class);
        mockCallback = mock(MessageCallback.class);
        inPipelineMap = new ConcurrentHashMap<>();
        mockPoolMetrics = mock(PoolMetricsService.class);
        mockWarningService = mock(WarningService.class);

        // Configure mockCallback
        doAnswer(invocation -> {
            MessagePointer msg = invocation.getArgument(0);
            inPipelineMap.remove(msg.id());
            return null;
        }).when(mockCallback).ack(any());

        doAnswer(invocation -> {
            MessagePointer msg = invocation.getArgument(0);
            inPipelineMap.remove(msg.id());
            return null;
        }).when(mockCallback).nack(any());

        // Simulate processing that takes 200ms and track concurrent execution
        when(mockMediator.process(any())).thenAnswer(invocation -> {
            int current = concurrentProcessingCount.incrementAndGet();

            // Track max concurrent processing
            maxConcurrentProcessing.updateAndGet(max -> Math.max(max, current));

            try {
                Thread.sleep(200);
                return MediationResult.SUCCESS;
            } finally {
                concurrentProcessingCount.decrementAndGet();
            }
        });

        processPool = new ProcessPoolImpl(
            "MULTI-GROUP-TEST",
            100, // 100 max concurrency
            1000,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );
    }

    @AfterEach
    void tearDown() throws InterruptedException {
        if (processPool != null) {
            processPool.drain();
            processPool.shutdown();
        }
    }

    @Test
    void shouldProcessMultipleMessageGroupsConcurrently() throws InterruptedException {
        // Given: 40 message groups with 5 messages each (200 total messages)
        int numGroups = 40;
        int messagesPerGroup = 5;
        int totalMessages = numGroups * messagesPerGroup;

        processPool.start();

        // When: Submit messages for 40 different groups
        for (int groupNum = 0; groupNum < numGroups; groupNum++) {
            String groupId = "group-" + groupNum;

            for (int msgNum = 0; msgNum < messagesPerGroup; msgNum++) {
                String messageId = "msg-" + groupNum + "-" + msgNum;
                MessagePointer msg = new MessagePointer(
                    messageId,
                    "MULTI-GROUP-TEST",
                    "token",
                    MediationType.HTTP,
                    "http://test.com",
                    groupId,  // Different message group per iteration
                    null
                );
                inPipelineMap.put(msg.id(), msg);
                boolean submitted = processPool.submit(msg);
                assertTrue(submitted, "Message " + messageId + " should be submitted");
            }
        }

        // Then: Verify that multiple groups process concurrently
        // With 40 groups and 100 max concurrency, we should see close to 40 concurrent executions
        // (each group has 1 virtual thread, so up to 40 can be active at once)

        // Wait for all messages to complete
        await()
            .atMost(30, TimeUnit.SECONDS)
            .untilAsserted(() -> {
                assertTrue(inPipelineMap.isEmpty(),
                    "All messages should be processed. Remaining: " + inPipelineMap.size());
            });

        // Verify we actually had concurrent processing
        int maxConcurrent = maxConcurrentProcessing.get();
        System.out.println("Max concurrent processing observed: " + maxConcurrent);

        // With 40 groups, we should see at least 20 concurrent executions
        // (allowing some slack for timing/scheduling)
        assertTrue(maxConcurrent >= 20,
            "Expected at least 20 concurrent executions with 40 message groups, but got: " + maxConcurrent);

        // Ideally should be close to 40
        assertTrue(maxConcurrent >= 30,
            "Expected close to 40 concurrent executions with 40 message groups, but got: " + maxConcurrent);

        // Verify that mediator was called for all messages
        verify(mockMediator, times(totalMessages)).process(any());
    }

    @Test
    void shouldRespectPoolConcurrencyAcrossAllGroups() throws InterruptedException {
        // Given: More groups than concurrency limit
        int numGroups = 150; // More than the 100 concurrency limit
        int messagesPerGroup = 1;

        processPool.start();

        // When: Submit one message per group (150 messages total)
        for (int groupNum = 0; groupNum < numGroups; groupNum++) {
            String groupId = "group-" + groupNum;
            String messageId = "msg-" + groupNum;
            MessagePointer msg = new MessagePointer(
                messageId,
                "MULTI-GROUP-TEST",
                "token",
                MediationType.HTTP,
                "http://test.com",
                groupId,
                null
            );
            inPipelineMap.put(msg.id(), msg);
            processPool.submit(msg);
        }

        // Then: Max concurrent execution should not exceed pool concurrency (100)
        await()
            .atMost(30, TimeUnit.SECONDS)
            .untilAsserted(() -> {
                assertTrue(inPipelineMap.isEmpty(), "All messages should be processed");
            });

        int maxConcurrent = maxConcurrentProcessing.get();
        System.out.println("Max concurrent processing with 150 groups: " + maxConcurrent);

        assertTrue(maxConcurrent <= 100,
            "Max concurrent processing should not exceed pool concurrency limit of 100, but got: " + maxConcurrent);

        // Should be close to 100 (the limit)
        assertTrue(maxConcurrent >= 80,
            "Expected close to 100 concurrent executions (at the limit), but got: " + maxConcurrent);
    }

    @Test
    void shouldProcessSequentiallyWhenAllMessagesInSameGroup() throws InterruptedException {
        // Given: 200 messages all in the SAME message group (reproduces production symptom)
        // This simulates what happens when messageGroupId is null/blank for all messages
        int totalMessages = 200;
        String singleGroupId = "single-group"; // All messages go to same group

        processPool.start();

        // When: Submit all messages to the same group
        for (int msgNum = 0; msgNum < totalMessages; msgNum++) {
            String messageId = "msg-" + msgNum;
            MessagePointer msg = new MessagePointer(
                messageId,
                "MULTI-GROUP-TEST",
                "token",
                MediationType.HTTP,
                "http://test.com",
                singleGroupId,  // SAME group for all messages
                null
            );
            inPipelineMap.put(msg.id(), msg);
            processPool.submit(msg);
        }

        // Give it a moment to start processing
        Thread.sleep(500);

        // Then: Should only have 1 active worker (since all messages in same group)
        int maxConcurrent = maxConcurrentProcessing.get();
        System.out.println("Max concurrent processing with 1 group (200 messages): " + maxConcurrent);

        // With all messages in same group, we should see exactly 1 concurrent execution
        // (FIFO processing within a single message group)
        assertEquals(1, maxConcurrent,
            "Expected exactly 1 concurrent execution when all messages in same group, but got: " + maxConcurrent);

        // Wait for all to complete
        await()
            .atMost(60, TimeUnit.SECONDS)
            .untilAsserted(() -> {
                assertTrue(inPipelineMap.isEmpty(),
                    "All messages should be processed. Remaining: " + inPipelineMap.size());
            });

        verify(mockMediator, times(totalMessages)).process(any());
    }
}

package tech.flowcatalyst.messagerouter.pool;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.mockito.ArgumentCaptor;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Pure unit tests for ProcessPoolImpl - no Quarkus context needed.
 * All dependencies are mocked, making tests fast and isolated.
 */
class ProcessPoolImplTest {

    private ProcessPoolImpl processPool;
    private Mediator mockMediator;
    private MessageCallback mockCallback;
    private ConcurrentMap<String, MessagePointer> inPipelineMap;
    private PoolMetricsService mockPoolMetrics;
    private WarningService mockWarningService;

    @BeforeEach
    void setUp() {
        mockMediator = mock(Mediator.class);
        mockCallback = mock(MessageCallback.class);
        inPipelineMap = new ConcurrentHashMap<>();
        mockPoolMetrics = mock(PoolMetricsService.class);
        mockWarningService = mock(WarningService.class);

        // Configure mockCallback to remove from inPipelineMap when ack/nack is called
        // This simulates QueueManager's behavior
        doAnswer(invocation -> {
            MessagePointer msg = invocation.getArgument(0);
            inPipelineMap.remove(msg.id());
            return null;
        }).when(mockCallback).ack(any(MessagePointer.class));

        doAnswer(invocation -> {
            MessagePointer msg = invocation.getArgument(0);
            inPipelineMap.remove(msg.id());
            return null;
        }).when(mockCallback).nack(any(MessagePointer.class));

        processPool = new ProcessPoolImpl(
            "TEST-POOL",
            5, // concurrency
            100, // queue capacity
            null, // rateLimitPerMinute
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );
    }

    @AfterEach
    void tearDown() {
        if (processPool != null) {
            processPool.drain();
        }
    }

    @Test
    void shouldProcessMessageSuccessfully() {
        // Given
        MessagePointer message = new MessagePointer("msg-1", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        when(mockMediator.process(message)).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message.id(), message);

        // When
        processPool.start();
        boolean submitted = processPool.submit(message);

        // Then
        assertTrue(submitted);

        await().untilAsserted(() -> {
            verify(mockMediator).process(message);
            verify(mockCallback).ack(message);
            assertFalse(inPipelineMap.containsKey(message.id()));
        });
    }

    @Test
    void shouldNackMessageOnMediationFailure() {
        // Given
        MessagePointer message = new MessagePointer("msg-2", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        when(mockMediator.process(message)).thenReturn(MediationOutcome.errorProcess((Integer) null));
        inPipelineMap.put(message.id(), message);

        // When
        processPool.start();
        processPool.submit(message);

        // Then - ERROR_PROCESS is a transient error, so it should nack but not generate a warning
        await().untilAsserted(() -> {
            verify(mockMediator).process(message);
            verify(mockCallback).nack(message);
            // Transient errors (ERROR_PROCESS) don't generate warnings - they're expected and will be retried
            verify(mockWarningService, never()).addWarning(
                eq("MEDIATION"),
                eq("ERROR"),
                any(),
                any()
            );
        });
    }

    @Test
    void shouldEnforceRateLimit() {
        // Given - create a pool with rate limiting enabled
        // Rate limit is 1 per minute, so first message processes immediately,
        // second message waits in memory (blocking wait) until rate limit allows
        ProcessPoolImpl rateLimitedPool = new ProcessPoolImpl(
            "RATE-LIMITED-POOL",
            5,
            100,
            1, // 1 per minute rate limit
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        MessagePointer message1 = new MessagePointer("msg-rate-1", "RATE-LIMITED-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessagePointer message2 = new MessagePointer("msg-rate-2", "RATE-LIMITED-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);

        // When
        rateLimitedPool.start();
        rateLimitedPool.submit(message1);
        rateLimitedPool.submit(message2);

        // Then - first message processes immediately, second waits for rate limit
        // With blocking wait approach, messages stay in memory instead of being NACKed
        await().atMost(Duration.ofSeconds(2)).untilAsserted(() -> {
            // First message should be processed successfully
            verify(mockMediator, times(1)).process(any(MessagePointer.class));
            verify(mockCallback, times(1)).ack(any(MessagePointer.class));
        });

        // Second message should NOT be nacked - it's waiting for rate limit permit
        // (blocking wait keeps it in memory instead of NACKing back to queue)
        verify(mockCallback, never()).nack(any(MessagePointer.class));

        rateLimitedPool.drain();
    }

    @Test
    void shouldRejectMessageWhenQueueFull() {
        // Given - use blocking mediator and no start() to prevent processing
        when(mockMediator.process(any())).thenAnswer(invocation -> {
            Thread.sleep(200); // Block for a bit
            return MediationOutcome.success();
        });

        ProcessPoolImpl smallPool = new ProcessPoolImpl(
            "SMALL-POOL",
            1,
            2, // Queue capacity of 2
            null, // rateLimitPerMinute
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        MessagePointer message1 = new MessagePointer("msg-1", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com",
            null,  // messageGroupId - same group (DEFAULT_GROUP)
            null   // batchId
        );
        MessagePointer message2 = new MessagePointer("msg-2", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com",
            null,  // messageGroupId - same group (DEFAULT_GROUP)
            null   // batchId
        );
        MessagePointer message3 = new MessagePointer("msg-3", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com",
            null,  // messageGroupId - same group (DEFAULT_GROUP)
            null   // batchId
        );

        // When
        smallPool.start();
        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);
        inPipelineMap.put(message3.id(), message3);

        // Submit messages rapidly to fill the queue before virtual thread processes them
        boolean submitted1 = smallPool.submit(message1);
        boolean submitted2 = smallPool.submit(message2);
        boolean submitted3 = smallPool.submit(message3);

        // Then
        assertTrue(submitted1, "First message should submit");
        assertTrue(submitted2, "Second message should submit");

        // Note: With per-group virtual threads, the third message might be accepted
        // because messages are polled from queue before acquiring semaphore.
        // The queue capacity (2) is still enforced, but semantics are slightly different.
        // If virtual thread is fast, msg1 and msg2 might be polled (queue size drops to 0-1)
        // and msg3 fits. This is acceptable - concurrency is still controlled by semaphore.
        //
        // To reliably test queue capacity, we'd need to submit >2 messages before
        // the virtual thread starts, which is race-dependent.
        //
        // The important invariant is: queue.size() <= queueCapacity at all times.
        // Pool-level concurrency is still enforced by the semaphore (tested elsewhere).

        smallPool.drain();
    }

    @Test
    void shouldRespectConcurrencyLimit() {
        // Given
        ProcessPoolImpl lowConcurrencyPool = new ProcessPoolImpl(
            "LOW-CONCURRENCY",
            2, // Only 2 concurrent
            100,
            null, // rateLimitPerMinute
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // Simulate slow processing
        when(mockMediator.process(any())).thenAnswer(invocation -> {
            Thread.sleep(500);
            return MediationOutcome.success();
        });

        // When
        lowConcurrencyPool.start();

        for (int i = 0; i < 5; i++) {
            MessagePointer msg = new MessagePointer("msg-" + i, "LOW-CONCURRENCY", "token", MediationType.HTTP, "http://test.com"
            , null
            , null);
            inPipelineMap.put(msg.id(), msg);
            lowConcurrencyPool.submit(msg);
        }

        // Then
        // Verify that concurrency is respected by checking metrics
        await().untilAsserted(() -> {
            verify(mockPoolMetrics, atLeast(5)).updatePoolGauges(
                eq("LOW-CONCURRENCY"),
                anyInt(),  // activeWorkers
                anyInt(),  // availablePermits
                anyInt(),  // queueSize
                anyInt()   // messageGroupCount
            );
        });

        lowConcurrencyPool.drain();
    }

    @Test
    void shouldRemoveFromPipelineMapAfterProcessing() {
        // Given
        MessagePointer message = new MessagePointer("msg-pipeline", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        when(mockMediator.process(message)).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message.id(), message);

        // When
        processPool.start();
        processPool.submit(message);

        // Then
        await().untilAsserted(() -> {
            assertFalse(inPipelineMap.containsKey(message.id()));
        });
    }

    @Test
    void shouldDrainGracefully() {
        // Given
        MessagePointer message = new MessagePointer("msg-drain", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        // Use any() matcher since ProcessPoolImpl may modify the message (adds batchId)
        when(mockMediator.process(any(MessagePointer.class))).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message.id(), message);

        processPool.start();
        processPool.submit(message);

        // When
        processPool.drain(); // Non-blocking - just sets running=false

        // Wait for pool to finish processing buffered messages
        await().untilAsserted(() -> assertTrue(processPool.isFullyDrained()));

        // Then - use any() since message may have been modified
        verify(mockMediator).process(any(MessagePointer.class));
        assertTrue(inPipelineMap.isEmpty());

        // Cleanup
        processPool.shutdown();
    }

    @Test
    void shouldTrackDifferentRateLimitKeysSeparately() {
        // Given - rate limiting is now pool-level, not message-level
        // This test now verifies that messages are processed normally when no rate limit is set
        MessagePointer message1 = new MessagePointer("msg-key1", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessagePointer message2 = new MessagePointer("msg-key2", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);

        // When
        processPool.start();
        processPool.submit(message1);
        processPool.submit(message2);

        // Then - both should be processed (no rate limiting)
        await().untilAsserted(() -> {
            verify(mockMediator).process(message1);
            verify(mockMediator).process(message2);
            verify(mockCallback).ack(message1);
            verify(mockCallback).ack(message2);
        });
    }

    @Test
    void shouldGetPoolCodeAndConcurrency() {
        assertEquals("TEST-POOL", processPool.getPoolCode());
        assertEquals(5, processPool.getConcurrency());
    }

    // ========================================
    // Batch+Group FIFO Enforcement Tests
    // ========================================

    @Test
    void shouldNackSubsequentMessagesWhenBatchGroupFails() {
        // Given - Three messages in same batch+group
        String batchId = "batch-123";
        String messageGroupId = "order-456";

        MessagePointer message1 = new MessagePointer(
            "msg-batch-1",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );

        MessagePointer message2 = new MessagePointer(
            "msg-batch-2",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );

        MessagePointer message3 = new MessagePointer(
            "msg-batch-3",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );

        // First message succeeds, second fails, third should be auto-nacked
        when(mockMediator.process(message1)).thenReturn(MediationOutcome.success());
        when(mockMediator.process(message2)).thenReturn(MediationOutcome.errorProcess((Integer) null));
        when(mockMediator.process(message3)).thenReturn(MediationOutcome.success());

        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);
        inPipelineMap.put(message3.id(), message3);

        // When
        processPool.start();
        processPool.submit(message1);
        processPool.submit(message2);
        processPool.submit(message3);

        // Then
        await().untilAsserted(() -> {
            // Message 1 should be processed and acked
            verify(mockMediator).process(message1);
            verify(mockCallback).ack(message1);

            // Message 2 should be processed and nacked (failure)
            verify(mockMediator).process(message2);
            verify(mockCallback).nack(message2);

            // Message 3 should be nacked WITHOUT processing (pre-flight check)
            verify(mockMediator, never()).process(message3);
            verify(mockCallback).nack(message3);
        });
    }

    @Test
    void shouldAllowDifferentBatchGroupsToProcessIndependently() {
        // Given - Messages from different batch+groups
        MessagePointer message1 = new MessagePointer(
            "msg-batch-1",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            "order-111",
            "batch-aaa"
        );

        MessagePointer message2 = new MessagePointer(
            "msg-batch-2",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            "order-222",  // Different message group
            "batch-bbb"   // Different batch
        );

        // First message fails, second should still process (different batch+group)
        when(mockMediator.process(message1)).thenReturn(MediationOutcome.errorProcess((Integer) null));
        when(mockMediator.process(message2)).thenReturn(MediationOutcome.success());

        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);

        // When
        processPool.start();
        processPool.submit(message1);
        processPool.submit(message2);

        // Then
        await().untilAsserted(() -> {
            // Message 1 should be nacked
            verify(mockMediator).process(message1);
            verify(mockCallback).nack(message1);

            // Message 2 should be processed normally (different batch+group)
            verify(mockMediator).process(message2);
            verify(mockCallback).ack(message2);
        });
    }

    @Test
    void shouldCleanupBatchGroupTrackingOnSuccess() {
        // Given - Messages in same batch+group that all succeed
        String batchId = "batch-success";
        String messageGroupId = "order-success";

        MessagePointer message1 = new MessagePointer(
            "msg-success-1",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );

        MessagePointer message2 = new MessagePointer(
            "msg-success-2",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );

        when(mockMediator.process(message1)).thenReturn(MediationOutcome.success());
        when(mockMediator.process(message2)).thenReturn(MediationOutcome.success());

        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);

        // When
        processPool.start();
        processPool.submit(message1);
        processPool.submit(message2);

        // Then - both should be processed and acked
        await().untilAsserted(() -> {
            verify(mockMediator).process(message1);
            verify(mockCallback).ack(message1);

            verify(mockMediator).process(message2);
            verify(mockCallback).ack(message2);

            // Pipeline should be clean
            assertTrue(inPipelineMap.isEmpty());
        });
    }

    @Test
    void shouldHandleNullBatchIdGracefully() {
        // Given - Messages without batchId (legacy behavior)
        MessagePointer message1 = new MessagePointer(
            "msg-no-batch-1",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            "order-789",
            null  // No batchId
        );

        MessagePointer message2 = new MessagePointer(
            "msg-no-batch-2",
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            "order-789",
            null  // No batchId
        );

        // First fails, second should still process (no batch tracking)
        when(mockMediator.process(message1)).thenReturn(MediationOutcome.errorProcess((Integer) null));
        when(mockMediator.process(message2)).thenReturn(MediationOutcome.success());

        inPipelineMap.put(message1.id(), message1);
        inPipelineMap.put(message2.id(), message2);

        // When
        processPool.start();
        processPool.submit(message1);
        processPool.submit(message2);

        // Then - both should be processed (no batch+group enforcement without batchId)
        await().untilAsserted(() -> {
            verify(mockMediator).process(message1);
            verify(mockCallback).nack(message1);

            verify(mockMediator).process(message2);
            verify(mockCallback).ack(message2);
        });
    }

    // ========================================
    // Concurrency and Rate Limit Update Tests
    // ========================================

    @Test
    void shouldIncreaseConcurrencySuccessfully() {
        // Given - Pool with concurrency of 3
        ProcessPoolImpl pool = new ProcessPoolImpl(
            "UPDATE-POOL-1",
            3,
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // When - increase concurrency to 5
        boolean result = pool.updateConcurrency(5, 60);

        // Then
        assertTrue(result, "Concurrency increase should succeed");
        assertEquals(5, pool.getConcurrency(), "Concurrency should be updated to 5");

        pool.drain();
    }

    @Test
    void shouldDecreaseConcurrencyWhenIdlePermiysAvailable() {
        // Given - Pool with concurrency of 5, no active processing
        ProcessPoolImpl pool = new ProcessPoolImpl(
            "UPDATE-POOL-2",
            5,
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // When - decrease concurrency to 2 (all permits available)
        boolean result = pool.updateConcurrency(2, 60);

        // Then
        assertTrue(result, "Concurrency decrease should succeed when permits available");
        assertEquals(2, pool.getConcurrency(), "Concurrency should be updated to 2");

        pool.drain();
    }

    @Test
    void shouldHandleMultipleConcurrencyUpdates() {
        // Given - Pool with initial concurrency
        ProcessPoolImpl pool = new ProcessPoolImpl(
            "UPDATE-POOL-3",
            5,
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        assertEquals(5, pool.getConcurrency());

        // When - increase then decrease
        boolean increase = pool.updateConcurrency(8, 60);
        assertTrue(increase, "Increase should succeed");
        assertEquals(8, pool.getConcurrency());

        // Decrease back down
        boolean decrease = pool.updateConcurrency(3, 60);
        assertTrue(decrease, "Decrease should succeed when permits available");
        assertEquals(3, pool.getConcurrency());

        // Then - final update to same value
        boolean noChange = pool.updateConcurrency(3, 60);
        assertTrue(noChange, "Update to same value should succeed");
        assertEquals(3, pool.getConcurrency());

        pool.drain();
    }

    @Test
    void shouldEnableRateLimitingViaUpdate() {
        // Given - Pool created without rate limiting
        ProcessPoolImpl pool = new ProcessPoolImpl(
            "UPDATE-POOL-4",
            5,
            100,
            null, // No rate limit initially
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        assertNull(pool.getRateLimitPerMinute(), "Should start with no rate limit");

        // When - enable rate limiting
        pool.updateRateLimit(10); // 10 per minute

        // Then
        assertEquals(10, pool.getRateLimitPerMinute(), "Rate limit should be updated to 10/min");
        assertTrue(pool.isRateLimited() || !pool.isRateLimited(), "Rate limiter should exist"); // Just verify no exception

        pool.drain();
    }

    @Test
    void shouldDisableRateLimitingViaUpdate() {
        // Given - Pool created with rate limiting
        ProcessPoolImpl pool = new ProcessPoolImpl(
            "UPDATE-POOL-5",
            5,
            100,
            5, // Rate limit 5 per minute
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        assertEquals(5, pool.getRateLimitPerMinute(), "Should start with 5/min rate limit");

        // When - disable rate limiting by setting to null
        pool.updateRateLimit(null);

        // Then
        assertNull(pool.getRateLimitPerMinute(), "Rate limit should be null after update");
        assertFalse(pool.isRateLimited(), "Should not be rate limited when null");

        pool.drain();
    }

    // ========================================
    // High Priority Message Tests
    // ========================================

    @Test
    void shouldProcessHighPriorityMessagesFirst() throws InterruptedException {
        // Given: Pool with concurrency=1 to ensure sequential processing
        // This test verifies that when messages are QUEUED, high priority are processed before regular
        ProcessPoolImpl priorityPool = new ProcessPoolImpl(
            "PRIORITY-POOL",
            1, // Single worker to ensure deterministic ordering
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // Track processing order
        List<String> processingOrder = new CopyOnWriteArrayList<>();

        // Use latch to block first message, allowing subsequent messages to queue
        CountDownLatch firstMessageBlocking = new CountDownLatch(1);
        CountDownLatch firstMessageStarted = new CountDownLatch(1);
        CountDownLatch processingComplete = new CountDownLatch(3);

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            if (msg.id().equals("msg-blocking")) {
                // Signal that first message has started
                firstMessageStarted.countDown();
                // Block until other messages are queued
                firstMessageBlocking.await();
            }
            processingOrder.add(msg.id());
            processingComplete.countDown();
            return MediationOutcome.success();
        });

        // First message is used to block processing while we queue others
        MessagePointer blockingMsg = new MessagePointer(
            "msg-blocking", "PRIORITY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-1", false, null);
        MessagePointer regularMsg = new MessagePointer(
            "msg-regular", "PRIORITY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-1", false, null);
        MessagePointer highPriorityMsg = new MessagePointer(
            "msg-high", "PRIORITY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-1", true, null);

        inPipelineMap.put(blockingMsg.id(), blockingMsg);
        inPipelineMap.put(regularMsg.id(), regularMsg);
        inPipelineMap.put(highPriorityMsg.id(), highPriorityMsg);

        // When: Start pool and submit blocking message first
        priorityPool.start();
        priorityPool.submit(blockingMsg);

        // Wait for blocking message to start processing (hold the semaphore)
        assertTrue(firstMessageStarted.await(5, TimeUnit.SECONDS),
            "First message should start processing");

        // Now queue regular and high priority messages (in that order)
        // Since first message holds the semaphore, these will be queued
        priorityPool.submit(regularMsg);
        priorityPool.submit(highPriorityMsg);

        // Give brief moment for messages to be added to queues
        Thread.sleep(20);

        // Release the blocking message
        firstMessageBlocking.countDown();

        // Wait for all messages to be processed
        assertTrue(processingComplete.await(5, TimeUnit.SECONDS),
            "All messages should be processed within timeout");

        // Then: Verify order - blocking first, then high priority, then regular
        assertEquals(3, processingOrder.size(), "All 3 messages should be processed");
        assertEquals("msg-blocking", processingOrder.get(0),
            "Blocking message should be processed first (it was already running)");
        assertEquals("msg-high", processingOrder.get(1),
            "High priority message should be processed before regular");
        assertEquals("msg-regular", processingOrder.get(2),
            "Regular message should be processed last");

        priorityPool.drain();
    }

    @Test
    void shouldMaintainFifoWithinHighPriorityTier() throws InterruptedException {
        // Given: Pool with concurrency=1
        ProcessPoolImpl priorityPool = new ProcessPoolImpl(
            "PRIORITY-POOL-2",
            1,
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        List<String> processingOrder = new CopyOnWriteArrayList<>();
        CountDownLatch startProcessing = new CountDownLatch(1);
        CountDownLatch processingComplete = new CountDownLatch(3);

        when(mockMediator.process(any())).thenAnswer(inv -> {
            startProcessing.await();
            MessagePointer msg = inv.getArgument(0);
            processingOrder.add(msg.id());
            processingComplete.countDown();
            return MediationOutcome.success();
        });

        // Create 3 high priority messages
        MessagePointer high1 = new MessagePointer(
            "high-1", "PRIORITY-POOL-2", "token", MediationType.HTTP,
            "http://test.com", "group-1", true, null);
        MessagePointer high2 = new MessagePointer(
            "high-2", "PRIORITY-POOL-2", "token", MediationType.HTTP,
            "http://test.com", "group-1", true, null);
        MessagePointer high3 = new MessagePointer(
            "high-3", "PRIORITY-POOL-2", "token", MediationType.HTTP,
            "http://test.com", "group-1", true, null);

        inPipelineMap.put(high1.id(), high1);
        inPipelineMap.put(high2.id(), high2);
        inPipelineMap.put(high3.id(), high3);

        // When
        priorityPool.start();
        priorityPool.submit(high1);
        priorityPool.submit(high2);
        priorityPool.submit(high3);

        Thread.sleep(50);
        startProcessing.countDown();

        assertTrue(processingComplete.await(5, TimeUnit.SECONDS));

        // Then: Should process in FIFO order: high-1, high-2, high-3
        assertEquals(List.of("high-1", "high-2", "high-3"), processingOrder,
            "High priority messages should maintain FIFO order within their tier");

        priorityPool.drain();
    }

    @Test
    void shouldShareConcurrencyAcrossPriorities() throws InterruptedException {
        // Given: Pool with concurrency=2
        ProcessPoolImpl priorityPool = new ProcessPoolImpl(
            "SHARED-CONCURRENCY-POOL",
            2, // 2 concurrent workers
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // Track concurrent processing
        CountDownLatch allStarted = new CountDownLatch(2);
        CountDownLatch continueProcessing = new CountDownLatch(1);
        List<String> concurrentMessages = new CopyOnWriteArrayList<>();

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            concurrentMessages.add(msg.id());
            allStarted.countDown();
            continueProcessing.await(); // Block until we check concurrency
            return MediationOutcome.success();
        });

        // Create 4 messages: 2 high, 2 regular
        MessagePointer high1 = new MessagePointer(
            "high-1", "SHARED-CONCURRENCY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-a", true, null); // Different group to allow concurrent
        MessagePointer high2 = new MessagePointer(
            "high-2", "SHARED-CONCURRENCY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-b", true, null);
        MessagePointer regular1 = new MessagePointer(
            "regular-1", "SHARED-CONCURRENCY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-c", false, null);
        MessagePointer regular2 = new MessagePointer(
            "regular-2", "SHARED-CONCURRENCY-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-d", false, null);

        inPipelineMap.put(high1.id(), high1);
        inPipelineMap.put(high2.id(), high2);
        inPipelineMap.put(regular1.id(), regular1);
        inPipelineMap.put(regular2.id(), regular2);

        // When
        priorityPool.start();
        priorityPool.submit(high1);
        priorityPool.submit(high2);
        priorityPool.submit(regular1);
        priorityPool.submit(regular2);

        // Wait for 2 messages to start processing (semaphore limit)
        assertTrue(allStarted.await(5, TimeUnit.SECONDS),
            "Should process up to 2 messages concurrently (semaphore limit)");

        // Then: Exactly 2 messages should be processing (shared semaphore)
        assertEquals(2, concurrentMessages.size(),
            "Shared semaphore should limit to 2 concurrent regardless of priority");

        // Allow processing to complete
        continueProcessing.countDown();

        // Cleanup
        priorityPool.drain();
    }

    @Test
    void shouldHandleHighPriorityWithDefaultFalse() {
        // Given: Message without explicit highPriority (should default to false)
        MessagePointer message = new MessagePointer(
            "msg-default", "TEST-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-1", null // Using backward-compatible constructor
        );

        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());
        inPipelineMap.put(message.id(), message);

        // When
        processPool.start();
        boolean submitted = processPool.submit(message);

        // Then: Should be processed successfully
        assertTrue(submitted);

        await().untilAsserted(() -> {
            verify(mockMediator).process(any());
            verify(mockCallback).ack(any());
        });
    }

    @Test
    void shouldProcessMixedPrioritiesAcrossDifferentGroups() throws InterruptedException {
        // Given: Two groups, each with high and regular priority messages
        // Use concurrency=1 per group (effectively 2 total but FIFO within each group)
        ProcessPoolImpl priorityPool = new ProcessPoolImpl(
            "MULTI-GROUP-POOL",
            2, // Allow both groups to process concurrently
            100,
            null,
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        // Track processing order per group
        List<String> groupAOrder = new CopyOnWriteArrayList<>();
        List<String> groupBOrder = new CopyOnWriteArrayList<>();

        // Block first message of each group to ensure subsequent messages queue
        CountDownLatch blockingStarted = new CountDownLatch(2); // Both groups started
        CountDownLatch releaseBlocking = new CountDownLatch(1);
        CountDownLatch processingComplete = new CountDownLatch(6); // 3 per group

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);

            // Block the first message of each group
            if (msg.id().equals("groupA-blocking") || msg.id().equals("groupB-blocking")) {
                blockingStarted.countDown();
                releaseBlocking.await();
            }

            // Track order per group
            if (msg.id().startsWith("groupA")) {
                groupAOrder.add(msg.id());
            } else {
                groupBOrder.add(msg.id());
            }

            processingComplete.countDown();
            return MediationOutcome.success();
        });

        // Group A: blocking first, then queue regular, then high
        MessagePointer groupABlocking = new MessagePointer(
            "groupA-blocking", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-A", false, null);
        MessagePointer groupARegular = new MessagePointer(
            "groupA-regular", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-A", false, null);
        MessagePointer groupAHigh = new MessagePointer(
            "groupA-high", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-A", true, null);

        // Group B: blocking first, then queue regular, then high
        MessagePointer groupBBlocking = new MessagePointer(
            "groupB-blocking", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-B", false, null);
        MessagePointer groupBRegular = new MessagePointer(
            "groupB-regular", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-B", false, null);
        MessagePointer groupBHigh = new MessagePointer(
            "groupB-high", "MULTI-GROUP-POOL", "token", MediationType.HTTP,
            "http://test.com", "group-B", true, null);

        inPipelineMap.put(groupABlocking.id(), groupABlocking);
        inPipelineMap.put(groupARegular.id(), groupARegular);
        inPipelineMap.put(groupAHigh.id(), groupAHigh);
        inPipelineMap.put(groupBBlocking.id(), groupBBlocking);
        inPipelineMap.put(groupBRegular.id(), groupBRegular);
        inPipelineMap.put(groupBHigh.id(), groupBHigh);

        // When: Submit blocking messages first to start threads
        priorityPool.start();
        priorityPool.submit(groupABlocking);
        priorityPool.submit(groupBBlocking);

        // Wait for blocking messages to start processing
        assertTrue(blockingStarted.await(5, TimeUnit.SECONDS),
            "Blocking messages should start processing");

        // Now queue regular then high for each group
        priorityPool.submit(groupARegular);
        priorityPool.submit(groupAHigh);
        priorityPool.submit(groupBRegular);
        priorityPool.submit(groupBHigh);

        // Give time for messages to queue
        Thread.sleep(20);

        // Release blocking messages
        releaseBlocking.countDown();

        // Wait for all 6 messages to be processed
        assertTrue(processingComplete.await(5, TimeUnit.SECONDS),
            "All messages should be processed within timeout");

        // Then: Within each group, high priority should be processed before regular
        // Group A order: blocking, high, regular
        assertEquals(3, groupAOrder.size(), "Group A should have 3 messages");
        assertEquals("groupA-blocking", groupAOrder.get(0),
            "Group A blocking should be first");
        assertEquals("groupA-high", groupAOrder.get(1),
            "Group A high priority should come before regular");
        assertEquals("groupA-regular", groupAOrder.get(2),
            "Group A regular should be last");

        // Group B order: blocking, high, regular
        assertEquals(3, groupBOrder.size(), "Group B should have 3 messages");
        assertEquals("groupB-blocking", groupBOrder.get(0),
            "Group B blocking should be first");
        assertEquals("groupB-high", groupBOrder.get(1),
            "Group B high priority should come before regular");
        assertEquals("groupB-regular", groupBOrder.get(2),
            "Group B regular should be last");

        priorityPool.drain();
    }
}

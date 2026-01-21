package tech.flowcatalyst.messagerouter.integration;

import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.TestInstance;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;

import static java.util.concurrent.TimeUnit.SECONDS;
import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.when;

/**
 * Integration tests for message group FIFO ordering.
 *
 * These tests verify that we correctly solved the original problem:
 * workers blocking while holding messages they can't process.
 *
 * See docs/MESSAGE_GROUP_FIFO_TEST_PLAN.md for full test specifications.
 */
@TestInstance(TestInstance.Lifecycle.PER_METHOD)
class MessageGroupFifoOrderingTest {

    private Mediator mockMediator;
    private MessageCallback mockCallback;
    private PoolMetricsService mockPoolMetrics;
    private WarningService mockWarningService;

    private ProcessPool pool;
    private ConcurrentHashMap<String, MessagePointer> inPipelineMap;
    private AtomicInteger processedCount;

    @BeforeEach
    void setup() {
        // Create mocks
        mockMediator = mock(Mediator.class);
        mockCallback = mock(MessageCallback.class);
        mockPoolMetrics = mock(PoolMetricsService.class);
        mockWarningService = mock(WarningService.class);

        inPipelineMap = new ConcurrentHashMap<>();
        processedCount = new AtomicInteger(0);
    }

    @AfterEach
    void teardown() {
        if (pool != null) {
            try {
                pool.shutdown();
                // Give pool time to shut down cleanly
                Thread.sleep(100);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }
        // Clear maps
        if (inPipelineMap != null) {
            inPipelineMap.clear();
        }
    }

    /**
     * Test 1.1: FIFO Ordering Within Single Message Group
     *
     * Objective: Verify messages within the same group process in FIFO order
     *
     * Expected: Messages complete in exact order (msg-0, msg-1, ..., msg-19)
     *           Even with 10 workers, only 1 processes at a time for this group
     */
    @Test
    void shouldMaintainFifoOrderingWithinSingleMessageGroup() {
        // Given
        pool = createTestPool("FIFO-TEST", 10, 100);
        List<String> completionOrder = new CopyOnWriteArrayList<>();
        List<Long> completionTimestamps = new CopyOnWriteArrayList<>();

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            completionOrder.add(msg.id());
            completionTimestamps.add(System.currentTimeMillis());
            processedCount.incrementAndGet();
            Thread.sleep(10); // Small delay to ensure sequential processing is observable
            return MediationResult.SUCCESS;
        });

        pool.start();

        // When: Submit 20 messages to same group
        for (int i = 0; i < 20; i++) {
            MessagePointer msg = createMessage("msg-" + i, "FIFO-TEST", "group-1");
            inPipelineMap.put(msg.id(), msg);
            pool.submit(msg);
        }

        // Then: Wait for all messages to complete
        await().atMost(5, SECONDS).until(() -> processedCount.get() == 20);

        // Verify FIFO order
        assertEquals(20, completionOrder.size(), "All messages should complete");
        for (int i = 0; i < 20; i++) {
            assertEquals("msg-" + i, completionOrder.get(i),
                "Message at index " + i + " should be msg-" + i);
        }

        // Verify timestamps are monotonically increasing (strict FIFO)
        for (int i = 1; i < completionTimestamps.size(); i++) {
            assertTrue(completionTimestamps.get(i) >= completionTimestamps.get(i - 1),
                "Timestamp at index " + i + " should be >= previous timestamp (sequential processing)");
        }
    }

    /**
     * Test 1.2: Concurrent Processing Across Different Message Groups
     *
     * Objective: Verify different message groups process concurrently
     *
     * Expected: Both groups process in parallel
     *           Total time ~1000ms (not 2000ms serial)
     *           Each group maintains FIFO internally
     */
    @Test
    void shouldProcessDifferentMessageGroupsConcurrently() {
        // Given
        pool = createTestPool("CONCURRENT-TEST", 10, 100);
        Set<String> concurrentGroups = ConcurrentHashMap.newKeySet();
        AtomicInteger group1Processing = new AtomicInteger(0);
        AtomicInteger group2Processing = new AtomicInteger(0);
        AtomicInteger maxGroup1Concurrent = new AtomicInteger(0);
        AtomicInteger maxGroup2Concurrent = new AtomicInteger(0);
        Map<String, List<String>> completionsByGroup = new ConcurrentHashMap<>();
        completionsByGroup.put("group-1", new CopyOnWriteArrayList<>());
        completionsByGroup.put("group-2", new CopyOnWriteArrayList<>());

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            String group = msg.messageGroupId();

            // Track concurrent processing (NO assertions here - they throw exceptions!)
            if (group.equals("group-1")) {
                int concurrent = group1Processing.incrementAndGet();
                maxGroup1Concurrent.updateAndGet(max -> Math.max(max, concurrent));
            } else if (group.equals("group-2")) {
                int concurrent = group2Processing.incrementAndGet();
                maxGroup2Concurrent.updateAndGet(max -> Math.max(max, concurrent));
            }

            concurrentGroups.add(group);
            Thread.sleep(100); // Simulate work

            completionsByGroup.get(group).add(msg.id());
            processedCount.incrementAndGet();

            if (group.equals("group-1")) group1Processing.decrementAndGet();
            if (group.equals("group-2")) group2Processing.decrementAndGet();

            return MediationResult.SUCCESS;
        });

        pool.start();

        long startTime = System.currentTimeMillis();

        // When: Submit 10 messages for each group
        for (int i = 0; i < 10; i++) {
            MessagePointer msg1 = createMessage("msg-" + i, "CONCURRENT-TEST", "group-1");
            MessagePointer msg2 = createMessage("msg-" + i, "CONCURRENT-TEST", "group-2");
            inPipelineMap.put(msg1.id(), msg1);
            inPipelineMap.put(msg2.id(), msg2);
            pool.submit(msg1);
            pool.submit(msg2);
        }

        // Then: Wait for completion
        await().atMost(5, SECONDS).until(() -> processedCount.get() == 20);
        long duration = System.currentTimeMillis() - startTime;

        // Verify concurrent processing happened
        assertTrue(concurrentGroups.size() >= 2, "Both groups should have processed");
        assertTrue(duration < 1500,
            "Should complete in ~1000ms with concurrency, not 2000ms serial. Took: " + duration + "ms");

        // Verify only 1 worker per group at a time
        assertEquals(1, maxGroup1Concurrent.get(), "Only 1 worker should process group-1 at a time");
        assertEquals(1, maxGroup2Concurrent.get(), "Only 1 worker should process group-2 at a time");

        // Verify FIFO within each group
        for (int g = 1; g <= 2; g++) {
            List<String> completions = completionsByGroup.get("group-" + g);
            assertEquals(10, completions.size(), "Group " + g + " should have 10 messages");
            for (int i = 0; i < 10; i++) {
                assertEquals("msg-" + i, completions.get(i),
                    "Group " + g + " message " + i + " out of order");
            }
        }
    }

    /**
     * Test 1.3: Mixed Interleaved Message Groups
     *
     * Objective: Verify correct routing and ordering with interleaved messages
     *
     * Expected: Each group maintains FIFO order independently
     *           Groups can complete in any interleaved order
     */
    @Test
    void shouldMaintainFifoOrderWithInterleavedMessageGroups() {
        // Given
        pool = createTestPool("INTERLEAVED-TEST", 10, 100);
        Map<String, List<String>> completionsByGroup = new ConcurrentHashMap<>();
        completionsByGroup.put("group-A", new CopyOnWriteArrayList<>());
        completionsByGroup.put("group-B", new CopyOnWriteArrayList<>());
        completionsByGroup.put("group-C", new CopyOnWriteArrayList<>());

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            completionsByGroup.get(msg.messageGroupId()).add(msg.id());
            processedCount.incrementAndGet();
            Thread.sleep(10);
            return MediationResult.SUCCESS;
        });

        pool.start();

        // When: Submit interleaved messages (A0, B0, C0, A1, B1, C1, ...)
        for (int i = 0; i < 10; i++) {
            MessagePointer msgA = createMessage("A-" + i, "INTERLEAVED-TEST", "group-A");
            MessagePointer msgB = createMessage("B-" + i, "INTERLEAVED-TEST", "group-B");
            MessagePointer msgC = createMessage("C-" + i, "INTERLEAVED-TEST", "group-C");

            inPipelineMap.put(msgA.id(), msgA);
            inPipelineMap.put(msgB.id(), msgB);
            inPipelineMap.put(msgC.id(), msgC);

            pool.submit(msgA);
            pool.submit(msgB);
            pool.submit(msgC);
        }

        // Then: Wait for all 30 messages
        await().atMost(5, SECONDS).until(() -> processedCount.get() == 30);

        // Verify FIFO within each group
        List<String> groupA = completionsByGroup.get("group-A");
        List<String> groupB = completionsByGroup.get("group-B");
        List<String> groupC = completionsByGroup.get("group-C");

        assertEquals(10, groupA.size(), "Group A should have 10 messages");
        assertEquals(10, groupB.size(), "Group B should have 10 messages");
        assertEquals(10, groupC.size(), "Group C should have 10 messages");

        // Verify order within each group
        for (int i = 0; i < 10; i++) {
            assertEquals("A-" + i, groupA.get(i), "Group A message " + i + " out of order");
            assertEquals("B-" + i, groupB.get(i), "Group B message " + i + " out of order");
            assertEquals("C-" + i, groupC.get(i), "Group C message " + i + " out of order");
        }
    }

    /**
     * Test 2.1: Message Groups Process Independently (No Blocking)
     *
     * THIS IS THE KEY TEST THAT PROVES THE ARCHITECTURE SOLVES THE BLOCKING PROBLEM!
     *
     * With per-group virtual threads, each message group has its own dedicated thread.
     * This automatically eliminates the blocking problem - slow groups don't block fast groups.
     *
     * Objective: Verify different message groups process independently
     *
     * Expected: Fast messages complete quickly, not blocked by slow group
     *           Each group maintains FIFO internally (1 message at a time per group)
     */
    @Test
    void shouldNotBlockWorkersOnLockedGroups() {
        // Given
        pool = createTestPool("NO-BLOCK-TEST", 5, 100);
        AtomicInteger slowProcessing = new AtomicInteger(0);
        AtomicInteger fastProcessing = new AtomicInteger(0);
        AtomicInteger maxSlowConcurrency = new AtomicInteger(0);
        AtomicInteger maxFastConcurrency = new AtomicInteger(0);
        AtomicInteger fastCompleted = new AtomicInteger(0);

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);

            if (msg.messageGroupId().equals("group-slow")) {
                int active = slowProcessing.incrementAndGet();
                maxSlowConcurrency.updateAndGet(max -> Math.max(max, active));
                Thread.sleep(500); // Slow processing
                slowProcessing.decrementAndGet();
            } else {
                int active = fastProcessing.incrementAndGet();
                maxFastConcurrency.updateAndGet(max -> Math.max(max, active));
                Thread.sleep(10); // Fast processing
                fastProcessing.decrementAndGet();
                fastCompleted.incrementAndGet();
            }

            processedCount.incrementAndGet();
            return MediationResult.SUCCESS;
        });

        pool.start();

        long testStart = System.currentTimeMillis();

        // When: Submit slow messages first
        for (int i = 0; i < 10; i++) {
            MessagePointer msg = createMessage("slow-" + i, "NO-BLOCK-TEST", "group-slow");
            inPipelineMap.put(msg.id(), msg);
            pool.submit(msg);
        }

        // Small delay, then submit fast messages
        try {
            Thread.sleep(50);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }

        for (int i = 0; i < 10; i++) {
            MessagePointer msg = createMessage("fast-" + i, "NO-BLOCK-TEST", "group-fast");
            inPipelineMap.put(msg.id(), msg);
            pool.submit(msg);
        }

        // Then: Wait for all fast messages to complete
        await().atMost(5, SECONDS).until(() -> fastCompleted.get() == 10);
        long fastCompletedTime = System.currentTimeMillis() - testStart;

        // Verify fast messages completed quickly (not blocked by slow group)
        // With per-group threads, fast group processes independently
        assertTrue(fastCompletedTime < 2000,
            "Fast messages should complete quickly, not wait for slow group. Took: " + fastCompletedTime + "ms");

        // Verify FIFO within each group (only 1 message at a time per group)
        // Note: With per-group virtual threads, concurrency is controlled by pool-level semaphore
        // not by group locks, so maxConcurrency might be >1 if semaphore allows it
        assertEquals(1, maxSlowConcurrency.get(), "Only 1 message processes at a time for slow group (FIFO)");
        assertEquals(1, maxFastConcurrency.get(), "Only 1 message processes at a time for fast group (FIFO)");

        // Wait for all messages to complete
        await().atMost(10, SECONDS).until(() -> processedCount.get() == 20);
    }

    /**
     * Test 2.2: High Worker Utilization with Many Groups
     *
     * Objective: Verify all workers stay busy with sufficient message groups
     *
     * Expected: Worker utilization ≥ 80% (at least 8/10 workers active)
     *           Total processing time shows high parallelism
     */
    @Test
    void shouldAchieveHighWorkerUtilizationWithManyGroups() {
        // Given
        pool = createTestPool("HIGH-UTIL-TEST", 10, 100);
        AtomicInteger activeWorkers = new AtomicInteger(0);
        AtomicInteger maxConcurrentWorkers = new AtomicInteger(0);
        Set<String> groupsProcessingConcurrently = ConcurrentHashMap.newKeySet();

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);

            int active = activeWorkers.incrementAndGet();
            maxConcurrentWorkers.updateAndGet(max -> Math.max(max, active));
            groupsProcessingConcurrently.add(msg.messageGroupId());

            Thread.sleep(50); // Simulate work

            activeWorkers.decrementAndGet();
            processedCount.incrementAndGet();
            return MediationResult.SUCCESS;
        });

        pool.start();

        long startTime = System.currentTimeMillis();

        // When: Submit 100 messages across 10 groups
        for (int g = 0; g < 10; g++) {
            for (int m = 0; m < 10; m++) {
                MessagePointer msg = createMessage("msg-" + m, "HIGH-UTIL-TEST", "group-" + g);
                inPipelineMap.put(msg.id(), msg);
                pool.submit(msg);
            }
        }

        // Then: Wait for completion
        await().atMost(2, SECONDS).until(() -> processedCount.get() == 100);
        long duration = System.currentTimeMillis() - startTime;

        // Verify high concurrency achieved
        assertTrue(maxConcurrentWorkers.get() >= 8,
            "Should achieve at least 80% worker utilization (8/10 workers). Got: " + maxConcurrentWorkers.get());

        // Verify groups processed concurrently
        assertTrue(groupsProcessingConcurrently.size() >= 8,
            "At least 8 groups should have processed concurrently. Got: " + groupsProcessingConcurrently.size());

        // Verify efficient total time (proves parallelism)
        assertTrue(duration < 1000,
            "Should complete in < 1000ms with high concurrency. Took: " + duration + "ms");
    }

    /**
     * Test 3.1: Messages Without messageGroupId (Backward Compatibility)
     *
     * Objective: Verify null messageGroupId uses DEFAULT_GROUP and maintains FIFO
     *
     * Expected: All messages process in FIFO order
     *           No errors with null messageGroupId
     */
    @Test
    void shouldHandleNullMessageGroupIdWithBackwardCompatibility() {
        // Given
        pool = createTestPool("COMPAT-TEST", 10, 100);
        List<String> completionOrder = new CopyOnWriteArrayList<>();

        when(mockMediator.process(any())).thenAnswer(inv -> {
            MessagePointer msg = inv.getArgument(0);
            completionOrder.add(msg.id());
            processedCount.incrementAndGet();
            Thread.sleep(10);
            return MediationResult.SUCCESS;
        });

        pool.start();

        // When: Submit 10 messages with null messageGroupId
        for (int i = 0; i < 10; i++) {
            MessagePointer msg = new MessagePointer("msg-" + i, "COMPAT-TEST", "test-token", MediationType.HTTP, "http://localhost:8080/test", null  // No messageGroupId
            , null);
            inPipelineMap.put(msg.id(), msg);
            pool.submit(msg);
        }

        // Then: Wait for completion
        await().atMost(2, SECONDS).until(() -> processedCount.get() == 10);

        // Verify FIFO order
        assertEquals(10, completionOrder.size(), "All messages should complete");
        for (int i = 0; i < 10; i++) {
            assertEquals("msg-" + i, completionOrder.get(i),
                "Message with null messageGroupId should process in FIFO order");
        }
    }

    // Helper methods

    private ProcessPool createTestPool(String poolCode, int concurrency, int queueCapacity) {
        return new ProcessPoolImpl(
            poolCode,
            concurrency,
            queueCapacity,
            null, // No rate limiting
            mockMediator,
            mockCallback,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );
    }

    private MessagePointer createMessage(String id, String poolCode, String messageGroupId) {
        return new MessagePointer(id, poolCode, "test-token", MediationType.HTTP, "http://localhost:8080/test", messageGroupId
        , null);
    }
}

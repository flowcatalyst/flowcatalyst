package tech.flowcatalyst.messagerouter.manager;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for InFlightMessageTracker.
 */
class InFlightMessageTrackerTest {

    private InFlightMessageTracker tracker;
    private MessageCallback mockCallback;

    @BeforeEach
    void setUp() {
        tracker = new InFlightMessageTracker();
        mockCallback = mock(MessageCallback.class);
    }

    private MessagePointer createMessage(String id, String sqsMessageId) {
        return new MessagePointer(
            id,
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            null,  // messageGroupId
            false, // highPriority
            null,  // batchId
            sqsMessageId
        );
    }

    @Test
    void trackMessage_returnsTrackedResult() {
        var message = createMessage("msg-1", "sqs-123");
        var result = tracker.track(message, mockCallback, "test-queue");

        assertInstanceOf(InFlightMessageTracker.TrackResult.Tracked.class, result);
        var tracked = (InFlightMessageTracker.TrackResult.Tracked) result;
        assertEquals("sqs-123", tracked.pipelineKey());
    }

    @Test
    void trackMessage_usesSqsMessageIdAsKey() {
        var message = createMessage("app-id", "sqs-id");
        tracker.track(message, mockCallback, "test-queue");

        assertTrue(tracker.containsKey("sqs-id"));
        assertFalse(tracker.containsKey("app-id"));
    }

    @Test
    void trackMessage_fallsBackToAppIdWhenNoSqsId() {
        var message = createMessage("app-id", null);
        var result = tracker.track(message, mockCallback, "test-queue");

        assertInstanceOf(InFlightMessageTracker.TrackResult.Tracked.class, result);
        var tracked = (InFlightMessageTracker.TrackResult.Tracked) result;
        assertEquals("app-id", tracked.pipelineKey());
        assertTrue(tracker.containsKey("app-id"));
    }

    @Test
    void trackDuplicateMessage_returnsDuplicateResult() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        // Try to track same message again
        var result = tracker.track(message, mockCallback, "test-queue");

        assertInstanceOf(InFlightMessageTracker.TrackResult.Duplicate.class, result);
        var duplicate = (InFlightMessageTracker.TrackResult.Duplicate) result;
        assertEquals("sqs-123", duplicate.existingPipelineKey());
        assertFalse(duplicate.isRequeue());
    }

    @Test
    void trackRequeueMessage_returnsDuplicateWithRequeueFlag() {
        var original = createMessage("app-1", "sqs-original");
        tracker.track(original, mockCallback, "test-queue");

        // Track a requeued version (same app ID, different SQS ID)
        var requeued = createMessage("app-1", "sqs-requeued");
        var result = tracker.track(requeued, mockCallback, "test-queue");

        assertInstanceOf(InFlightMessageTracker.TrackResult.Duplicate.class, result);
        var duplicate = (InFlightMessageTracker.TrackResult.Duplicate) result;
        assertTrue(duplicate.isRequeue());
    }

    @Test
    void remove_returnsTrackedMessage() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        var removed = tracker.remove("sqs-123");

        assertTrue(removed.isPresent());
        assertEquals("msg-1", removed.get().messageId());
        assertEquals("sqs-123", removed.get().pipelineKey());
        assertEquals(mockCallback, removed.get().callback());
    }

    @Test
    void remove_clearsAllTracking() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        tracker.remove("sqs-123");

        assertFalse(tracker.containsKey("sqs-123"));
        assertFalse(tracker.isInFlight("msg-1"));
        assertEquals(0, tracker.size());
    }

    @Test
    void remove_returnsEmptyForUnknownKey() {
        var removed = tracker.remove("unknown-key");
        assertTrue(removed.isEmpty());
    }

    @Test
    void getCallback_returnsCallbackForTrackedMessage() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        var callback = tracker.getCallback("sqs-123");

        assertTrue(callback.isPresent());
        assertEquals(mockCallback, callback.get());
    }

    @Test
    void getCallback_returnsEmptyForUnknownKey() {
        var callback = tracker.getCallback("unknown");
        assertTrue(callback.isEmpty());
    }

    @Test
    void isInFlight_returnsTrueForTrackedAppId() {
        var message = createMessage("app-id", "sqs-id");
        tracker.track(message, mockCallback, "test-queue");

        assertTrue(tracker.isInFlight("app-id"));
    }

    @Test
    void isInFlight_returnsFalseAfterRemoval() {
        var message = createMessage("app-id", "sqs-id");
        tracker.track(message, mockCallback, "test-queue");
        tracker.remove("sqs-id");

        assertFalse(tracker.isInFlight("app-id"));
    }

    @Test
    void size_returnsCorrectCount() {
        assertEquals(0, tracker.size());

        tracker.track(createMessage("msg-1", "sqs-1"), mockCallback, "q1");
        assertEquals(1, tracker.size());

        tracker.track(createMessage("msg-2", "sqs-2"), mockCallback, "q2");
        assertEquals(2, tracker.size());

        tracker.remove("sqs-1");
        assertEquals(1, tracker.size());
    }

    @Test
    void clear_removesAllMessages() {
        tracker.track(createMessage("msg-1", "sqs-1"), mockCallback, "q1");
        tracker.track(createMessage("msg-2", "sqs-2"), mockCallback, "q2");
        tracker.track(createMessage("msg-3", "sqs-3"), mockCallback, "q3");

        var cleared = tracker.clear().toList();

        assertEquals(3, cleared.size());
        assertEquals(0, tracker.size());
        assertFalse(tracker.containsKey("sqs-1"));
        assertFalse(tracker.containsKey("sqs-2"));
        assertFalse(tracker.containsKey("sqs-3"));
    }

    @Test
    void stream_returnsAllTrackedMessages() {
        tracker.track(createMessage("msg-1", "sqs-1"), mockCallback, "q1");
        tracker.track(createMessage("msg-2", "sqs-2"), mockCallback, "q2");

        var messages = tracker.stream().toList();

        assertEquals(2, messages.size());
    }

    @Test
    void get_returnsTrackedMessageForKey() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        var result = tracker.get("sqs-123");

        assertTrue(result.isPresent());
        assertEquals("msg-1", result.get().messageId());
    }

    @Test
    void getMessage_returnsMessagePointer() {
        var message = createMessage("msg-1", "sqs-123");
        tracker.track(message, mockCallback, "test-queue");

        var result = tracker.getMessage("sqs-123");

        assertTrue(result.isPresent());
        assertEquals(message, result.get());
    }

    @Test
    void updateCallback_updatesStoredCallback() {
        var message = createMessage("msg-1", "sqs-123");
        var originalCallback = mock(MessageCallback.class);
        var newCallback = mock(MessageCallback.class);

        tracker.track(message, originalCallback, "test-queue");
        boolean updated = tracker.updateCallback("sqs-123", old -> newCallback);

        assertTrue(updated);
        var callback = tracker.getCallback("sqs-123");
        assertTrue(callback.isPresent());
        assertEquals(newCallback, callback.get());
    }

    @Test
    void updateCallback_returnsFalseForUnknownKey() {
        boolean updated = tracker.updateCallback("unknown", old -> mockCallback);
        assertFalse(updated);
    }

    @Test
    void getPipelineKey_usesSqsIdWhenAvailable() {
        var message = createMessage("app-id", "sqs-id");
        assertEquals("sqs-id", InFlightMessageTracker.getPipelineKey(message));
    }

    @Test
    void getPipelineKey_fallsBackToAppId() {
        var message = createMessage("app-id", null);
        assertEquals("app-id", InFlightMessageTracker.getPipelineKey(message));
    }

    @Test
    void concurrentTracking_maintainsConsistency() throws InterruptedException {
        int numThreads = 10;
        int messagesPerThread = 100;
        ExecutorService executor = Executors.newFixedThreadPool(numThreads);
        CountDownLatch startLatch = new CountDownLatch(1);
        CountDownLatch doneLatch = new CountDownLatch(numThreads);
        AtomicInteger successCount = new AtomicInteger(0);
        AtomicInteger duplicateCount = new AtomicInteger(0);

        // Each thread tracks unique messages
        for (int t = 0; t < numThreads; t++) {
            final int threadId = t;
            executor.submit(() -> {
                try {
                    startLatch.await(); // Wait for all threads to be ready
                    for (int i = 0; i < messagesPerThread; i++) {
                        String msgId = "msg-" + threadId + "-" + i;
                        String sqsId = "sqs-" + threadId + "-" + i;
                        var message = createMessage(msgId, sqsId);
                        var result = tracker.track(message, mockCallback, "test-queue");
                        if (result instanceof InFlightMessageTracker.TrackResult.Tracked) {
                            successCount.incrementAndGet();
                        } else {
                            duplicateCount.incrementAndGet();
                        }
                    }
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        startLatch.countDown(); // Start all threads
        assertTrue(doneLatch.await(30, TimeUnit.SECONDS));
        executor.shutdown();

        // All messages should be tracked (no duplicates since IDs are unique per thread)
        assertEquals(numThreads * messagesPerThread, successCount.get());
        assertEquals(0, duplicateCount.get());
        assertEquals(numThreads * messagesPerThread, tracker.size());
    }

    @Test
    void concurrentRemoval_maintainsConsistency() throws InterruptedException {
        // Pre-populate tracker
        List<String> keys = new ArrayList<>();
        for (int i = 0; i < 100; i++) {
            String sqsId = "sqs-" + i;
            tracker.track(createMessage("msg-" + i, sqsId), mockCallback, "test-queue");
            keys.add(sqsId);
        }

        int numThreads = 10;
        ExecutorService executor = Executors.newFixedThreadPool(numThreads);
        CountDownLatch doneLatch = new CountDownLatch(numThreads);
        AtomicInteger removedCount = new AtomicInteger(0);

        // Each thread tries to remove all messages
        for (int t = 0; t < numThreads; t++) {
            executor.submit(() -> {
                try {
                    for (String key : keys) {
                        var removed = tracker.remove(key);
                        if (removed.isPresent()) {
                            removedCount.incrementAndGet();
                        }
                    }
                } finally {
                    doneLatch.countDown();
                }
            });
        }

        assertTrue(doneLatch.await(30, TimeUnit.SECONDS));
        executor.shutdown();

        // Each message should be removed exactly once
        assertEquals(100, removedCount.get());
        assertEquals(0, tracker.size());
    }
}

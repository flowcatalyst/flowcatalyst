package tech.flowcatalyst.messagerouter.pool;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.ArgumentCaptor;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.mediator.MediationError;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Duration;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for OutcomeHandler.
 */
class OutcomeHandlerTest {

    private OutcomeHandler handler;
    private MessageCallback mockCallback;
    private PoolMetricsService mockMetrics;
    private WarningService mockWarningService;
    private ConcurrentHashMap<String, Boolean> failedBatchGroups;
    private ConcurrentHashMap<String, AtomicInteger> batchGroupMessageCount;

    @BeforeEach
    void setUp() {
        mockCallback = mock(MessageCallback.class);
        mockMetrics = mock(PoolMetricsService.class);
        mockWarningService = mock(WarningService.class);
        failedBatchGroups = new ConcurrentHashMap<>();
        batchGroupMessageCount = new ConcurrentHashMap<>();

        handler = new OutcomeHandler(
            "TEST-POOL",
            mockCallback,
            mockMetrics,
            mockWarningService,
            failedBatchGroups,
            batchGroupMessageCount
        );
    }

    private MessagePointer createMessage(String id) {
        return createMessage(id, null, null);
    }

    private MessagePointer createMessage(String id, String messageGroupId, String batchId) {
        return new MessagePointer(
            id,
            "TEST-POOL",
            "test-token",
            MediationType.HTTP,
            "http://localhost:8080/test",
            messageGroupId,
            batchId
        );
    }

    @Test
    void handleSuccess_acksMessage() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.success();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockCallback).ack(message);
        verify(mockCallback, never()).nack(any());
    }

    @Test
    void handleSuccess_recordsSuccessMetric() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.success();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockMetrics).recordProcessingSuccess("TEST-POOL", 100L);
    }

    @Test
    void handleConfigError_acksMessage() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorConfig();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockCallback).ack(message);
        verify(mockCallback, never()).nack(any());
    }

    @Test
    void handleConfigError_recordsFailureMetric() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorConfig();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockMetrics).recordProcessingFailure("TEST-POOL", 100L, "ERROR_CONFIG");
    }

    @Test
    void handleProcessError_nacksMessage() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorProcess((Integer) null);

        handler.handleOutcome(message, outcome, 100L);

        verify(mockCallback).nack(message);
        verify(mockCallback, never()).ack(any());
    }

    @Test
    void handleProcessError_recordsTransientMetric() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorProcess((Integer) null);

        handler.handleOutcome(message, outcome, 100L);

        verify(mockMetrics).recordProcessingTransient("TEST-POOL", 100L);
    }

    @Test
    void handleConnectionError_nacksMessage() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorConnection();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockCallback).nack(message);
        verify(mockCallback, never()).ack(any());
    }

    @Test
    void handleConnectionError_recordsFailureMetric() {
        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorConnection();

        handler.handleOutcome(message, outcome, 100L);

        verify(mockMetrics).recordProcessingFailure("TEST-POOL", 100L, "ERROR_CONNECTION");
    }

    @Test
    void handleNullOutcome_treatsAsTransientError() {
        var message = createMessage("msg-1");

        handler.handleOutcome(message, null, 100L);

        verify(mockCallback).nack(message);
        verify(mockWarningService).addWarning(
            eq("MEDIATOR_NULL_RESULT"),
            eq("CRITICAL"),
            contains("null outcome"),
            any()
        );
    }

    @Test
    void handleFailure_marksBatchGroupFailed() {
        var message = createMessage("msg-1", "group-1", "batch-1");
        var outcome = MediationOutcome.errorProcess((Integer) null);

        handler.handleOutcome(message, outcome, 100L);

        String batchGroupKey = OutcomeHandler.getBatchGroupKey(message);
        assertTrue(failedBatchGroups.containsKey(batchGroupKey));
    }

    @Test
    void shouldAutoNack_returnsTrueForFailedBatchGroup() {
        var message = createMessage("msg-1", "group-1", "batch-1");
        String batchGroupKey = OutcomeHandler.getBatchGroupKey(message);
        failedBatchGroups.put(batchGroupKey, Boolean.TRUE);

        assertTrue(handler.shouldAutoNack(message));
    }

    @Test
    void shouldAutoNack_returnsFalseForNonFailedBatchGroup() {
        var message = createMessage("msg-1", "group-1", "batch-1");

        assertFalse(handler.shouldAutoNack(message));
    }

    @Test
    void shouldAutoNack_returnsFalseWithNoBatchId() {
        var message = createMessage("msg-1", "group-1", null);

        assertFalse(handler.shouldAutoNack(message));
    }

    @Test
    void handleAutoNack_nacksMessageWithoutProcessing() {
        var message = createMessage("msg-1", "group-1", "batch-1");

        handler.handleAutoNack(message);

        verify(mockCallback).nack(message);
        verify(mockMetrics).recordProcessingFailure("TEST-POOL", 0, "BATCH_GROUP_FAILED");
    }

    @Test
    void trackBatchGroupMessage_incrementsCount() {
        var message = createMessage("msg-1", "group-1", "batch-1");
        String batchGroupKey = OutcomeHandler.getBatchGroupKey(message);

        handler.trackBatchGroupMessage(message);

        assertTrue(batchGroupMessageCount.containsKey(batchGroupKey));
        assertEquals(1, batchGroupMessageCount.get(batchGroupKey).get());
    }

    @Test
    void decrementAndCleanupBatchGroup_decrementsCount() {
        var message = createMessage("msg-1", "group-1", "batch-1");
        String batchGroupKey = OutcomeHandler.getBatchGroupKey(message);
        batchGroupMessageCount.put(batchGroupKey, new AtomicInteger(2));

        handler.decrementAndCleanupBatchGroup(batchGroupKey);

        assertEquals(1, batchGroupMessageCount.get(batchGroupKey).get());
    }

    @Test
    void decrementAndCleanupBatchGroup_cleansUpWhenZero() {
        var message = createMessage("msg-1", "group-1", "batch-1");
        String batchGroupKey = OutcomeHandler.getBatchGroupKey(message);
        batchGroupMessageCount.put(batchGroupKey, new AtomicInteger(1));
        failedBatchGroups.put(batchGroupKey, Boolean.TRUE);

        handler.decrementAndCleanupBatchGroup(batchGroupKey);

        assertFalse(batchGroupMessageCount.containsKey(batchGroupKey));
        assertFalse(failedBatchGroups.containsKey(batchGroupKey));
    }

    @Test
    void getBatchGroupKey_returnsCombinedKey() {
        var message = createMessage("msg-1", "my-group", "my-batch");
        String key = OutcomeHandler.getBatchGroupKey(message);
        assertEquals("my-batch|my-group", key);
    }

    @Test
    void getBatchGroupKey_usesDefaultGroupWhenNull() {
        var message = createMessage("msg-1", null, "my-batch");
        String key = OutcomeHandler.getBatchGroupKey(message);
        assertEquals("my-batch|__DEFAULT__", key);
    }

    @Test
    void getBatchGroupKey_returnsNullWhenNoBatchId() {
        var message = createMessage("msg-1", "my-group", null);
        String key = OutcomeHandler.getBatchGroupKey(message);
        assertNull(key);
    }

    @Test
    void getError_returnsErrorFromOutcome() {
        var error = new MediationError.Timeout(Duration.ofSeconds(30));
        var outcome = MediationOutcome.errorProcess(error);

        var result = OutcomeHandler.getError(outcome);

        assertTrue(result.isPresent());
        assertEquals(error, result.get());
    }

    @Test
    void getError_returnsEmptyForSuccessOutcome() {
        var outcome = MediationOutcome.success();

        var result = OutcomeHandler.getError(outcome);

        assertTrue(result.isEmpty());
    }

    // Test with visibility control callback
    @Test
    void handleProcessErrorWithDelay_setsVisibilityDelay() {
        var visibilityCallback = mock(MessageVisibilityCallback.class);
        handler = new OutcomeHandler(
            "TEST-POOL",
            visibilityCallback,
            mockMetrics,
            mockWarningService,
            failedBatchGroups,
            batchGroupMessageCount
        );

        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorProcess(60); // 60 second delay

        handler.handleOutcome(message, outcome, 100L);

        verify(visibilityCallback).setVisibilityDelay(message, 60);
        verify(visibilityCallback).nack(message);
    }

    @Test
    void handleConnectionError_resetsVisibilityToDefault() {
        var visibilityCallback = mock(MessageVisibilityCallback.class);
        handler = new OutcomeHandler(
            "TEST-POOL",
            visibilityCallback,
            mockMetrics,
            mockWarningService,
            failedBatchGroups,
            batchGroupMessageCount
        );

        var message = createMessage("msg-1");
        var outcome = MediationOutcome.errorConnection();

        handler.handleOutcome(message, outcome, 100L);

        verify(visibilityCallback).resetVisibilityToDefault(message);
        verify(visibilityCallback).nack(message);
    }

    @Test
    void handleAutoNack_setsFastFailVisibility() {
        var visibilityCallback = mock(MessageVisibilityCallback.class);
        handler = new OutcomeHandler(
            "TEST-POOL",
            visibilityCallback,
            mockMetrics,
            mockWarningService,
            failedBatchGroups,
            batchGroupMessageCount
        );

        var message = createMessage("msg-1", "group-1", "batch-1");

        handler.handleAutoNack(message);

        verify(visibilityCallback).setFastFailVisibility(message);
        verify(visibilityCallback).nack(message);
    }

    // Combined interface for testing visibility control
    interface MessageVisibilityCallback extends MessageCallback, MessageVisibilityControl {}
}

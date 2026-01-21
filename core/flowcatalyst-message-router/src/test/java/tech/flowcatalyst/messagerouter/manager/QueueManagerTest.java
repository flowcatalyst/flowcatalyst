package tech.flowcatalyst.messagerouter.manager;

import io.micrometer.core.instrument.MeterRegistry;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.client.MessageRouterConfigClient;
import tech.flowcatalyst.messagerouter.factory.MediatorFactory;
import tech.flowcatalyst.messagerouter.factory.QueueConsumerFactory;
import tech.flowcatalyst.messagerouter.health.QueueValidationService;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.lang.reflect.Field;
import java.util.concurrent.ConcurrentHashMap;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Pure unit tests for QueueManager - no Quarkus context, no reflection.
 * Uses package-private constructor for dependency injection.
 */
class QueueManagerTest {

    private QueueManager queueManager;
    private ConcurrentHashMap<String, ProcessPool> processPools;
    private ConcurrentHashMap<String, MessagePointer> inPipelineMap;
    private InFlightMessageTracker inFlightTracker;

    private Mediator mockMediator;
    private MediatorFactory mockMediatorFactory;
    private PoolMetricsService mockPoolMetrics;
    private QueueMetricsService mockQueueMetrics;
    private WarningService mockWarningService;
    private MeterRegistry mockMeterRegistry;
    private MessageRouterConfigClient mockConfigClient;
    private QueueConsumerFactory mockQueueConsumerFactory;
    private QueueValidationService mockQueueValidationService;

    @BeforeEach
    void setUp() throws Exception {
        // Create mocks
        mockMediator = mock(Mediator.class);
        mockMediatorFactory = mock(MediatorFactory.class);
        mockPoolMetrics = mock(PoolMetricsService.class);
        mockQueueMetrics = mock(QueueMetricsService.class);
        mockWarningService = mock(WarningService.class);
        mockMeterRegistry = mock(MeterRegistry.class);
        mockConfigClient = mock(MessageRouterConfigClient.class);
        mockQueueConsumerFactory = mock(QueueConsumerFactory.class);
        mockQueueValidationService = mock(QueueValidationService.class);

        // Configure mediator factory to return mock mediator
        when(mockMediatorFactory.createMediator(any())).thenReturn(mockMediator);

        // Create QueueManager using test constructor (no reflection needed!)
        queueManager = new QueueManager(
            mockConfigClient,
            mockQueueConsumerFactory,
            mockMediatorFactory,
            mockQueueValidationService,
            mockPoolMetrics,
            mockQueueMetrics,
            mockWarningService,
            mockMeterRegistry,
            true,  // messageRouterEnabled
            2000,  // maxPools
            1000   // poolWarningThreshold
        );

        // Access internal fields (still need reflection for these, but only for verification)
        processPools = getPrivateField(queueManager, "processPools");
        inPipelineMap = getPrivateField(queueManager, "inPipelineMap");
        inFlightTracker = getPrivateField(queueManager, "inFlightTracker");

        // Clear any existing state
        processPools.clear();
        inPipelineMap.clear();
        inFlightTracker.clear();
    }

    @AfterEach
    void tearDown() {
        // Clean up pools
        for (ProcessPool pool : processPools.values()) {
            try {
                pool.drain();
            } catch (Exception e) {
                // Ignore cleanup errors
            }
        }

        processPools.clear();
        inPipelineMap.clear();
        inFlightTracker.clear();

        // Reset mocks
        reset(mockMediator, mockMediatorFactory, mockPoolMetrics, mockWarningService);
    }

    @Test
    void shouldRouteMessageToCorrectPool() {
        // Given
        ProcessPool pool = createAndRegisterPool("TEST-POOL", 5, 100);

        MessagePointer message = new MessagePointer("msg-1", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessageCallback mockCallback = mock(MessageCallback.class);
        when(mockMediator.process(message)).thenReturn(MediationOutcome.success());

        // When
        boolean routed = queueManager.routeMessage(message, mockCallback, "test-queue");

        // Then
        assertTrue(routed, "Message should be routed successfully");
        assertTrue(inPipelineMap.containsKey(message.id()), "Message should be in pipeline map");
        assertTrue(inFlightTracker.isInFlight(message.id()), "Callback should be registered");

        // Wait for processing - use any() matcher since batchId gets added
        await().untilAsserted(() -> {
            verify(mockMediator).process(any(MessagePointer.class));
        });
    }

    @Test
    void shouldDetectDuplicateMessages() throws InterruptedException {
        // Given
        ProcessPool pool = createAndRegisterPool("TEST-POOL", 5, 100);

        MessagePointer message = new MessagePointer("msg-duplicate", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessageCallback mockCallback1 = mock(MessageCallback.class);
        MessageCallback mockCallback2 = mock(MessageCallback.class);

        // Block mediator to prevent message from being processed before assertions
        java.util.concurrent.CountDownLatch processLatch = new java.util.concurrent.CountDownLatch(1);
        when(mockMediator.process(any(MessagePointer.class))).thenAnswer(invocation -> {
            processLatch.await(); // Block until test releases
            return MediationOutcome.success();
        });

        // When
        boolean firstRoute = queueManager.routeMessage(message, mockCallback1, "test-queue");
        boolean secondRoute = queueManager.routeMessage(message, mockCallback2, "test-queue");

        // Then
        assertTrue(firstRoute, "First message should be routed");
        assertFalse(secondRoute, "Duplicate message should be rejected");
        assertEquals(1, inPipelineMap.size(), "Only one message should be in pipeline");

        // Release the blocked message processing
        processLatch.countDown();
    }

    @Test
    void shouldHandleQueueFull() {
        // Given - create pool with minimal capacity
        // Concurrency=1 (1 worker), queueCapacity=2 (2 slots in buffer)
        // Total capacity = 1 processing + 2 queued = 3 messages
        ProcessPool smallPool = createAndRegisterPool("SMALL-POOL", 1, 2);

        // Block the mediator to fill the queue
        when(mockMediator.process(any())).thenAnswer(invocation -> {
            Thread.sleep(300);
            return MediationOutcome.success();
        });

        MessagePointer message1 = new MessagePointer("msg-1", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com", null
            , null);
        MessagePointer message2 = new MessagePointer("msg-2", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com", null
            , null);
        MessagePointer message3 = new MessagePointer("msg-3", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com", null
            , null);
        MessagePointer message4 = new MessagePointer("msg-4", "SMALL-POOL", "token", MediationType.HTTP, "http://test.com", null
            , null);

        MessageCallback mockCallback = mock(MessageCallback.class);

        // When - submit 3 messages to fill the pool (1 processing + 2 queued)
        boolean routed1 = queueManager.routeMessage(message1, mockCallback, "test-queue");
        boolean routed2 = queueManager.routeMessage(message2, mockCallback, "test-queue");
        boolean routed3 = queueManager.routeMessage(message3, mockCallback, "test-queue");

        // Give time for processing to start and queue to fill
        try { Thread.sleep(100); } catch (InterruptedException e) {}

        // Now submit 4th message - should be rejected
        boolean routed4 = queueManager.routeMessage(message4, mockCallback, "test-queue");

        // Then
        assertTrue(routed1, "First message should be routed");
        assertTrue(routed2, "Second message should be routed");
        assertTrue(routed3, "Third message should be routed");
        assertFalse(routed4, "Fourth message should be rejected - queue full");

        assertFalse(inPipelineMap.containsKey(message4.id()), "Rejected message should not be in pipeline");
        assertFalse(inFlightTracker.isInFlight(message4.id()), "Rejected message callback should not be registered");

        // Verify warning was added
        verify(mockWarningService, atLeastOnce()).addWarning(
            eq("QUEUE_FULL"),
            eq("WARN"),
            contains("SMALL-POOL"),
            eq("QueueManager")
        );
    }

    @Test
    void shouldDelegateAckToCallback() {
        // Given
        ProcessPool pool = createAndRegisterPool("TEST-POOL", 5, 100);

        MessagePointer message = new MessagePointer("msg-ack", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessageCallback mockCallback = mock(MessageCallback.class);
        when(mockMediator.process(any(MessagePointer.class))).thenReturn(MediationOutcome.success());

        // Route the message
        queueManager.routeMessage(message, mockCallback, "test-queue");

        // Wait for processing to complete - use any() matcher since batchId gets added
        await().untilAsserted(() -> {
            verify(mockMediator).process(any(MessagePointer.class));
        });

        // When
        queueManager.ack(message);

        // Then - use any() matcher since message may have batchId added
        verify(mockCallback).ack(any(MessagePointer.class));
        assertFalse(inFlightTracker.isInFlight(message.id()), "Callback should be removed after ack");
    }

    @Test
    void shouldDelegateNackToCallback() {
        // Given
        ProcessPool pool = createAndRegisterPool("TEST-POOL", 5, 100);

        MessagePointer message = new MessagePointer("msg-nack", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        MessageCallback mockCallback = mock(MessageCallback.class);
        when(mockMediator.process(any(MessagePointer.class))).thenReturn(tech.flowcatalyst.messagerouter.model.MediationOutcome.errorProcess((Integer) null));

        // Route the message
        queueManager.routeMessage(message, mockCallback, "test-queue");

        // Wait for processing to complete - use any() matcher since batchId gets added
        await().untilAsserted(() -> {
            verify(mockMediator).process(any(MessagePointer.class));
        });

        // When
        queueManager.nack(message);

        // Then - use any() matcher since message may have batchId added
        verify(mockCallback).nack(any(MessagePointer.class));
        assertFalse(inFlightTracker.isInFlight(message.id()), "Callback should be removed after nack");
    }

    @Test
    void shouldHandleAckWithoutCallback() {
        // Given
        MessagePointer message = new MessagePointer("msg-orphan", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        // When - ack without registering callback first
        assertDoesNotThrow(() -> queueManager.ack(message));

        // Then - should not throw exception
    }

    @Test
    void shouldHandleNackWithoutCallback() {
        // Given
        MessagePointer message = new MessagePointer("msg-orphan-nack", "TEST-POOL", "test-token", MediationType.HTTP, "http://localhost:8080/test"
        , null
            , null);

        // When - nack without registering callback first
        assertDoesNotThrow(() -> queueManager.nack(message));

        // Then - should not throw exception
    }

    @Test
    void shouldRouteMultipleMessagesToDifferentPools() {
        // Given
        ProcessPool pool1 = createAndRegisterPool("POOL-1", 5, 100);
        ProcessPool pool2 = createAndRegisterPool("POOL-2", 5, 100);

        MessagePointer message1 = new MessagePointer("msg-1", "POOL-1", "token", MediationType.HTTP, "http://test.com", null
            , null);
        MessagePointer message2 = new MessagePointer("msg-2", "POOL-2", "token", MediationType.HTTP, "http://test.com", null
            , null);

        MessageCallback mockCallback1 = mock(MessageCallback.class);
        MessageCallback mockCallback2 = mock(MessageCallback.class);

        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());

        // When
        boolean routed1 = queueManager.routeMessage(message1, mockCallback1, "test-queue");
        boolean routed2 = queueManager.routeMessage(message2, mockCallback2, "test-queue");

        // Then
        assertTrue(routed1, "Message 1 should be routed to POOL-1");
        assertTrue(routed2, "Message 2 should be routed to POOL-2");
        assertEquals(2, inPipelineMap.size(), "Both messages should be in pipeline");

        await().untilAsserted(() -> {
            verify(mockMediator, times(2)).process(any(MessagePointer.class));
        });
    }

    /**
     * Helper method to create and register a pool for testing
     */
    private ProcessPool createAndRegisterPool(String poolCode, int concurrency, int queueCapacity) {
        ProcessPoolImpl pool = new ProcessPoolImpl(
            poolCode,
            concurrency,
            queueCapacity,
            null, // rateLimitPerMinute
            mockMediator,
            queueManager,
            inPipelineMap,
            mockPoolMetrics,
            mockWarningService
        );

        pool.start();
        processPools.put(poolCode, pool);

        return pool;
    }

    /**
     * Helper method to access private fields via reflection
     * (Only needed for verification, not for setup anymore!)
     */
    @SuppressWarnings("unchecked")
    private <T> T getPrivateField(Object obj, String fieldName) throws Exception {
        Field field = obj.getClass().getDeclaredField(fieldName);
        field.setAccessible(true);
        return (T) field.get(obj);
    }
}

package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentMap;
import java.util.concurrent.atomic.AtomicInteger;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Integration tests for pool-level rate limiting.
 *
 * Rate limiting is now configured at the pool level via ProcessPoolImpl constructor,
 * not per-message via MessagePointer fields.
 */
@QuarkusTest
@Tag("integration")
class RateLimiterIntegrationTest {

    private ProcessPoolImpl processPool;
    private Mediator mockMediator;
    private MessageCallback mockCallback;
    private ConcurrentMap<String, MessagePointer> inPipelineMap;
    private PoolMetricsService mockPoolMetrics;
    private WarningService mockWarningService;

    void createPoolWithRateLimit(Integer rateLimitPerMinute) {
        mockMediator = mock(Mediator.class);
        mockCallback = mock(MessageCallback.class);
        inPipelineMap = new ConcurrentHashMap<>();
        mockPoolMetrics = mock(PoolMetricsService.class);
        mockWarningService = mock(WarningService.class);

        processPool = new ProcessPoolImpl(
            "RATE-LIMIT-POOL",
            10,
            100,
            rateLimitPerMinute, // Pool-level rate limit
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
    void shouldAllowMessagesWithinRateLimit() {
        // Given: Pool with rate limit of 60 per minute
        createPoolWithRateLimit(60);
        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());

        processPool.start();

        // When: Submit 5 messages (well within limit)
        for (int i = 0; i < 5; i++) {
            MessagePointer message = new MessagePointer("msg-" + i, "RATE-LIMIT-POOL", "token", MediationType.HTTP, "http://localhost:8080/test", null
            , null);
            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: All messages should be processed
        await().untilAsserted(() -> {
            verify(mockMediator, times(5)).process(any());
            verify(mockCallback, times(5)).ack(any());
        });
    }

    @Test
    void shouldEnforceRateLimitAndWaitForPermits() {
        // Given: Pool with rate limit of 5 per minute
        createPoolWithRateLimit(5);
        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());
        AtomicInteger ackedCount = new AtomicInteger(0);

        doAnswer(invocation -> {
            ackedCount.incrementAndGet();
            return null;
        }).when(mockCallback).ack(any());

        processPool.start();

        // When: Submit 10 messages quickly (exceeds pool limit of 5/min)
        for (int i = 0; i < 10; i++) {
            MessagePointer message = new MessagePointer("msg-rate-" + i, "RATE-LIMIT-POOL", "token", MediationType.HTTP, "http://localhost:8080/test", null
            , null);
            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: First 5 should be processed immediately, rest wait for permits
        // Implementation now waits for permits instead of NACKing (messages stay in memory)
        await().untilAsserted(() -> {
            assertTrue(ackedCount.get() >= 5, "Should ack at least 5 messages within rate limit");
            // Rate limit exceeded should be recorded for messages that had to wait
            verify(mockPoolMetrics, atLeast(1)).recordRateLimitExceeded("RATE-LIMIT-POOL");
        });
    }

    @Test
    void shouldHandleHighRateLimit() {
        // Given: Pool with high rate limit of 600 per minute
        createPoolWithRateLimit(600);
        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());

        processPool.start();

        // When: Submit 50 messages (well within high limit)
        for (int i = 0; i < 50; i++) {
            MessagePointer message = new MessagePointer("msg-high-" + i, "RATE-LIMIT-POOL", "token", MediationType.HTTP, "http://localhost:8080/test", null
            , null);
            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: All messages should be processed
        await().untilAsserted(() -> {
            verify(mockMediator, times(50)).process(any());
            verify(mockCallback, times(50)).ack(any());
        });
    }

    @Test
    void shouldProcessMessagesWithoutRateLimitImmediately() {
        // Given: Pool with no rate limit (null)
        createPoolWithRateLimit(null);
        when(mockMediator.process(any())).thenReturn(MediationOutcome.success());

        processPool.start();

        // When: Submit 10 messages (no rate limiting)
        for (int i = 0; i < 10; i++) {
            MessagePointer message = new MessagePointer("msg-no-limit-" + i, "RATE-LIMIT-POOL", "token", MediationType.HTTP, "http://localhost:8080/test", null
            , null);
            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: All should be processed immediately
        await().untilAsserted(() -> {
            verify(mockMediator, times(10)).process(any());
            verify(mockCallback, times(10)).ack(any());
        });
    }

    @Test
    void shouldGetRateLimitPerMinuteFromPool() {
        // Given: Pool with rate limit of 100 per minute
        createPoolWithRateLimit(100);

        // Then: getRateLimitPerMinute should return configured value
        assertEquals(100, processPool.getRateLimitPerMinute());
    }

    @Test
    void shouldReturnNullWhenNoRateLimitConfigured() {
        // Given: Pool with no rate limit
        createPoolWithRateLimit(null);

        // Then: getRateLimitPerMinute should return null
        assertNull(processPool.getRateLimitPerMinute());
    }
}

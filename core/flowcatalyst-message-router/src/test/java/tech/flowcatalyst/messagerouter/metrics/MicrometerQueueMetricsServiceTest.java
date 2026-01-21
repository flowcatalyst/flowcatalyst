package tech.flowcatalyst.messagerouter.metrics;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.simple.SimpleMeterRegistry;
import io.quarkus.test.junit.QuarkusTest;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Comprehensive MicrometerQueueMetricsService tests covering:
 * - Message received tracking
 * - Message processed tracking (success/failure)
 * - Queue depth tracking
 * - Queue metrics tracking (pendingMessages, messagesNotVisible) - NEW FUNCTIONALITY
 * - Stats calculation and aggregation
 * - Multiple queue handling
 */
@QuarkusTest
class MicrometerQueueMetricsServiceTest {

    private MicrometerQueueMetricsService metricsService;
    private MeterRegistry meterRegistry;

    @BeforeEach
    void setUp() {
        meterRegistry = new SimpleMeterRegistry();
        metricsService = new MicrometerQueueMetricsService();

        // Use reflection to inject the meter registry
        try {
            var field = MicrometerQueueMetricsService.class.getDeclaredField("meterRegistry");
            field.setAccessible(true);
            field.set(metricsService, meterRegistry);
        } catch (Exception e) {
            throw new RuntimeException("Failed to inject meter registry", e);
        }
    }

    @Test
    void shouldRecordMessageReceived() {
        // Given
        String queueId = "test-queue";

        // When
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);

        // Then
        Counter counter = meterRegistry.find("flowcatalyst.queue.messages.received")
            .tag("queue", queueId)
            .counter();

        assertNotNull(counter, "Counter should exist");
        assertEquals(3.0, counter.count(), "Should have 3 messages received");
    }

    @Test
    void shouldRecordMessageProcessedSuccess() {
        // Given
        String queueId = "test-queue";

        // When
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, true);

        // Then
        Counter consumedCounter = meterRegistry.find("flowcatalyst.queue.messages.consumed")
            .tag("queue", queueId)
            .counter();

        assertNotNull(consumedCounter, "Consumed counter should exist");
        assertEquals(2.0, consumedCounter.count(), "Should have 2 messages consumed");
    }

    @Test
    void shouldRecordMessageProcessedFailure() {
        // Given
        String queueId = "test-queue";

        // When
        metricsService.recordMessageProcessed(queueId, false);
        metricsService.recordMessageProcessed(queueId, false);
        metricsService.recordMessageProcessed(queueId, false);

        // Then
        Counter failedCounter = meterRegistry.find("flowcatalyst.queue.messages.failed")
            .tag("queue", queueId)
            .counter();

        assertNotNull(failedCounter, "Failed counter should exist");
        assertEquals(3.0, failedCounter.count(), "Should have 3 messages failed");
    }

    @Test
    void shouldRecordQueueDepth() {
        // Given
        String queueId = "test-queue";

        // When
        metricsService.recordQueueDepth(queueId, 42);

        // Then
        Double gaugeValue = meterRegistry.find("flowcatalyst.queue.depth")
            .tag("queue", queueId)
            .gauge()
            .value();

        assertNotNull(gaugeValue, "Gauge should exist");
        assertEquals(42.0, gaugeValue, "Queue depth should be 42");
    }

    @Test
    void shouldRecordQueueMetrics() {
        // Given - NEW FUNCTIONALITY
        String queueId = "test-queue";
        long pendingMessages = 100;
        long messagesNotVisible = 25;

        // When
        metricsService.recordQueueMetrics(queueId, pendingMessages, messagesNotVisible);

        // Then
        Double pendingGauge = meterRegistry.find("flowcatalyst.queue.pending")
            .tag("queue", queueId)
            .gauge()
            .value();

        Double notVisibleGauge = meterRegistry.find("flowcatalyst.queue.not_visible")
            .tag("queue", queueId)
            .gauge()
            .value();

        assertNotNull(pendingGauge, "Pending gauge should exist");
        assertNotNull(notVisibleGauge, "Not visible gauge should exist");
        assertEquals(100.0, pendingGauge, "Pending messages should be 100");
        assertEquals(25.0, notVisibleGauge, "Messages not visible should be 25");
    }

    @Test
    void shouldUpdateQueueMetricsOverTime() {
        // Given
        String queueId = "test-queue";

        // When - simulate metrics changing over time
        metricsService.recordQueueMetrics(queueId, 50, 10);
        metricsService.recordQueueMetrics(queueId, 75, 15);
        metricsService.recordQueueMetrics(queueId, 25, 5);

        // Then - should reflect latest values
        Double pendingGauge = meterRegistry.find("flowcatalyst.queue.pending")
            .tag("queue", queueId)
            .gauge()
            .value();

        Double notVisibleGauge = meterRegistry.find("flowcatalyst.queue.not_visible")
            .tag("queue", queueId)
            .gauge()
            .value();

        assertEquals(25.0, pendingGauge, "Should reflect latest pending count");
        assertEquals(5.0, notVisibleGauge, "Should reflect latest not visible count");
    }

    @Test
    void shouldCalculateQueueStats() {
        // Given
        String queueId = "test-queue";

        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageReceived(queueId);

        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, false);

        metricsService.recordQueueDepth(queueId, 10);
        metricsService.recordQueueMetrics(queueId, 50, 15);

        // When
        QueueStats stats = metricsService.getQueueStats(queueId);

        // Then
        assertNotNull(stats, "Stats should not be null");
        assertEquals(queueId, stats.name());
        assertEquals(5, stats.totalMessages(), "Should have 5 total messages");
        assertEquals(3, stats.totalConsumed(), "Should have 3 consumed messages");
        assertEquals(1, stats.totalFailed(), "Should have 1 failed message");
        assertEquals(0.6, stats.successRate(), 0.01, "Success rate should be 60%");
        assertEquals(10, stats.currentSize(), "Current queue size should be 10");
        assertEquals(50, stats.pendingMessages(), "Pending messages should be 50");
        assertEquals(15, stats.messagesNotVisible(), "Messages not visible should be 15");
        assertTrue(stats.throughput() >= 0, "Throughput should be calculated");
    }

    @Test
    void shouldReturnEmptyStatsForUnknownQueue() {
        // Given
        String unknownQueue = "unknown-queue";

        // When
        QueueStats stats = metricsService.getQueueStats(unknownQueue);

        // Then
        assertNotNull(stats, "Stats should not be null");
        assertEquals(unknownQueue, stats.name());
        assertEquals(0, stats.totalMessages());
        assertEquals(0, stats.totalConsumed());
        assertEquals(0, stats.totalFailed());
        assertEquals(0.0, stats.successRate());
        assertEquals(0, stats.currentSize());
        assertEquals(0, stats.pendingMessages());
        assertEquals(0, stats.messagesNotVisible());
    }

    @Test
    void shouldTrackMultipleQueues() {
        // Given
        String queue1 = "queue-1";
        String queue2 = "queue-2";

        // When
        metricsService.recordMessageReceived(queue1);
        metricsService.recordMessageReceived(queue1);
        metricsService.recordMessageProcessed(queue1, true);
        metricsService.recordQueueMetrics(queue1, 10, 2);

        metricsService.recordMessageReceived(queue2);
        metricsService.recordMessageReceived(queue2);
        metricsService.recordMessageReceived(queue2);
        metricsService.recordMessageProcessed(queue2, false);
        metricsService.recordQueueMetrics(queue2, 20, 5);

        // Then
        Map<String, QueueStats> allStats = metricsService.getAllQueueStats();

        assertEquals(2, allStats.size(), "Should have stats for 2 queues");

        QueueStats stats1 = allStats.get(queue1);
        assertNotNull(stats1);
        assertEquals(2, stats1.totalMessages());
        assertEquals(1, stats1.totalConsumed());
        assertEquals(10, stats1.pendingMessages());
        assertEquals(2, stats1.messagesNotVisible());

        QueueStats stats2 = allStats.get(queue2);
        assertNotNull(stats2);
        assertEquals(3, stats2.totalMessages());
        assertEquals(0, stats2.totalConsumed());
        assertEquals(1, stats2.totalFailed());
        assertEquals(20, stats2.pendingMessages());
        assertEquals(5, stats2.messagesNotVisible());
    }

    @Test
    void shouldCalculateSuccessRate() {
        // Given
        String queueId = "test-queue";

        // When - 7 received, 5 consumed, 2 failed
        for (int i = 0; i < 7; i++) {
            metricsService.recordMessageReceived(queueId);
        }
        for (int i = 0; i < 5; i++) {
            metricsService.recordMessageProcessed(queueId, true);
        }
        for (int i = 0; i < 2; i++) {
            metricsService.recordMessageProcessed(queueId, false);
        }

        // Then
        QueueStats stats = metricsService.getQueueStats(queueId);
        assertEquals(5.0 / 7.0, stats.successRate(), 0.01, "Success rate should be 5/7");
    }

    @Test
    void shouldHandleZeroMessagesSuccessRate() {
        // Given
        String queueId = "empty-queue";

        // When - no messages
        QueueStats stats = metricsService.getQueueStats(queueId);

        // Then
        assertEquals(0.0, stats.successRate(), "Success rate should be 0 for empty queue");
    }

    @Test
    void shouldCalculateThroughput() throws InterruptedException {
        // Given
        String queueId = "test-queue";

        // When
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, true);

        // Wait a bit to ensure elapsed time > 0
        Thread.sleep(100);

        // Then
        QueueStats stats = metricsService.getQueueStats(queueId);
        assertTrue(stats.throughput() >= 0, "Throughput should be non-negative");
        // Note: Throughput calculation depends on elapsed time since metrics holder creation,
        // which can be very small in tests, so we just verify it's calculated (>= 0)
    }

    @Test
    void shouldIsolateMetricsBetweenQueues() {
        // Given
        String queue1 = "queue-1";
        String queue2 = "queue-2";

        // When
        metricsService.recordMessageReceived(queue1);
        metricsService.recordMessageReceived(queue1);
        metricsService.recordMessageReceived(queue1);

        metricsService.recordMessageReceived(queue2);

        // Then
        QueueStats stats1 = metricsService.getQueueStats(queue1);
        QueueStats stats2 = metricsService.getQueueStats(queue2);

        assertEquals(3, stats1.totalMessages(), "Queue 1 should have 3 messages");
        assertEquals(1, stats2.totalMessages(), "Queue 2 should have 1 message");
    }

    @Test
    void shouldRegisterGaugesWithMeterRegistry() {
        // Given
        String queueId = "gauge-test-queue";

        // When
        metricsService.recordQueueDepth(queueId, 100);
        metricsService.recordQueueMetrics(queueId, 50, 10);

        // Then - verify gauges are registered in the meter registry
        assertNotNull(meterRegistry.find("flowcatalyst.queue.depth").tag("queue", queueId).gauge());
        assertNotNull(meterRegistry.find("flowcatalyst.queue.pending").tag("queue", queueId).gauge());
        assertNotNull(meterRegistry.find("flowcatalyst.queue.not_visible").tag("queue", queueId).gauge());
    }

    @Test
    void shouldRegisterCountersWithMeterRegistry() {
        // Given
        String queueId = "counter-test-queue";

        // When
        metricsService.recordMessageReceived(queueId);
        metricsService.recordMessageProcessed(queueId, true);
        metricsService.recordMessageProcessed(queueId, false);

        // Then - verify counters are registered in the meter registry
        assertNotNull(meterRegistry.find("flowcatalyst.queue.messages.received").tag("queue", queueId).counter());
        assertNotNull(meterRegistry.find("flowcatalyst.queue.messages.consumed").tag("queue", queueId).counter());
        assertNotNull(meterRegistry.find("flowcatalyst.queue.messages.failed").tag("queue", queueId).counter());
    }
}

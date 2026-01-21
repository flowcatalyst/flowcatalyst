package tech.flowcatalyst.messagerouter.health;

import io.quarkus.test.InjectMock;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.PoolStats;

import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

@QuarkusTest
class InfrastructureHealthServiceTest {

    @InjectMock
    PoolMetricsService poolMetricsService;

    @Inject
    InfrastructureHealthService healthService;

    @BeforeEach
    void setUp() {
        reset(poolMetricsService);
    }

    // Note: Test for disabled message router is covered in integration tests
    // where we can properly configure application properties

    @Test
    void shouldReturnHealthyWhenAllPoolsActive() {
        // Given
        Map<String, PoolStats> poolStats = Map.of(
            "POOL-A", createPoolStats("POOL-A"),
            "POOL-B", createPoolStats("POOL-B")
        );

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("POOL-A"))
            .thenReturn(System.currentTimeMillis() - 10_000); // 10 seconds ago
        when(poolMetricsService.getLastActivityTimestamp("POOL-B"))
            .thenReturn(System.currentTimeMillis() - 5_000); // 5 seconds ago

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertTrue(health.healthy());
        assertEquals("Infrastructure is operational", health.message());
        assertNull(health.issues());
    }

    @Test
    void shouldReturnUnhealthyWhenQueueManagerNotInitialized() {
        // Given - metrics service throws exception (QueueManager not initialized)
        when(poolMetricsService.getAllPoolStats()).thenThrow(new RuntimeException("Not initialized"));

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertFalse(health.healthy());
        assertEquals("Infrastructure issues detected", health.message());
        assertNotNull(health.issues());
        assertTrue(health.issues().contains("QueueManager not initialized"));
    }

    @Test
    void shouldReturnUnhealthyWhenNoProcessPoolsExist() {
        // Given - no pools
        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of());

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertFalse(health.healthy());
        assertEquals("Infrastructure issues detected", health.message());
        assertNotNull(health.issues());
        assertTrue(health.issues().contains("No active process pools"));
    }

    @Test
    void shouldReturnUnhealthyWhenAllPoolsStalled() {
        // Given - all pools stalled (no activity in > 2 minutes)
        long stalledTimestamp = System.currentTimeMillis() - 150_000; // 2.5 minutes ago

        Map<String, PoolStats> poolStats = Map.of(
            "POOL-A", createPoolStats("POOL-A"),
            "POOL-B", createPoolStats("POOL-B")
        );

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("POOL-A")).thenReturn(stalledTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("POOL-B")).thenReturn(stalledTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertFalse(health.healthy());
        assertEquals("Infrastructure issues detected", health.message());
        assertNotNull(health.issues());
        assertTrue(health.issues().stream()
            .anyMatch(issue -> issue.contains("All process pools appear stalled")));
    }

    @Test
    void shouldReturnHealthyWhenSomePoolsStalled() {
        // Given - only one pool stalled, others active
        long stalledTimestamp = System.currentTimeMillis() - 150_000; // 2.5 minutes ago
        long activeTimestamp = System.currentTimeMillis() - 5_000; // 5 seconds ago

        Map<String, PoolStats> poolStats = Map.of(
            "POOL-A", createPoolStats("POOL-A"),
            "POOL-B", createPoolStats("POOL-B"),
            "POOL-C", createPoolStats("POOL-C")
        );

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("POOL-A")).thenReturn(stalledTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("POOL-B")).thenReturn(activeTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("POOL-C")).thenReturn(activeTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then - should be healthy because not ALL pools are stalled
        assertTrue(health.healthy());
        assertEquals("Infrastructure is operational", health.message());
        assertNull(health.issues());
    }

    @Test
    void shouldReturnHealthyWhenPoolsHaveNoActivityYet() {
        // Given - pools exist but have null timestamps (no messages processed yet)
        Map<String, PoolStats> poolStats = Map.of(
            "POOL-A", createPoolStats("POOL-A"),
            "POOL-B", createPoolStats("POOL-B")
        );

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("POOL-A")).thenReturn(null);
        when(poolMetricsService.getLastActivityTimestamp("POOL-B")).thenReturn(null);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then - should be healthy (startup state, waiting for messages)
        assertTrue(health.healthy());
        assertEquals("Infrastructure is operational", health.message());
        assertNull(health.issues());
    }

    @Test
    void shouldReturnHealthyWhenMixOfNullAndRecentActivity() {
        // Given - some pools with null timestamps, others with recent activity
        Map<String, PoolStats> poolStats = Map.of(
            "POOL-A", createPoolStats("POOL-A"),
            "POOL-B", createPoolStats("POOL-B")
        );

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("POOL-A")).thenReturn(null);
        when(poolMetricsService.getLastActivityTimestamp("POOL-B"))
            .thenReturn(System.currentTimeMillis() - 10_000);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertTrue(health.healthy());
        assertEquals("Infrastructure is operational", health.message());
        assertNull(health.issues());
    }

    @Test
    void shouldHandleEmptyPoolStatsMap() {
        // Given
        when(poolMetricsService.getAllPoolStats()).thenReturn(new HashMap<>());

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertFalse(health.healthy());
        assertTrue(health.issues().contains("No active process pools"));
    }

    private PoolStats createPoolStats(String poolCode) {
        return new PoolStats(
            poolCode,
            100L,  // totalProcessed
            90L,   // totalSucceeded
            10L,   // totalFailed
            0L,    // totalRateLimited
            0.9,   // successRate
            3,     // activeWorkers
            2,     // availablePermits
            5,     // maxConcurrency
            10,    // queueSize
            100,   // maxQueueCapacity
            150.0, // averageProcessingTimeMs
            100L,  // totalProcessed5min
            90L,   // totalSucceeded5min
            10L,   // totalFailed5min
            0.9,   // successRate5min
            100L,  // totalProcessed30min
            90L,   // totalSucceeded30min
            10L,   // totalFailed30min
            0.9,   // successRate30min
            0L,    // totalRateLimited5min
            0L     // totalRateLimited30min
        );
    }
}

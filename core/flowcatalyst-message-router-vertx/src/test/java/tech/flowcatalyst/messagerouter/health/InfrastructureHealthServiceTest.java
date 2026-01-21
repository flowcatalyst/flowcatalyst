package tech.flowcatalyst.messagerouter.health;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.mockito.Mock;
import org.mockito.MockitoAnnotations;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.PoolStats;

import java.lang.reflect.Field;
import java.util.HashMap;
import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for InfrastructureHealthService.
 * Uses Mockito mocks directly without Quarkus context for faster execution.
 */
class InfrastructureHealthServiceTest {

    @Mock
    PoolMetricsService poolMetricsService;

    @Mock
    QueueManager queueManager;

    private InfrastructureHealthService healthService;

    @BeforeEach
    void setUp() throws Exception {
        MockitoAnnotations.openMocks(this);

        healthService = new InfrastructureHealthService();

        // Inject mocks using reflection
        setPrivateField(healthService, "poolMetricsService", poolMetricsService);
        setPrivateField(healthService, "queueManager", queueManager);
        setPrivateField(healthService, "messageRouterEnabled", true);

        reset(poolMetricsService);
    }

    private void setPrivateField(Object target, String fieldName, Object value) throws Exception {
        Field field = target.getClass().getDeclaredField(fieldName);
        field.setAccessible(true);
        field.set(target, value);
    }

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

    @Test
    void shouldReturnHealthyWhenMessageRouterDisabled() throws Exception {
        // Given - message router is disabled
        setPrivateField(healthService, "messageRouterEnabled", false);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then
        assertTrue(health.healthy());
        assertEquals("Message router is disabled", health.message());
        assertNull(health.issues());
    }

    @Test
    void shouldReturnHealthyAtExactTimeoutBoundary() {
        // Given - pool at 115 seconds (below 120s threshold with buffer)
        long exactBoundaryTimestamp = System.currentTimeMillis() - 115_000;

        Map<String, PoolStats> poolStats = Map.of("BOUNDARY-POOL", createPoolStats("BOUNDARY-POOL"));

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("BOUNDARY-POOL")).thenReturn(exactBoundaryTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then - should be healthy (115s is under 120s threshold)
        assertTrue(health.healthy());
        assertNull(health.issues());
    }

    @Test
    void shouldReturnUnhealthyJustOverTimeoutBoundary() {
        // Given - pool at 120 seconds + 1ms (just over threshold)
        long justOverTimestamp = System.currentTimeMillis() - 120_001;

        Map<String, PoolStats> poolStats = Map.of("JUST-OVER-POOL", createPoolStats("JUST-OVER-POOL"));

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("JUST-OVER-POOL")).thenReturn(justOverTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = healthService.checkHealth();

        // Then - should be unhealthy
        assertFalse(health.healthy());
        assertTrue(health.issues().stream()
            .anyMatch(issue -> issue.contains("All process pools appear stalled")));
    }

    @Test
    void shouldRecoverWhenPoolBecomesActive() {
        // Given - initially stalled pool
        long stalledTimestamp = System.currentTimeMillis() - 150_000; // 2.5 minutes ago
        Map<String, PoolStats> poolStats = Map.of("RECOVERY-POOL", createPoolStats("RECOVERY-POOL"));

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("RECOVERY-POOL")).thenReturn(stalledTimestamp);

        // When - check health (should be unhealthy)
        InfrastructureHealthService.InfrastructureHealth health1 = healthService.checkHealth();

        // Then - unhealthy
        assertFalse(health1.healthy());

        // Given - pool becomes active (recent timestamp)
        long recentTimestamp = System.currentTimeMillis() - 5_000; // 5 seconds ago
        when(poolMetricsService.getLastActivityTimestamp("RECOVERY-POOL")).thenReturn(recentTimestamp);

        // When - check health again
        InfrastructureHealthService.InfrastructureHealth health2 = healthService.checkHealth();

        // Then - should be healthy now
        assertTrue(health2.healthy());
        assertNull(health2.issues());
    }

    @Test
    void shouldHandlePoolTransitioningFromNullToStale() {
        // Given - pool starts with null timestamp
        Map<String, PoolStats> poolStats = Map.of("TRANSITION-POOL", createPoolStats("TRANSITION-POOL"));

        when(poolMetricsService.getAllPoolStats()).thenReturn(poolStats);
        when(poolMetricsService.getLastActivityTimestamp("TRANSITION-POOL")).thenReturn(null);

        // When - first check
        InfrastructureHealthService.InfrastructureHealth health1 = healthService.checkHealth();

        // Then - healthy
        assertTrue(health1.healthy());

        // Given - pool processes a message long ago
        long oldTimestamp = System.currentTimeMillis() - 200_000; // Over 3 minutes
        when(poolMetricsService.getLastActivityTimestamp("TRANSITION-POOL")).thenReturn(oldTimestamp);

        // When - second check
        InfrastructureHealthService.InfrastructureHealth health2 = healthService.checkHealth();

        // Then - should now be unhealthy
        assertFalse(health2.healthy());
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

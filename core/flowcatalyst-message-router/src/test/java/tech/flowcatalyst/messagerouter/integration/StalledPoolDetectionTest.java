package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.InjectMock;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.messagerouter.health.InfrastructureHealthService;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.PoolStats;

import java.util.Map;

import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Integration test for stalled pool detection in infrastructure health checks
 */
@QuarkusTest
@Tag("integration")
class StalledPoolDetectionTest {

    @Inject
    InfrastructureHealthService infrastructureHealthService;

    @InjectMock
    PoolMetricsService poolMetricsService;

    @BeforeEach
    void setUp() {
        reset(poolMetricsService);
        // Stub void method to prevent Mockito state corruption from background calls
        doNothing().when(poolMetricsService).updatePoolGauges(anyString(), anyInt(), anyInt(), anyInt(), anyInt());
    }

    @Test
    void shouldDetectStalledPoolAfterTimeout() {
        // Given - pool with old timestamp (> 2 minutes ago)
        long stalledTimestamp = System.currentTimeMillis() - 150_000; // 2.5 minutes ago

        PoolStats poolStats = new PoolStats(
            "STALLED-POOL",
            100L,
            90L,
            10L,
            0L,
            0.9,
            3,
            2,
            5,
            10,
            100,
            150.0,
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

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("STALLED-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("STALLED-POOL")).thenReturn(stalledTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then - should be unhealthy because all pools are stalled
        assertFalse(health.healthy());
        assertNotNull(health.issues());
        assertTrue(health.issues().stream()
            .anyMatch(issue -> issue.contains("stalled")));
    }

    @Test
    void shouldDetectMultipleStalledPools() {
        // Given - multiple pools, all stalled
        long stalledTimestamp = System.currentTimeMillis() - 180_000; // 3 minutes ago

        PoolStats pool1 = createPoolStats("STALLED-POOL-1");
        PoolStats pool2 = createPoolStats("STALLED-POOL-2");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of(
            "STALLED-POOL-1", pool1,
            "STALLED-POOL-2", pool2
        ));
        when(poolMetricsService.getLastActivityTimestamp("STALLED-POOL-1")).thenReturn(stalledTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("STALLED-POOL-2")).thenReturn(stalledTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then
        assertFalse(health.healthy());
        assertTrue(health.issues().stream()
            .anyMatch(issue -> issue.contains("All process pools appear stalled")));
    }

    @Test
    void shouldRecoverWhenPoolBecomesActive() {
        // Given - initially stalled pool
        long stalledTimestamp = System.currentTimeMillis() - 150_000; // 2.5 minutes ago
        PoolStats poolStats = createPoolStats("RECOVERY-POOL");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("RECOVERY-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("RECOVERY-POOL")).thenReturn(stalledTimestamp);

        // When - check health (should be unhealthy)
        InfrastructureHealthService.InfrastructureHealth health1 = infrastructureHealthService.checkHealth();

        // Then - unhealthy
        assertFalse(health1.healthy());

        // Given - pool becomes active (recent timestamp)
        long recentTimestamp = System.currentTimeMillis() - 5_000; // 5 seconds ago
        when(poolMetricsService.getLastActivityTimestamp("RECOVERY-POOL")).thenReturn(recentTimestamp);

        // When - check health again
        InfrastructureHealthService.InfrastructureHealth health2 = infrastructureHealthService.checkHealth();

        // Then - should be healthy now
        assertTrue(health2.healthy());
        assertNull(health2.issues());
    }

    @Test
    void shouldNotFailWhenSomePoolsStalledButNotAll() {
        // Given - mix of stalled and active pools
        long stalledTimestamp = System.currentTimeMillis() - 150_000;
        long activeTimestamp = System.currentTimeMillis() - 10_000;

        PoolStats stalledPool = createPoolStats("STALLED-POOL");
        PoolStats activePool1 = createPoolStats("ACTIVE-POOL-1");
        PoolStats activePool2 = createPoolStats("ACTIVE-POOL-2");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of(
            "STALLED-POOL", stalledPool,
            "ACTIVE-POOL-1", activePool1,
            "ACTIVE-POOL-2", activePool2
        ));

        when(poolMetricsService.getLastActivityTimestamp("STALLED-POOL")).thenReturn(stalledTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("ACTIVE-POOL-1")).thenReturn(activeTimestamp);
        when(poolMetricsService.getLastActivityTimestamp("ACTIVE-POOL-2")).thenReturn(activeTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then - should be healthy because not ALL pools are stalled
        assertTrue(health.healthy());
        assertNull(health.issues());
    }

    @Test
    void shouldHandlePoolWithNoActivityDuringStartup() {
        // Given - pool exists but has never processed anything (null timestamp)
        PoolStats poolStats = createPoolStats("NEW-POOL");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("NEW-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("NEW-POOL")).thenReturn(null);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then - should be healthy (startup state)
        assertTrue(health.healthy());
        assertNull(health.issues());
    }

    @Test
    void shouldHandlePoolTransitioningFromNullToStale() {
        // Given - pool starts with null timestamp
        PoolStats poolStats = createPoolStats("TRANSITION-POOL");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("TRANSITION-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("TRANSITION-POOL")).thenReturn(null);

        // When - first check
        InfrastructureHealthService.InfrastructureHealth health1 = infrastructureHealthService.checkHealth();

        // Then - healthy
        assertTrue(health1.healthy());

        // Given - pool processes a message long ago
        long oldTimestamp = System.currentTimeMillis() - 200_000; // Over 3 minutes
        when(poolMetricsService.getLastActivityTimestamp("TRANSITION-POOL")).thenReturn(oldTimestamp);

        // When - second check
        InfrastructureHealthService.InfrastructureHealth health2 = infrastructureHealthService.checkHealth();

        // Then - should now be unhealthy
        assertFalse(health2.healthy());
    }

    @Test
    void shouldHandleExactTimeoutBoundary() {
        // Given - pool at 115 seconds (safely below 120s boundary with buffer for test execution time)
        long exactTimeoutTimestamp = System.currentTimeMillis() - 115_000;

        PoolStats poolStats = createPoolStats("BOUNDARY-POOL");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("BOUNDARY-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("BOUNDARY-POOL")).thenReturn(exactTimeoutTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then - should be healthy (115s is under 120s threshold)
        assertTrue(health.healthy());
        assertNull(health.issues());
    }

    @Test
    void shouldHandleJustOverTimeoutBoundary() {
        // Given - pool at 120 seconds + 1ms
        long justOverTimestamp = System.currentTimeMillis() - 120_001;

        PoolStats poolStats = createPoolStats("JUST-OVER-POOL");

        when(poolMetricsService.getAllPoolStats()).thenReturn(Map.of("JUST-OVER-POOL", poolStats));
        when(poolMetricsService.getLastActivityTimestamp("JUST-OVER-POOL")).thenReturn(justOverTimestamp);

        // When
        InfrastructureHealthService.InfrastructureHealth health = infrastructureHealthService.checkHealth();

        // Then - should be unhealthy
        assertFalse(health.healthy());
        assertTrue(health.issues().stream()
            .anyMatch(issue -> issue.contains("stalled")));
    }

    private PoolStats createPoolStats(String poolCode) {
        return new PoolStats(
            poolCode,
            100L,
            90L,
            10L,
            0L,
            0.9,
            3,
            2,
            5,
            10,
            100,
            150.0,
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

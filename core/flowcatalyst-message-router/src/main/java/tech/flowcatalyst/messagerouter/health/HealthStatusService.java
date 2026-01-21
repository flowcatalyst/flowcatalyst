package tech.flowcatalyst.messagerouter.health;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.metrics.CircuitBreakerMetricsService;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueStats;
import tech.flowcatalyst.messagerouter.model.CircuitBreakerStats;
import tech.flowcatalyst.messagerouter.model.HealthStatus;
import tech.flowcatalyst.messagerouter.model.PoolStats;
import tech.flowcatalyst.messagerouter.model.Warning;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.lang.management.ManagementFactory;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

@ApplicationScoped
public class HealthStatusService {

    private static final Logger LOG = Logger.getLogger(HealthStatusService.class);

    private static final double QUEUE_SUCCESS_THRESHOLD = 0.90; // 90%
    private static final double POOL_SUCCESS_THRESHOLD = 0.90; // 90%
    private static final int MAX_WARNINGS_HEALTHY = 5;
    private static final int MAX_WARNINGS_WARNING = 20;
    private static final long HEALTH_STATUS_TIMEOUT_MINUTES = 30;  // Warnings older than 30 min don't count for health
    private static final long AUTO_ACKNOWLEDGE_TIMEOUT_HOURS = 8;  // Auto-clear warnings older than 8 hours

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @Inject
    QueueMetricsService queueMetricsService;

    @Inject
    PoolMetricsService poolMetricsService;

    @Inject
    WarningService warningService;

    @Inject
    CircuitBreakerMetricsService circuitBreakerMetricsService;

    @Inject
    tech.flowcatalyst.messagerouter.manager.QueueManager queueManager;

    private final long startTimeMillis = System.currentTimeMillis();

    public HealthStatus getHealthStatus() {
        // If message router is disabled, return a simple HEALTHY status
        if (!messageRouterEnabled) {
            long uptimeMillis = System.currentTimeMillis() - startTimeMillis;
            HealthStatus.HealthDetails details = new HealthStatus.HealthDetails(
                0, 0, 0, 0, 0, 0, 0,
                "Message router is disabled"
            );
            return new HealthStatus("HEALTHY", Instant.now(), uptimeMillis, details);
        }

        Map<String, QueueStats> queueStats = queueMetricsService.getAllQueueStats();
        Map<String, PoolStats> poolStats = poolMetricsService.getAllPoolStats();
        List<Warning> allWarnings = warningService.getUnacknowledgedWarnings();

        long now = System.currentTimeMillis();
        long thirtyMinutesAgoMs = now - (HEALTH_STATUS_TIMEOUT_MINUTES * 60 * 1000);
        long eightHoursAgoMs = now - (AUTO_ACKNOWLEDGE_TIMEOUT_HOURS * 60 * 60 * 1000);

        // Auto-acknowledge and remove warnings older than 8 hours
        int autoAcknowledgedCount = 0;
        for (Warning warning : allWarnings) {
            if (warning.timestamp().toEpochMilli() < eightHoursAgoMs && !warning.acknowledged()) {
                warningService.acknowledgeWarning(warning.id());
                autoAcknowledgedCount++;
            }
        }
        if (autoAcknowledgedCount > 0) {
            LOG.infof("Auto-acknowledged %d warnings older than %d hours", autoAcknowledgedCount, AUTO_ACKNOWLEDGE_TIMEOUT_HOURS);
        }

        // Filter out warnings older than 30 minutes for health status calculation
        // This prevents stale warnings from keeping the system in WARNING status
        // but they're still visible in the dashboard and logs
        List<Warning> warnings = allWarnings.stream()
            .filter(w -> w.timestamp().toEpochMilli() >= thirtyMinutesAgoMs)
            .toList();

        Map<String, CircuitBreakerStats> circuitBreakers = circuitBreakerMetricsService.getAllCircuitBreakerStats();
        Map<String, tech.flowcatalyst.messagerouter.manager.QueueManager.QueueConsumerHealth> consumerHealth =
            queueManager.getConsumerHealthStatus();

        int totalQueues = queueStats.size();
        int healthyQueues = 0;
        int totalPools = poolStats.size();
        int healthyPools = 0;
        int activeWarnings = warnings.size();
        int criticalWarnings = (int) warnings.stream()
            .filter(w -> "CRITICAL".equalsIgnoreCase(w.severity()))
            .count();
        int circuitBreakersOpen = (int) circuitBreakers.values().stream()
            .filter(cb -> "OPEN".equalsIgnoreCase(cb.state()))
            .count();
        int totalConsumers = consumerHealth.size();
        int healthyConsumers = (int) consumerHealth.values().stream()
            .filter(tech.flowcatalyst.messagerouter.manager.QueueManager.QueueConsumerHealth::isHealthy)
            .count();

        List<String> degradationReasons = new ArrayList<>();

        // Check queue health - use 30-minute rolling window success rate
        for (QueueStats stats : queueStats.values()) {
            // Use 30-minute success rate for health calculation
            // Only check if there's been activity in the last 30 minutes
            if (stats.totalConsumed30min() == 0 || stats.successRate30min() >= QUEUE_SUCCESS_THRESHOLD) {
                healthyQueues++;
            } else {
                degradationReasons.add(String.format("Queue %s has low success rate (last 30min): %.1f%%",
                    stats.name(), stats.successRate30min()));
            }
        }

        // Check pool health - use 30-minute rolling window success rate
        for (PoolStats stats : poolStats.values()) {
            // Use 30-minute success rate for health calculation
            // Only check if there's been activity in the last 30 minutes
            if (stats.totalProcessed30min() == 0 || stats.successRate30min() >= POOL_SUCCESS_THRESHOLD) {
                healthyPools++;
            } else {
                degradationReasons.add(String.format("Pool %s has low success rate (last 30min): %.1f%%",
                    stats.poolCode(), stats.successRate30min()));
            }
        }

        // Check consumer health
        for (tech.flowcatalyst.messagerouter.manager.QueueManager.QueueConsumerHealth health : consumerHealth.values()) {
            if (!health.isHealthy()) {
                long secondsSinceLastPoll = health.timeSinceLastPollMs() > 0 ?
                    health.timeSinceLastPollMs() / 1000 : -1;
                degradationReasons.add(String.format("Consumer for queue %s is unhealthy (last poll %ds ago)",
                    health.queueIdentifier(), secondsSinceLastPoll));
            }
        }

        // Check warnings
        if (criticalWarnings > 0) {
            degradationReasons.add(String.format("%d critical warnings", criticalWarnings));
        }

        // Check circuit breakers
        if (circuitBreakersOpen > 0) {
            degradationReasons.add(String.format("%d circuit breakers open", circuitBreakersOpen));
        }

        // Determine overall status
        String overallStatus = determineOverallStatus(
            totalQueues, healthyQueues,
            totalPools, healthyPools,
            activeWarnings, criticalWarnings,
            circuitBreakersOpen,
            totalConsumers, healthyConsumers
        );

        long uptimeMillis = System.currentTimeMillis() - startTimeMillis;

        HealthStatus.HealthDetails details = new HealthStatus.HealthDetails(
            totalQueues,
            healthyQueues,
            totalPools,
            healthyPools,
            activeWarnings,
            criticalWarnings,
            circuitBreakersOpen,
            degradationReasons.isEmpty() ? null : String.join("; ", degradationReasons)
        );

        return new HealthStatus(
            overallStatus,
            Instant.now(),
            uptimeMillis,
            details
        );
    }

    private String determineOverallStatus(
            int totalQueues, int healthyQueues,
            int totalPools, int healthyPools,
            int activeWarnings, int criticalWarnings,
            int circuitBreakersOpen,
            int totalConsumers, int healthyConsumers) {

        // DEGRADED: Infrastructure is broken or unable to process messages
        // Only check for critical infrastructure issues, not operational warnings

        if (criticalWarnings > 0) {
            return "DEGRADED";  // Critical infrastructure failures (restart failures, pool limits, etc)
        }

        if (circuitBreakersOpen > 0) {
            return "DEGRADED";  // Circuit breakers open = infrastructure issue
        }

        if (totalQueues > 0 && healthyQueues == 0) {
            return "DEGRADED";  // All queues unhealthy = cannot process
        }

        if (totalPools > 0 && healthyPools == 0) {
            return "DEGRADED";  // All pools unhealthy = cannot process
        }

        // DEGRADED: All consumers are unhealthy (stalled/hung)
        if (totalConsumers > 0 && healthyConsumers == 0) {
            return "DEGRADED";  // All consumers stalled = cannot fetch messages
        }

        // WARNING: Some pools/queues are unhealthy, but system can still process
        if (totalQueues > 0 && healthyQueues < totalQueues) {
            return "WARNING";  // Some queues have issues, but not all
        }

        if (totalPools > 0 && healthyPools < totalPools) {
            return "WARNING";  // Some pools have issues, but not all
        }

        // WARNING: Some consumers are unhealthy
        if (totalConsumers > 0 && healthyConsumers < totalConsumers) {
            return "WARNING";  // Some consumers stalled, but not all
        }

        // HEALTHY: Router is operational
        // Note: Active warnings are not included - warnings are for visibility only,
        // not for determining if the router is working. Message processing failures
        // are operational issues, not router failures.
        return "HEALTHY";
    }
}

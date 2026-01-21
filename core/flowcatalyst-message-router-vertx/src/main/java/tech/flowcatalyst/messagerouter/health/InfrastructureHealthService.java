package tech.flowcatalyst.messagerouter.health;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Service for checking infrastructure health.
 * Returns unhealthy status only if the message router infrastructure itself is compromised,
 * not if downstream services are failing.
 */
@ApplicationScoped
public class InfrastructureHealthService {

    private static final Logger LOG = Logger.getLogger(InfrastructureHealthService.class);

    // If no processing activity in last 2 minutes and pools exist, consider infrastructure stalled
    private static final long ACTIVITY_TIMEOUT_MS = 120_000; // 2 minutes

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @Inject
    QueueManager queueManager;

    @Inject
    PoolMetricsService poolMetricsService;

    /**
     * Checks if the message router infrastructure is healthy.
     *
     * @return InfrastructureHealth object with status and details
     */
    public InfrastructureHealth checkHealth() {
        // If message router is disabled, it's healthy (not running = not broken)
        if (!messageRouterEnabled) {
            return new InfrastructureHealth(true, "Message router is disabled", null);
        }

        List<String> issues = new ArrayList<>();

        // Check 1: QueueManager initialization and pools exist
        Map<String, Long> poolActivity;
        try {
            var allStats = poolMetricsService.getAllPoolStats();
            if (allStats.isEmpty()) {
                issues.add("No active process pools");
            }
            poolActivity = checkProcessPoolActivity();
        } catch (Exception e) {
            LOG.error("Failed to check QueueManager initialization", e);
            issues.add("QueueManager not initialized");
            poolActivity = new java.util.HashMap<>();
        }

        // Check 2: Process pools with activity are not stalled
        if (!poolActivity.isEmpty()) {
            List<String> stalledPools = checkForStalledPools(poolActivity);
            if (!stalledPools.isEmpty() && stalledPools.size() == poolActivity.size()) {
                // Only fail if ALL pools with activity are stalled
                issues.add("All process pools appear stalled (no activity in " + (ACTIVITY_TIMEOUT_MS / 1000) + "s)");
            }
        }

        boolean healthy = issues.isEmpty();
        String message = healthy
            ? "Infrastructure is operational"
            : "Infrastructure issues detected";

        return new InfrastructureHealth(healthy, message, issues.isEmpty() ? null : issues);
    }

    /**
     * Get last activity timestamp for each pool
     */
    private Map<String, Long> checkProcessPoolActivity() {
        Map<String, Long> poolActivity = new java.util.HashMap<>();
        try {
            var allStats = poolMetricsService.getAllPoolStats();
            for (String poolCode : allStats.keySet()) {
                Long lastActivity = poolMetricsService.getLastActivityTimestamp(poolCode);
                if (lastActivity != null) {
                    poolActivity.put(poolCode, lastActivity);
                }
            }
        } catch (Exception e) {
            LOG.error("Failed to check process pool activity", e);
        }
        return poolActivity;
    }

    /**
     * Check which pools haven't processed messages recently
     */
    private List<String> checkForStalledPools(Map<String, Long> poolActivity) {
        List<String> stalledPools = new ArrayList<>();
        long currentTime = System.currentTimeMillis();

        for (Map.Entry<String, Long> entry : poolActivity.entrySet()) {
            String poolCode = entry.getKey();
            Long lastActivity = entry.getValue();

            if (lastActivity == null) {
                // Pool exists but has never processed anything
                // This is OK during startup or if no messages have arrived
                continue;
            }

            long timeSinceActivity = currentTime - lastActivity;
            if (timeSinceActivity > ACTIVITY_TIMEOUT_MS) {
                stalledPools.add(poolCode);
                LOG.warnf("Pool [%s] has not processed messages in %d seconds",
                    poolCode, timeSinceActivity / 1000);
            }
        }

        return stalledPools;
    }

    /**
     * Infrastructure health check result
     */
    public record InfrastructureHealth(
        boolean healthy,
        String message,
        List<String> issues
    ) {}
}

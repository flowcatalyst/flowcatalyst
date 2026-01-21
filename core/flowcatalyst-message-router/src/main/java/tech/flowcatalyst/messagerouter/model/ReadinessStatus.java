package tech.flowcatalyst.messagerouter.model;

import java.time.Instant;
import java.util.List;

/**
 * Simple health status for Kubernetes liveness and readiness probes.
 * This is separate from the more complex HealthStatus used by the monitoring dashboard.
 */
public record ReadinessStatus(
    String status,
    Instant timestamp,
    List<String> issues
) {
    /**
     * Create a healthy status
     */
    public static ReadinessStatus healthy(String status) {
        return new ReadinessStatus(status, Instant.now(), List.of());
    }

    /**
     * Create an unhealthy status with issues
     */
    public static ReadinessStatus unhealthy(String status, List<String> issues) {
        return new ReadinessStatus(status, Instant.now(), issues);
    }
}

package tech.flowcatalyst.messagerouter.model;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.time.Instant;

/**
 * Overall system health status
 */
@Schema(description = "Overall system health status")
public record HealthStatus(
    @Schema(description = "Health status", examples = {"HEALTHY", "WARNING", "DEGRADED"})
    String status,

    @Schema(description = "Timestamp of health check", examples = {"2025-10-16T10:52:20Z"})
    Instant timestamp,

    @Schema(description = "System uptime in milliseconds", examples = {"3600000", "86400000"})
    long uptimeMillis,

    @Schema(description = "Detailed health information")
    HealthDetails details
) {
    @Schema(description = "Detailed health breakdown")
    public record HealthDetails(
        @Schema(description = "Total number of queues", examples = {"3", "5", "10"})
        int totalQueues,

        @Schema(description = "Number of healthy queues", examples = {"3", "5", "8"})
        int healthyQueues,

        @Schema(description = "Total number of pools", examples = {"3", "5", "10"})
        int totalPools,

        @Schema(description = "Number of healthy pools", examples = {"3", "5", "8"})
        int healthyPools,

        @Schema(description = "Number of active warnings", examples = {"0", "5", "20"})
        int activeWarnings,

        @Schema(description = "Number of critical warnings", examples = {"0", "2", "5"})
        int criticalWarnings,

        @Schema(description = "Number of open circuit breakers", examples = {"0", "1", "3"})
        int circuitBreakersOpen,

        @Schema(description = "Reason for degraded status", examples = {"Queue flow-catalyst-high-priority.fifo has low success rate: 85.5%", "2 critical warnings"})
        String degradationReason
    ) {}
}

package tech.flowcatalyst.messagerouter.model;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.time.Instant;

/**
 * Represents a system warning
 */
@Schema(description = "System warning or error notification")
public record Warning(
    @Schema(description = "Unique warning identifier (UUID)", examples = {"550e8400-e29b-41d4-a716-446655440000"})
    String id,

    @Schema(description = "Warning category",
            examples = {"QUEUE_BACKLOG", "QUEUE_GROWING", "MEDIATION", "CONFIGURATION", "POOL_LIMIT"})
    String category,

    @Schema(description = "Severity level", examples = {"CRITICAL", "ERROR", "WARNING", "INFO"})
    String severity,

    @Schema(description = "Warning message describing the issue",
            examples = {"Queue flow-catalyst-high-priority.fifo depth is 2500 (threshold: 1000)",
                       "Max pool limit reached (2000/2000) - cannot create pool [POOL-ABC]"})
    String message,

    @Schema(description = "Timestamp when warning was created", examples = {"2025-10-16T10:52:20Z"})
    Instant timestamp,

    @Schema(description = "Source component that generated the warning",
            examples = {"QueueHealthMonitor", "ProcessPool:POOL-HIGH", "QueueManager"})
    String source,

    @Schema(description = "Whether the warning has been acknowledged", examples = {"false", "true"})
    boolean acknowledged
) {}

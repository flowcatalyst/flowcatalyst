package tech.flowcatalyst.messagerouter.model;

import org.eclipse.microprofile.openapi.annotations.media.Schema;

/**
 * Statistics for a processing pool
 */
@Schema(description = "Statistics for a processing pool")
public record PoolStats(
    @Schema(description = "Pool identifier code", examples = {"POOL-HIGH", "POOL-MEDIUM", "POOL-LOW"})
    String poolCode,

    @Schema(description = "Total number of messages processed (all-time)", examples = {"128647", "0", "5432"})
    long totalProcessed,

    @Schema(description = "Total number of successfully processed messages (all-time)", examples = {"128637", "0", "5420"})
    long totalSucceeded,

    @Schema(description = "Total number of failed messages (all-time)", examples = {"10", "0", "12"})
    long totalFailed,

    @Schema(description = "Total number of rate-limited messages (all-time)", examples = {"0", "5", "100"})
    long totalRateLimited,

    @Schema(description = "All-time success rate (0.0 to 1.0)", examples = {"0.9999222679114165", "1.0", "0.0"})
    double successRate,

    @Schema(description = "Number of currently active workers", examples = {"10", "0", "5"})
    int activeWorkers,

    @Schema(description = "Number of available permits for new work", examples = {"0", "5", "2"})
    int availablePermits,

    @Schema(description = "Maximum concurrency for this pool", examples = {"10", "5", "2"})
    int maxConcurrency,

    @Schema(description = "Current number of messages in queue", examples = {"500", "0", "250"})
    int queueSize,

    @Schema(description = "Maximum queue capacity", examples = {"500", "500", "500"})
    int maxQueueCapacity,

    @Schema(description = "Average processing time in milliseconds", examples = {"103.82261537385249", "0.0", "250.5"})
    double averageProcessingTimeMs,

    @Schema(description = "Number of messages processed in last 5 minutes", examples = {"100", "0", "50"})
    long totalProcessed5min,

    @Schema(description = "Number of successfully processed messages in last 5 minutes", examples = {"98", "0", "48"})
    long totalSucceeded5min,

    @Schema(description = "Number of failed messages in last 5 minutes", examples = {"2", "0", "2"})
    long totalFailed5min,

    @Schema(description = "Success rate based on last 5 minutes (0.0 to 1.0)", examples = {"0.98", "1.0", "0.96"})
    double successRate5min,

    @Schema(description = "Number of messages processed in last 30 minutes", examples = {"1234", "0", "567"})
    long totalProcessed30min,

    @Schema(description = "Number of successfully processed messages in last 30 minutes", examples = {"1230", "0", "560"})
    long totalSucceeded30min,

    @Schema(description = "Number of failed messages in last 30 minutes", examples = {"4", "0", "7"})
    long totalFailed30min,

    @Schema(description = "Success rate based on last 30 minutes (0.0 to 1.0)", examples = {"0.9967532467532468", "1.0", "0.0"})
    double successRate30min,

    @Schema(description = "Number of rate-limited messages in last 5 minutes", examples = {"0", "5", "20"})
    long totalRateLimited5min,

    @Schema(description = "Number of rate-limited messages in last 30 minutes", examples = {"0", "10", "50"})
    long totalRateLimited30min
) {}

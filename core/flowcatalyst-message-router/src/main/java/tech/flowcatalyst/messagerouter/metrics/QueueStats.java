package tech.flowcatalyst.messagerouter.metrics;

import com.fasterxml.jackson.annotation.JsonProperty;
import org.eclipse.microprofile.openapi.annotations.media.Schema;

@Schema(description = "Statistics for a message queue")
public record QueueStats(
    @Schema(description = "Queue name or identifier", examples = {"flow-catalyst-high-priority.fifo", "flow-catalyst-medium-priority.fifo"})
    @JsonProperty("name") String name,

    @Schema(description = "Total number of messages received (all-time)", examples = {"150000", "0", "5000"})
    @JsonProperty("totalMessages") long totalMessages,

    @Schema(description = "Total number of messages consumed (all-time)", examples = {"149950", "0", "4995"})
    @JsonProperty("totalConsumed") long totalConsumed,

    @Schema(description = "Total number of failed messages (all-time)", examples = {"50", "0", "5"})
    @JsonProperty("totalFailed") long totalFailed,

    @Schema(description = "All-time success rate (0.0 to 1.0)", examples = {"0.9996666666666667", "1.0", "0.999"})
    @JsonProperty("successRate") double successRate,

    @Schema(description = "Current queue depth/size", examples = {"500", "0", "150"})
    @JsonProperty("currentSize") long currentSize,

    @Schema(description = "Throughput in messages per second", examples = {"25.5", "0.0", "10.2"})
    @JsonProperty("throughput") double throughput,

    @Schema(description = "Number of pending messages in queue", examples = {"500", "0", "150"})
    @JsonProperty("pendingMessages") long pendingMessages,

    @Schema(description = "Number of messages currently being processed (not visible)", examples = {"10", "0", "5"})
    @JsonProperty("messagesNotVisible") long messagesNotVisible,

    @Schema(description = "Total number of messages received in last 5 minutes", examples = {"100", "0", "50"})
    @JsonProperty("totalMessages5min") long totalMessages5min,

    @Schema(description = "Total number of messages consumed in last 5 minutes", examples = {"98", "0", "48"})
    @JsonProperty("totalConsumed5min") long totalConsumed5min,

    @Schema(description = "Total number of failed messages in last 5 minutes", examples = {"2", "0", "2"})
    @JsonProperty("totalFailed5min") long totalFailed5min,

    @Schema(description = "Success rate based on last 5 minutes (0.0 to 1.0)", examples = {"0.98", "1.0", "0.96"})
    @JsonProperty("successRate5min") double successRate5min,

    @Schema(description = "Total number of messages received in last 30 minutes", examples = {"1500", "0", "500"})
    @JsonProperty("totalMessages30min") long totalMessages30min,

    @Schema(description = "Total number of messages consumed in last 30 minutes", examples = {"1495", "0", "495"})
    @JsonProperty("totalConsumed30min") long totalConsumed30min,

    @Schema(description = "Total number of failed messages in last 30 minutes", examples = {"5", "0", "5"})
    @JsonProperty("totalFailed30min") long totalFailed30min,

    @Schema(description = "Success rate based on last 30 minutes (0.0 to 1.0)", examples = {"0.9966666666666667", "1.0", "0.99"})
    @JsonProperty("successRate30min") double successRate30min,

    @Schema(description = "Total number of deferred messages (rate limiting, capacity - not failures)", examples = {"100", "0", "50"})
    @JsonProperty("totalDeferred") long totalDeferred
) {
    public static QueueStats empty(String name) {
        return new QueueStats(name, 0, 0, 0, 0.0, 0, 0.0, 0, 0, 0, 0, 0, 0.0, 0, 0, 0, 0.0, 0);
    }
}

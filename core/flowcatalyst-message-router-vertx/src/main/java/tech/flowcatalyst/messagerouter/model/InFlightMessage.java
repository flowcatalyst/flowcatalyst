package tech.flowcatalyst.messagerouter.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import org.eclipse.microprofile.openapi.annotations.media.Schema;

import java.time.Instant;

/**
 * Represents a message currently in flight (being processed)
 */
@Schema(description = "A message currently being processed by the message router")
public record InFlightMessage(
    @Schema(description = "Application message ID from MessagePointer", example = "01K9XYM11VFTAPJEPJBR8070FY")
    @JsonProperty("messageId") String messageId,

    @Schema(description = "Broker message ID (e.g., SQS message ID)", example = "d8e4f6a0-1234-5678-9abc-def012345678")
    @JsonProperty("brokerMessageId") String brokerMessageId,

    @Schema(description = "Queue identifier", example = "FC-staging-order-queue.fifo")
    @JsonProperty("queueId") String queueId,

    @Schema(description = "Timestamp when message entered the pipeline", example = "2025-11-13T06:30:00Z")
    @JsonProperty("addedToInPipelineAt") Instant addedToInPipelineAt,

    @Schema(description = "How long the message has been in flight (milliseconds)", example = "5000")
    @JsonProperty("elapsedTimeMs") long elapsedTimeMs,

    @Schema(description = "Pool code this message is being processed by", example = "staging-order-CONCURRENCY-10")
    @JsonProperty("poolCode") String poolCode
) {
    /**
     * Creates an InFlightMessage from message details and timestamp
     *
     * @param messageId Application message ID from MessagePointer
     * @param brokerMessageId Broker message ID (e.g., SQS message ID)
     * @param queueId Queue identifier
     * @param addedAt Timestamp when message entered the pipeline
     * @param poolCode Pool code this message is being processed by
     */
    public static InFlightMessage from(String messageId, String brokerMessageId, String queueId, long addedAt, String poolCode) {
        long now = System.currentTimeMillis();
        long elapsedMs = now - addedAt;
        return new InFlightMessage(
            messageId,
            brokerMessageId,
            queueId,
            Instant.ofEpochMilli(addedAt),
            elapsedMs,
            poolCode
        );
    }
}

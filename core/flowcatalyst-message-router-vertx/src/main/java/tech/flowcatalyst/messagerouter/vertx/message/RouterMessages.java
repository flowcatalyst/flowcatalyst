package tech.flowcatalyst.messagerouter.vertx.message;

import tech.flowcatalyst.messagerouter.model.InFlightMessage;
import tech.flowcatalyst.messagerouter.model.MediationType;

import java.util.List;
import java.util.Set;

/**
 * Message records for event bus communication between verticles.
 * <p>
 * Using records with Jackson instead of Vert.x JsonObject provides:
 * <ul>
 *   <li>Type safety at compile time</li>
 *   <li>Cleaner, more readable code</li>
 *   <li>Automatic serialization via Jackson</li>
 *   <li>Pattern matching with sealed interfaces</li>
 * </ul>
 */
public final class RouterMessages {

    private RouterMessages() {}

    // ==================== Router Messages ====================

    /**
     * Batch of messages from a queue consumer.
     * Sent to: router.batch
     */
    public record BatchRequest(
            List<QueuedMessage> messages,
            String queueIdentifier
    ) {}

    /**
     * Individual message from SQS queue with routing metadata.
     */
    public record QueuedMessage(
            String id,
            String sqsMessageId,
            String poolCode,
            String authToken,
            MediationType mediationType,
            String mediationTarget,
            String messageGroupId,
            String queueUrl,
            String receiptHandle
    ) {}

    /**
     * ACK request to delete message from SQS.
     * Sent to: router.ack
     */
    public record AckRequest(String sqsMessageId) {}

    /**
     * NACK request to change message visibility.
     * Sent to: router.nack
     */
    public record NackRequest(String sqsMessageId, int delaySeconds) {}

    /**
     * Query for in-flight messages.
     * Sent to: router.query.in-flight
     */
    public record InFlightQuery(int limit, String filter) {}

    /**
     * Result of in-flight query (typed, not JsonArray).
     */
    public record InFlightQueryResult(List<InFlightMessage> messages) {}

    /**
     * Result of pool stats query.
     */
    public record PoolStatsResult(Set<String> activePoolCodes) {}

    // ==================== Pool Messages ====================

    /**
     * Message routed to a pool for processing.
     * Sent to: pool.{code}
     */
    public record PoolMessage(
            String id,
            String sqsMessageId,
            String poolCode,
            String authToken,
            MediationType mediationType,
            String mediationTarget,
            String messageGroupId,
            String batchId
    ) {}

    /**
     * Configuration update for a pool.
     * Sent to: pool.{code}.config
     */
    public record PoolConfigUpdate(
            int concurrency,
            Integer rateLimitPerMinute
    ) {}

    /**
     * Result of submitting a message to a pool (for backpressure).
     */
    public sealed interface SubmitResult permits Accepted, Rejected {
    }

    /**
     * Message was accepted by the pool.
     */
    public record Accepted() implements SubmitResult {}

    /**
     * Message was rejected (pool at capacity, shutting down, etc).
     */
    public record Rejected(String reason) implements SubmitResult {}

    /**
     * Result of a configuration update.
     */
    public sealed interface ConfigResult permits ConfigUpdated, ConfigFailed {
    }

    /**
     * Configuration was updated successfully.
     */
    public record ConfigUpdated() implements ConfigResult {}

    /**
     * Configuration update failed.
     */
    public record ConfigFailed(String reason) implements ConfigResult {}

    // ==================== Mediator Messages ====================

    /**
     * Request for message mediation (HTTP delivery).
     * Sent to: mediator.{code}
     */
    public record MediationRequest(
            String id,
            String sqsMessageId,
            String authToken,
            MediationType mediationType,
            String mediationTarget,
            String messageGroupId
    ) {}

    /**
     * Result of mediation attempt.
     * Reply from: mediator.{code}
     */
    public record MediationResult(
            Outcome outcome,
            int delaySeconds,
            String errorMessage
    ) {
        public enum Outcome {
            SUCCESS,
            NACK,
            ERROR_CONFIG
        }

        public static MediationResult success() {
            return new MediationResult(Outcome.SUCCESS, 0, null);
        }

        public static MediationResult nack(int delaySeconds, String errorMessage) {
            return new MediationResult(Outcome.NACK, delaySeconds, errorMessage);
        }

        public static MediationResult configError(String errorMessage) {
            return new MediationResult(Outcome.ERROR_CONFIG, 0, errorMessage);
        }
    }

    // ==================== Generic Replies ====================

    /**
     * Simple acknowledgement reply.
     */
    public record OkReply() {}
}

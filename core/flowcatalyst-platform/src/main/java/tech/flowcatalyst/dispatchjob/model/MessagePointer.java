package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * Message pointer containing routing and mediation information.
 *
 * NOTE: This is a copy of tech.flowcatalyst.messagerouter.model.MessagePointer
 * kept in sync for the backend's dispatch job functionality.
 *
 * @param id Unique message identifier (used for deduplication)
 * @param poolCode Processing pool identifier (e.g., "POOL-HIGH", "order-service")
 * @param authToken Authentication token for downstream service calls
 * @param mediationType Type of mediation to perform (HTTP, WEBHOOK, etc.)
 * @param mediationTarget Target endpoint URL for mediation
 * @param messageGroupId Optional message group ID for FIFO ordering within business entities
 * @param batchId Internal batch identifier (NOT part of external contract)
 * @param sqsMessageId AWS SQS internal message ID for pipeline tracking
 */
public record MessagePointer(
    @JsonProperty("id") String id,
    @JsonProperty("poolCode") String poolCode,
    @JsonProperty("authToken") String authToken,
    @JsonProperty("mediationType") MediationType mediationType,
    @JsonProperty("mediationTarget") String mediationTarget,
    @JsonProperty(value = "messageGroupId", required = true) String messageGroupId,
    @JsonIgnore String batchId,
    @JsonIgnore String sqsMessageId
) {
    /**
     * Constructor without sqsMessageId for backward compatibility
     */
    public MessagePointer(String id, String poolCode, String authToken, MediationType mediationType,
                         String mediationTarget, String messageGroupId, String batchId) {
        this(id, poolCode, authToken, mediationType, mediationTarget, messageGroupId, batchId, null);
    }
}

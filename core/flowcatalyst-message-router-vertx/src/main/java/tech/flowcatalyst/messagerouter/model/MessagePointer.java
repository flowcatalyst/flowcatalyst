package tech.flowcatalyst.messagerouter.model;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * Message pointer containing routing and mediation information.
 *
 * <p>This record is serialized/deserialized to/from queue messages and contains all
 * information needed to route and process a message through the system.
 *
 * @param id Unique message identifier (used for deduplication)
 * @param poolCode Processing pool identifier (e.g., "POOL-HIGH", "order-service")
 * @param authToken Authentication token for downstream service calls
 * @param mediationType Type of mediation to perform (HTTP, WEBHOOK, etc.)
 * @param mediationTarget Target endpoint URL for mediation
 * @param messageGroupId Optional message group ID for FIFO ordering within business entities.
 *                       Messages with the same messageGroupId are processed sequentially,
 *                       while messages with different messageGroupIds are processed concurrently.
 *                       <p>Examples:
 *                       <ul>
 *                         <li>"order-12345" - All events for this order process in FIFO order</li>
 *                         <li>"user-67890" - All events for this user process in FIFO order</li>
 *                         <li>null - Uses DEFAULT_GROUP, processes independently</li>
 *                       </ul>
 *                       See <a href="../../../../../../MESSAGE_GROUP_FIFO.md">MESSAGE_GROUP_FIFO.md</a>
 *                       for detailed documentation.
 * @param batchId Internal batch identifier (NOT part of external contract, populated during routing).
 *                Used to track messages from the same batch for FIFO ordering enforcement.
 *                When a message in a batch+group fails, all subsequent messages in that
 *                batch+group are automatically nacked to preserve FIFO guarantees.
 *                <p><b>IMPORTANT:</b> This field is marked with @JsonIgnore and is never
 *                serialized/deserialized. It is purely for internal processing within the
 *                message router after messages are pulled from the queue.
 */
public record MessagePointer(
    @JsonProperty("id") String id,
    @JsonProperty("poolCode") String poolCode,
    @JsonProperty("authToken") String authToken,
    @JsonProperty("mediationType") MediationType mediationType,
    @JsonProperty("mediationTarget") String mediationTarget,
    @JsonProperty(value = "messageGroupId", required = true) String messageGroupId,
    @JsonIgnore String batchId,  // Internal only - never part of queue message contract
    @JsonIgnore String sqsMessageId  // AWS SQS internal message ID for pipeline tracking
) {
    /**
     * Constructor without sqsMessageId for backward compatibility
     */
    public MessagePointer(String id, String poolCode, String authToken, MediationType mediationType,
                         String mediationTarget, String messageGroupId, String batchId) {
        this(id, poolCode, authToken, mediationType, mediationTarget, messageGroupId, batchId, null);
    }

    /**
     * Create a MessagePointer from a Vert.x JsonObject.
     */
    public static MessagePointer fromJson(io.vertx.core.json.JsonObject json) {
        String mediationTypeStr = json.getString("mediationType", "HTTP");
        MediationType mediationType;
        try {
            mediationType = MediationType.valueOf(mediationTypeStr);
        } catch (Exception e) {
            mediationType = MediationType.HTTP;
        }

        return new MessagePointer(
            json.getString("id"),
            json.getString("poolCode"),
            json.getString("authToken"),
            mediationType,
            json.getString("mediationTarget"),
            json.getString("messageGroupId"),
            json.getString("batchId"),
            json.getString("sqsMessageId")
        );
    }
}

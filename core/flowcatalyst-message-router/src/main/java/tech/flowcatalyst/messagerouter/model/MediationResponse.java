package tech.flowcatalyst.messagerouter.model;

import com.fasterxml.jackson.annotation.JsonProperty;
import org.eclipse.microprofile.openapi.annotations.media.Schema;

/**
 * Response from a mediation endpoint indicating whether the message should be acknowledged.
 *
 * <p>The endpoint returns HTTP 200 with this DTO to indicate:
 * <ul>
 *   <li><b>ack: true</b> - Message processing is complete, ACK it and mark as success</li>
 *   <li><b>ack: false</b> - Message is accepted but not ready to be processed yet (e.g., notBefore time not reached).
 *       Nack it and retry via queue visibility timeout. Optionally specify a delay in seconds.</li>
 * </ul>
 *
 * <h2>Delay Behavior (ack=false)</h2>
 * <p>When {@code ack=false}, the optional {@code delaySeconds} field controls when the message becomes visible again:</p>
 * <ul>
 *   <li>If {@code delaySeconds} is set (1-43200), the message will be invisible for that many seconds</li>
 *   <li>If {@code delaySeconds} is null/0, the default visibility timeout (30s) is used</li>
 *   <li>SQS: Uses ChangeMessageVisibility API</li>
 *   <li>ActiveMQ: Uses scheduled redelivery delay</li>
 * </ul>
 */
@Schema(description = "Response from mediation endpoint indicating acknowledgment status")
public record MediationResponse(
    @JsonProperty("ack")
    @Schema(description = "Whether the message should be acknowledged (true) or nacked for retry (false)",
            examples = {"true", "false"})
    boolean ack,

    @JsonProperty("message")
    @Schema(description = "Optional message or reason (e.g., delay reason if ack=false)",
            examples = {"", "notBefore time not reached", "Processing scheduled for later"})
    String message,

    @JsonProperty("delaySeconds")
    @Schema(description = "Optional delay in seconds before the message becomes visible again (only used when ack=false). " +
            "Valid range: 1-43200 (12 hours). If null or 0, uses default visibility timeout (30s).",
            examples = {"30", "60", "300"})
    Integer delaySeconds
) {
    /** Maximum delay allowed (12 hours = 43200 seconds, SQS limit) */
    public static final int MAX_DELAY_SECONDS = 43200;

    /** Default delay when none specified */
    public static final int DEFAULT_DELAY_SECONDS = 30;

    // Allow construction with just ack, defaulting message to empty string and delay to null
    public MediationResponse(boolean ack) {
        this(ack, "", null);
    }

    // Allow construction with ack and message, defaulting delay to null
    public MediationResponse(boolean ack, String message) {
        this(ack, message, null);
    }

    /**
     * Get the effective delay in seconds, clamped to valid range.
     * Returns DEFAULT_DELAY_SECONDS if delaySeconds is null or 0.
     *
     * @return delay in seconds (1-43200)
     */
    public int getEffectiveDelaySeconds() {
        if (delaySeconds == null || delaySeconds <= 0) {
            return DEFAULT_DELAY_SECONDS;
        }
        return Math.min(delaySeconds, MAX_DELAY_SECONDS);
    }
}

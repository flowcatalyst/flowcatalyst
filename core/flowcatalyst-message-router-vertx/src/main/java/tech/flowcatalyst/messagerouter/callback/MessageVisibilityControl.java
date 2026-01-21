package tech.flowcatalyst.messagerouter.callback;

import tech.flowcatalyst.messagerouter.model.MessagePointer;

/**
 * Optional interface for message callbacks that support visibility timeout control.
 * Allows fast-fail scenarios (rate limit, pool full) to retry quickly (1 second)
 * while real processing failures use the default visibility timeout.
 *
 * <p>Implementations:
 * <ul>
 *   <li>SQS: Use ChangeMessageVisibility API</li>
 *   <li>ActiveMQ: Uses scheduled redelivery delay</li>
 * </ul>
 */
public interface MessageVisibilityControl {

    /**
     * Set message visibility to 10 seconds for fast retry on transient failures.
     * Used when message couldn't be processed due to:
     * - Rate limiting
     * - Pool queue full
     *
     * @param message the message to adjust visibility for
     */
    void setFastFailVisibility(MessagePointer message);

    /**
     * Reset message visibility to default (typically 30+ seconds) for real processing failures.
     * Used when message was processed but failed due to:
     * - Downstream service error (4xx, 5xx)
     * - Connection timeout
     * - Business logic failure
     *
     * @param message the message to adjust visibility for
     */
    void resetVisibilityToDefault(MessagePointer message);

    /**
     * Set a custom visibility delay for the message.
     * Used when the mediation response specifies a delay (ack=false with delaySeconds).
     *
     * <p>This allows the downstream service to control when the message should be retried,
     * for example when a "notBefore" time hasn't been reached yet.</p>
     *
     * @param message the message to adjust visibility for
     * @param delaySeconds the delay in seconds before the message becomes visible again (1-43200)
     */
    void setVisibilityDelay(MessagePointer message, int delaySeconds);

    /**
     * Extend message visibility timeout to keep message invisible while processing continues.
     * This prevents message redelivery when processing takes longer than the initial visibility timeout.
     * Should be called periodically (e.g., every 15 seconds) while message is being processed.
     *
     * @param message the message to extend visibility for
     * @param visibilityTimeoutSeconds how long to keep the message invisible (typically 30-60 seconds)
     */
    void extendVisibility(MessagePointer message, int visibilityTimeoutSeconds);
}

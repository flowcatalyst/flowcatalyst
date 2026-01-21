package tech.flowcatalyst.messagerouter.callback;

import tech.flowcatalyst.messagerouter.model.MessagePointer;

/**
 * Callback interface for queue consumers to notify about message acknowledgment
 */
public interface MessageCallback {

    /**
     * @param message the message to acknowledge
     */
    void ack(MessagePointer message);

    /**
     * @param message the message to nack
     */
    void nack(MessagePointer message);

    /**
     * @param message the message to nack
     * @param delaySeconds delay before the message becomes visible again
     */
    default void nack(MessagePointer message, int delaySeconds) {
        nack(message);
    }
}

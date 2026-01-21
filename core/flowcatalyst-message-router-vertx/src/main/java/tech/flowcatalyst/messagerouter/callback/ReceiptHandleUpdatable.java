package tech.flowcatalyst.messagerouter.callback;

/**
 * Interface for callbacks that support updating the receipt handle.
 * This is used when SQS redelivers a message (due to visibility timeout expiring)
 * while the original message is still being processed. By updating the receipt handle,
 * we ensure the ACK uses the valid (latest) receipt handle.
 */
public interface ReceiptHandleUpdatable {

    /**
     * Update the receipt handle to a new value.
     * Called when a redelivery of the same message is detected.
     *
     * @param newReceiptHandle the new receipt handle from the redelivered message
     */
    void updateReceiptHandle(String newReceiptHandle);

    /**
     * Get the current receipt handle.
     *
     * @return the current receipt handle
     */
    String getReceiptHandle();
}

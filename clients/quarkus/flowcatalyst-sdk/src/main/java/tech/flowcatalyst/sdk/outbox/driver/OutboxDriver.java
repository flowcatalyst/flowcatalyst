package tech.flowcatalyst.sdk.outbox.driver;

import java.util.List;
import java.util.Map;

/**
 * Interface for outbox message persistence drivers.
 */
public interface OutboxDriver {

    /**
     * Insert a single message into the outbox.
     *
     * @param message The message to insert
     */
    void insert(Map<String, Object> message);

    /**
     * Insert multiple messages into the outbox in a batch.
     *
     * @param messages The messages to insert
     */
    void insertBatch(List<Map<String, Object>> messages);
}

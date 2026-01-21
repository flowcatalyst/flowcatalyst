package tech.flowcatalyst.messagerouter.embedded;

import io.agroal.api.AgroalDataSource;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.consumer.AbstractQueueConsumer;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.UUID;

/**
 * Embedded SQLite queue consumer with full SQS FIFO semantics:
 * - Message groups (FIFO ordering per group)
 * - Visibility timeout (message locking)
 * - Deduplication
 * - ACK/NACK with configurable retry delays
 *
 * Thread-safe: SQLite handles concurrent access via database locking.
 * Virtual thread compatible: No thread affinity issues.
 */
public class EmbeddedQueueConsumer extends AbstractQueueConsumer {

    private static final Logger LOG = Logger.getLogger(EmbeddedQueueConsumer.class);

    private final AgroalDataSource dataSource;
    private final String queueUri;
    private final int visibilityTimeoutSeconds;
    private final int receiveTimeoutMs;

    public EmbeddedQueueConsumer(
            AgroalDataSource dataSource,
            String queueUri,
            int connections,
            QueueManager queueManager,
            QueueMetricsService queueMetrics,
            WarningService warningService,
            int visibilityTimeoutSeconds,
            int receiveTimeoutMs) {
        super(queueManager, queueMetrics, warningService, connections);
        this.dataSource = dataSource;
        this.queueUri = queueUri;
        this.visibilityTimeoutSeconds = visibilityTimeoutSeconds;
        this.receiveTimeoutMs = receiveTimeoutMs;
    }

    @Override
    public String getQueueIdentifier() {
        return queueUri;
    }

    @Override
    protected void consumeMessages() {
        LOG.infof("Embedded queue consumer started for [%s]", queueUri);

        while (running.get()) {
            updateHeartbeat();

            try {
                // Dequeue message with message group ordering
                EmbeddedMessage message = dequeueMessage();

                if (message == null) {
                    // No messages available, sleep briefly
                    Thread.sleep(receiveTimeoutMs);
                    continue;
                }

                // Process as single-message batch for consistency with SQS consumer
                java.util.List<RawMessage> batch = java.util.List.of(new RawMessage(
                    message.messageJson,
                    null,  // messageGroupId will be extracted from MessagePointer body
                    new EmbeddedMessageCallback(message.receiptHandle),
                    null   // No SQS message ID for embedded queue
                ));
                processMessageBatch(batch);

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                LOG.info("Embedded queue consumer interrupted, exiting");
                break;
            } catch (Exception e) {
                LOG.errorf(e, "Error consuming from embedded queue [%s]", queueUri);
                try {
                    Thread.sleep(1000); // Brief pause on error
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }

        LOG.infof("Embedded queue consumer stopped for [%s]", queueUri);
    }

    /**
     * Dequeue next message with proper message group ordering.
     * Returns oldest visible message from the oldest message group.
     */
    private EmbeddedMessage dequeueMessage() throws Exception {
        long now = System.currentTimeMillis();
        long visibilityTimeout = now + (visibilityTimeoutSeconds * 1000L);
        String receiptHandle = UUID.randomUUID().toString();

        String sql = """
            WITH next_group AS (
                SELECT message_group_id
                FROM queue_messages
                WHERE visible_at <= ?
                ORDER BY id
                LIMIT 1
            )
            UPDATE queue_messages
            SET visible_at = ?,
                receipt_handle = ?,
                receive_count = receive_count + 1,
                first_received_at = COALESCE(first_received_at, ?)
            WHERE id = (
                SELECT id
                FROM queue_messages
                WHERE message_group_id IN (SELECT message_group_id FROM next_group)
                  AND visible_at <= ?
                ORDER BY id
                LIMIT 1
            )
            RETURNING id, message_id, message_group_id, message_json, receipt_handle, receive_count
            """;

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            stmt.setLong(1, now);
            stmt.setLong(2, visibilityTimeout);
            stmt.setString(3, receiptHandle);
            stmt.setLong(4, now);
            stmt.setLong(5, now);

            try (ResultSet rs = stmt.executeQuery()) {
                if (rs.next()) {
                    return new EmbeddedMessage(
                        rs.getLong("id"),
                        rs.getString("message_id"),
                        rs.getString("message_group_id"),
                        rs.getString("message_json"),
                        rs.getString("receipt_handle"),
                        rs.getInt("receive_count")
                    );
                }
            }
        }

        return null;
    }

    @Override
    protected void pollQueueMetrics() {
        while (running.get()) {
            try {
                long now = System.currentTimeMillis();

                String sql = """
                    SELECT
                        COUNT(CASE WHEN visible_at <= ? THEN 1 END) as visible_messages,
                        COUNT(CASE WHEN visible_at > ? THEN 1 END) as invisible_messages
                    FROM queue_messages
                    """;

                try (Connection conn = dataSource.getConnection();
                     PreparedStatement stmt = conn.prepareStatement(sql)) {

                    stmt.setLong(1, now);
                    stmt.setLong(2, now);

                    try (ResultSet rs = stmt.executeQuery()) {
                        if (rs.next()) {
                            long visibleMessages = rs.getLong("visible_messages");
                            long invisibleMessages = rs.getLong("invisible_messages");

                            queueMetrics.recordQueueMetrics(queueUri, visibleMessages, invisibleMessages);
                        }
                    }
                }

                Thread.sleep(5000); // Poll every 5 seconds

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                LOG.errorf(e, "Error polling embedded queue metrics for [%s]", queueUri);
                try {
                    Thread.sleep(5000);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }
    }

    /**
     * Embedded queue message representation
     */
    private record EmbeddedMessage(
        long id,
        String messageId,
        String messageGroupId,
        String messageJson,
        String receiptHandle,
        int receiveCount
    ) {}

    /**
     * Embedded-specific message callback with ACK/NACK and visibility control.
     * Implements full SQS semantics.
     */
    private class EmbeddedMessageCallback implements MessageCallback, MessageVisibilityControl {
        private final String receiptHandle;

        EmbeddedMessageCallback(String receiptHandle) {
            this.receiptHandle = receiptHandle;
        }

        @Override
        public void ack(MessagePointer message) {
            // ACK: Delete message from queue
            String sql = "DELETE FROM queue_messages WHERE receipt_handle = ?";

            try (Connection conn = dataSource.getConnection();
                 PreparedStatement stmt = conn.prepareStatement(sql)) {

                stmt.setString(1, receiptHandle);
                int deleted = stmt.executeUpdate();

                if (deleted > 0) {
                    LOG.debugf("ACK message [%s] - deleted from queue", message.id());
                } else {
                    LOG.warnf("ACK failed for message [%s] - receipt handle not found", message.id());
                }

            } catch (Exception e) {
                LOG.errorf(e, "Error acknowledging message [%s]", message.id());
            }
        }

        @Override
        public void nack(MessagePointer message) {
            // NACK: Reset visibility to 30 seconds (default retry delay)
            resetVisibilityToDefault(message);
        }

        @Override
        public void setFastFailVisibility(MessagePointer message) {
            // Fast-fail: Set visibility to 1 second for quick retry
            setVisibility(message, 1);
        }

        @Override
        public void resetVisibilityToDefault(MessagePointer message) {
            // Default: Set visibility to 30 seconds for normal retry
            setVisibility(message, 30);
        }

        @Override
        public void extendVisibility(MessagePointer message, int visibilityTimeoutSeconds) {
            // Extend visibility to keep message invisible while processing
            setVisibility(message, visibilityTimeoutSeconds);
        }

        @Override
        public void setVisibilityDelay(MessagePointer message, int delaySeconds) {
            // Set custom delay for retry (used when MediationResponse specifies delaySeconds)
            setVisibility(message, delaySeconds);
        }

        private void setVisibility(MessagePointer message, int seconds) {
            long visibleAt = System.currentTimeMillis() + (seconds * 1000L);

            String sql = "UPDATE queue_messages SET visible_at = ? WHERE receipt_handle = ?";

            try (Connection conn = dataSource.getConnection();
                 PreparedStatement stmt = conn.prepareStatement(sql)) {

                stmt.setLong(1, visibleAt);
                stmt.setString(2, receiptHandle);
                int updated = stmt.executeUpdate();

                if (updated > 0) {
                    LOG.debugf("Set visibility to %ds for message [%s]", seconds, message.id());
                } else {
                    LOG.warnf("Failed to set visibility for message [%s] - receipt handle not found", message.id());
                }

            } catch (Exception e) {
                LOG.errorf(e, "Error setting visibility for message [%s]", message.id());
            }
        }
    }
}

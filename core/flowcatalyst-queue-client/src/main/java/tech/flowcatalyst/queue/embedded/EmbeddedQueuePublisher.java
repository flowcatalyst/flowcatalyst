package tech.flowcatalyst.queue.embedded;

import org.jboss.logging.Logger;
import tech.flowcatalyst.queue.*;

import java.sql.*;
import java.util.ArrayList;
import java.util.List;
import java.util.UUID;

/**
 * SQLite-based embedded queue publisher with full SQS FIFO semantics.
 * Useful for development and single-node deployments.
 *
 * Features:
 * - Message visibility timeout for reliable processing
 * - Receipt handles for ACK/NACK operations
 * - 5-minute deduplication window (matches SQS)
 * - FIFO ordering per message group
 *
 * Supports two modes:
 * - Standalone: Creates and manages its own JDBC connection (dispatch-scheduler)
 * - Shared: Uses an externally provided connection (message-router with Agroal)
 */
public class EmbeddedQueuePublisher implements QueuePublisher {

    private static final Logger LOG = Logger.getLogger(EmbeddedQueuePublisher.class);
    private static final long DEDUP_WINDOW_MS = 5 * 60 * 1000; // 5 minutes

    private final Connection connection;
    private final boolean ownsConnection;

    /**
     * Create a standalone publisher that manages its own connection.
     * Used by dispatch-scheduler and other standalone consumers.
     *
     * @param config Queue configuration with database path
     * @throws SQLException if connection or schema initialization fails
     */
    public EmbeddedQueuePublisher(QueueConfig config) throws SQLException {
        String dbPath = config.embeddedDbPath().orElse(":memory:");
        this.connection = DriverManager.getConnection("jdbc:sqlite:" + dbPath);
        this.ownsConnection = true;
        EmbeddedQueueSchema.initialize(connection);
    }

    /**
     * Create a publisher using an externally managed connection.
     * Used by message-router with Agroal DataSource.
     * Schema must be initialized separately (e.g., by a CDI startup observer).
     *
     * @param connection Externally managed connection
     */
    public EmbeddedQueuePublisher(Connection connection) {
        this.connection = connection;
        this.ownsConnection = false;
    }

    /**
     * Get the underlying connection.
     * Used by consumers that need to share the same database.
     */
    public Connection getConnection() {
        return connection;
    }

    @Override
    public QueuePublishResult publish(QueueMessage message) {
        try {
            // Check deduplication if dedup ID provided
            if (message.deduplicationId() != null && isDuplicate(message.deduplicationId())) {
                LOG.debugf("Message [%s] deduplicated (dedup ID: %s)", message.messageId(), message.deduplicationId());
                return QueuePublishResult.deduplicated(message.messageId());
            }

            long now = System.currentTimeMillis();
            String receiptHandle = UUID.randomUUID().toString();

            // Insert message
            String insertSql = """
                INSERT INTO queue_messages
                (message_id, message_group_id, message_deduplication_id, message_json,
                 created_at, visible_at, receipt_handle, receive_count)
                VALUES (?, ?, ?, ?, ?, ?, ?, 0)
                """;

            try (PreparedStatement stmt = connection.prepareStatement(insertSql)) {
                stmt.setString(1, message.messageId());
                stmt.setString(2, message.messageGroupId());
                stmt.setString(3, message.deduplicationId());
                stmt.setString(4, message.body());
                stmt.setLong(5, now);
                stmt.setLong(6, now); // Immediately visible
                stmt.setString(7, receiptHandle);

                stmt.executeUpdate();
            }

            // Record deduplication entry if dedup ID provided
            if (message.deduplicationId() != null) {
                recordDeduplication(message.deduplicationId(), message.messageId(), now);
            }

            // Periodically clean up old deduplication entries
            cleanupOldDeduplicationEntries(now - DEDUP_WINDOW_MS);

            LOG.debugf("Published message [%s] to group [%s]", message.messageId(), message.messageGroupId());
            return QueuePublishResult.success(message.messageId());

        } catch (SQLException e) {
            LOG.errorf(e, "Failed to publish message [%s] to embedded queue", message.messageId());
            return QueuePublishResult.failure(message.messageId(), e.getMessage());
        }
    }

    @Override
    public QueuePublishResult publishBatch(List<QueueMessage> messages) {
        if (messages.isEmpty()) {
            return QueuePublishResult.success(List.of());
        }

        List<String> published = new ArrayList<>();
        List<String> failed = new ArrayList<>();
        StringBuilder errors = new StringBuilder();

        for (QueueMessage message : messages) {
            QueuePublishResult result = publish(message);
            if (result.deduplicated()) {
                // Deduplicated messages are not failures, just not published
                LOG.debugf("Message [%s] deduplicated in batch", message.messageId());
            } else if (result.success()) {
                published.add(message.messageId());
            } else {
                failed.add(message.messageId());
                if (!errors.isEmpty()) errors.append("; ");
                errors.append(message.messageId()).append(": ").append(result.errorMessage().orElse("unknown error"));
            }
        }

        if (failed.isEmpty()) {
            return QueuePublishResult.success(published);
        } else if (published.isEmpty()) {
            return QueuePublishResult.failure(failed, errors.toString());
        } else {
            return QueuePublishResult.partial(published, failed, errors.toString());
        }
    }

    @Override
    public long getQueueDepth() {
        String sql = "SELECT COUNT(*) FROM queue_messages WHERE visible_at <= ?";
        long now = System.currentTimeMillis();

        try (PreparedStatement stmt = connection.prepareStatement(sql)) {
            stmt.setLong(1, now);

            try (ResultSet rs = stmt.executeQuery()) {
                if (rs.next()) {
                    return rs.getLong(1);
                }
            }
        } catch (SQLException e) {
            LOG.warnf("Failed to get queue depth: %s", e.getMessage());
        }
        return -1;
    }

    @Override
    public QueueType getQueueType() {
        return QueueType.EMBEDDED;
    }

    @Override
    public boolean isHealthy() {
        try {
            return connection != null && !connection.isClosed();
        } catch (SQLException e) {
            return false;
        }
    }

    @Override
    public void close() {
        if (ownsConnection) {
            try {
                if (connection != null && !connection.isClosed()) {
                    connection.close();
                }
            } catch (SQLException e) {
                LOG.warnf("Error closing embedded queue connection: %s", e.getMessage());
            }
        }
    }

    // ========================================================================
    // Consumer support methods (used by message-router's EmbeddedQueueConsumer)
    // ========================================================================

    /**
     * Delete a message by receipt handle (ACK).
     * Called when a message has been successfully processed.
     *
     * @param receiptHandle The receipt handle of the message to delete
     */
    public void deleteMessage(String receiptHandle) {
        String sql = "DELETE FROM queue_messages WHERE receipt_handle = ?";

        try (PreparedStatement stmt = connection.prepareStatement(sql)) {
            stmt.setString(1, receiptHandle);
            int rows = stmt.executeUpdate();

            if (rows > 0) {
                LOG.debugf("Deleted message with receipt handle [%s]", receiptHandle);
            }
        } catch (SQLException e) {
            LOG.errorf(e, "Failed to delete message with receipt handle [%s]", receiptHandle);
        }
    }

    /**
     * Extend or reset visibility timeout (NACK or extend).
     * Sets a new visibility time for the message.
     *
     * @param receiptHandle The receipt handle of the message
     * @param visibilityTimeoutSeconds New visibility timeout in seconds
     */
    public void changeMessageVisibility(String receiptHandle, int visibilityTimeoutSeconds) {
        String sql = "UPDATE queue_messages SET visible_at = ? WHERE receipt_handle = ?";
        long newVisibleAt = System.currentTimeMillis() + (visibilityTimeoutSeconds * 1000L);

        try (PreparedStatement stmt = connection.prepareStatement(sql)) {
            stmt.setLong(1, newVisibleAt);
            stmt.setString(2, receiptHandle);
            int rows = stmt.executeUpdate();

            if (rows > 0) {
                LOG.debugf("Changed visibility for receipt handle [%s] to %d seconds",
                        receiptHandle, visibilityTimeoutSeconds);
            }
        } catch (SQLException e) {
            LOG.errorf(e, "Failed to change visibility for receipt handle [%s]", receiptHandle);
        }
    }

    // ========================================================================
    // Internal helper methods
    // ========================================================================

    /**
     * Check if message with dedup ID was already seen in the last 5 minutes.
     */
    private boolean isDuplicate(String dedupId) throws SQLException {
        String sql = "SELECT message_id FROM message_deduplication WHERE message_deduplication_id = ?";

        try (PreparedStatement stmt = connection.prepareStatement(sql)) {
            stmt.setString(1, dedupId);

            try (ResultSet rs = stmt.executeQuery()) {
                return rs.next();
            }
        }
    }

    /**
     * Record deduplication entry for 5-minute window.
     */
    private void recordDeduplication(String dedupId, String messageId, long timestamp) throws SQLException {
        String sql = """
            INSERT OR REPLACE INTO message_deduplication
            (message_deduplication_id, message_id, created_at)
            VALUES (?, ?, ?)
            """;

        try (PreparedStatement stmt = connection.prepareStatement(sql)) {
            stmt.setString(1, dedupId);
            stmt.setString(2, messageId);
            stmt.setLong(3, timestamp);

            stmt.executeUpdate();
        }
    }

    /**
     * Clean up deduplication entries older than the dedup window.
     */
    private void cleanupOldDeduplicationEntries(long olderThan) {
        try {
            String sql = "DELETE FROM message_deduplication WHERE created_at < ?";

            try (PreparedStatement stmt = connection.prepareStatement(sql)) {
                stmt.setLong(1, olderThan);
                int deleted = stmt.executeUpdate();

                if (deleted > 0) {
                    LOG.debugf("Cleaned up %d old deduplication entries", deleted);
                }
            }
        } catch (SQLException e) {
            LOG.warnf(e, "Error cleaning up deduplication entries");
        }
    }
}

package tech.flowcatalyst.queue.embedded;

import org.jboss.logging.Logger;

import java.sql.Connection;
import java.sql.SQLException;
import java.sql.Statement;

/**
 * SQLite schema for the embedded queue.
 * Provides full SQS FIFO semantics including:
 * - Message visibility timeout
 * - Receipt handles for ACK/NACK
 * - 5-minute deduplication window
 * - Message group ordering
 *
 * This class provides static methods so it can be used from both:
 * - Direct JDBC connections (dispatch-scheduler)
 * - Agroal DataSource connections (message-router)
 */
public final class EmbeddedQueueSchema {

    private static final Logger LOG = Logger.getLogger(EmbeddedQueueSchema.class);

    private EmbeddedQueueSchema() {
        // Utility class
    }

    /**
     * Initialize the embedded queue schema.
     * Creates tables and indexes if they don't exist.
     * Safe to call multiple times (idempotent).
     *
     * @param conn JDBC connection to SQLite database
     * @throws SQLException if schema creation fails
     */
    public static void initialize(Connection conn) throws SQLException {
        LOG.debug("Initializing embedded queue schema...");

        try (Statement stmt = conn.createStatement()) {
            // Main queue table with SQS FIFO semantics
            stmt.execute("""
                CREATE TABLE IF NOT EXISTS queue_messages (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    message_id TEXT UNIQUE NOT NULL,
                    message_group_id TEXT NOT NULL,
                    message_deduplication_id TEXT,
                    message_json TEXT NOT NULL,
                    created_at INTEGER NOT NULL,
                    visible_at INTEGER NOT NULL,
                    receipt_handle TEXT UNIQUE NOT NULL,
                    receive_count INTEGER DEFAULT 0,
                    first_received_at INTEGER
                )
                """);

            // Index for efficient message group ordering with visibility
            stmt.execute("""
                CREATE INDEX IF NOT EXISTS idx_group_visibility
                ON queue_messages(message_group_id, visible_at, id)
                """);

            // Index for finding next available message across groups
            stmt.execute("""
                CREATE INDEX IF NOT EXISTS idx_visibility_id
                ON queue_messages(visible_at, id)
                """);

            // Deduplication tracking (messages seen in last 5 minutes)
            stmt.execute("""
                CREATE TABLE IF NOT EXISTS message_deduplication (
                    message_deduplication_id TEXT PRIMARY KEY,
                    message_id TEXT NOT NULL,
                    created_at INTEGER NOT NULL
                )
                """);

            stmt.execute("""
                CREATE INDEX IF NOT EXISTS idx_dedup_created
                ON message_deduplication(created_at)
                """);

            LOG.debug("Embedded queue schema initialized successfully");
        }
    }
}

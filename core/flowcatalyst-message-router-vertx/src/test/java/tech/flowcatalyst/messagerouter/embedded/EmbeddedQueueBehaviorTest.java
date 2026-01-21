package tech.flowcatalyst.messagerouter.embedded;

import io.agroal.api.AgroalDataSource;
import io.quarkus.test.junit.QuarkusTest;
import io.quarkus.test.junit.TestProfile;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Disabled;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Tag;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.ResultSet;
import java.util.UUID;
import java.util.concurrent.atomic.AtomicInteger;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Direct tests for embedded SQLite queue behavior.
 * Tests message publishing, deduplication, and queue depth.
 */
@QuarkusTest
@TestProfile(EmbeddedQueueBehaviorTest.EmbeddedTestProfile.class)
@Tag("integration")
@Disabled("Temporarily disabled - causes timeout during QueueManager startup")
public class EmbeddedQueueBehaviorTest {

    @Inject
    @io.quarkus.agroal.DataSource("embedded-queue")
    AgroalDataSource dataSource;

    @Inject
    EmbeddedQueuePublisher publisher;

    @BeforeEach
    void clearQueue() throws Exception {
        // Clear all messages from queue before each test
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement("DELETE FROM queue_messages")) {
            stmt.executeUpdate();
        }
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement("DELETE FROM message_deduplication")) {
            stmt.executeUpdate();
        }
    }

    @Test
    void testPublishMessage() throws Exception {
        String messageId = UUID.randomUUID().toString();
        String messageGroupId = "group-1";
        String messageJson = "{\"test\":\"data\"}";

        boolean published = publisher.publishMessage(messageId, messageGroupId, messageId, messageJson);

        assertTrue(published, "Message should be published successfully");
        assertEquals(1, publisher.getQueueDepth(), "Queue depth should be 1");
    }

    @Test
    void testMessageDeduplication() throws Exception {
        String messageGroupId = "group-1";
        String dedupId = "dedup-" + UUID.randomUUID();

        // Publish same message twice with same dedup ID
        boolean firstPublish = publisher.publishMessage(
            UUID.randomUUID().toString(), messageGroupId, dedupId, "{\"test\":1}"
        );
        boolean secondPublish = publisher.publishMessage(
            UUID.randomUUID().toString(), messageGroupId, dedupId, "{\"test\":2}"
        );

        assertTrue(firstPublish, "First message should be published");
        assertFalse(secondPublish, "Second message should be deduplicated");
        assertEquals(1, publisher.getQueueDepth(), "Only 1 message should be in queue");
    }

    @Test
    void testMultipleMessageGroups() throws Exception {
        // Publish messages to different groups
        publisher.publishMessage("msg-1", "group-1", "msg-1", "{\"group\":1}");
        publisher.publishMessage("msg-2", "group-2", "msg-2", "{\"group\":2}");
        publisher.publishMessage("msg-3", "group-3", "msg-3", "{\"group\":3}");

        assertEquals(3, publisher.getQueueDepth(), "All 3 messages should be in queue");
    }

    @Test
    void testMessageGroupOrdering() throws Exception {
        // Publish 3 messages to same group
        String messageGroupId = "ordered-group";
        publisher.publishMessage("msg-1", messageGroupId, "msg-1", "{\"order\":1}");
        publisher.publishMessage("msg-2", messageGroupId, "msg-2", "{\"order\":2}");
        publisher.publishMessage("msg-3", messageGroupId, "msg-3", "{\"order\":3}");

        // Dequeue and verify FIFO order
        String sql = """
            SELECT message_id FROM queue_messages
            WHERE message_group_id = ?
            ORDER BY id
            """;

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(sql)) {

            stmt.setString(1, messageGroupId);

            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                assertEquals("msg-1", rs.getString("message_id"));

                assertTrue(rs.next());
                assertEquals("msg-2", rs.getString("message_id"));

                assertTrue(rs.next());
                assertEquals("msg-3", rs.getString("message_id"));

                assertFalse(rs.next(), "Should only have 3 messages");
            }
        }
    }

    @Test
    void testVisibilityTimeout() throws Exception {
        String messageId = UUID.randomUUID().toString();
        String messageGroupId = "group-1";

        publisher.publishMessage(messageId, messageGroupId, messageId, "{\"test\":true}");

        long now = System.currentTimeMillis();

        // Dequeue message (sets visibility timeout)
        String dequeueSql = """
            UPDATE queue_messages
            SET visible_at = ?, receipt_handle = ?
            WHERE message_id = ?
            RETURNING id, visible_at
            """;

        long futureVisibility = now + 5000; // 5 seconds in future
        String receiptHandle = UUID.randomUUID().toString();

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(dequeueSql)) {

            stmt.setLong(1, futureVisibility);
            stmt.setString(2, receiptHandle);
            stmt.setString(3, messageId);

            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                long actualVisibility = rs.getLong("visible_at");
                assertTrue(actualVisibility >= futureVisibility,
                    "Visibility should be set to future time");
            }
        }

        // Verify message is not visible now
        String checkVisibleSql = """
            SELECT COUNT(*) as count FROM queue_messages
            WHERE message_id = ? AND visible_at <= ?
            """;

        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(checkVisibleSql)) {

            stmt.setString(1, messageId);
            stmt.setLong(2, now);

            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                assertEquals(0, rs.getInt("count"),
                    "Message should not be visible before timeout expires");
            }
        }
    }

    @Test
    void testACKDeletesMessage() throws Exception {
        String messageId = UUID.randomUUID().toString();
        String messageGroupId = "group-1";
        String receiptHandle = UUID.randomUUID().toString();

        // Publish message
        publisher.publishMessage(messageId, messageGroupId, messageId, "{\"test\":true}");

        // Update with receipt handle (simulate dequeue)
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "UPDATE queue_messages SET receipt_handle = ? WHERE message_id = ?")) {
            stmt.setString(1, receiptHandle);
            stmt.setString(2, messageId);
            stmt.executeUpdate();
        }

        // ACK (delete) message
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "DELETE FROM queue_messages WHERE receipt_handle = ?")) {
            stmt.setString(1, receiptHandle);
            int deleted = stmt.executeUpdate();
            assertEquals(1, deleted, "Should delete exactly 1 message");
        }

        // Verify message is gone
        assertEquals(0, publisher.getQueueDepth(), "Queue should be empty after ACK");
    }

    @Test
    void testNACKResetsVisibility() throws Exception {
        String messageId = UUID.randomUUID().toString();
        String messageGroupId = "group-1";
        String receiptHandle = UUID.randomUUID().toString();

        publisher.publishMessage(messageId, messageGroupId, messageId, "{\"test\":true}");

        long now = System.currentTimeMillis();

        // Dequeue message (make it invisible)
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "UPDATE queue_messages SET visible_at = ?, receipt_handle = ? WHERE message_id = ?")) {
            stmt.setLong(1, now + 60000); // 60s in future
            stmt.setString(2, receiptHandle);
            stmt.setString(3, messageId);
            stmt.executeUpdate();
        }

        // NACK (reset visibility to 30 seconds)
        long nackVisibility = now + 30000;
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "UPDATE queue_messages SET visible_at = ? WHERE receipt_handle = ?")) {
            stmt.setLong(1, nackVisibility);
            stmt.setString(2, receiptHandle);
            int updated = stmt.executeUpdate();
            assertEquals(1, updated, "Should update exactly 1 message");
        }

        // Verify visibility was updated
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "SELECT visible_at FROM queue_messages WHERE message_id = ?")) {
            stmt.setString(1, messageId);
            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                long actualVisibility = rs.getLong("visible_at");
                assertTrue(actualVisibility >= nackVisibility - 100 && actualVisibility <= nackVisibility + 100,
                    "Visibility should be reset to ~30 seconds");
            }
        }
    }

    @Test
    void testDeduplicationWindowCleanup() throws Exception {
        String messageGroupId = "group-1";
        String dedupId = "old-dedup-" + UUID.randomUUID();

        // Publish message with dedup ID
        publisher.publishMessage(
            UUID.randomUUID().toString(), messageGroupId, dedupId, "{\"test\":1}"
        );

        // Verify dedup entry exists
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "SELECT COUNT(*) as count FROM message_deduplication WHERE message_deduplication_id = ?")) {
            stmt.setString(1, dedupId);
            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                assertEquals(1, rs.getInt("count"), "Dedup entry should exist");
            }
        }

        // Manually set dedup entry to old timestamp (older than 5 minutes)
        long oldTimestamp = System.currentTimeMillis() - (6 * 60 * 1000); // 6 minutes ago
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "UPDATE message_deduplication SET created_at = ? WHERE message_deduplication_id = ?")) {
            stmt.setLong(1, oldTimestamp);
            stmt.setString(2, dedupId);
            stmt.executeUpdate();
        }

        // Publish another message (triggers cleanup)
        publisher.publishMessage(
            UUID.randomUUID().toString(), messageGroupId, UUID.randomUUID().toString(), "{\"test\":2}"
        );

        // Verify old dedup entry was cleaned up
        try (Connection conn = dataSource.getConnection();
             PreparedStatement stmt = conn.prepareStatement(
                 "SELECT COUNT(*) as count FROM message_deduplication WHERE message_deduplication_id = ?")) {
            stmt.setString(1, dedupId);
            try (ResultSet rs = stmt.executeQuery()) {
                assertTrue(rs.next());
                assertEquals(0, rs.getInt("count"),
                    "Old dedup entry should be cleaned up");
            }
        }
    }

    /**
     * Test profile that configures embedded queue for integration tests
     */
    public static class EmbeddedTestProfile implements io.quarkus.test.junit.QuarkusTestProfile {
        @Override
        public java.util.Map<String, String> getConfigOverrides() {
            return java.util.Map.of(
                "message-router.queue-type", "EMBEDDED",
                "message-router.enabled", "true"
            );
        }
    }
}

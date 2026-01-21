package tech.flowcatalyst.messagerouter.embedded;

import io.agroal.api.AgroalDataSource;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.queue.QueueMessage;
import tech.flowcatalyst.queue.QueuePublishResult;

import java.sql.Connection;
import java.sql.SQLException;

/**
 * CDI wrapper for the embedded queue publisher.
 * Delegates to queue-client's EmbeddedQueuePublisher using Agroal DataSource.
 *
 * This provides a CDI-injectable bean for message-router while using the shared
 * schema and implementation from queue-client.
 */
@ApplicationScoped
public class EmbeddedQueuePublisher {

    private static final Logger LOG = Logger.getLogger(EmbeddedQueuePublisher.class);

    @Inject
    @io.quarkus.agroal.DataSource("embedded-queue")
    AgroalDataSource dataSource;

    /**
     * Publish a message to the embedded queue.
     *
     * @param messageId Message ID (must be unique)
     * @param messageGroupId Message group ID for FIFO ordering
     * @param messageDeduplicationId Optional deduplication ID
     * @param messageJson JSON payload
     * @return true if published, false if deduplicated
     */
    public boolean publishMessage(
            String messageId,
            String messageGroupId,
            String messageDeduplicationId,
            String messageJson) {

        try (Connection conn = dataSource.getConnection()) {
            tech.flowcatalyst.queue.embedded.EmbeddedQueuePublisher publisher =
                new tech.flowcatalyst.queue.embedded.EmbeddedQueuePublisher(conn);

            QueueMessage message = new QueueMessage(
                messageId,
                messageGroupId,
                messageDeduplicationId,
                messageJson
            );

            QueuePublishResult result = publisher.publish(message);

            if (result.deduplicated()) {
                LOG.debugf("Message [%s] deduplicated (dedup ID: %s)", messageId, messageDeduplicationId);
                return false;
            }

            if (result.success()) {
                LOG.debugf("Published message [%s] to group [%s]", messageId, messageGroupId);
                return true;
            }

            LOG.errorf("Failed to publish message [%s]: %s", messageId, result.errorMessage());
            throw new RuntimeException("Failed to publish message: " + result.errorMessage());

        } catch (SQLException e) {
            LOG.errorf(e, "Error publishing message [%s]", messageId);
            throw new RuntimeException("Failed to publish message", e);
        }
    }

    /**
     * Get queue depth (approximate message count).
     */
    public long getQueueDepth() {
        try (Connection conn = dataSource.getConnection()) {
            tech.flowcatalyst.queue.embedded.EmbeddedQueuePublisher publisher =
                new tech.flowcatalyst.queue.embedded.EmbeddedQueuePublisher(conn);
            return publisher.getQueueDepth();
        } catch (SQLException e) {
            LOG.errorf(e, "Error getting queue depth");
            return 0;
        }
    }
}

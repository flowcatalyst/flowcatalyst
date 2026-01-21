package tech.flowcatalyst.queue.nats;

import io.nats.client.Connection;
import io.nats.client.JetStream;
import io.nats.client.api.PublishAck;
import io.nats.client.impl.NatsMessage;
import org.jboss.logging.Logger;
import tech.flowcatalyst.queue.*;

import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.List;

/**
 * NATS JetStream implementation of QueuePublisher.
 * Uses JetStream for durable message delivery with acknowledgment.
 */
public class NatsQueuePublisher implements QueuePublisher {

    private static final Logger LOG = Logger.getLogger(NatsQueuePublisher.class);

    private final Connection connection;
    private final JetStream jetStream;
    private final String subject;
    private final String streamName;

    public NatsQueuePublisher(Connection connection, QueueConfig config) {
        this.connection = connection;
        this.subject = config.queueUrl(); // Subject pattern like "flowcatalyst.dispatch"
        this.streamName = config.natsStreamName().orElse("FLOWCATALYST");

        try {
            this.jetStream = connection.jetStream();
        } catch (Exception e) {
            throw new IllegalStateException("Failed to create JetStream context", e);
        }
    }

    @Override
    public QueuePublishResult publish(QueueMessage message) {
        try {
            String publishSubject = buildSubject(message);

            NatsMessage natsMessage = NatsMessage.builder()
                .subject(publishSubject)
                .data(message.body().getBytes(StandardCharsets.UTF_8))
                .build();

            PublishAck ack = jetStream.publish(natsMessage);

            LOG.debugf("Published message [%s] to NATS subject [%s], stream seq: %d",
                message.messageId(), publishSubject, ack.getSeqno());

            return QueuePublishResult.success(message.messageId());

        } catch (Exception e) {
            LOG.errorf(e, "Failed to publish message [%s] to NATS", message.messageId());
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
            if (result.success()) {
                published.add(message.messageId());
            } else {
                failed.add(message.messageId());
                if (!errors.isEmpty()) errors.append("; ");
                errors.append(message.messageId()).append(": ")
                    .append(result.errorMessage().orElse("Unknown error"));
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

    /**
     * Build the full subject for a message.
     * If messageGroupId is provided, append it to create subject-based routing.
     * Example: "flowcatalyst.dispatch.client123"
     */
    private String buildSubject(QueueMessage message) {
        if (message.messageGroupId() != null && !message.messageGroupId().isEmpty()) {
            return subject + "." + message.messageGroupId();
        }
        return subject;
    }

    @Override
    public long getQueueDepth() {
        try {
            var jsm = connection.jetStreamManagement();
            var streamInfo = jsm.getStreamInfo(streamName);
            return streamInfo.getStreamState().getMsgCount();
        } catch (Exception e) {
            LOG.warnf("Failed to get NATS queue depth: %s", e.getMessage());
            return -1;
        }
    }

    @Override
    public QueueType getQueueType() {
        return QueueType.NATS;
    }

    @Override
    public boolean isHealthy() {
        try {
            return connection.getStatus() == Connection.Status.CONNECTED;
        } catch (Exception e) {
            LOG.warnf("NATS health check failed: %s", e.getMessage());
            return false;
        }
    }

    @Override
    public void close() {
        // Connection is managed by CDI producer, don't close it here
    }
}
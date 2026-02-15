package tech.flowcatalyst.dispatchjob.queue;

import org.jboss.logging.Logger;
import tech.flowcatalyst.queue.QueueMessage;
import tech.flowcatalyst.queue.QueuePublishResult;
import tech.flowcatalyst.queue.QueuePublisher;
import tech.flowcatalyst.queue.QueueType;

import java.util.List;

/**
 * No-op queue publisher for when messaging is disabled.
 * Silently succeeds for all publish operations.
 */
public class NoOpQueuePublisher implements QueuePublisher {

    private static final Logger LOG = Logger.getLogger(NoOpQueuePublisher.class);

    @Override
    public QueuePublishResult publish(QueueMessage message) {
        LOG.debugf("No-op publish: messageId=%s", message.messageId());
        return QueuePublishResult.success(message.messageId());
    }

    @Override
    public QueuePublishResult publishBatch(List<QueueMessage> messages) {
        LOG.debugf("No-op publishBatch: %d messages", messages.size());
        var ids = messages.stream().map(QueueMessage::messageId).toList();
        return QueuePublishResult.success(ids);
    }

    @Override
    public long getQueueDepth() {
        return 0;
    }

    @Override
    public QueueType getQueueType() {
        return QueueType.EMBEDDED;
    }

    @Override
    public boolean isHealthy() {
        return true;
    }
}

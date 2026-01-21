package tech.flowcatalyst.outbox.processor;

import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;

import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Semaphore;

/**
 * Routes outbox items to the appropriate MessageGroupProcessor based on type and message group.
 * Maintains a map of active processors and a semaphore for limiting concurrent processing.
 */
@ApplicationScoped
public class GroupDistributor {

    private static final Logger LOG = Logger.getLogger(GroupDistributor.class);

    @Inject
    OutboxProcessorConfig config;

    @Inject
    MessageGroupProcessorFactory processorFactory;

    private final ConcurrentHashMap<String, MessageGroupProcessor> processors = new ConcurrentHashMap<>();
    private Semaphore concurrencySemaphore;

    void onStart(@Observes StartupEvent event) {
        concurrencySemaphore = new Semaphore(config.maxConcurrentGroups());
        LOG.infof("GroupDistributor initialized with max %d concurrent groups", config.maxConcurrentGroups());
    }

    /**
     * Distribute an item to its message group processor.
     * Creates a new processor if one doesn't exist for the group.
     *
     * @param item The outbox item to process
     */
    public void distribute(OutboxItem item) {
        // Create a unique key combining type and message group
        String groupKey = item.type() + ":" + (item.messageGroup() != null ? item.messageGroup() : "default");

        MessageGroupProcessor processor = processors.computeIfAbsent(
            groupKey,
            k -> {
                LOG.debugf("Creating new processor for group: %s", groupKey);
                return processorFactory.create(item.type(), item.messageGroup(), concurrencySemaphore);
            }
        );

        processor.enqueue(item);
    }

    /**
     * Get the number of active message group processors.
     */
    public int getActiveProcessorCount() {
        return processors.size();
    }

    /**
     * Get the total number of items queued across all processors.
     */
    public int getTotalQueuedItems() {
        return processors.values().stream()
            .mapToInt(MessageGroupProcessor::getQueueSize)
            .sum();
    }
}

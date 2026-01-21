package tech.flowcatalyst.outbox.processor;

import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;

import java.util.List;
import java.util.concurrent.ArrayBlockingQueue;
import java.util.concurrent.BlockingQueue;
import java.util.concurrent.atomic.AtomicBoolean;

/**
 * In-memory buffer between the poller and the group processors.
 * Provides backpressure when processing can't keep up with polling.
 */
@ApplicationScoped
public class GlobalBuffer {

    private static final Logger LOG = Logger.getLogger(GlobalBuffer.class);

    @Inject
    OutboxProcessorConfig config;

    @Inject
    GroupDistributor distributor;

    private BlockingQueue<OutboxItem> buffer;
    private final AtomicBoolean running = new AtomicBoolean(false);

    void onStart(@Observes StartupEvent event) {
        if (!config.enabled()) {
            LOG.info("Outbox processor disabled, GlobalBuffer not started");
            return;
        }

        buffer = new ArrayBlockingQueue<>(config.globalBufferSize());
        running.set(true);

        // Start distributor thread
        Thread.startVirtualThread(this::runDistributor);
        LOG.infof("GlobalBuffer started with capacity %d", config.globalBufferSize());
    }

    void onShutdown(@Observes ShutdownEvent event) {
        running.set(false);
        LOG.info("GlobalBuffer shutdown initiated");
    }

    /**
     * Add items to the buffer for processing.
     * Returns the number of items that could not be added (buffer full).
     *
     * @param items List of items to add
     * @return Number of items that could not be added due to buffer full
     */
    public int addAll(List<OutboxItem> items) {
        int rejected = 0;
        for (OutboxItem item : items) {
            if (!buffer.offer(item)) {
                rejected++;
                // Item stays in PROCESSING, will be recovered by crash recovery
                LOG.warnf("Buffer full, item %s will be recovered later", item.id());
            }
        }
        return rejected;
    }

    /**
     * Get the current number of items in the buffer.
     */
    public int getBufferSize() {
        return buffer != null ? buffer.size() : 0;
    }

    /**
     * Get the buffer capacity.
     */
    public int getBufferCapacity() {
        return buffer != null ? buffer.remainingCapacity() + buffer.size() : 0;
    }

    private void runDistributor() {
        LOG.info("Distributor thread started");

        while (running.get()) {
            try {
                // Block until an item is available (with timeout to check running flag)
                OutboxItem item = buffer.poll(java.time.Duration.ofMillis(100).toMillis(), java.util.concurrent.TimeUnit.MILLISECONDS);
                if (item != null) {
                    distributor.distribute(item);
                }
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                LOG.info("Distributor thread interrupted");
                break;
            }
        }

        // Drain remaining items on shutdown
        OutboxItem item;
        while ((item = buffer.poll()) != null) {
            distributor.distribute(item);
        }

        LOG.info("Distributor thread stopped");
    }
}

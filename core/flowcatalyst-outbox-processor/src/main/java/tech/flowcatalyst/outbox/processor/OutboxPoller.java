package tech.flowcatalyst.outbox.processor;

import io.quarkus.runtime.StartupEvent;
import io.quarkus.scheduler.Scheduled;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.repository.OutboxRepository;
import tech.flowcatalyst.standby.StandbyService;

import java.util.List;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Scheduled service that polls the outbox tables for pending items.
 * Uses a single-poller, status-based architecture with NO row locking.
 *
 * <p>Architecture:
 * <ol>
 *   <li>Check if sufficient capacity before polling</li>
 *   <li>Fetch pending items (simple SELECT)</li>
 *   <li>Mark items as in-progress IMMEDIATELY after fetch</li>
 *   <li>Add items to global buffer</li>
 * </ol>
 *
 * <p>Only runs on the primary instance when hot standby is enabled.
 */
@ApplicationScoped
public class OutboxPoller {

    private static final Logger LOG = Logger.getLogger(OutboxPoller.class);

    @Inject
    StandbyService standbyService;

    @Inject
    OutboxProcessorConfig config;

    @Inject
    OutboxRepository repository;

    @Inject
    GlobalBuffer globalBuffer;

    private final AtomicBoolean polling = new AtomicBoolean(false);
    private final AtomicInteger inFlightCount = new AtomicInteger(0);

    /**
     * Crash recovery on startup - reset stuck items (status=9) back to pending (status=0).
     */
    void onStartup(@Observes StartupEvent event) {
        if (!config.enabled()) {
            return;
        }

        LOG.info("Running crash recovery on startup...");
        doCrashRecovery();
    }

    /**
     * Performs crash recovery - resets stuck items back to pending.
     */
    private void doCrashRecovery() {
        for (OutboxItemType type : OutboxItemType.values()) {
            try {
                List<OutboxItem> stuckItems = repository.fetchStuckItems(type);
                if (!stuckItems.isEmpty()) {
                    List<String> ids = stuckItems.stream().map(OutboxItem::id).toList();
                    repository.resetStuckItems(type, ids);
                    LOG.infof("Reset %d stuck %s items during crash recovery", ids.size(), type);
                }
            } catch (Exception e) {
                LOG.errorf(e, "Error during crash recovery for %s", type);
            }
        }
    }

    /**
     * Main polling loop - runs at configured interval.
     * Polls both events and dispatch jobs tables.
     */
    @Scheduled(every = "${outbox-processor.poll-interval:1s}", concurrentExecution = Scheduled.ConcurrentExecution.SKIP)
    void poll() {
        if (!config.enabled()) {
            return;
        }

        if (!standbyService.isPrimary()) {
            LOG.trace("Not primary instance, skipping poll");
            return;
        }

        if (!polling.compareAndSet(false, true)) {
            LOG.debug("Previous poll still in progress, skipping");
            return;
        }

        try {
            // Check if there's sufficient capacity BEFORE polling
            int currentInFlight = inFlightCount.get();
            int availableSlots = config.maxInFlight() - currentInFlight;

            if (availableSlots < config.pollBatchSize()) {
                LOG.debugf("Skipping poll - insufficient capacity (available=%d, needed=%d)",
                    availableSlots, config.pollBatchSize());
                return;
            }

            // Poll events
            pollItemType(OutboxItemType.EVENT);

            // Poll dispatch jobs
            pollItemType(OutboxItemType.DISPATCH_JOB);

        } catch (Exception e) {
            LOG.errorf(e, "Error during poll cycle");
        } finally {
            polling.set(false);
        }
    }

    /**
     * Polls items of a specific type.
     */
    private void pollItemType(OutboxItemType type) {
        try {
            // 1. Fetch pending items (simple SELECT, no locking)
            List<OutboxItem> items = repository.fetchPending(type, config.pollBatchSize());

            if (items.isEmpty()) {
                return;
            }

            // 2. Mark as in-progress IMMEDIATELY (before buffering)
            List<String> ids = items.stream().map(OutboxItem::id).toList();
            repository.markAsInProgress(type, ids);

            // 3. Acquire in-flight permits for the actual fetched count
            inFlightCount.addAndGet(items.size());

            LOG.debugf("Fetched and marked %d %s items as in-progress", items.size(), type);

            // 4. Add to buffer
            int rejected = globalBuffer.addAll(items);
            if (rejected > 0) {
                LOG.warnf("Buffer rejected %d items - items remain in-progress and will be recovered on restart", rejected);
            }

        } catch (Exception e) {
            LOG.errorf(e, "Error polling %s items", type);
        }
    }

    /**
     * Release in-flight permits when processing completes.
     * Called by the message group processor after API calls complete.
     */
    public void releaseInFlight(int count) {
        int newCount = inFlightCount.addAndGet(-count);
        LOG.tracef("Released %d in-flight permits, remaining=%d", count, newCount);
    }

    /**
     * Get current in-flight count for metrics.
     */
    public int getInFlightCount() {
        return inFlightCount.get();
    }

    /**
     * Periodic recovery - resets items that have been in error states
     * longer than the configured timeout back to PENDING for retry.
     *
     * <p>Recovers items with these statuses:
     * <ul>
     *   <li>IN_PROGRESS (9) - stuck items</li>
     *   <li>BAD_REQUEST (2) - may be fixed by config changes</li>
     *   <li>INTERNAL_ERROR (3) - server may have recovered</li>
     *   <li>UNAUTHORIZED (4) - credentials may have been updated</li>
     *   <li>FORBIDDEN (5) - permissions may have been granted</li>
     *   <li>GATEWAY_ERROR (6) - gateway may have recovered</li>
     * </ul>
     */
    @Scheduled(every = "${outbox-processor.recovery-interval:60s}", concurrentExecution = Scheduled.ConcurrentExecution.SKIP)
    void periodicRecovery() {
        if (!config.enabled()) {
            return;
        }

        if (!standbyService.isPrimary()) {
            LOG.trace("Not primary instance, skipping periodic recovery");
            return;
        }

        for (OutboxItemType type : OutboxItemType.values()) {
            try {
                List<OutboxItem> recoverableItems = repository.fetchRecoverableItems(
                    type,
                    config.processingTimeoutSeconds(),
                    config.pollBatchSize()
                );

                if (!recoverableItems.isEmpty()) {
                    List<String> ids = recoverableItems.stream().map(OutboxItem::id).toList();
                    repository.resetRecoverableItems(type, ids);
                    LOG.infof("Periodic recovery: reset %d %s items back to PENDING", ids.size(), type);
                }
            } catch (Exception e) {
                LOG.errorf(e, "Error during periodic recovery for %s", type);
            }
        }
    }
}

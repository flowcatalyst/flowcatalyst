package tech.flowcatalyst.dispatchscheduler;

import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;

import java.util.Queue;
import java.util.concurrent.ConcurrentLinkedQueue;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.Consumer;

/**
 * In-memory queue for a single message group.
 *
 * Ensures only 1 job is on the external queue at a time per message group.
 * Uses virtual threads for async dispatch.
 */
public class MessageGroupQueue {

    private static final Logger LOG = Logger.getLogger(MessageGroupQueue.class);

    private final String messageGroup;
    private final Queue<DispatchJob> pendingJobs = new ConcurrentLinkedQueue<>();
    private final AtomicBoolean hasJobInFlight = new AtomicBoolean(false);
    private final AtomicInteger pendingCount = new AtomicInteger(0);
    private final Consumer<DispatchJob> dispatchFunction;

    public MessageGroupQueue(String messageGroup, Consumer<DispatchJob> dispatchFunction) {
        this.messageGroup = messageGroup;
        this.dispatchFunction = dispatchFunction;
    }

    /**
     * Add jobs to this message group queue.
     * Jobs are sorted by sequence number and creation time before dispatching.
     *
     * @param jobs List of jobs to add
     */
    public void addJobs(java.util.List<DispatchJob> jobs) {
        // Sort by sequence then by createdAt for consistent ordering
        jobs.stream()
            .sorted((a, b) -> {
                int seqCompare = Integer.compare(a.sequence, b.sequence);
                if (seqCompare != 0) return seqCompare;
                return a.createdAt.compareTo(b.createdAt);
            })
            .forEach(job -> {
                pendingJobs.add(job);
                pendingCount.incrementAndGet();
            });

        LOG.debugf("Added %d jobs to message group [%s], pending count: %d",
            (Object) jobs.size(), messageGroup, pendingCount.get());

        tryDispatchNext();
    }

    /**
     * Called when the current in-flight job has been dispatched to the external queue.
     * Triggers dispatch of the next job in this group.
     */
    public void onCurrentJobDispatched() {
        hasJobInFlight.set(false);
        tryDispatchNext();
    }

    /**
     * Try to dispatch the next job if no job is currently in flight.
     * Uses virtual thread for async execution.
     */
    private void tryDispatchNext() {
        if (hasJobInFlight.compareAndSet(false, true)) {
            DispatchJob next = pendingJobs.poll();
            if (next != null) {
                pendingCount.decrementAndGet();
                LOG.debugf("Dispatching job [%s] for message group [%s]", next.id, messageGroup);

                // Use virtual thread for dispatch
                Thread.startVirtualThread(() -> {
                    try {
                        dispatchFunction.accept(next);
                    } catch (Exception e) {
                        LOG.errorf(e, "Failed to dispatch job [%s] for message group [%s]",
                            next.id, messageGroup);
                        // On failure, release the in-flight flag so next job can be processed
                        onCurrentJobDispatched();
                    }
                });
            } else {
                // No more jobs, release the in-flight flag
                hasJobInFlight.set(false);
            }
        }
    }

    /**
     * Check if this queue has any pending jobs.
     */
    public boolean hasPendingJobs() {
        return pendingCount.get() > 0;
    }

    /**
     * Get the number of pending jobs in this queue.
     */
    public int getPendingCount() {
        return pendingCount.get();
    }

    /**
     * Check if there's a job currently being dispatched.
     */
    public boolean hasJobInFlight() {
        return hasJobInFlight.get();
    }

    /**
     * Get the message group ID for this queue.
     */
    public String getMessageGroup() {
        return messageGroup;
    }
}

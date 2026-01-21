package tech.flowcatalyst.dispatchscheduler;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;

import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.Semaphore;

/**
 * Coordinates dispatch across message groups.
 *
 * Maintains in-memory queues per message group and ensures only 1 job
 * per group is dispatched to the external queue at a time.
 *
 * Uses a semaphore to limit concurrent group dispatches.
 */
@ApplicationScoped
public class MessageGroupDispatcher {

    private static final Logger LOG = Logger.getLogger(MessageGroupDispatcher.class);

    private final Map<String, MessageGroupQueue> groupQueues = new ConcurrentHashMap<>();
    private Semaphore concurrencySemaphore;

    @Inject
    DispatchSchedulerConfig config;

    @Inject
    JobDispatcher jobDispatcher;

    /**
     * Initialize the dispatcher with configured concurrency limits.
     */
    public void initialize() {
        this.concurrencySemaphore = new Semaphore(config.maxConcurrentGroups());
        LOG.infof("MessageGroupDispatcher initialized with max concurrent groups: %d",
            config.maxConcurrentGroups());
    }

    /**
     * Submit jobs for a message group.
     * Only 1 job per group is dispatched to the external queue at a time.
     *
     * @param messageGroup The message group identifier
     * @param jobs List of jobs for this group
     */
    public void submitJobs(String messageGroup, List<DispatchJob> jobs) {
        if (jobs.isEmpty()) {
            return;
        }

        MessageGroupQueue queue = groupQueues.computeIfAbsent(
            messageGroup,
            k -> new MessageGroupQueue(k, this::dispatchJob)
        );

        queue.addJobs(jobs);
    }

    /**
     * Called when a job has been processed by the external queue.
     * Triggers dispatch of the next job in the group.
     *
     * @param messageGroup The message group
     * @param jobId The job that was processed
     */
    public void onJobProcessed(String messageGroup, String jobId) {
        MessageGroupQueue queue = groupQueues.get(messageGroup);
        if (queue != null) {
            LOG.debugf("Job [%s] processed for group [%s], triggering next dispatch", jobId, messageGroup);
            queue.onCurrentJobDispatched();
        }
    }

    /**
     * Internal dispatch function called by MessageGroupQueue.
     * Uses semaphore to limit concurrent dispatches.
     */
    private void dispatchJob(DispatchJob job) {
        try {
            // Acquire semaphore permit (blocks if max concurrent reached)
            concurrencySemaphore.acquire();
            try {
                boolean success = jobDispatcher.dispatch(job);

                if (success) {
                    LOG.debugf("Successfully dispatched job [%s] for group [%s]",
                        job.id, job.messageGroup);
                } else {
                    LOG.warnf("Failed to dispatch job [%s] for group [%s]",
                        job.id, job.messageGroup);
                }

                // Trigger next dispatch in this group
                MessageGroupQueue queue = groupQueues.get(job.messageGroup);
                if (queue != null) {
                    queue.onCurrentJobDispatched();
                }

            } finally {
                concurrencySemaphore.release();
            }

        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            LOG.warnf("Dispatch interrupted for job [%s]", job.id);
        }
    }

    /**
     * Get the number of active message group queues.
     */
    public int getActiveGroupCount() {
        return (int) groupQueues.values().stream()
            .filter(q -> q.hasPendingJobs() || q.hasJobInFlight())
            .count();
    }

    /**
     * Get total pending jobs across all groups.
     */
    public int getTotalPendingJobs() {
        return groupQueues.values().stream()
            .mapToInt(MessageGroupQueue::getPendingCount)
            .sum();
    }

    /**
     * Get available permits in the concurrency semaphore.
     */
    public int getAvailablePermits() {
        return concurrencySemaphore != null ? concurrencySemaphore.availablePermits() : 0;
    }

    /**
     * Clean up empty queues (no pending jobs and no in-flight jobs).
     */
    public void cleanupEmptyQueues() {
        groupQueues.entrySet().removeIf(entry -> {
            MessageGroupQueue queue = entry.getValue();
            return !queue.hasPendingJobs() && !queue.hasJobInFlight();
        });
    }
}

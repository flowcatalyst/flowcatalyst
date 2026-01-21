package tech.flowcatalyst.dispatchscheduler;

import io.quarkus.scheduler.Scheduled;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.standby.StandbyService;

import java.time.Instant;
import java.util.List;

/**
 * Safety net poller for stale QUEUED jobs.
 *
 * <p>This service periodically checks for dispatch jobs that have been in QUEUED
 * status for too long (indicating they were never picked up from the queue, possibly
 * due to queue delivery failure after creation).</p>
 *
 * <p>Stale QUEUED jobs are reset to PENDING status so they can be picked up by
 * the regular {@link PendingJobPoller} and re-sent to the queue.</p>
 *
 * <p>This provides a safety net for the synchronous event-to-dispatch pipeline:
 * if queue send fails during event/job creation, the job will eventually be
 * recovered by this poller.</p>
 */
@ApplicationScoped
public class StaleQueuedJobPoller {

    private static final Logger LOG = Logger.getLogger(StaleQueuedJobPoller.class);
    private static final int MAX_BATCH_SIZE = 100;

    @Inject
    StandbyService standbyService;

    @Inject
    DispatchSchedulerConfig config;

    @Inject
    DispatchJobRepository dispatchJobRepository;

    /**
     * Scheduled task to poll for stale QUEUED jobs.
     * Uses configurable poll interval (default: 60 seconds).
     */
    @Scheduled(every = "${dispatch-scheduler.stale-queued-poll-interval:60s}", identity = "stale-queued-job-poller")
    void pollStaleQueuedJobs() {
        if (!config.enabled()) {
            return;
        }

        // Only run on primary instance
        if (!standbyService.isPrimary()) {
            LOG.trace("Not primary instance, skipping stale QUEUED poll");
            return;
        }

        try {
            doPoll();
        } catch (Exception e) {
            LOG.errorf(e, "Error polling for stale QUEUED jobs");
        }
    }

    /**
     * Main polling logic.
     */
    private void doPoll() {
        int thresholdMinutes = config.staleQueuedThresholdMinutes();
        Instant threshold = Instant.now().minusSeconds(thresholdMinutes * 60L);

        // Find QUEUED jobs older than threshold
        List<DispatchJob> staleJobs = dispatchJobRepository.findStaleQueued(threshold, MAX_BATCH_SIZE);

        if (staleJobs.isEmpty()) {
            LOG.trace("No stale QUEUED jobs found");
            return;
        }

        LOG.infof("Found %d stale QUEUED jobs (older than %d minutes), resetting to PENDING",
            staleJobs.size(), thresholdMinutes);

        // Reset to PENDING for re-processing by PendingJobPoller
        List<String> ids = staleJobs.stream()
            .map(j -> j.id)
            .toList();

        dispatchJobRepository.updateStatusBatch(ids, DispatchStatus.PENDING);

        LOG.infof("Reset %d stale QUEUED jobs to PENDING status", ids.size());
    }
}

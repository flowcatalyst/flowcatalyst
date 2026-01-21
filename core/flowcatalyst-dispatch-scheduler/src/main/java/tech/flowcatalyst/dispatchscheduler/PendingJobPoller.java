package tech.flowcatalyst.dispatchscheduler;

import io.quarkus.runtime.StartupEvent;
import io.quarkus.scheduler.Scheduled;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.standby.StandbyService;

import java.util.*;
import java.util.stream.Collectors;

/**
 * Polls for PENDING dispatch jobs and submits them to the MessageGroupDispatcher.
 *
 * This service runs on a configurable schedule and only executes on the
 * primary instance (using StandbyService for leader election).
 */
@ApplicationScoped
public class PendingJobPoller {

    private static final Logger LOG = Logger.getLogger(PendingJobPoller.class);
    private static final String DEFAULT_MESSAGE_GROUP = "default";

    @Inject
    StandbyService standbyService;

    @Inject
    DispatchSchedulerConfig config;

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    MessageGroupDispatcher groupDispatcher;

    @Inject
    BlockOnErrorChecker blockOnErrorChecker;

    void onStart(@Observes StartupEvent ev) {
        if (!config.enabled()) {
            LOG.info("Dispatch scheduler is disabled");
            return;
        }

        groupDispatcher.initialize();
        LOG.infof("Dispatch scheduler started with poll interval: %s, batch size: %d",
            config.pollInterval(), config.batchSize());
    }

    /**
     * Scheduled task to poll for pending jobs.
     * Uses configurable poll interval (default: 5 seconds).
     */
    @Scheduled(every = "${dispatch-scheduler.poll-interval:5s}", identity = "dispatch-scheduler-poll")
    void pollPendingJobs() {
        if (!config.enabled()) {
            return;
        }

        // Only run on primary instance
        if (!standbyService.isPrimary()) {
            LOG.trace("Not primary instance, skipping poll");
            return;
        }

        try {
            doPoll();
        } catch (Exception e) {
            LOG.errorf(e, "Error polling for pending jobs");
        }
    }

    /**
     * Main polling logic.
     */
    private void doPoll() {
        // 1. Query PENDING jobs (batch size from config)
        List<DispatchJob> pendingJobs = dispatchJobRepository.findPendingJobs(config.batchSize());

        if (pendingJobs.isEmpty()) {
            LOG.trace("No pending jobs found");
            return;
        }

        LOG.debugf("Found %d pending jobs to process", pendingJobs.size());

        // 2. Group jobs by messageGroup
        Map<String, List<DispatchJob>> jobsByGroup = pendingJobs.stream()
            .collect(Collectors.groupingBy(
                job -> job.messageGroup != null ? job.messageGroup : DEFAULT_MESSAGE_GROUP
            ));

        // 3. Check for blocked groups (BLOCK_ON_ERROR mode)
        Set<String> blockedGroups = blockOnErrorChecker.getBlockedGroups(jobsByGroup.keySet());

        // 4. Process each group
        for (Map.Entry<String, List<DispatchJob>> entry : jobsByGroup.entrySet()) {
            String messageGroup = entry.getKey();
            List<DispatchJob> groupJobs = entry.getValue();

            // Check if group is blocked
            if (blockedGroups.contains(messageGroup)) {
                LOG.debugf("Message group [%s] is blocked due to ERROR jobs, skipping %d jobs",
                    messageGroup, groupJobs.size());
                continue;
            }

            // Filter jobs based on DispatchMode
            List<DispatchJob> dispatchableJobs = filterByDispatchMode(messageGroup, groupJobs, blockedGroups);

            if (!dispatchableJobs.isEmpty()) {
                LOG.debugf("Submitting %d jobs for message group [%s]",
                    dispatchableJobs.size(), messageGroup);
                groupDispatcher.submitJobs(messageGroup, dispatchableJobs);
            }
        }

        // 5. Periodic cleanup of empty queues
        groupDispatcher.cleanupEmptyQueues();
    }

    /**
     * Filter jobs based on DispatchMode rules.
     *
     * IMMEDIATE: Always dispatch
     * NEXT_ON_ERROR: Skip failed groups but continue with others
     * BLOCK_ON_ERROR: Block entire group on any error
     */
    private List<DispatchJob> filterByDispatchMode(
            String messageGroup,
            List<DispatchJob> jobs,
            Set<String> blockedGroups) {

        List<DispatchJob> result = new ArrayList<>();

        for (DispatchJob job : jobs) {
            DispatchMode mode = job.mode != null ? job.mode : DispatchMode.IMMEDIATE;

            switch (mode) {
                case IMMEDIATE:
                    // Always dispatch
                    result.add(job);
                    break;

                case NEXT_ON_ERROR:
                    // Dispatch if no ERROR jobs ahead in queue
                    // (implementation simplified: dispatch if group not fully blocked)
                    if (!blockedGroups.contains(messageGroup)) {
                        result.add(job);
                    }
                    break;

                case BLOCK_ON_ERROR:
                    // Only dispatch if no ERROR jobs in group
                    if (!blockedGroups.contains(messageGroup)) {
                        result.add(job);
                    }
                    break;
            }
        }

        return result;
    }
}

package tech.flowcatalyst.dispatchscheduler;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;

import java.util.HashSet;
import java.util.Set;

/**
 * Checks if message groups are blocked due to ERROR status jobs.
 *
 * For BLOCK_ON_ERROR mode, ANY error in a message group blocks further
 * dispatching for that group until the error is resolved.
 */
@ApplicationScoped
public class BlockOnErrorChecker {

    private static final Logger LOG = Logger.getLogger(BlockOnErrorChecker.class);

    @Inject
    DispatchJobRepository dispatchJobRepository;

    /**
     * Check if a message group is blocked due to ERROR status jobs.
     *
     * @param messageGroup The message group to check
     * @return true if the group has any ERROR jobs
     */
    public boolean isGroupBlocked(String messageGroup) {
        if (messageGroup == null) {
            return false;
        }

        long errorCount = dispatchJobRepository.countByMessageGroupAndStatus(messageGroup, DispatchStatus.ERROR);
        boolean blocked = errorCount > 0;

        if (blocked) {
            LOG.debugf("Message group [%s] is blocked with %d ERROR jobs", messageGroup, errorCount);
        }

        return blocked;
    }

    /**
     * Get blocked groups from a set of message groups.
     *
     * @param messageGroups Set of message groups to check
     * @return Set of groups that have ERROR jobs
     */
    public Set<String> getBlockedGroups(Set<String> messageGroups) {
        if (messageGroups == null || messageGroups.isEmpty()) {
            return Set.of();
        }

        Set<String> blockedGroups = new HashSet<>();
        for (String group : messageGroups) {
            if (isGroupBlocked(group)) {
                blockedGroups.add(group);
            }
        }

        return blockedGroups;
    }
}

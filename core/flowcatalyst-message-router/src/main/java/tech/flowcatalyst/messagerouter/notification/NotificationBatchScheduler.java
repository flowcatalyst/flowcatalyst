package tech.flowcatalyst.messagerouter.notification;

import io.quarkus.scheduler.Scheduled;
import io.smallrye.common.annotation.RunOnVirtualThread;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;

/**
 * Scheduler that triggers batched notification sending.
 * This is separate from BatchingNotificationService to avoid CDI issues
 * with constructor injection vs @Scheduled methods.
 */
@ApplicationScoped
public class NotificationBatchScheduler {

    private static final Logger LOG = Logger.getLogger(NotificationBatchScheduler.class);

    @Inject
    BatchingNotificationService batchingService;

    private volatile boolean shutdownInProgress = false;

    @jakarta.annotation.PreDestroy
    void onShutdown() {
        shutdownInProgress = true;
        LOG.info("NotificationBatchScheduler shutting down - scheduled tasks will exit");
    }

    /**
     * Trigger batch send every 5 minutes
     */
    @Scheduled(every = "${notification.batch.interval:5m}")
    @RunOnVirtualThread
    void triggerBatchSend() {
        if (shutdownInProgress) {
            return;
        }

        try {
            batchingService.sendBatch();
        } catch (Exception e) {
            LOG.errorf(e, "Error triggering batch send");
        }
    }
}

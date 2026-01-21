package tech.flowcatalyst.messagerouter.notification;

import jakarta.enterprise.context.ApplicationScoped;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.Warning;

/**
 * Placeholder notification service that logs notifications instead of sending them.
 * In the future, this can be replaced with implementations for email, Slack, PagerDuty, etc.
 */
@ApplicationScoped
public class NoOpNotificationService implements NotificationService {

    private static final Logger LOG = Logger.getLogger(NoOpNotificationService.class);

    @Override
    public void notifyWarning(Warning warning) {
        LOG.infof("NOTIFICATION [WARNING]: [%s] %s - %s (source: %s)",
            warning.severity(), warning.category(), warning.message(), warning.source());
    }

    @Override
    public void notifyCriticalError(String message, String source) {
        LOG.errorf("NOTIFICATION [CRITICAL]: %s (source: %s)", message, source);
    }

    @Override
    public void notifySystemEvent(String eventType, String message) {
        LOG.infof("NOTIFICATION [EVENT]: [%s] %s", eventType, message);
    }

    @Override
    public boolean isEnabled() {
        return false; // Placeholder implementation is not enabled
    }
}

package tech.flowcatalyst.messagerouter.notification;

import tech.flowcatalyst.messagerouter.model.Warning;

/**
 * Service for sending notifications about system events and warnings
 */
public interface NotificationService {

    /**
     * Send a notification for a warning
     */
    void notifyWarning(Warning warning);

    /**
     * Send a notification for a critical error
     */
    void notifyCriticalError(String message, String source);

    /**
     * Send a notification for a system event
     */
    void notifySystemEvent(String eventType, String message);

    /**
     * Check if notifications are enabled
     */
    boolean isEnabled();
}

package tech.flowcatalyst.standby;

/**
 * Warning service interface for standby module.
 * Consumers of this module should provide an implementation.
 *
 * This is a minimal interface that covers only what the standby module needs.
 */
public interface StandbyWarningService {

    /**
     * Add a warning.
     *
     * @param id       Unique warning identifier
     * @param severity Severity level (e.g., "CRITICAL", "WARN", "INFO")
     * @param title    Short title for the warning
     * @param message  Detailed warning message
     */
    void addWarning(String id, String severity, String title, String message);

    /**
     * Acknowledge (dismiss) a warning by ID.
     *
     * @param id The warning ID to acknowledge
     */
    void acknowledgeWarning(String id);
}

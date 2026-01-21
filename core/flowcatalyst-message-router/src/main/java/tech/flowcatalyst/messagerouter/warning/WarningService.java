package tech.flowcatalyst.messagerouter.warning;

import tech.flowcatalyst.messagerouter.model.Warning;

import java.util.List;

/**
 * Service for managing system warnings
 */
public interface WarningService {

    /**
     * Add a new warning
     */
    void addWarning(String category, String severity, String message, String source);

    /**
     * Get all warnings
     */
    List<Warning> getAllWarnings();

    /**
     * Get warnings by severity
     */
    List<Warning> getWarningsBySeverity(String severity);

    /**
     * Get unacknowledged warnings
     */
    List<Warning> getUnacknowledgedWarnings();

    /**
     * Acknowledge a warning
     */
    boolean acknowledgeWarning(String warningId);

    /**
     * Clear all warnings
     */
    void clearAllWarnings();

    /**
     * Clear warnings older than specified hours
     */
    void clearOldWarnings(int hoursOld);
}

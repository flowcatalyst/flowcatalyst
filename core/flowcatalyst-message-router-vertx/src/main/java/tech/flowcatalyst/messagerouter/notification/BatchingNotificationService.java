package tech.flowcatalyst.messagerouter.notification;

import jakarta.enterprise.inject.Vetoed;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.Warning;

import java.time.Instant;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentLinkedQueue;
import java.util.stream.Collectors;

/**
 * Batching wrapper for notification services.
 * Collects warnings over a configurable interval (default 5 minutes)
 * and sends a single summary notification to all registered delegates.
 * Only sends notifications for warnings at or above the configured minimum severity.
 * This class is created by NotificationServiceProducer and must be @Vetoed to avoid CDI discovery.
 */
@Vetoed
public class BatchingNotificationService implements NotificationService {

    private static final Logger LOG = Logger.getLogger(BatchingNotificationService.class);

    private final List<NotificationService> delegates;
    private final String minSeverity;
    private final Queue<Warning> warningBatch = new ConcurrentLinkedQueue<>();
    private final Map<String, Integer> categoryCount = new ConcurrentHashMap<>();
    private Instant batchStartTime = Instant.now();

    private static final List<String> SEVERITY_ORDER = List.of("INFO", "WARNING", "ERROR", "CRITICAL");

    public BatchingNotificationService(List<NotificationService> delegates, String minSeverity) {
        this.delegates = delegates;
        this.minSeverity = minSeverity;
        LOG.infof("BatchingNotificationService initialized with %d delegates, min severity: %s",
            delegates.size(), minSeverity);
    }

    @Override
    public void notifyWarning(Warning warning) {
        // Only batch warnings at or above minimum severity
        if (meetsMinSeverity(warning.severity())) {
            warningBatch.add(warning);
            categoryCount.merge(warning.category(), 1, Integer::sum);
        }
    }

    @Override
    public void notifyCriticalError(String message, String source) {
        // Critical errors always sent
        Warning warning = createWarning("CRITICAL_ERROR", "CRITICAL", message, source);
        warningBatch.add(warning);
        categoryCount.merge("CRITICAL_ERROR", 1, Integer::sum);
    }

    @Override
    public void notifySystemEvent(String eventType, String message) {
        // System events sent as INFO (may be filtered)
        if (meetsMinSeverity("INFO")) {
            Warning warning = createWarning("SYSTEM_EVENT_" + eventType, "INFO", message, "System");
            warningBatch.add(warning);
            categoryCount.merge("SYSTEM_EVENT_" + eventType, 1, Integer::sum);
        }
    }

    /**
     * Send batched notifications.
     * Called by NotificationBatchScheduler.
     */
    public void sendBatch() {
        if (warningBatch.isEmpty()) {
            LOG.debug("No warnings to send in this batch period");
            return;
        }

        try {
            // Create summary
            List<Warning> warnings = new ArrayList<>(warningBatch);
            Instant batchEndTime = Instant.now();

            LOG.infof("Sending batched notification: %d warnings from %s to %s",
                warnings.size(), batchStartTime, batchEndTime);

            // Group warnings by severity
            Map<String, List<Warning>> warningsBySeverity = warnings.stream()
                .collect(Collectors.groupingBy(Warning::severity));

            // Send summary to all delegates
            for (NotificationService delegate : delegates) {
                try {
                    sendSummaryToDelegate(delegate, warnings, warningsBySeverity, batchStartTime, batchEndTime);
                } catch (Exception e) {
                    LOG.errorf(e, "Failed to send notification via delegate: %s", delegate.getClass().getSimpleName());
                }
            }

            // Clear batch
            warningBatch.clear();
            categoryCount.clear();
            batchStartTime = Instant.now();

        } catch (Exception e) {
            LOG.errorf(e, "Error sending batched notifications");
        }
    }

    /**
     * Send summary notification to a single delegate
     */
    private void sendSummaryToDelegate(NotificationService delegate,
                                       List<Warning> allWarnings,
                                       Map<String, List<Warning>> warningsBySeverity,
                                       Instant startTime,
                                       Instant endTime) {

        // Build summary message
        StringBuilder summary = new StringBuilder();
        summary.append(String.format("FlowCatalyst Warning Summary (%s to %s)\n\n",
            startTime, endTime));

        // Add warnings by severity (in order: CRITICAL, ERROR, WARNING, INFO)
        for (String severity : SEVERITY_ORDER) {
            List<Warning> warningsForSeverity = warningsBySeverity.get(severity);
            if (warningsForSeverity != null && !warningsForSeverity.isEmpty()) {
                summary.append(String.format("%s Issues (%d):\n", severity, warningsForSeverity.size()));

                // Group by category and show counts
                Map<String, List<Warning>> byCategory = warningsForSeverity.stream()
                    .collect(Collectors.groupingBy(Warning::category));

                byCategory.forEach((category, categoryWarnings) -> {
                    if (categoryWarnings.size() == 1) {
                        Warning w = categoryWarnings.get(0);
                        summary.append(String.format("  - %s: %s\n", category, w.message()));
                    } else {
                        summary.append(String.format("  - %s: %d occurrences\n",
                            category, categoryWarnings.size()));
                        // Show first warning as example
                        summary.append(String.format("    Example: %s\n",
                            categoryWarnings.get(0).message()));
                    }
                });
                summary.append("\n");
            }
        }

        summary.append(String.format("Total Warnings: %d\n", allWarnings.size()));

        // Create synthetic warning with summary
        Warning summaryWarning = createWarning(
            "BATCH_SUMMARY",
            getHighestSeverity(warningsBySeverity.keySet()),
            summary.toString(),
            "BatchingNotificationService"
        );

        // Send to delegate
        delegate.notifyWarning(summaryWarning);
    }

    /**
     * Get the highest severity from a set of severities
     */
    private String getHighestSeverity(Set<String> severities) {
        return SEVERITY_ORDER.stream()
            .filter(severities::contains)
            .reduce((first, second) -> second) // Last one is highest
            .orElse("INFO");
    }

    /**
     * Check if severity meets minimum threshold
     */
    private boolean meetsMinSeverity(String severity) {
        int minIndex = SEVERITY_ORDER.indexOf(minSeverity);
        int severityIndex = SEVERITY_ORDER.indexOf(severity);
        return severityIndex >= minIndex;
    }

    /**
     * Create a warning instance
     */
    private Warning createWarning(String category, String severity, String message, String source) {
        return new Warning(
            UUID.randomUUID().toString(),
            category,
            severity,
            message,
            Instant.now(),
            source,
            false
        );
    }

    @Override
    public boolean isEnabled() {
        return !delegates.isEmpty() && delegates.stream().anyMatch(NotificationService::isEnabled);
    }
}

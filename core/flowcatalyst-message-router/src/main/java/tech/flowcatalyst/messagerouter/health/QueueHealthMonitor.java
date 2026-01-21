package tech.flowcatalyst.messagerouter.health;

import io.quarkus.scheduler.Scheduled;
import io.smallrye.common.annotation.RunOnVirtualThread;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueStats;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Monitors queue health and generates warnings for operational issues:
 * - QUEUE_BACKLOG: Queue depth exceeds threshold
 * - QUEUE_GROWING: Queue growing for 3+ consecutive check periods
 */
@ApplicationScoped
public class QueueHealthMonitor {

    private static final Logger LOG = Logger.getLogger(QueueHealthMonitor.class);

    @ConfigProperty(name = "queue.health.monitor.enabled", defaultValue = "true")
    boolean monitorEnabled;

    @ConfigProperty(name = "queue.health.backlog.threshold", defaultValue = "1000")
    long backlogThreshold;

    @ConfigProperty(name = "queue.health.growth.threshold", defaultValue = "100")
    long growthThreshold;

    @Inject
    QueueMetricsService queueMetricsService;

    @Inject
    WarningService warningService;

    // Track queue size history for growth detection
    private final Map<String, QueueSizeHistory> queueHistory = new ConcurrentHashMap<>();
    private volatile boolean shutdownInProgress = false;

    @jakarta.annotation.PreDestroy
    void onShutdown() {
        shutdownInProgress = true;
        LOG.info("QueueHealthMonitor shutting down - scheduled tasks will exit");
    }

    /**
     * Monitor queue health every 30 seconds
     */
    @Scheduled(every = "30s")
    @RunOnVirtualThread
    void monitorQueueHealth() {
        if (!monitorEnabled || shutdownInProgress) {
            return;
        }

        try {
            Map<String, QueueStats> allStats = queueMetricsService.getAllQueueStats();

            for (Map.Entry<String, QueueStats> entry : allStats.entrySet()) {
                String queueName = entry.getKey();
                QueueStats stats = entry.getValue();

                checkQueueBacklog(queueName, stats);
                checkQueueGrowth(queueName, stats);
            }
        } catch (Exception e) {
            LOG.errorf(e, "Error monitoring queue health");
        }
    }

    /**
     * Check if queue depth exceeds backlog threshold
     */
    private void checkQueueBacklog(String queueName, QueueStats stats) {
        long currentSize = stats.pendingMessages();

        if (currentSize > backlogThreshold) {
            LOG.warnf("Queue backlog detected: %s has %d pending messages (threshold: %d)",
                queueName, currentSize, backlogThreshold);

            warningService.addWarning(
                "QUEUE_BACKLOG",
                "WARNING",
                String.format("Queue %s depth is %d (threshold: %d)",
                    queueName, currentSize, backlogThreshold),
                "QueueHealthMonitor"
            );
        }
    }

    /**
     * Check if queue is growing for 3+ consecutive periods (90 seconds)
     */
    private void checkQueueGrowth(String queueName, QueueStats stats) {
        long currentSize = stats.pendingMessages();

        QueueSizeHistory history = queueHistory.computeIfAbsent(
            queueName,
            k -> new QueueSizeHistory()
        );

        long previousSize = history.lastSize;
        history.lastSize = currentSize;

        // Skip first check (no history yet)
        if (previousSize < 0) {
            return;
        }

        long growth = currentSize - previousSize;

        if (growth >= growthThreshold) {
            // Queue is growing
            history.consecutiveGrowthPeriods++;

            if (history.consecutiveGrowthPeriods >= 3) {
                LOG.warnf("Queue growth detected: %s has grown for %d consecutive periods (current: %d, growth: +%d)",
                    queueName, history.consecutiveGrowthPeriods, currentSize, growth);

                warningService.addWarning(
                    "QUEUE_GROWING",
                    "WARNING",
                    String.format("Queue %s growing for %d periods (current depth: %d, growth rate: +%d/30s)",
                        queueName, history.consecutiveGrowthPeriods, currentSize, growth),
                    "QueueHealthMonitor"
                );

                // Don't keep incrementing forever - cap at 10 to avoid warning spam
                if (history.consecutiveGrowthPeriods > 10) {
                    history.consecutiveGrowthPeriods = 10;
                }
            }
        } else {
            // Reset counter if queue stopped growing
            if (history.consecutiveGrowthPeriods > 0) {
                LOG.debugf("Queue %s stopped growing (was growing for %d periods)",
                    queueName, history.consecutiveGrowthPeriods);
            }
            history.consecutiveGrowthPeriods = 0;
        }
    }

    /**
     * Tracks queue size history for growth detection
     */
    private static class QueueSizeHistory {
        long lastSize = -1; // -1 = no history yet
        int consecutiveGrowthPeriods = 0;
    }
}

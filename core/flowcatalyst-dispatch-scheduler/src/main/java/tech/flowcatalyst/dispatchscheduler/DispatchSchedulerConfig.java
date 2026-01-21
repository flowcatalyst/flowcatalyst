package tech.flowcatalyst.dispatchscheduler;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import tech.flowcatalyst.queue.QueueType;

import java.util.Optional;

/**
 * Configuration for the dispatch scheduler service.
 */
@ConfigMapping(prefix = "dispatch-scheduler")
public interface DispatchSchedulerConfig {

    /**
     * Whether the scheduler is enabled.
     */
    @WithDefault("true")
    boolean enabled();

    /**
     * Interval between polling for pending jobs.
     * Uses Quarkus duration format (e.g., "5s", "1m", "500ms").
     */
    @WithDefault("5s")
    String pollInterval();

    /**
     * Number of jobs to fetch per poll cycle.
     */
    @WithDefault("20")
    int batchSize();

    /**
     * Maximum number of message groups being dispatched concurrently.
     */
    @WithDefault("10")
    int maxConcurrentGroups();

    /**
     * Type of queue to publish to.
     */
    @WithDefault("EMBEDDED")
    QueueType queueType();

    /**
     * Queue URL (for SQS) or queue name (for others).
     */
    Optional<String> queueUrl();

    /**
     * Path for embedded SQLite database.
     */
    @WithDefault("./dispatch-queue.db")
    String embeddedDbPath();

    /**
     * Processing endpoint URL that receives dispatched jobs.
     */
    @WithDefault("http://localhost:8080/api/dispatch/process")
    String processingEndpoint();

    /**
     * Default dispatch pool code to use when none specified.
     */
    @WithDefault("DISPATCH-POOL")
    String defaultDispatchPoolCode();

    /**
     * Threshold in minutes for considering QUEUED jobs as stale.
     * Jobs in QUEUED status older than this are reset to PENDING.
     */
    @WithDefault("15")
    int staleQueuedThresholdMinutes();

    /**
     * Interval between polling for stale QUEUED jobs.
     * Uses Quarkus duration format (e.g., "60s", "1m").
     */
    @WithDefault("60s")
    String staleQueuedPollInterval();
}

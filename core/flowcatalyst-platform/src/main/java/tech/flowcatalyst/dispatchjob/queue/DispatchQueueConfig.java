package tech.flowcatalyst.dispatchjob.queue;

import io.smallrye.config.ConfigMapping;
import io.smallrye.config.WithDefault;
import io.smallrye.config.WithName;
import tech.flowcatalyst.queue.QueueType;

import java.util.Optional;

/**
 * Configuration for the dispatch job queue publisher.
 */
@ConfigMapping(prefix = "flowcatalyst.dispatch")
public interface DispatchQueueConfig {

    /**
     * Queue type: SQS, NATS, EMBEDDED.
     */
    @WithName("queue-type")
    @WithDefault("SQS")
    QueueType queueType();

    /**
     * Queue URL (for SQS) or subject (for NATS).
     */
    @WithName("queue-url")
    Optional<String> queueUrl();

    /**
     * Processing endpoint URL that receives dispatched jobs.
     */
    @WithName("processing-endpoint")
    @WithDefault("http://localhost:8080/api/dispatch/process")
    String processingEndpoint();

    /**
     * Default dispatch pool code when none specified on the job.
     */
    @WithName("default-pool-code")
    @WithDefault("DISPATCH-POOL")
    String defaultPoolCode();
}

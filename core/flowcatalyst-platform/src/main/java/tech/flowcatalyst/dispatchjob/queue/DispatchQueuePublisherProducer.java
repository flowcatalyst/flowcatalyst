package tech.flowcatalyst.dispatchjob.queue;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Produces;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.queue.QueueConfig;
import tech.flowcatalyst.queue.QueuePublisher;
import tech.flowcatalyst.queue.QueuePublisherFactory;
import tech.flowcatalyst.queue.QueueType;

/**
 * CDI producer that creates a {@link QueuePublisher} for dispatch job publishing.
 *
 * <p>When messaging is disabled, produces a no-op publisher that silently succeeds.
 * Otherwise, creates a publisher based on the configured queue type.</p>
 */
@ApplicationScoped
public class DispatchQueuePublisherProducer {

    private static final Logger LOG = Logger.getLogger(DispatchQueuePublisherProducer.class);

    @Inject
    DispatchQueueConfig dispatchConfig;

    @Inject
    QueuePublisherFactory queuePublisherFactory;

    @ConfigProperty(name = "flowcatalyst.features.messaging-enabled", defaultValue = "true")
    boolean messagingEnabled;

    @Produces
    @ApplicationScoped
    @DispatchQueue
    public QueuePublisher dispatchQueuePublisher() {
        if (!messagingEnabled) {
            LOG.info("Messaging disabled — using no-op dispatch queue publisher");
            return new NoOpQueuePublisher();
        }

        var queueType = dispatchConfig.queueType();
        var queueUrl = dispatchConfig.queueUrl();

        QueueConfig queueConfig = switch (queueType) {
            case SQS -> QueueConfig.sqsFifo(queueUrl.orElseThrow(
                () -> new IllegalStateException("flowcatalyst.dispatch.queue-url is required for SQS")));
            case EMBEDDED -> QueueConfig.embeddedInMemory();
            case NATS -> QueueConfig.nats(queueUrl.orElse("flowcatalyst.dispatch"));
            case ACTIVEMQ -> QueueConfig.activeMq(queueUrl.orElse("dispatch-queue"));
        };

        LOG.infof("Initializing dispatch queue publisher: type=%s, url=%s",
            queueType, queueUrl.orElse("(default)"));

        return queuePublisherFactory.create(queueConfig);
    }
}

package tech.flowcatalyst.queue;

import io.nats.client.Connection;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import tech.flowcatalyst.queue.embedded.EmbeddedQueuePublisher;
import tech.flowcatalyst.queue.nats.NatsQueuePublisher;
import tech.flowcatalyst.queue.sqs.SqsQueuePublisher;

import java.sql.SQLException;

/**
 * Factory for creating QueuePublisher instances based on configuration.
 */
@ApplicationScoped
public class QueuePublisherFactory {

    private static final Logger LOG = Logger.getLogger(QueuePublisherFactory.class);

    @Inject
    Instance<SqsClient> sqsClientInstance;

    @Inject
    Instance<Connection> natsConnectionInstance;

    /**
     * Create a QueuePublisher based on the provided configuration.
     *
     * @param config Queue configuration
     * @return QueuePublisher instance
     * @throws IllegalStateException if the queue type is not supported or dependencies are missing
     */
    public QueuePublisher create(QueueConfig config) {
        return switch (config.queueType()) {
            case SQS -> createSqsPublisher(config);
            case NATS -> createNatsPublisher(config);
            case EMBEDDED -> createEmbeddedPublisher(config);
            case ACTIVEMQ -> throw new UnsupportedOperationException(
                "ActiveMQ publisher not yet implemented");
        };
    }

    private QueuePublisher createSqsPublisher(QueueConfig config) {
        if (sqsClientInstance.isUnsatisfied()) {
            throw new IllegalStateException(
                "SqsClient not available. Add quarkus-amazon-sqs dependency and configure AWS credentials.");
        }

        SqsClient sqsClient = sqsClientInstance.get();
        LOG.infof("Creating SQS publisher for queue: %s (fifo=%s)",
            config.queueUrl(), config.fifoEnabled());

        return new SqsQueuePublisher(sqsClient, config);
    }

    private QueuePublisher createNatsPublisher(QueueConfig config) {
        if (natsConnectionInstance.isUnsatisfied()) {
            throw new IllegalStateException(
                "NATS Connection not available. Add jnats dependency and configure NATS connection.");
        }

        Connection connection = natsConnectionInstance.get();
        LOG.infof("Creating NATS publisher for subject: %s (stream=%s)",
            config.queueUrl(), config.natsStreamName().orElse("FLOWCATALYST"));

        return new NatsQueuePublisher(connection, config);
    }

    private QueuePublisher createEmbeddedPublisher(QueueConfig config) {
        String dbPath = config.embeddedDbPath().orElse(":memory:");
        LOG.infof("Creating embedded queue publisher with database: %s", dbPath);

        try {
            return new EmbeddedQueuePublisher(config);
        } catch (SQLException e) {
            throw new IllegalStateException("Failed to create embedded queue publisher", e);
        }
    }
}

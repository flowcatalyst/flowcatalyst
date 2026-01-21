package tech.flowcatalyst.messagerouter.factory;

import io.nats.client.Connection;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.jms.ConnectionFactory;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.config.QueueType;
import io.agroal.api.AgroalDataSource;
import tech.flowcatalyst.messagerouter.consumer.ActiveMqQueueConsumer;
import tech.flowcatalyst.messagerouter.consumer.NatsQueueConsumer;
import tech.flowcatalyst.messagerouter.consumer.QueueConsumer;
import tech.flowcatalyst.messagerouter.consumer.SqsQueueConsumer;
import tech.flowcatalyst.messagerouter.embedded.EmbeddedQueueConsumer;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.warning.WarningService;

@ApplicationScoped
public class QueueConsumerFactoryImpl implements QueueConsumerFactory {

    private static final Logger LOG = Logger.getLogger(QueueConsumerFactoryImpl.class);

    @ConfigProperty(name = "message-router.queue-type")
    QueueType queueType;

    @ConfigProperty(name = "message-router.sqs.max-messages-per-poll")
    int sqsMaxMessagesPerPoll;

    @ConfigProperty(name = "message-router.sqs.wait-time-seconds")
    int sqsWaitTimeSeconds;

    @ConfigProperty(name = "message-router.activemq.receive-timeout-ms")
    int activemqReceiveTimeoutMs;

    @ConfigProperty(name = "message-router.metrics.poll-interval-seconds")
    int metricsPollIntervalSeconds;

    @ConfigProperty(name = "message-router.embedded.visibility-timeout-seconds", defaultValue = "30")
    int embeddedVisibilityTimeoutSeconds;

    @ConfigProperty(name = "message-router.embedded.receive-timeout-ms", defaultValue = "1000")
    int embeddedReceiveTimeoutMs;

    // NATS configuration
    @ConfigProperty(name = "message-router.nats.stream-name", defaultValue = "FLOWCATALYST")
    String natsStreamName;

    @ConfigProperty(name = "message-router.nats.consumer-name", defaultValue = "flowcatalyst-router")
    String natsConsumerName;

    @ConfigProperty(name = "message-router.nats.max-messages-per-poll", defaultValue = "10")
    int natsMaxMessagesPerPoll;

    @ConfigProperty(name = "message-router.nats.poll-timeout-seconds", defaultValue = "20")
    int natsPollTimeoutSeconds;

    @Inject
    QueueManager queueManager;

    @Inject
    QueueMetricsService queueMetrics;

    @Inject
    WarningService warningService;

    @Inject
    SqsClient sqsClient;

    @Inject
    ConnectionFactory connectionFactory;

    @Inject
    Connection natsConnection;

    @Inject
    @io.quarkus.agroal.DataSource("embedded-queue")
    AgroalDataSource embeddedQueueDataSource;

    @Override
    public QueueConsumer createConsumer(QueueConfig queueConfig, int connections) {
        LOG.infof("Creating %s consumer with %d connections", queueType, connections);

        return switch (queueType) {
            case SQS -> {
                String queueUrl = queueConfig.queueUri();
                LOG.infof("Using SYNC SQS consumer for queue [%s]", queueUrl);
                yield new SqsQueueConsumer(
                    sqsClient,
                    queueUrl,
                    connections,
                    queueManager,
                    queueMetrics,
                    warningService,
                    sqsMaxMessagesPerPoll,
                    sqsWaitTimeSeconds,
                    metricsPollIntervalSeconds
                );
            }
            case ACTIVEMQ -> {
                String queueUri = queueConfig.queueUri();
                LOG.infof("Using ActiveMQ consumer for queue [%s]", queueUri);
                yield new ActiveMqQueueConsumer(
                    connectionFactory,
                    queueUri,
                    connections,
                    queueManager,
                    queueMetrics,
                    warningService,
                    activemqReceiveTimeoutMs,
                    metricsPollIntervalSeconds
                );
            }
            case NATS -> {
                String subject = queueConfig.queueUri();  // Use queueUri as subject filter
                LOG.infof("Using NATS JetStream consumer for stream [%s], subject [%s]", natsStreamName, subject);
                yield new NatsQueueConsumer(
                    natsConnection,
                    natsStreamName,
                    natsConsumerName,
                    subject,
                    connections,
                    queueManager,
                    queueMetrics,
                    warningService,
                    natsMaxMessagesPerPoll,
                    natsPollTimeoutSeconds,
                    metricsPollIntervalSeconds
                );
            }
            case EMBEDDED -> {
                String queueUri = queueConfig.queueUri();
                LOG.infof("Using Embedded SQLite Queue consumer for queue [%s]", queueUri);
                yield new EmbeddedQueueConsumer(
                    embeddedQueueDataSource,
                    queueUri,
                    connections,
                    queueManager,
                    queueMetrics,
                    warningService,
                    embeddedVisibilityTimeoutSeconds,
                    embeddedReceiveTimeoutMs
                );
            }
        };
    }
}

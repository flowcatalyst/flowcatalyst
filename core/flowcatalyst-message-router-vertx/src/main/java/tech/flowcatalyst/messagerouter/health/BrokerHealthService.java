package tech.flowcatalyst.messagerouter.health;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import io.nats.client.Connection.Status;
import jakarta.jms.ConnectionFactory;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.GetQueueUrlRequest;
import software.amazon.awssdk.services.sqs.model.GetQueueUrlResponse;
import tech.flowcatalyst.messagerouter.config.QueueType;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Service for checking broker (SQS/ActiveMQ) connectivity and health.
 * Provides explicit health checks for external messaging dependencies.
 */
@ApplicationScoped
public class BrokerHealthService {

    private static final Logger LOG = Logger.getLogger(BrokerHealthService.class);

    @ConfigProperty(name = "message-router.queue-type")
    QueueType queueType;

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @Inject
    SqsClient sqsClient;

    @Inject
    ConnectionFactory connectionFactory;

    @Inject
    io.nats.client.Connection natsConnection;

    @Inject
    MeterRegistry meterRegistry;

    // Metrics
    private Counter connectionAttempts;
    private Counter connectionSuccesses;
    private Counter connectionFailures;
    private AtomicInteger brokerAvailable;

    /**
     * Initialize metrics on startup
     */
    @PostConstruct
    void init() {
        connectionAttempts = Counter.builder("flowcatalyst.broker.connection.attempts")
            .description("Total broker connection attempts")
            .register(meterRegistry);

        connectionSuccesses = Counter.builder("flowcatalyst.broker.connection.successes")
            .description("Successful broker connections")
            .register(meterRegistry);

        connectionFailures = Counter.builder("flowcatalyst.broker.connection.failures")
            .description("Failed broker connections")
            .register(meterRegistry);

        brokerAvailable = new AtomicInteger(0);
        meterRegistry.gauge("flowcatalyst.broker.available", brokerAvailable);

        LOG.info("BrokerHealthService initialized with metrics");
    }

    /**
     * Check broker connectivity based on configured queue type.
     * This is a quick connectivity check, not a full queue validation.
     *
     * @return list of issues found, empty if healthy
     */
    public List<String> checkBrokerConnectivity() {
        if (!messageRouterEnabled) {
            LOG.debug("Message router disabled, skipping broker connectivity check");
            return List.of();
        }

        List<String> issues = new ArrayList<>();

        connectionAttempts.increment();

        try {
            boolean connected = switch (queueType) {
                case SQS -> checkSqsConnectivity();
                case ACTIVEMQ -> checkActiveMqConnectivity();
                case NATS -> checkNatsConnectivity();
                case EMBEDDED -> true; // Embedded queue is always available (SQLite)
            };

            if (connected) {
                connectionSuccesses.increment();
                brokerAvailable.set(1);
                LOG.debugf("Broker connectivity check passed for %s", queueType);
            } else {
                connectionFailures.increment();
                brokerAvailable.set(0);
                issues.add(String.format("%s broker is not accessible", queueType));
            }

        } catch (Exception e) {
            connectionFailures.increment();
            brokerAvailable.set(0);
            LOG.errorf(e, "Broker connectivity check failed for %s", queueType);
            issues.add(String.format("%s broker connectivity check failed: %s", queueType, e.getMessage()));
        }

        return issues;
    }

    /**
     * Check if a specific SQS queue is accessible.
     * This verifies both AWS credentials and queue existence.
     *
     * @param queueName the queue name to check
     * @return list of issues found, empty if accessible
     */
    public List<String> checkSqsQueueAccessible(String queueName) {
        List<String> issues = new ArrayList<>();

        try {
            GetQueueUrlResponse response = sqsClient.getQueueUrl(
                GetQueueUrlRequest.builder()
                    .queueName(queueName)
                    .build()
            );

            if (response.queueUrl() == null || response.queueUrl().isBlank()) {
                issues.add(String.format("SQS queue [%s] URL is empty", queueName));
            } else {
                LOG.debugf("SQS queue [%s] is accessible: %s", queueName, response.queueUrl());
            }

        } catch (Exception e) {
            LOG.errorf(e, "Failed to access SQS queue: %s", queueName);
            issues.add(String.format("Cannot access SQS queue [%s]: %s", queueName, e.getMessage()));
        }

        return issues;
    }

    /**
     * Check basic SQS connectivity by attempting to list queues.
     * This validates AWS credentials and network connectivity.
     *
     * @return true if SQS is accessible, false otherwise
     */
    private boolean checkSqsConnectivity() {
        try {
            // Try to list queues as a basic connectivity check
            // This will fail fast if credentials are wrong or network is down
            sqsClient.listQueues();
            return true;
        } catch (Exception e) {
            LOG.errorf(e, "SQS connectivity check failed");
            return false;
        }
    }

    /**
     * Check ActiveMQ connectivity by creating and closing a test connection.
     * This validates ActiveMQ broker availability and credentials.
     *
     * @return true if ActiveMQ is accessible, false otherwise
     */
    private boolean checkActiveMqConnectivity() {
        jakarta.jms.Connection testConnection = null;
        try {
            testConnection = connectionFactory.createConnection();
            testConnection.start();
            return true;
        } catch (Exception e) {
            LOG.errorf(e, "ActiveMQ connectivity check failed");
            return false;
        } finally {
            if (testConnection != null) {
                try {
                    testConnection.close();
                } catch (Exception e) {
                    LOG.warn("Error closing test connection", e);
                }
            }
        }
    }

    /**
     * Check NATS JetStream connectivity.
     * This validates NATS broker availability using the connection status.
     *
     * @return true if NATS is accessible, false otherwise
     */
    private boolean checkNatsConnectivity() {
        try {
            Status status = natsConnection.getStatus();
            return status == Status.CONNECTED;
        } catch (Exception e) {
            LOG.errorf(e, "NATS connectivity check failed");
            return false;
        }
    }

    /**
     * Get the current broker type
     */
    public QueueType getBrokerType() {
        return queueType;
    }
}

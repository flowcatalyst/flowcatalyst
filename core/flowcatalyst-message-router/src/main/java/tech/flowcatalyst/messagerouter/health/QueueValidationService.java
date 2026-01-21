package tech.flowcatalyst.messagerouter.health;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.jms.ConnectionFactory;
import jakarta.jms.JMSException;
import jakarta.jms.Session;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.GetQueueUrlRequest;
import software.amazon.awssdk.services.sqs.model.QueueDoesNotExistException;
import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.config.QueueType;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.ArrayList;
import java.util.List;

/**
 * Service for validating queue existence and accessibility.
 * Raises warnings for missing or inaccessible queues without stopping the system.
 */
@ApplicationScoped
public class QueueValidationService {

    private static final Logger LOG = Logger.getLogger(QueueValidationService.class);

    @ConfigProperty(name = "message-router.queue-type")
    QueueType queueType;

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @Inject
    SqsClient sqsClient;

    @Inject
    ConnectionFactory connectionFactory;

    @Inject
    WarningService warningService;

    /**
     * Validate all configured queues and raise warnings for any that are missing or inaccessible.
     * Does NOT throw exceptions - continues processing even if some queues are missing.
     *
     * @param queueConfigs the list of queue configurations to validate
     * @return list of validation issues (warnings, not errors)
     */
    public List<String> validateQueues(List<QueueConfig> queueConfigs) {
        if (!messageRouterEnabled) {
            LOG.debug("Message router disabled, skipping queue validation");
            return List.of();
        }

        List<String> issues = new ArrayList<>();

        for (QueueConfig config : queueConfigs) {
            String queueIdentifier = config.queueName() != null ? config.queueName() : config.queueUri();

            try {
                boolean accessible = switch (queueType) {
                    case SQS -> validateSqsQueue(config);
                    case ACTIVEMQ -> validateActiveMqQueue(config);
                    case NATS -> true; // NATS streams are created automatically by NatsConnectionProducer
                    case EMBEDDED -> true; // Embedded queues are always accessible (SQLite file-based)
                };

                if (!accessible) {
                    String issue = String.format("Queue [%s] is not accessible", queueIdentifier);
                    issues.add(issue);
                    raiseQueueWarning(queueIdentifier, "Queue is not accessible");
                }

            } catch (Exception e) {
                String issue = String.format("Failed to validate queue [%s]: %s", queueIdentifier, e.getMessage());
                issues.add(issue);
                raiseQueueWarning(queueIdentifier, "Validation failed: " + e.getMessage());
            }
        }

        if (issues.isEmpty()) {
            LOG.infof("All %d queues validated successfully", queueConfigs.size());
        } else {
            LOG.warnf("Queue validation found %d issues (system will continue processing other queues)", issues.size());
        }

        return issues;
    }

    /**
     * Validate a single SQS queue.
     * Checks if the queue exists and is accessible with current credentials.
     *
     * @param config the queue configuration
     * @return true if queue is accessible, false otherwise
     */
    private boolean validateSqsQueue(QueueConfig config) {
        try {
            String queueName = config.queueName();
            String queueUrl = config.queueUri();

            if (queueUrl != null && !queueUrl.isBlank()) {
                // If URI is provided, validate it directly
                // We could call GetQueueAttributes but that requires parsing the queue name from URL
                // For now, just check if it's a valid URL format
                if (!queueUrl.startsWith("http")) {
                    LOG.warnf("Queue URI [%s] doesn't appear to be a valid URL", queueUrl);
                    return false;
                }
                LOG.debugf("Queue URI provided [%s], assuming valid", queueUrl);
                return true;
            } else if (queueName != null && !queueName.isBlank()) {
                // Validate queue name by getting its URL
                sqsClient.getQueueUrl(
                    GetQueueUrlRequest.builder()
                        .queueName(queueName)
                        .build()
                );
                LOG.debugf("SQS queue [%s] validated successfully", queueName);
                return true;
            } else {
                LOG.warn("Queue configuration has neither name nor URI");
                return false;
            }

        } catch (QueueDoesNotExistException e) {
            LOG.warnf("SQS queue [%s] does not exist", config.queueName());
            return false;
        } catch (Exception e) {
            LOG.errorf(e, "Failed to validate SQS queue [%s]", config.queueName());
            return false;
        }
    }

    /**
     * Validate a single ActiveMQ queue.
     * Checks if we can create a session and access the queue.
     *
     * @param config the queue configuration
     * @return true if queue is accessible, false otherwise
     */
    private boolean validateActiveMqQueue(QueueConfig config) {
        Session session = null;
        try {
            // Create a temporary session to test queue access
            // Note: ActiveMQ will create the queue if it doesn't exist (auto-create behavior)
            // So this mainly validates that we can connect to the broker
            session = connectionFactory.createConnection().createSession(false, Session.AUTO_ACKNOWLEDGE);

            String queueName = config.queueName();
            if (queueName == null || queueName.isBlank()) {
                LOG.warn("ActiveMQ queue configuration has no queue name");
                return false;
            }

            // Try to create a queue object (this doesn't actually create the queue on the broker)
            session.createQueue(queueName);

            LOG.debugf("ActiveMQ queue [%s] validated successfully", queueName);
            return true;

        } catch (JMSException e) {
            LOG.errorf(e, "Failed to validate ActiveMQ queue [%s]", config.queueName());
            return false;
        } finally {
            if (session != null) {
                try {
                    session.close();
                } catch (JMSException e) {
                    LOG.warn("Error closing validation session", e);
                }
            }
        }
    }

    /**
     * Raise a warning for a queue issue.
     * This warning will be visible in the monitoring dashboard.
     */
    private void raiseQueueWarning(String queueIdentifier, String reason) {
        warningService.addWarning(
            "QUEUE_VALIDATION",
            "WARN",
            String.format("Queue [%s] validation failed: %s. System will continue processing other queues.",
                queueIdentifier, reason),
            "QueueValidationService"
        );

        LOG.warnf("Queue [%s] validation issue: %s (system will continue)", queueIdentifier, reason);
    }
}

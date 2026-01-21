package tech.flowcatalyst.messagerouter.consumer;

import jakarta.jms.*;
import org.apache.activemq.ActiveMQConnection;
import org.apache.activemq.ActiveMQSession;
import org.apache.activemq.RedeliveryPolicy;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

public class ActiveMqQueueConsumer extends AbstractQueueConsumer {

    private static final Logger LOG = Logger.getLogger(ActiveMqQueueConsumer.class);

    // ActiveMQ-specific: Individual acknowledge mode for per-message nack
    private static final int INDIVIDUAL_ACKNOWLEDGE = ActiveMQSession.INDIVIDUAL_ACKNOWLEDGE;

    // RedeliveryPolicy configuration: 30 second delay, no exponential backoff
    private static final long INITIAL_REDELIVERY_DELAY_MS = 30_000; // 30 seconds
    private static final int MAX_REDELIVERIES = -1; // Unlimited (rely on DLQ configuration)

    private final ConnectionFactory connectionFactory;
    private final String queueUri;
    private Connection sharedConnection;
    private final int receiveTimeoutMs;
    private final int metricsPollIntervalMs;

    public ActiveMqQueueConsumer(
            ConnectionFactory connectionFactory,
            String queueUri,
            int connections,
            QueueManager queueManager,
            tech.flowcatalyst.messagerouter.metrics.QueueMetricsService queueMetrics,
            WarningService warningService,
            int receiveTimeoutMs,
            int metricsPollIntervalSeconds) {
        super(queueManager, queueMetrics, warningService, connections);
        this.connectionFactory = connectionFactory;
        this.queueUri = queueUri;
        this.receiveTimeoutMs = receiveTimeoutMs;
        this.metricsPollIntervalMs = metricsPollIntervalSeconds * 1000;

        // Create shared connection that will be reused by all consumer threads
        try {
            this.sharedConnection = connectionFactory.createConnection();

            // Configure RedeliveryPolicy: 30 second delay on redeliveries, no exponential backoff
            if (this.sharedConnection instanceof ActiveMQConnection activeMqConn) {
                RedeliveryPolicy redeliveryPolicy = new RedeliveryPolicy();
                redeliveryPolicy.setInitialRedeliveryDelay(INITIAL_REDELIVERY_DELAY_MS);
                redeliveryPolicy.setUseExponentialBackOff(false);
                redeliveryPolicy.setMaximumRedeliveries(MAX_REDELIVERIES);
                activeMqConn.setRedeliveryPolicy(redeliveryPolicy);
                LOG.infof("Configured ActiveMQ RedeliveryPolicy for [%s]: initialDelay=%dms, exponentialBackoff=false",
                    queueUri, INITIAL_REDELIVERY_DELAY_MS);
            }

            this.sharedConnection.start();
            LOG.infof("Created shared ActiveMQ connection for queue [%s]", queueUri);
        } catch (JMSException e) {
            LOG.errorf(e, "Failed to create shared ActiveMQ connection for queue [%s]", queueUri);
            throw new RuntimeException("Failed to create ActiveMQ connection", e);
        }
    }

    @Override
    public String getQueueIdentifier() {
        return queueUri;
    }

    @Override
    protected void consumeMessages() {
        Session session = null;
        MessageConsumer consumer = null;

        try {
            // Create session with INDIVIDUAL_ACKNOWLEDGE mode to prevent head-of-line blocking
            // This allows per-message nack without affecting other messages in the session
            session = sharedConnection.createSession(false, INDIVIDUAL_ACKNOWLEDGE);
            Queue queue = session.createQueue(queueUri);
            consumer = session.createConsumer(queue);

            LOG.debugf("Created ActiveMQ consumer for queue [%s] with INDIVIDUAL_ACKNOWLEDGE mode", queueUri);

            while (running.get()) {
                try {
                    // Update heartbeat to indicate consumer is alive and polling
                    updateHeartbeat();

                    Message jmsMessage = consumer.receive(receiveTimeoutMs);
                    if (jmsMessage instanceof TextMessage textMessage) {
                        String messageBody = textMessage.getText();
                        // Process as single-message batch for consistency with SQS consumer
                        java.util.List<RawMessage> batch = java.util.List.of(new RawMessage(
                            messageBody,
                            null,  // messageGroupId will be extracted from MessagePointer body
                            new ActiveMqMessageCallback(jmsMessage, session),
                            null   // No SQS message ID for ActiveMQ
                        ));
                        processMessageBatch(batch);
                    }
                } catch (Exception e) {
                    LOG.error("Error receiving message from ActiveMQ", e);
                }
            }

        } catch (Exception e) {
            LOG.error("Error in ActiveMQ consumer", e);
        } finally {
            closeResources(consumer, session);
        }
    }

    @Override
    protected void pollQueueMetrics() {
        Session session = null;
        QueueBrowser browser = null;

        try {
            // Create session from shared connection for metrics polling
            session = sharedConnection.createSession(false, Session.AUTO_ACKNOWLEDGE);
            Queue queue = session.createQueue(queueUri);
            browser = session.createBrowser(queue);

            while (running.get()) {
                try {
                    // Count pending messages using QueueBrowser
                    long pendingMessages = 0;
                    var enumeration = browser.getEnumeration();
                    if (enumeration != null) {
                        while (enumeration.hasMoreElements()) {
                            enumeration.nextElement();
                            pendingMessages++;
                        }
                    }

                    // For ActiveMQ, we approximate messagesNotVisible as 0
                    // (would require JMX to get accurate in-flight count)
                    queueMetrics.recordQueueMetrics(queueUri, pendingMessages, 0);

                    Thread.sleep(metricsPollIntervalMs);
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    break;
                } catch (Exception e) {
                    if (running.get()) {
                        LOG.error("Error polling ActiveMQ queue metrics", e);
                        Thread.sleep(metricsPollIntervalMs); // Back off on error
                    }
                }
            }
        } catch (Exception e) {
            LOG.error("Error initializing ActiveMQ metrics polling", e);
        } finally {
            closeBrowserResources(browser, session);
        }
        LOG.debugf("ActiveMQ queue metrics polling for queue [%s] exited cleanly", queueUri);
    }

    @Override
    public void stop() {
        super.stop();
        // Close shared connection when consumer is stopped
        try {
            if (sharedConnection != null) {
                sharedConnection.close();
                LOG.infof("Closed shared ActiveMQ connection for queue [%s]", queueUri);
            }
        } catch (Exception e) {
            LOG.error("Error closing shared ActiveMQ connection", e);
        }
    }

    private void closeResources(MessageConsumer consumer, Session session) {
        try {
            if (consumer != null) consumer.close();
            if (session != null) session.close();
        } catch (Exception e) {
            LOG.error("Error closing ActiveMQ resources", e);
        }
    }

    private void closeBrowserResources(QueueBrowser browser, Session session) {
        try {
            if (browser != null) browser.close();
            if (session != null) session.close();
        } catch (Exception e) {
            LOG.error("Error closing ActiveMQ browser resources", e);
        }
    }

    /**
     * Inner class for ActiveMQ-specific message callback.
     *
     * Uses INDIVIDUAL_ACKNOWLEDGE mode to prevent head-of-line blocking:
     * - ack(): Acknowledges only this specific message
     * - nack(): NO-OP (like SQS) - message redelivered after 30s delay (RedeliveryPolicy)
     *
     * Implements MessageVisibilityControl for delay support:
     * - setVisibilityDelay(): Sets AMQ_SCHEDULED_DELAY property for custom redelivery delay
     */
    private static class ActiveMqMessageCallback implements MessageCallback,
            tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl {
        private final Message jmsMessage;
        private final Session session;
        private int customDelayMs = 0; // Custom delay in milliseconds (0 means use default)

        ActiveMqMessageCallback(Message jmsMessage, Session session) {
            this.jmsMessage = jmsMessage;
            this.session = session;
        }

        @Override
        public void ack(MessagePointer message) {
            try {
                // With INDIVIDUAL_ACKNOWLEDGE, this acknowledges ONLY this message
                jmsMessage.acknowledge();
                LOG.debugf("Acknowledged message [%s] in ActiveMQ", message.id());
            } catch (Exception e) {
                LOG.errorf(e, "Error acknowledging message in ActiveMQ: %s", message.id());
            }
        }

        @Override
        public void nack(MessagePointer message) {
            // NO-OP for basic nack
            // With INDIVIDUAL_ACKNOWLEDGE, simply NOT acknowledging the message
            // is sufficient. The message will be redelivered after the configured
            // RedeliveryPolicy delay (default 30s) or custom delay if set.
            //
            // Note: If setVisibilityDelay was called, the custom delay will be applied
            // via AMQ_SCHEDULED_DELAY property on next redelivery.
            if (customDelayMs > 0) {
                LOG.debugf("Nacked message [%s] - will be redelivered after custom delay of %dms", message.id(), customDelayMs);
            } else {
                LOG.debugf("Nacked message [%s] - will be redelivered after 30s delay", message.id());
            }
        }

        @Override
        public void setFastFailVisibility(MessagePointer message) {
            // For ActiveMQ, we can't dynamically change visibility like SQS
            // The message will use the RedeliveryPolicy delay
            // Set a short delay for fast retry
            customDelayMs = 10_000; // 10 seconds
            try {
                jmsMessage.setLongProperty("AMQ_SCHEDULED_DELAY", customDelayMs);
                LOG.debugf("Set fast-fail delay (10s) for message [%s]", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to set AMQ_SCHEDULED_DELAY for message [%s]", message.id());
            }
        }

        @Override
        public void resetVisibilityToDefault(MessagePointer message) {
            // Reset to default RedeliveryPolicy delay (30 seconds)
            customDelayMs = 30_000;
            try {
                jmsMessage.setLongProperty("AMQ_SCHEDULED_DELAY", customDelayMs);
                LOG.debugf("Reset delay to 30s for message [%s]", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to reset AMQ_SCHEDULED_DELAY for message [%s]", message.id());
            }
        }

        @Override
        public void setVisibilityDelay(MessagePointer message, int delaySeconds) {
            // Set custom redelivery delay using ActiveMQ's scheduled delay feature
            customDelayMs = delaySeconds * 1000;
            try {
                jmsMessage.setLongProperty("AMQ_SCHEDULED_DELAY", customDelayMs);
                LOG.infof("Set custom delay to %ds for message [%s]", delaySeconds, message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to set AMQ_SCHEDULED_DELAY for message [%s]", message.id());
            }
        }
    }
}

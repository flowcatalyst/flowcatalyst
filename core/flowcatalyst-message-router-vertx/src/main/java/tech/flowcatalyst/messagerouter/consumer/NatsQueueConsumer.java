package tech.flowcatalyst.messagerouter.consumer;

import io.nats.client.*;
import io.nats.client.api.ConsumerInfo;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/**
 * NATS JetStream queue consumer implementation.
 *
 * <p>Uses a pull-based durable consumer with explicit acknowledgment for reliable
 * message processing. Messages are fetched in batches and processed through the
 * standard AbstractQueueConsumer pipeline.</p>
 *
 * <h2>ACK/NACK Behavior</h2>
 * <ul>
 *   <li><b>ack()</b> - Message processed successfully, removed from stream</li>
 *   <li><b>nakWithDelay(10s)</b> - Fast-fail for rate limiting, pool full, batch+group failed</li>
 *   <li><b>nakWithDelay(120s)</b> - ERROR_PROCESS (5xx, timeout, connection error)</li>
 *   <li><b>nakWithDelay(custom)</b> - Custom delay from MediationResponse</li>
 *   <li><b>inProgress()</b> - Extend ack wait for long-running processing</li>
 * </ul>
 *
 * <h2>Configuration</h2>
 * <p>Requires a NATS JetStream stream with WorkQueue retention policy for
 * at-most-once delivery semantics after acknowledgment.</p>
 */
public class NatsQueueConsumer extends AbstractQueueConsumer {

    private static final Logger LOG = Logger.getLogger(NatsQueueConsumer.class);

    // Default visibility/ack wait times (matching SQS configuration)
    private static final int DEFAULT_ACK_WAIT_SECONDS = 120;
    private static final int FAST_FAIL_DELAY_SECONDS = 10;
    private static final int ERROR_PROCESS_DELAY_SECONDS = 120;

    private final Connection natsConnection;
    private final JetStream jetStream;
    private final String streamName;
    private final String consumerName;
    private final String subject;
    private final int maxMessagesPerPoll;
    private final Duration pollTimeout;
    private final int metricsPollIntervalMs;

    // Track message sequences that were processed but failed to ack
    // (similar to SQS pendingDeleteSqsMessageIds)
    private final Set<String> pendingAckSequences = ConcurrentHashMap.newKeySet();

    // Pull subscription for fetching messages
    private JetStreamSubscription subscription;

    public NatsQueueConsumer(
            Connection natsConnection,
            String streamName,
            String consumerName,
            String subject,
            int connections,
            QueueManager queueManager,
            QueueMetricsService queueMetrics,
            WarningService warningService,
            int maxMessagesPerPoll,
            int pollTimeoutSeconds,
            int metricsPollIntervalSeconds) {
        super(queueManager, queueMetrics, warningService, connections);
        this.natsConnection = natsConnection;
        this.streamName = streamName;
        this.consumerName = consumerName;
        this.subject = subject;
        this.maxMessagesPerPoll = maxMessagesPerPoll;
        this.pollTimeout = Duration.ofSeconds(pollTimeoutSeconds);
        this.metricsPollIntervalMs = metricsPollIntervalSeconds * 1000;

        try {
            // Initialize JetStream context
            this.jetStream = natsConnection.jetStream();

            // Create pull subscription options binding to the durable consumer
            PullSubscribeOptions options = PullSubscribeOptions.builder()
                .stream(streamName)
                .durable(consumerName)
                .build();

            // Subscribe to the subject
            this.subscription = jetStream.subscribe(subject, options);

            LOG.infof("NATS JetStream consumer created - stream: %s, consumer: %s, subject: %s",
                streamName, consumerName, subject);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to initialize NATS JetStream consumer for stream [%s]", streamName);
            throw new RuntimeException("Failed to initialize NATS JetStream consumer", e);
        }
    }

    @Override
    public String getQueueIdentifier() {
        return streamName + "/" + consumerName;
    }

    @Override
    protected void consumeMessages() {
        while (running.get()) {
            try {
                // Update heartbeat to indicate consumer is alive and polling
                updateHeartbeat();

                // Fetch batch of messages (like SQS long polling)
                List<Message> messages = subscription.fetch(maxMessagesPerPoll, pollTimeout);

                if (messages == null || messages.isEmpty()) {
                    // Empty batch - apply backoff delay
                    try {
                        Thread.sleep(1000);
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                    continue;
                }

                // Check for previously-acked messages that reappeared and build batch
                List<RawMessage> messagesToProcess = new ArrayList<>();
                for (Message msg : messages) {
                    String sequenceId = getSequenceId(msg);

                    if (pendingAckSequences.remove(sequenceId)) {
                        // Already processed successfully, just ack again
                        try {
                            msg.ack();
                            LOG.infof("NATS message [%s] was previously processed - acking again", sequenceId);
                        } catch (Exception e) {
                            LOG.warnf(e, "Failed to ack previously processed NATS message [%s]", sequenceId);
                        }
                    } else {
                        messagesToProcess.add(new RawMessage(
                            new String(msg.getData(), StandardCharsets.UTF_8),
                            null,  // messageGroupId will be extracted from MessagePointer body
                            new NatsMessageCallback(msg, sequenceId),
                            sequenceId  // Use sequence as unique ID for pipeline tracking
                        ));
                    }
                }

                // Process remaining messages through the standard pipeline
                if (!messagesToProcess.isEmpty()) {
                    processMessageBatch(messagesToProcess);
                }

                // Adaptive delay based on batch size (same as SQS)
                if (messages.size() < maxMessagesPerPoll) {
                    try {
                        Thread.sleep(50); // 50ms delay for partial batch
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }

            } catch (Exception e) {
                if (running.get()) {
                    LOG.error("Error polling messages from NATS JetStream", e);
                    try {
                        Thread.sleep(1000); // Back off on error
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                } else {
                    // Shutting down, exit cleanly
                    LOG.debug("Exception during shutdown, exiting cleanly");
                    break;
                }
            }
        }
        LOG.infof("NATS consumer for stream [%s] consumer [%s] polling loop exited cleanly",
            streamName, consumerName);
    }

    /**
     * Poll NATS JetStream for consumer metrics (pending messages, in-flight messages)
     */
    @Override
    protected void pollQueueMetrics() {
        while (running.get()) {
            try {
                // Get consumer info for metrics
                JetStreamManagement jsm = natsConnection.jetStreamManagement();
                ConsumerInfo consumerInfo = jsm.getConsumerInfo(streamName, consumerName);

                if (consumerInfo != null) {
                    long pendingMessages = consumerInfo.getNumPending();
                    long inFlightMessages = consumerInfo.getNumAckPending();

                    queueMetrics.recordQueueMetrics(
                        getQueueIdentifier(),
                        pendingMessages,
                        inFlightMessages
                    );
                }

                Thread.sleep(metricsPollIntervalMs);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                if (running.get()) {
                    LOG.error("Error polling NATS consumer metrics", e);
                    try {
                        Thread.sleep(metricsPollIntervalMs);
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }
            }
        }
        LOG.debugf("NATS consumer metrics polling for [%s] exited cleanly", getQueueIdentifier());
    }

    @Override
    public void stop() {
        super.stop();
        // Unsubscribe and close resources
        try {
            if (subscription != null) {
                subscription.unsubscribe();
                LOG.infof("Unsubscribed NATS consumer for stream [%s]", streamName);
            }
        } catch (Exception e) {
            LOG.error("Error unsubscribing NATS consumer", e);
        }
    }

    /**
     * Get unique sequence ID for a NATS message.
     * Uses stream name + stream sequence number for uniqueness.
     */
    private String getSequenceId(Message msg) {
        if (msg.metaData() != null) {
            return streamName + ":" + msg.metaData().streamSequence();
        }
        // Fallback to reply subject if metadata not available
        return msg.getReplyTo() != null ? msg.getReplyTo() : String.valueOf(msg.hashCode());
    }

    /**
     * Inner class for NATS-specific message callback with visibility control.
     *
     * <p>Implements the same ack/nack semantics as SQS:</p>
     * <ul>
     *   <li>ack() - Remove message from stream</li>
     *   <li>nack() - Nack with default 120s delay (matches SQS queue visibility)</li>
     *   <li>setFastFailVisibility() - Nack with 10s delay for quick retry</li>
     *   <li>resetVisibilityToDefault() - Nack with 120s delay for processing errors</li>
     *   <li>setVisibilityDelay() - Nack with custom delay</li>
     *   <li>extendVisibility() - Extend ack wait using inProgress()</li>
     * </ul>
     */
    private class NatsMessageCallback implements MessageCallback, MessageVisibilityControl {

        private final Message natsMessage;
        private final String sequenceId;

        NatsMessageCallback(Message natsMessage, String sequenceId) {
            this.natsMessage = natsMessage;
            this.sequenceId = sequenceId;
        }

        @Override
        public void ack(MessagePointer message) {
            try {
                natsMessage.ack();
                LOG.infof("NATS: ACKed message [%s] (seq: %s)", message.id(), sequenceId);
            } catch (Exception e) {
                // Message may have been redelivered due to ack timeout
                // Track sequence for deletion when it reappears
                pendingAckSequences.add(sequenceId);
                LOG.warnf("NATS: Failed to ack message [%s] (seq: %s) - added to pending set: %s",
                    message.id(), sequenceId, e.getMessage());
            }
        }

        @Override
        public void nack(MessagePointer message) {
            // Default nack uses the ERROR_PROCESS delay (120s) to match SQS visibility timeout
            try {
                natsMessage.nakWithDelay(Duration.ofSeconds(ERROR_PROCESS_DELAY_SECONDS));
                LOG.infof("NATS: NACKed message [%s] with %ds delay", message.id(), ERROR_PROCESS_DELAY_SECONDS);
            } catch (Exception e) {
                LOG.warnf(e, "NATS: Failed to nack message [%s]", message.id());
            }
        }

        @Override
        public void setFastFailVisibility(MessagePointer message) {
            // Fast-fail: 10 second delay for rate limiting, pool full, batch+group failed
            try {
                natsMessage.nakWithDelay(Duration.ofSeconds(FAST_FAIL_DELAY_SECONDS));
                LOG.debugf("NATS: Fast-fail nack (%ds) for message [%s]", FAST_FAIL_DELAY_SECONDS, message.id());
            } catch (Exception e) {
                LOG.warnf(e, "NATS: Failed to set fast-fail for message [%s]", message.id());
            }
        }

        @Override
        public void resetVisibilityToDefault(MessagePointer message) {
            // Reset to default: 120 second delay for processing errors (matches SQS queue visibility)
            try {
                natsMessage.nakWithDelay(Duration.ofSeconds(ERROR_PROCESS_DELAY_SECONDS));
                LOG.debugf("NATS: Reset visibility (%ds) for message [%s]", ERROR_PROCESS_DELAY_SECONDS, message.id());
            } catch (Exception e) {
                LOG.warnf(e, "NATS: Failed to reset visibility for message [%s]", message.id());
            }
        }

        @Override
        public void setVisibilityDelay(MessagePointer message, int delaySeconds) {
            // Custom delay from MediationResponse (1-43200 seconds)
            try {
                // Clamp to valid range (NATS supports long delays)
                int effectiveDelay = Math.max(1, Math.min(delaySeconds, 43200));
                natsMessage.nakWithDelay(Duration.ofSeconds(effectiveDelay));
                LOG.infof("NATS: Custom delay (%ds) for message [%s]", effectiveDelay, message.id());
            } catch (Exception e) {
                LOG.warnf(e, "NATS: Failed to set custom delay for message [%s]", message.id());
            }
        }

        @Override
        public void extendVisibility(MessagePointer message, int visibilityTimeoutSeconds) {
            // Extend ack wait using inProgress() - resets the ack timeout timer
            try {
                natsMessage.inProgress();
                LOG.debugf("NATS: Extended visibility (inProgress) for message [%s]", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "NATS: Failed to extend visibility for message [%s]", message.id());
            }
        }
    }
}

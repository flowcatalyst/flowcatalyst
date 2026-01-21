package tech.flowcatalyst.messagerouter.consumer;

import com.fasterxml.jackson.core.JsonParseException;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.jboss.logging.Logger;
import org.jboss.logging.MDC;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicLong;

public abstract class AbstractQueueConsumer implements QueueConsumer {

    private static final Logger LOG = Logger.getLogger(AbstractQueueConsumer.class);
    private static final long POLL_TIMEOUT_MS = 60_000; // 60 seconds

    protected final QueueManager queueManager;
    protected final QueueMetricsService queueMetrics;
    protected final WarningService warningService;
    protected final ObjectMapper objectMapper;
    protected final ExecutorService executorService;
    protected final AtomicBoolean running = new AtomicBoolean(false);
    protected final AtomicLong lastPollTime = new AtomicLong(0);
    protected final int connections;

    protected AbstractQueueConsumer(QueueManager queueManager, QueueMetricsService queueMetrics, WarningService warningService, int connections) {
        this.queueManager = queueManager;
        this.queueMetrics = queueMetrics;
        this.warningService = warningService;
        this.connections = connections;
        this.objectMapper = new ObjectMapper();
        this.executorService = Executors.newVirtualThreadPerTaskExecutor();
    }

    @Override
    public void start() {
        if (running.compareAndSet(false, true)) {
            LOG.infof("Starting consumer for queue [%s] with %d connections", getQueueIdentifier(), connections);
            startConsumption();
            // Start queue metrics polling if supported
            executorService.submit(this::pollQueueMetrics);
        }
    }

    /**
     * Start the consumption process. Subclasses can override to control threading model.
     * Default implementation creates N threads that each call consumeMessages().
     */
    protected void startConsumption() {
        for (int i = 0; i < connections; i++) {
            executorService.submit(this::consumeMessages);
        }
    }

    @Override
    public void stop() {
        LOG.infof("Stopping consumer for queue [%s] - current polls will complete naturally", getQueueIdentifier());
        running.set(false);

        // Initiate shutdown but don't wait here
        // QueueManager will wait for all consumers to finish in parallel
        executorService.shutdown();
    }

    @Override
    public boolean isFullyStopped() {
        return executorService.isTerminated();
    }

    /**
     * Batch message processing - parses messages and routes them as a batch
     */
    protected void processMessageBatch(List<RawMessage> rawMessages) {
        String queueId = getQueueIdentifier();

        if (!rawMessages.isEmpty()) {
            LOG.warnf("==> QueueConsumer: Received %d message(s) from queue [%s] <==", rawMessages.size(), queueId);
        }

        List<QueueManager.BatchMessage> batchMessages = new ArrayList<>();

        // Track message IDs seen in this batch to dedupe duplicates
        java.util.Set<String> seenMessageIds = new java.util.HashSet<>();

        // Parse all messages and build batch
        for (RawMessage raw : rawMessages) {
            try {
                // Parse message body to MessagePointer first (to get message ID and messageGroupId)
                MessagePointer parsedMessage = objectMapper.readValue(raw.body(), MessagePointer.class);
                String messageId = parsedMessage.id();
                String messageGroupId = parsedMessage.messageGroupId();  // Extract from MessagePointer body

                LOG.warnf("==> QueueConsumer: *** MESSAGE ID: %s *** from queue [%s] - poolCode: %s, target: %s, messageGroupId: %s",
                    messageId, queueId, parsedMessage.poolCode(), parsedMessage.mediationTarget(),
                    messageGroupId != null ? messageGroupId : "NULL (will default to __DEFAULT__)");

                // Dedupe: If same message ID appears twice in batch, ACK the duplicate immediately
                if (!seenMessageIds.add(messageId)) {
                    LOG.warnf("Duplicate message ID [%s] in same batch - ACKing duplicate to remove from queue", messageId);
                    raw.callback().ack(new MessagePointer(
                        messageId,
                        parsedMessage.poolCode(),
                        null,
                        parsedMessage.mediationType(),
                        parsedMessage.mediationTarget(),
                        messageGroupId,
                        null
                    ));
                    continue;
                }

                // Only record metrics for NEW messages (not redeliveries)
                // Use SQS MessageId for deduplication check (if available)
                String pipelineKey = raw.sqsMessageId() != null ? raw.sqsMessageId() : messageId;
                if (!queueManager.isMessageInPipeline(pipelineKey)) {
                    queueMetrics.recordMessageReceived(queueId);
                } else {
                    // Log redelivery without counting it as a new message exchange
                    LOG.debugf("Message [%s] (SQS: %s) is a redelivery (already in pipeline), not counting as new exchange",
                        messageId, raw.sqsMessageId());
                }

                // Use the already-parsed MessagePointer directly (it already has messageGroupId from JSON body)
                // batchId is null here - it will be populated by QueueManager during routing
                MessagePointer messagePointer = new MessagePointer(
                    parsedMessage.id(),
                    parsedMessage.poolCode(),
                    parsedMessage.authToken(),
                    parsedMessage.mediationType(),
                    parsedMessage.mediationTarget(),
                    messageGroupId,  // Use messageGroupId from MessagePointer JSON body
                    null  // batchId populated by QueueManager.routeMessageBatch()
                );

                // Add to batch
                batchMessages.add(new QueueManager.BatchMessage(
                    messagePointer,
                    raw.callback(),
                    queueId,
                    raw.sqsMessageId()  // Pass SQS MessageId for pipeline tracking
                ));

            } catch (JsonParseException e) {
                // Malformed message - poison pill that will never parse correctly
                LOG.warnf(e, "Malformed message from queue [%s], acknowledging to remove from queue: %s",
                    queueId, raw.body().substring(0, Math.min(100, raw.body().length())));

                warningService.addWarning(
                    "MALFORMED_MESSAGE",
                    "WARN",
                    String.format("Malformed message from queue [%s]: %s",
                        queueId, e.getMessage()),
                    "AbstractQueueConsumer"
                );

                queueMetrics.recordMessageProcessed(queueId, false);

                // ACK the message to remove it from the queue and prevent infinite retries
                raw.callback().ack(new MessagePointer(
                    "unknown",
                    "unknown",
                    null,
                    tech.flowcatalyst.messagerouter.model.MediationType.HTTP,
                    "unknown",
                    null,  // No messageGroupId for unparseable messages
                    null   // No batchId for unparseable messages
                ));

            } catch (Exception e) {
                LOG.errorf(e, "Error parsing message from queue [%s]", queueId);
                queueMetrics.recordMessageProcessed(queueId, false);
                onMessageError(raw.body(), e);
            }
        }

        // Route entire batch to queue manager
        if (!batchMessages.isEmpty()) {
            queueManager.routeMessageBatch(batchMessages);
        }
    }

    /**
     * Record for raw message with metadata.
     * Public to allow access from subclasses in different packages.
     */
    public record RawMessage(
        String body,
        String messageGroupId,
        MessageCallback callback,
        String sqsMessageId  // AWS SQS internal message ID for deduplication
    ) {}

    /**
     * Queue-specific implementation to consume messages
     */
    protected abstract void consumeMessages();

    /**
     * Poll queue-specific metrics (pending messages, in-flight messages)
     * Override to implement queue-specific metrics polling
     */
    protected void pollQueueMetrics() {
        // Default: do nothing
        // Subclasses can override to implement queue-specific metrics polling
    }

    /**
     * Called when message parsing or processing fails
     * Override to implement queue-specific error handling (e.g., move to DLQ)
     */
    protected void onMessageError(String rawMessage, Exception error) {
        // Default: do nothing, let queue visibility timeout handle it
    }

    /**
     * Updates the heartbeat timestamp to indicate the consumer is actively polling.
     * Subclasses should call this at the start of each poll iteration.
     */
    protected void updateHeartbeat() {
        lastPollTime.set(System.currentTimeMillis());
    }

    @Override
    public long getLastPollTime() {
        return lastPollTime.get();
    }

    @Override
    public boolean isHealthy() {
        // Consumer is unhealthy if:
        // 1. It has stopped running
        if (!running.get()) {
            return false;
        }

        // 2. It hasn't polled in the last 60 seconds (stalled/hung)
        long lastPoll = lastPollTime.get();
        if (lastPoll == 0) {
            // Never polled - give it grace period if it just started
            return true;
        }

        long timeSinceLastPoll = System.currentTimeMillis() - lastPoll;
        return timeSinceLastPoll < POLL_TIMEOUT_MS;
    }
}

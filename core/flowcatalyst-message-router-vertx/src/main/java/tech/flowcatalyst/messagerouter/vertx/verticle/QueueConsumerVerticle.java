package tech.flowcatalyst.messagerouter.vertx.verticle;

import io.vertx.core.AbstractVerticle;
import io.vertx.core.json.JsonObject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.BatchRequest;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.QueuedMessage;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;
import java.util.function.Supplier;

/**
 * Queue consumer verticle for polling SQS messages.
 * <p>
 * Uses blocking SQS client - fine on virtual threads.
 * <p>
 * Threading: Virtual Thread (blocking OK)
 */
public class QueueConsumerVerticle extends AbstractVerticle {

    private static final Logger LOG = Logger.getLogger(QueueConsumerVerticle.class);

    private String queueIdentifier;
    private String queueUrl;
    private SqsClient sqsClient;
    private QueueMetricsService queueMetrics;
    private volatile boolean running = true;
    private Instant lastPollTime;

    private final Supplier<SqsClient> sqsClientSupplier;

    public QueueConsumerVerticle(Supplier<SqsClient> sqsClientSupplier, QueueMetricsService queueMetrics) {
        this.sqsClientSupplier = sqsClientSupplier;
        this.queueMetrics = queueMetrics;
    }

    @Override
    public void start() {
        this.queueIdentifier = config().getString("queueIdentifier");
        this.queueUrl = config().getString("queueUrl");

        LOG.infof("QueueConsumerVerticle [%s] starting for queue [%s]", queueIdentifier, queueUrl);

        // Blocking SQS client
        this.sqsClient = sqsClientSupplier.get();

        // Register health check handler
        vertx.eventBus().consumer("consumer." + queueIdentifier + ".health", msg -> {
            msg.reply(new io.vertx.core.json.JsonObject()
                    .put("healthy", isHealthy())
                    .put("lastPollTime", lastPollTime != null ? lastPollTime.toEpochMilli() : 0)
                    .put("running", running));
        });

        // Start polling in a blocking loop on virtual thread
        vertx.executeBlocking(() -> {
            pollLoop();
            return null;
        }, false);

        LOG.infof("QueueConsumerVerticle [%s] started", queueIdentifier);
    }

    @Override
    public void stop() {
        LOG.infof("QueueConsumerVerticle [%s] stopping", queueIdentifier);
        running = false;
    }

    private void pollLoop() {
        while (running) {
            try {
                // Blocking long poll
                ReceiveMessageResponse response = sqsClient.receiveMessage(
                        ReceiveMessageRequest.builder()
                                .queueUrl(queueUrl)
                                .maxNumberOfMessages(10)
                                .waitTimeSeconds(20)
                                .attributeNames(QueueAttributeName.ALL)
                                .messageAttributeNames("All")
                                .build());

                lastPollTime = Instant.now();

                List<Message> messages = response.messages();
                if (!messages.isEmpty()) {
                    LOG.debugf("Received %d messages from queue [%s]", messages.size(), queueIdentifier);

                    // Convert to typed batch request
                    List<QueuedMessage> queuedMessages = new ArrayList<>();
                    for (Message sqsMsg : messages) {
                        queuedMessages.add(convertToQueuedMessage(sqsMsg));
                    }

                    BatchRequest batch = new BatchRequest(queuedMessages, queueIdentifier);

                    // Blocking send - wait for acknowledgement
                    try {
                        vertx.eventBus()
                                .request("router.batch", batch)
                                .toCompletionStage()
                                .toCompletableFuture()
                                .get(30, TimeUnit.SECONDS);
                    } catch (Exception e) {
                        LOG.errorf("Failed to send batch to router: %s", e.getMessage());
                        // Don't crash - continue polling
                    }
                }

                // Update queue metrics periodically
                updateQueueMetrics();

            } catch (SqsException e) {
                LOG.errorf("SQS poll error for queue [%s]: %s", queueIdentifier, e.getMessage());
                try {
                    Thread.sleep(5000); // Backoff on error
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    return;
                }
            } catch (Exception e) {
                LOG.errorf("Unexpected error in poll loop for queue [%s]: %s", queueIdentifier, e.getMessage());
                try {
                    Thread.sleep(1000);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }

        LOG.infof("Poll loop exited for queue [%s]", queueIdentifier);
    }

    private QueuedMessage convertToQueuedMessage(Message sqsMsg) {
        // Parse message body to get MessagePointer fields
        JsonObject body;
        try {
            body = new JsonObject(sqsMsg.body());
        } catch (Exception e) {
            body = new JsonObject();
        }

        // Parse mediation type
        MediationType mediationType;
        try {
            mediationType = MediationType.valueOf(body.getString("mediationType", "HTTP"));
        } catch (Exception e) {
            mediationType = MediationType.HTTP;
        }

        return new QueuedMessage(
                body.getString("id"),
                sqsMsg.messageId(),
                body.getString("poolCode"),
                body.getString("authToken"),
                mediationType,
                body.getString("mediationTarget"),
                sqsMsg.attributes().get(MessageSystemAttributeName.MESSAGE_GROUP_ID),
                queueUrl,
                sqsMsg.receiptHandle()
        );
    }

    private void updateQueueMetrics() {
        try {
            GetQueueAttributesResponse attrs = sqsClient.getQueueAttributes(
                    GetQueueAttributesRequest.builder()
                            .queueUrl(queueUrl)
                            .attributeNames(
                                    QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES,
                                    QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES_NOT_VISIBLE
                            )
                            .build());

            int pending = Integer.parseInt(
                    attrs.attributes().getOrDefault(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES, "0"));
            int notVisible = Integer.parseInt(
                    attrs.attributes().getOrDefault(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES_NOT_VISIBLE, "0"));

            queueMetrics.recordQueueMetrics(queueIdentifier, pending, notVisible);
        } catch (Exception e) {
            LOG.debugf("Failed to update queue metrics for [%s]: %s", queueIdentifier, e.getMessage());
        }
    }

    // === ACCESSORS FOR MONITORING ===

    public String getQueueIdentifier() {
        return queueIdentifier;
    }

    public boolean isHealthy() {
        if (lastPollTime == null) {
            return false;
        }
        // Healthy if we've polled within the last minute
        return Instant.now().toEpochMilli() - lastPollTime.toEpochMilli() < 60_000;
    }

    public Instant getLastPollTime() {
        return lastPollTime;
    }
}

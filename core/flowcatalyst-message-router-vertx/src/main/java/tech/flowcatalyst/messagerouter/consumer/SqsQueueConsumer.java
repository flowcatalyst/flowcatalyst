package tech.flowcatalyst.messagerouter.consumer;

import org.jboss.logging.Logger;
import software.amazon.awssdk.awscore.AwsRequestOverrideConfiguration;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Duration;
import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

public class SqsQueueConsumer extends AbstractQueueConsumer {

    private static final Logger LOG = Logger.getLogger(SqsQueueConsumer.class);

    private final SqsClient sqsClient;
    private final String queueUrl;
    private final int maxMessagesPerPoll;
    private final int waitTimeSeconds;
    private final int metricsPollIntervalMs;

    // Track SQS message IDs that were successfully processed but failed to delete
    // (due to expired receipt handle). When these reappear, delete them immediately.
    // Uses SQS's internal MessageId (not our application message ID) to correctly
    // distinguish redeliveries from new instructions with the same application ID.
    private final Set<String> pendingDeleteSqsMessageIds = ConcurrentHashMap.newKeySet();

    public SqsQueueConsumer(
            SqsClient sqsClient,
            String queueUrl,
            int connections,
            QueueManager queueManager,
            tech.flowcatalyst.messagerouter.metrics.QueueMetricsService queueMetrics,
            WarningService warningService,
            int maxMessagesPerPoll,
            int waitTimeSeconds,
            int metricsPollIntervalSeconds) {
        super(queueManager, queueMetrics, warningService, connections);
        this.sqsClient = sqsClient;
        this.queueUrl = queueUrl;
        this.maxMessagesPerPoll = maxMessagesPerPoll;
        this.waitTimeSeconds = waitTimeSeconds;
        this.metricsPollIntervalMs = metricsPollIntervalSeconds * 1000;

        LOG.warnf("*** SQS CONSUMER CREATED WITH ENHANCED LOGGING (v2) *** Queue: %s, MaxMessages: %d, WaitTime: %ds",
            queueUrl, maxMessagesPerPoll, waitTimeSeconds);
    }

    @Override
    public String getQueueIdentifier() {
        return queueUrl;
    }

    @Override
    protected void consumeMessages() {
        while (running.get()) {
            try {
                // Update heartbeat to indicate consumer is alive and polling
                updateHeartbeat();

                // Configure per-request timeout (25s = 20s long poll + 5s buffer)
                AwsRequestOverrideConfiguration overrideConfig = AwsRequestOverrideConfiguration.builder()
                    .apiCallTimeout(Duration.ofSeconds(25))
                    .build();

                ReceiveMessageRequest receiveRequest = ReceiveMessageRequest.builder()
                    .queueUrl(queueUrl)
                    .maxNumberOfMessages(maxMessagesPerPoll)
                    .waitTimeSeconds(waitTimeSeconds)
                    .overrideConfiguration(overrideConfig)
                    .build();

                // This will block for up to 25 seconds (enforced by SDK timeout)
                ReceiveMessageResponse response = sqsClient.receiveMessage(receiveRequest);
                List<Message> messages = response.messages();

                // Check for messages that need to be deleted (previously processed but delete failed)
                // and convert remaining to RawMessage objects for batch processing
                List<RawMessage> messagesToProcess = new ArrayList<>();
                for (Message msg : messages) {
                    String sqsMessageId = msg.messageId();
                    if (pendingDeleteSqsMessageIds.remove(sqsMessageId)) {
                        // This SQS message was already processed successfully, just delete it
                        LOG.infof("SQS message [%s] was previously processed - deleting from queue now", sqsMessageId);
                        try {
                            DeleteMessageRequest deleteRequest = DeleteMessageRequest.builder()
                                .queueUrl(queueUrl)
                                .receiptHandle(msg.receiptHandle())
                                .build();
                            sqsClient.deleteMessage(deleteRequest);
                            LOG.infof("SQS message [%s] deleted successfully", sqsMessageId);
                        } catch (Exception e) {
                            LOG.warnf(e, "Failed to delete previously processed SQS message [%s]", sqsMessageId);
                        }
                    } else {
                        messagesToProcess.add(new RawMessage(
                            msg.body(),
                            null,  // messageGroupId will be extracted from MessagePointer body in processMessageBatch
                            new SqsMessageCallback(msg.receiptHandle(), sqsMessageId),
                            sqsMessageId  // Pass SQS MessageId for pipeline tracking
                        ));
                    }
                }

                // Process remaining messages
                processMessageBatch(messagesToProcess);

                // Add adaptive delay based on batch size to improve batching efficiency:
                // - Empty batch (0 messages): 1000ms delay (queue likely empty)
                // - Partial batch (1-9 messages): 50ms delay (allow accumulation)
                // - Full batch (10 messages): No delay (keep consuming at full speed)
                if (messages.isEmpty()) {
                    try {
                        Thread.sleep(1000); // 1 second delay when queue is empty
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                } else if (messages.size() < maxMessagesPerPoll) {
                    try {
                        Thread.sleep(50); // 50ms delay for partial batch
                    } catch (InterruptedException e) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }

                // After processing messages, check if we should stop polling
                // This allows the current poll to complete before stopping
                if (!running.get()) {
                    LOG.debug("Stop requested, exiting polling loop after completing current batch");
                    break;
                }

            } catch (Exception e) {
                if (running.get()) {
                    LOG.error("Error polling messages from SQS", e);
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
        LOG.infof("SQS consumer for queue [%s] polling loop exited cleanly", queueUrl);
    }

    /**
     * Periodically poll SQS for queue metrics (pending messages, in-flight messages)
     */
    @Override
    protected void pollQueueMetrics() {
        while (running.get()) {
            try {
                // Configure per-request timeout for metrics polling (10s is plenty)
                AwsRequestOverrideConfiguration overrideConfig = AwsRequestOverrideConfiguration.builder()
                    .apiCallTimeout(Duration.ofSeconds(10))
                    .build();

                GetQueueAttributesRequest request = GetQueueAttributesRequest.builder()
                    .queueUrl(queueUrl)
                    .attributeNames(
                        QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES,
                        QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES_NOT_VISIBLE
                    )
                    .overrideConfiguration(overrideConfig)
                    .build();

                GetQueueAttributesResponse response = sqsClient.getQueueAttributes(request);

                if (response != null && response.attributes() != null) {
                    long pendingMessages = Long.parseLong(
                        response.attributes().getOrDefault(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES, "0")
                    );
                    long messagesNotVisible = Long.parseLong(
                        response.attributes().getOrDefault(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES_NOT_VISIBLE, "0")
                    );

                    queueMetrics.recordQueueMetrics(queueUrl, pendingMessages, messagesNotVisible);
                }

                Thread.sleep(metricsPollIntervalMs);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                if (running.get()) {
                    LOG.error("Error polling SQS queue metrics", e);
                    try {
                        Thread.sleep(metricsPollIntervalMs); // Back off on error
                    } catch (InterruptedException ie) {
                        Thread.currentThread().interrupt();
                        break;
                    }
                }
            }
        }
        LOG.debugf("SQS queue metrics polling for queue [%s] exited cleanly", queueUrl);
    }

    /**
     * Inner class for SQS-specific message callback with visibility control.
     * The receipt handle can be updated when SQS redelivers a message (visibility timeout expired).
     */
    private class SqsMessageCallback implements MessageCallback, tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl,
            tech.flowcatalyst.messagerouter.callback.ReceiptHandleUpdatable {
        private volatile String receiptHandle;  // Mutable - can be updated on redelivery
        private final String sqsMessageId;

        SqsMessageCallback(String receiptHandle, String sqsMessageId) {
            this.receiptHandle = receiptHandle;
            this.sqsMessageId = sqsMessageId;
        }

        @Override
        public void updateReceiptHandle(String newReceiptHandle) {
            LOG.infof("SQS: Updating receipt handle for message (SQS ID: %s) due to redelivery", sqsMessageId);
            this.receiptHandle = newReceiptHandle;
        }

        @Override
        public String getReceiptHandle() {
            return this.receiptHandle;
        }

        @Override
        public void ack(MessagePointer message) {
            try {
                LOG.infof("SQS: ACKing message [%s] - calling SQS DeleteMessage API", message.id());
                DeleteMessageRequest deleteRequest = DeleteMessageRequest.builder()
                    .queueUrl(queueUrl)
                    .receiptHandle(receiptHandle)
                    .build();

                sqsClient.deleteMessage(deleteRequest);
                LOG.infof("SQS: Successfully deleted message [%s] from queue", message.id());
            } catch (ReceiptHandleIsInvalidException e) {
                // Receipt handle expired - track SQS message ID for deletion when message reappears
                pendingDeleteSqsMessageIds.add(sqsMessageId);
                LOG.infof("SQS: Receipt handle expired for message [%s] (SQS ID: %s) - added to pending delete set", message.id(), sqsMessageId);
            } catch (SqsException e) {
                // Check if this is a receipt handle error (common and expected)
                if (e.getMessage() != null && e.getMessage().contains("receipt handle has expired")) {
                    pendingDeleteSqsMessageIds.add(sqsMessageId);
                    LOG.infof("SQS: Receipt handle expired for message [%s] (SQS ID: %s) - added to pending delete set", message.id(), sqsMessageId);
                } else {
                    LOG.errorf(e, "SQS: Unexpected error deleting message [%s] from SQS - message may reappear", message.id());
                }
            } catch (Exception e) {
                LOG.errorf(e, "SQS: Unexpected error deleting message [%s] from SQS - message may reappear", message.id());
            }
        }

        @Override
        public void nack(MessagePointer message) {
            // For SQS, this is a no-op - we rely on visibility timeout
            // Message will become visible again after timeout
            LOG.infof("SQS: NACKing message [%s] - will become visible again after visibility timeout", message.id());
        }

        @Override
        public void setFastFailVisibility(MessagePointer message) {
            try {
                ChangeMessageVisibilityRequest request = ChangeMessageVisibilityRequest.builder()
                    .queueUrl(queueUrl)
                    .receiptHandle(receiptHandle)
                    .visibilityTimeout(10) // 10 seconds for rate limit retry
                    .build();

                sqsClient.changeMessageVisibility(request);
                LOG.debugf("Set fast-fail visibility (1s) for message [%s]", message.id());
            } catch (ReceiptHandleIsInvalidException e) {
                LOG.debugf("Receipt handle invalid for message [%s], cannot change visibility", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to set fast-fail visibility for message [%s]", message.id());
            }
        }

        @Override
        public void resetVisibilityToDefault(MessagePointer message) {
            try {
                // Reset to default visibility (30 seconds) for real processing failures
                // This provides standard retry backoff for downstream errors
                ChangeMessageVisibilityRequest request = ChangeMessageVisibilityRequest.builder()
                    .queueUrl(queueUrl)
                    .receiptHandle(receiptHandle)
                    .visibilityTimeout(30) // Standard 30-second visibility for real failures
                    .build();

                sqsClient.changeMessageVisibility(request);
                LOG.debugf("Reset visibility to 30s for message [%s]", message.id());
            } catch (ReceiptHandleIsInvalidException e) {
                LOG.debugf("Receipt handle invalid for message [%s], cannot change visibility", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to reset visibility for message [%s]", message.id());
            }
        }

        @Override
        public void setVisibilityDelay(MessagePointer message, int delaySeconds) {
            try {
                // Clamp delay to SQS limits: 0-43200 seconds (12 hours)
                int effectiveDelay = Math.max(0, Math.min(delaySeconds, 43200));

                ChangeMessageVisibilityRequest request = ChangeMessageVisibilityRequest.builder()
                    .queueUrl(queueUrl)
                    .receiptHandle(receiptHandle)
                    .visibilityTimeout(effectiveDelay)
                    .build();

                sqsClient.changeMessageVisibility(request);
                LOG.infof("Set custom visibility delay to %ds for message [%s]", effectiveDelay, message.id());
            } catch (ReceiptHandleIsInvalidException e) {
                LOG.debugf("Receipt handle invalid for message [%s], cannot set visibility delay", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to set visibility delay for message [%s]", message.id());
            }
        }

        @Override
        public void extendVisibility(MessagePointer message, int visibilityTimeoutSeconds) {
            try {
                ChangeMessageVisibilityRequest request = ChangeMessageVisibilityRequest.builder()
                    .queueUrl(queueUrl)
                    .receiptHandle(receiptHandle)
                    .visibilityTimeout(visibilityTimeoutSeconds)
                    .build();

                sqsClient.changeMessageVisibility(request);
                LOG.debugf("Extended visibility to %ds for message [%s]", visibilityTimeoutSeconds, message.id());
            } catch (ReceiptHandleIsInvalidException e) {
                LOG.debugf("Receipt handle invalid for message [%s], cannot extend visibility", message.id());
            } catch (Exception e) {
                LOG.warnf(e, "Failed to extend visibility for message [%s]", message.id());
            }
        }
    }
}

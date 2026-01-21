package tech.flowcatalyst.queue.sqs;

import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.queue.*;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.stream.Collectors;

/**
 * SQS implementation of QueuePublisher.
 * Supports both standard and FIFO queues.
 */
public class SqsQueuePublisher implements QueuePublisher {

    private static final Logger LOG = Logger.getLogger(SqsQueuePublisher.class);
    private static final int SQS_MAX_BATCH_SIZE = 10;

    private final SqsClient sqsClient;
    private final String queueUrl;
    private final boolean fifoEnabled;

    public SqsQueuePublisher(SqsClient sqsClient, QueueConfig config) {
        this.sqsClient = sqsClient;
        this.queueUrl = config.queueUrl();
        this.fifoEnabled = config.fifoEnabled();
    }

    @Override
    public QueuePublishResult publish(QueueMessage message) {
        try {
            SendMessageRequest.Builder requestBuilder = SendMessageRequest.builder()
                .queueUrl(queueUrl)
                .messageBody(message.body());

            if (fifoEnabled) {
                requestBuilder.messageGroupId(message.messageGroupId() != null
                    ? message.messageGroupId()
                    : "default");
                requestBuilder.messageDeduplicationId(message.deduplicationId() != null
                    ? message.deduplicationId()
                    : message.messageId());
            }

            SendMessageResponse response = sqsClient.sendMessage(requestBuilder.build());

            LOG.debugf("Published message [%s] to SQS, messageId: %s",
                message.messageId(), response.messageId());

            return QueuePublishResult.success(message.messageId());

        } catch (Exception e) {
            LOG.errorf(e, "Failed to publish message [%s] to SQS", message.messageId());
            return QueuePublishResult.failure(message.messageId(), e.getMessage());
        }
    }

    @Override
    public QueuePublishResult publishBatch(List<QueueMessage> messages) {
        if (messages.isEmpty()) {
            return QueuePublishResult.success(List.of());
        }

        List<String> allPublished = new ArrayList<>();
        List<String> allFailed = new ArrayList<>();
        StringBuilder errors = new StringBuilder();

        // Split into batches of 10 (SQS limit)
        for (int i = 0; i < messages.size(); i += SQS_MAX_BATCH_SIZE) {
            List<QueueMessage> batch = messages.subList(i,
                Math.min(i + SQS_MAX_BATCH_SIZE, messages.size()));

            try {
                List<SendMessageBatchRequestEntry> entries = batch.stream()
                    .map(this::toEntry)
                    .collect(Collectors.toList());

                SendMessageBatchRequest request = SendMessageBatchRequest.builder()
                    .queueUrl(queueUrl)
                    .entries(entries)
                    .build();

                SendMessageBatchResponse response = sqsClient.sendMessageBatch(request);

                // Collect successful sends
                response.successful().forEach(success ->
                    allPublished.add(success.id()));

                // Collect failed sends
                response.failed().forEach(failure -> {
                    allFailed.add(failure.id());
                    if (!errors.isEmpty()) errors.append("; ");
                    errors.append(failure.id()).append(": ").append(failure.message());
                });

            } catch (Exception e) {
                LOG.errorf(e, "Failed to publish batch to SQS");
                batch.forEach(m -> allFailed.add(m.messageId()));
                if (!errors.isEmpty()) errors.append("; ");
                errors.append(e.getMessage());
            }
        }

        if (allFailed.isEmpty()) {
            return QueuePublishResult.success(allPublished);
        } else if (allPublished.isEmpty()) {
            return QueuePublishResult.failure(allFailed, errors.toString());
        } else {
            return QueuePublishResult.partial(allPublished, allFailed, errors.toString());
        }
    }

    private SendMessageBatchRequestEntry toEntry(QueueMessage message) {
        SendMessageBatchRequestEntry.Builder builder = SendMessageBatchRequestEntry.builder()
            .id(message.messageId())
            .messageBody(message.body());

        if (fifoEnabled) {
            builder.messageGroupId(message.messageGroupId() != null
                ? message.messageGroupId()
                : "default");
            builder.messageDeduplicationId(message.deduplicationId() != null
                ? message.deduplicationId()
                : message.messageId());
        }

        return builder.build();
    }

    @Override
    public long getQueueDepth() {
        try {
            GetQueueAttributesRequest request = GetQueueAttributesRequest.builder()
                .queueUrl(queueUrl)
                .attributeNames(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES)
                .build();

            GetQueueAttributesResponse response = sqsClient.getQueueAttributes(request);
            String count = response.attributes().get(QueueAttributeName.APPROXIMATE_NUMBER_OF_MESSAGES);
            return count != null ? Long.parseLong(count) : 0;

        } catch (Exception e) {
            LOG.warnf("Failed to get queue depth: %s", e.getMessage());
            return -1;
        }
    }

    @Override
    public QueueType getQueueType() {
        return QueueType.SQS;
    }

    @Override
    public boolean isHealthy() {
        try {
            // Simple health check - get queue attributes
            GetQueueAttributesRequest request = GetQueueAttributesRequest.builder()
                .queueUrl(queueUrl)
                .attributeNames(QueueAttributeName.QUEUE_ARN)
                .build();
            sqsClient.getQueueAttributes(request);
            return true;
        } catch (Exception e) {
            LOG.warnf("SQS health check failed: %s", e.getMessage());
            return false;
        }
    }

    @Override
    public void close() {
        // SqsClient is managed by Quarkus DI, don't close it here
    }
}

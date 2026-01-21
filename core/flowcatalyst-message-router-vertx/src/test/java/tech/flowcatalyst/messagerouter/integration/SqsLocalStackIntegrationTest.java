package tech.flowcatalyst.messagerouter.integration;

import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.messagerouter.consumer.SqsQueueConsumer;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;

import java.util.List;
import java.util.Map;
import java.util.UUID;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Integration test using real LocalStack SQS.
 * This test verifies the complete SQS message flow with a real queue.
 */
@QuarkusTest
@QuarkusTestResource(LocalStackTestResource.class)
@Tag("integration")
class SqsLocalStackIntegrationTest {

    @Inject
    SqsClient sqsClient;

    @Inject
    QueueManager queueManager;

    @Inject
    QueueMetricsService queueMetrics;

    @Inject
    tech.flowcatalyst.messagerouter.warning.WarningService warningService;

    private String testQueueUrl;
    private SqsQueueConsumer consumer;

    @BeforeEach
    void setUp() {
        // Create a unique test queue
        String queueName = "test-queue-" + UUID.randomUUID().toString();
        CreateQueueRequest createQueueRequest = CreateQueueRequest.builder()
            .queueName(queueName)
            .build();

        CreateQueueResponse createQueueResponse = sqsClient.createQueue(createQueueRequest);
        testQueueUrl = createQueueResponse.queueUrl();

        // Create consumer
        consumer = new SqsQueueConsumer(
            sqsClient,
            testQueueUrl,
            1,
            queueManager,
            queueMetrics,
            warningService,
            10, // maxMessagesPerPoll
            20, // waitTimeSeconds
            5   // metricsPollIntervalSeconds
        );
    }

    @AfterEach
    void tearDown() {
        if (consumer != null) {
            consumer.stop();
        }

        if (testQueueUrl != null) {
            try {
                DeleteQueueRequest deleteRequest = DeleteQueueRequest.builder()
                    .queueUrl(testQueueUrl)
                    .build();
                sqsClient.deleteQueue(deleteRequest);
            } catch (Exception e) {
                // Ignore cleanup errors
            }
        }
    }

    @Test
    void shouldCreateQueueInLocalStack() {
        assertNotNull(testQueueUrl);
        assertTrue(testQueueUrl.contains("test-queue-"));
    }

    @Test
    void shouldSendAndReceiveMessageFromLocalStack() {
        // Given
        String messageBody = """
            {
                "id": "localstack-msg-1",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8081/test/webhook"
            }
            """;

        // When - Send message
        SendMessageRequest sendRequest = SendMessageRequest.builder()
            .queueUrl(testQueueUrl)
            .messageBody(messageBody)
            .build();

        SendMessageResponse sendResponse = sqsClient.sendMessage(sendRequest);
        assertNotNull(sendResponse.messageId());

        // Then - Receive message
        ReceiveMessageRequest receiveRequest = ReceiveMessageRequest.builder()
            .queueUrl(testQueueUrl)
            .maxNumberOfMessages(1)
            .waitTimeSeconds(5)
            .build();

        ReceiveMessageResponse receiveResponse = sqsClient.receiveMessage(receiveRequest);
        List<Message> messages = receiveResponse.messages();

        assertEquals(1, messages.size());
        assertEquals(messageBody.trim(), messages.get(0).body().trim());
    }

    @Test
    void shouldDeleteMessageAfterAck() {
        // Given
        String messageBody = """
            {
                "id": "delete-msg",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8081/test/webhook"
            }
            """;

        SendMessageRequest sendRequest = SendMessageRequest.builder()
            .queueUrl(testQueueUrl)
            .messageBody(messageBody)
            .build();

        sqsClient.sendMessage(sendRequest);

        // When - Receive and delete
        ReceiveMessageRequest receiveRequest = ReceiveMessageRequest.builder()
            .queueUrl(testQueueUrl)
            .maxNumberOfMessages(1)
            .waitTimeSeconds(5)
            .build();

        ReceiveMessageResponse receiveResponse = sqsClient.receiveMessage(receiveRequest);
        Message message = receiveResponse.messages().get(0);

        DeleteMessageRequest deleteRequest = DeleteMessageRequest.builder()
            .queueUrl(testQueueUrl)
            .receiptHandle(message.receiptHandle())
            .build();

        sqsClient.deleteMessage(deleteRequest);

        // Then - Verify message is deleted
        ReceiveMessageResponse secondReceive = sqsClient.receiveMessage(
            ReceiveMessageRequest.builder()
                .queueUrl(testQueueUrl)
                .maxNumberOfMessages(1)
                .waitTimeSeconds(1)
                .build()
        );

        assertTrue(secondReceive.messages().isEmpty(), "Message should be deleted");
    }

    @Test
    void shouldRedeliverMessageAfterVisibilityTimeout() {
        // Given
        CreateQueueRequest createQueueRequest = CreateQueueRequest.builder()
            .queueName("short-visibility-" + UUID.randomUUID())
            .attributes(Map.of(QueueAttributeName.VISIBILITY_TIMEOUT, "1")) // 1 second visibility
            .build();

        String shortVisibilityQueueUrl = sqsClient.createQueue(createQueueRequest).queueUrl();

        try {
            String messageBody = """
                {
                    "id": "redelivery-msg",
                    "poolCode": "POOL-A",
                    "authToken": "test-token",
                    "mediationType": "HTTP",
                    "mediationTarget": "http://localhost:8081/test/webhook"
                }
                """;

            sqsClient.sendMessage(SendMessageRequest.builder()
                .queueUrl(shortVisibilityQueueUrl)
                .messageBody(messageBody)
                .build());

            // When - Receive but don't delete (simulating nack)
            ReceiveMessageResponse firstReceive = sqsClient.receiveMessage(
                ReceiveMessageRequest.builder()
                    .queueUrl(shortVisibilityQueueUrl)
                    .maxNumberOfMessages(1)
                    .waitTimeSeconds(1)
                    .build()
            );

            assertEquals(1, firstReceive.messages().size());

            // Wait for visibility timeout
            Thread.sleep(2000);

            // Then - Message should be available again
            ReceiveMessageResponse secondReceive = sqsClient.receiveMessage(
                ReceiveMessageRequest.builder()
                    .queueUrl(shortVisibilityQueueUrl)
                    .maxNumberOfMessages(1)
                    .waitTimeSeconds(1)
                    .build()
            );

            assertEquals(1, secondReceive.messages().size(), "Message should be redelivered after visibility timeout");

        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            fail("Test interrupted");
        } finally {
            sqsClient.deleteQueue(DeleteQueueRequest.builder().queueUrl(shortVisibilityQueueUrl).build());
        }
    }

    @Test
    void shouldHandleMultipleMessagesInBatch() {
        // Given - Send 5 messages
        for (int i = 0; i < 5; i++) {
            String messageBody = String.format("""
                {
                    "id": "batch-msg-%d",
                    "poolCode": "POOL-A",
                    "authToken": "test-token",
                    "mediationType": "HTTP",
                    "mediationTarget": "http://localhost:8081/test/webhook"
                }
                """, i);

            sqsClient.sendMessage(SendMessageRequest.builder()
                .queueUrl(testQueueUrl)
                .messageBody(messageBody)
                .build());
        }

        // When - Receive messages
        ReceiveMessageResponse receiveResponse = sqsClient.receiveMessage(
            ReceiveMessageRequest.builder()
                .queueUrl(testQueueUrl)
                .maxNumberOfMessages(10)
                .waitTimeSeconds(5)
                .build()
        );

        // Then
        assertEquals(5, receiveResponse.messages().size());
    }

    @Test
    void shouldUseLongPolling() {
        // Given - Empty queue
        long startTime = System.currentTimeMillis();

        // When - Long poll with no messages
        ReceiveMessageResponse response = sqsClient.receiveMessage(
            ReceiveMessageRequest.builder()
                .queueUrl(testQueueUrl)
                .maxNumberOfMessages(1)
                .waitTimeSeconds(2) // Long poll for 2 seconds
                .build()
        );

        long duration = System.currentTimeMillis() - startTime;

        // Then
        assertTrue(response.messages().isEmpty());
        assertTrue(duration >= 2000, "Should wait at least 2 seconds for long polling");
    }
}

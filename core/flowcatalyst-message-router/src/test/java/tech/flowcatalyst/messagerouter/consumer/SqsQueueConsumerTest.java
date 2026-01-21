package tech.flowcatalyst.messagerouter.consumer;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.mockito.ArgumentCaptor;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.*;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;

import java.util.List;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class SqsQueueConsumerTest {

    private SqsQueueConsumer sqsConsumer;
    private SqsClient mockSqsClient;
    private QueueManager mockQueueManager;
    private QueueMetricsService mockQueueMetrics;
    private tech.flowcatalyst.messagerouter.warning.WarningService mockWarningService;

    private final String queueUrl = "https://sqs.us-east-1.amazonaws.com/123456789/test-queue";

    @BeforeEach
    void setUp() {
        mockSqsClient = mock(SqsClient.class);
        mockQueueManager = mock(QueueManager.class);
        mockQueueMetrics = mock(QueueMetricsService.class);
        mockWarningService = mock(tech.flowcatalyst.messagerouter.warning.WarningService.class);

        sqsConsumer = new SqsQueueConsumer(
            mockSqsClient,
            queueUrl,
            1, // 1 connection
            mockQueueManager,
            mockQueueMetrics,
            mockWarningService,
            10, // maxMessagesPerPoll
            20, // waitTimeSeconds
            5   // metricsPollIntervalSeconds
        );
    }

    @AfterEach
    void tearDown() {
        if (sqsConsumer != null) {
            sqsConsumer.stop();
        }
    }

    @Test
    void shouldPollAndProcessMessages() {
        // Given
        String messageBody = """
            {
                "id": "msg-1",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message sqsMessage = Message.builder()
            .body(messageBody)
            .messageId("sqs-msg-123")
            .receiptHandle("receipt-123")
            .build();

        ReceiveMessageResponse response = ReceiveMessageResponse.builder()
            .messages(sqsMessage)
            .build();

        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(response)
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build()); // Empty on second call

        doNothing().when(mockQueueManager).routeMessageBatch(anyList());

        // When
        sqsConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
            verify(mockQueueMetrics).recordMessageReceived(queueUrl);
        });
    }

    @Test
    void shouldDeleteMessageOnAck() {
        // Given
        String messageBody = """
            {
                "id": "msg-ack",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        String receiptHandle = "receipt-ack-123";

        Message sqsMessage = Message.builder()
            .body(messageBody)
            .messageId("sqs-msg-ack")
            .receiptHandle(receiptHandle)
            .build();

        ReceiveMessageResponse response = ReceiveMessageResponse.builder()
            .messages(sqsMessage)
            .build();

        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(response)
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        sqsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-ack", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);
        callback.ack(testMessage);

        // Then
        ArgumentCaptor<DeleteMessageRequest> deleteCaptor = ArgumentCaptor.forClass(DeleteMessageRequest.class);
        verify(mockSqsClient).deleteMessage(deleteCaptor.capture());
        assertEquals(queueUrl, deleteCaptor.getValue().queueUrl());
        assertEquals(receiptHandle, deleteCaptor.getValue().receiptHandle());
    }

    @Test
    void shouldNotDeleteMessageOnNack() {
        // Given
        String messageBody = """
            {
                "id": "msg-nack",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message sqsMessage = Message.builder()
            .body(messageBody)
            .messageId("sqs-msg-nack")
            .receiptHandle("receipt-nack-123")
            .build();

        ReceiveMessageResponse response = ReceiveMessageResponse.builder()
            .messages(sqsMessage)
            .build();

        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(response)
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        sqsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-nack", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);
        callback.nack(testMessage);

        // Then
        // Nack should NOT delete the message (relies on visibility timeout)
        verify(mockSqsClient, never()).deleteMessage(any(DeleteMessageRequest.class));
    }

    @Test
    void shouldUseLongPolling() {
        // Given
        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        // When
        sqsConsumer.start();

        // Then
        await().untilAsserted(() -> {
            ArgumentCaptor<ReceiveMessageRequest> requestCaptor = ArgumentCaptor.forClass(ReceiveMessageRequest.class);
            verify(mockSqsClient, atLeastOnce()).receiveMessage(requestCaptor.capture());

            ReceiveMessageRequest request = requestCaptor.getValue();
            assertEquals(queueUrl, request.queueUrl());
            assertEquals(20, request.waitTimeSeconds()); // Long polling
            assertEquals(10, request.maxNumberOfMessages());
        });
    }

    @Test
    void shouldStopGracefully() {
        // Given
        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        sqsConsumer.start();

        // When
        sqsConsumer.stop();

        // Then - just verify it doesn't throw an exception
        assertTrue(true, "Stop should complete without errors");
    }

    @Test
    void shouldGetQueueIdentifier() {
        assertEquals(queueUrl, sqsConsumer.getQueueIdentifier());
    }

    @Test
    void shouldHandleInvalidJson() {
        // Given
        Message invalidMessage = Message.builder()
            .body("{ invalid json }")
            .messageId("sqs-msg-invalid")
            .receiptHandle("receipt-invalid")
            .build();

        ReceiveMessageResponse response = ReceiveMessageResponse.builder()
            .messages(invalidMessage)
            .build();

        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(response)
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        // When
        sqsConsumer.start();

        // Then
        await().untilAsserted(() -> {
            // Malformed messages should NOT be counted in recordMessageReceived
            // They're data quality issues that should be caught upstream
            verify(mockQueueMetrics, never()).recordMessageReceived(queueUrl);
            verify(mockQueueMetrics).recordMessageProcessed(queueUrl, false);
            verify(mockQueueManager, never()).routeMessageBatch(anyList());
        });
    }

    @Test
    void shouldSetFastFailVisibilityTo10SecondsThenResetTo30Seconds() {
        // Given
        String messageBody = """
            {
                "id": "msg-visibility",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        String receiptHandle = "receipt-visibility-123";

        Message sqsMessage = Message.builder()
            .body(messageBody)
            .messageId("sqs-msg-visibility")
            .receiptHandle(receiptHandle)
            .build();

        ReceiveMessageResponse response = ReceiveMessageResponse.builder()
            .messages(sqsMessage)
            .build();

        when(mockSqsClient.receiveMessage(any(ReceiveMessageRequest.class)))
            .thenReturn(response)
            .thenReturn(ReceiveMessageResponse.builder().messages(List.of()).build());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        sqsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-visibility", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);

        // Verify callback supports visibility control
        assertTrue(callback instanceof MessageVisibilityControl, "Callback should implement MessageVisibilityControl");

        MessageVisibilityControl visibilityControl = (MessageVisibilityControl) callback;

        // Scenario 1: Rate limit exceeded - set fast-fail visibility (1 second)
        visibilityControl.setFastFailVisibility(testMessage);

        ArgumentCaptor<ChangeMessageVisibilityRequest> visibilityCaptor1 = ArgumentCaptor.forClass(ChangeMessageVisibilityRequest.class);
        verify(mockSqsClient, times(1)).changeMessageVisibility(visibilityCaptor1.capture());

        ChangeMessageVisibilityRequest request1 = visibilityCaptor1.getValue();
        assertEquals(queueUrl, request1.queueUrl());
        assertEquals(receiptHandle, request1.receiptHandle());
        assertEquals(10, request1.visibilityTimeout(), "Fast-fail visibility should be 10 seconds");

        // Scenario 2: Message processed but failed (real error) - reset to default (30 seconds)
        visibilityControl.resetVisibilityToDefault(testMessage);

        ArgumentCaptor<ChangeMessageVisibilityRequest> visibilityCaptor2 = ArgumentCaptor.forClass(ChangeMessageVisibilityRequest.class);
        verify(mockSqsClient, times(2)).changeMessageVisibility(visibilityCaptor2.capture());

        // Get the second call (index 1)
        ChangeMessageVisibilityRequest request2 = visibilityCaptor2.getAllValues().get(1);
        assertEquals(queueUrl, request2.queueUrl());
        assertEquals(receiptHandle, request2.receiptHandle());
        assertEquals(30, request2.visibilityTimeout(), "Reset visibility should be 30 seconds");
    }
}

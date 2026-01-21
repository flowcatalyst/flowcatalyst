package tech.flowcatalyst.messagerouter.consumer;

import io.nats.client.*;
import io.nats.client.api.ConsumerInfo;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.mockito.ArgumentCaptor;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.List;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

class NatsQueueConsumerTest {

    private NatsQueueConsumer natsConsumer;
    private Connection mockConnection;
    private JetStream mockJetStream;
    private JetStreamSubscription mockSubscription;
    private JetStreamManagement mockJsm;
    private QueueManager mockQueueManager;
    private QueueMetricsService mockQueueMetrics;
    private WarningService mockWarningService;

    private final String streamName = "FLOWCATALYST";
    private final String consumerName = "flowcatalyst-router";
    private final String subject = "flowcatalyst.dispatch.>";

    @BeforeEach
    void setUp() throws Exception {
        mockConnection = mock(Connection.class);
        mockJetStream = mock(JetStream.class);
        mockSubscription = mock(JetStreamSubscription.class);
        mockJsm = mock(JetStreamManagement.class);
        mockQueueManager = mock(QueueManager.class);
        mockQueueMetrics = mock(QueueMetricsService.class);
        mockWarningService = mock(WarningService.class);

        when(mockConnection.jetStream()).thenReturn(mockJetStream);
        when(mockConnection.jetStreamManagement()).thenReturn(mockJsm);
        when(mockJetStream.subscribe(eq(subject), any(PullSubscribeOptions.class))).thenReturn(mockSubscription);

        natsConsumer = new NatsQueueConsumer(
            mockConnection,
            streamName,
            consumerName,
            subject,
            1, // 1 connection
            mockQueueManager,
            mockQueueMetrics,
            mockWarningService,
            10, // maxMessagesPerPoll
            20, // pollTimeoutSeconds
            5   // metricsPollIntervalSeconds
        );
    }

    @AfterEach
    void tearDown() {
        if (natsConsumer != null) {
            natsConsumer.stop();
        }
    }

    @Test
    void shouldPollAndProcessMessages() throws Exception {
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

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of()); // Empty on second call

        doNothing().when(mockQueueManager).routeMessageBatch(anyList());

        // When
        natsConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
            verify(mockQueueMetrics).recordMessageReceived(streamName + "/" + consumerName);
        });
    }

    @Test
    void shouldAckMessageOnSuccess() throws Exception {
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

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

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
        verify(natsMessage).ack();
    }

    @Test
    void shouldNackMessageWithDelayOnFailure() throws Exception {
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

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-nack", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);
        callback.nack(testMessage);

        // Then - nack should call nakWithDelay(120s) for ERROR_PROCESS
        verify(natsMessage).nakWithDelay(Duration.ofSeconds(120));
    }

    @Test
    void shouldSetFastFailVisibilityTo10Seconds() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "msg-fastfail",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        assertTrue(callback instanceof MessageVisibilityControl, "Callback should implement MessageVisibilityControl");

        MessageVisibilityControl visibilityControl = (MessageVisibilityControl) callback;
        MessagePointer testMessage = new MessagePointer("msg-fastfail", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);

        // Fast-fail for rate limiting, pool full, etc.
        visibilityControl.setFastFailVisibility(testMessage);

        // Then - should call nakWithDelay(10s)
        verify(natsMessage).nakWithDelay(Duration.ofSeconds(10));
    }

    @Test
    void shouldResetVisibilityTo120Seconds() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "msg-reset",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        assertTrue(callback instanceof MessageVisibilityControl);

        MessageVisibilityControl visibilityControl = (MessageVisibilityControl) callback;
        MessagePointer testMessage = new MessagePointer("msg-reset", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);

        // Reset visibility for processing errors (5xx, timeout)
        visibilityControl.resetVisibilityToDefault(testMessage);

        // Then - should call nakWithDelay(120s) to match SQS queue visibility timeout
        verify(natsMessage).nakWithDelay(Duration.ofSeconds(120));
    }

    @Test
    void shouldSetCustomVisibilityDelay() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "msg-custom",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        assertTrue(callback instanceof MessageVisibilityControl);

        MessageVisibilityControl visibilityControl = (MessageVisibilityControl) callback;
        MessagePointer testMessage = new MessagePointer("msg-custom", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);

        // Custom delay from MediationResponse
        visibilityControl.setVisibilityDelay(testMessage, 300);

        // Then - should call nakWithDelay with custom duration
        verify(natsMessage).nakWithDelay(Duration.ofSeconds(300));
    }

    @Test
    void shouldExtendVisibilityUsingInProgress() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "msg-extend",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        Message natsMessage = createMockNatsMessage(messageBody, 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(natsMessage))
            .thenReturn(List.of());

        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        natsConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        assertTrue(callback instanceof MessageVisibilityControl);

        MessageVisibilityControl visibilityControl = (MessageVisibilityControl) callback;
        MessagePointer testMessage = new MessagePointer("msg-extend", "POOL-A", "token", MediationType.HTTP, "http://test.com", null, null);

        // Extend visibility for long-running processing
        visibilityControl.extendVisibility(testMessage, 60);

        // Then - should call inProgress() to reset ack timeout
        verify(natsMessage).inProgress();
    }

    @Test
    void shouldStopGracefully() throws Exception {
        // Given
        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of());

        natsConsumer.start();

        // When
        natsConsumer.stop();

        // Then
        verify(mockSubscription).unsubscribe();
    }

    @Test
    void shouldGetQueueIdentifier() {
        assertEquals(streamName + "/" + consumerName, natsConsumer.getQueueIdentifier());
    }

    @Test
    void shouldHandleInvalidJson() throws Exception {
        // Given
        Message invalidMessage = createMockNatsMessage("{ invalid json }", 1L);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of(invalidMessage))
            .thenReturn(List.of());

        // When
        natsConsumer.start();

        // Then
        await().untilAsserted(() -> {
            // Malformed messages should be marked as failed
            verify(mockQueueMetrics).recordMessageProcessed(streamName + "/" + consumerName, false);
            verify(mockQueueManager, never()).routeMessageBatch(anyList());
        });
    }

    @Test
    void shouldPollQueueMetrics() throws Exception {
        // Given
        ConsumerInfo mockConsumerInfo = mock(ConsumerInfo.class);
        when(mockConsumerInfo.getNumPending()).thenReturn(100L);
        when(mockConsumerInfo.getNumAckPending()).thenReturn(10L);
        when(mockJsm.getConsumerInfo(streamName, consumerName)).thenReturn(mockConsumerInfo);

        when(mockSubscription.fetch(eq(10), any(Duration.class)))
            .thenReturn(List.of());

        // When
        natsConsumer.start();

        // Then
        await().atMost(Duration.ofSeconds(10)).untilAsserted(() -> {
            verify(mockQueueMetrics, atLeastOnce()).recordQueueMetrics(
                eq(streamName + "/" + consumerName),
                eq(100L),
                eq(10L)
            );
        });
    }

    /**
     * Helper method to create a mock NATS message with proper metadata.
     */
    private Message createMockNatsMessage(String body, long streamSequence) {
        Message mockMessage = mock(Message.class);
        io.nats.client.impl.NatsJetStreamMetaData mockMetaData = mock(io.nats.client.impl.NatsJetStreamMetaData.class);

        when(mockMessage.getData()).thenReturn(body.getBytes(StandardCharsets.UTF_8));
        when(mockMessage.metaData()).thenReturn(mockMetaData);
        when(mockMetaData.streamSequence()).thenReturn(streamSequence);

        return mockMessage;
    }
}

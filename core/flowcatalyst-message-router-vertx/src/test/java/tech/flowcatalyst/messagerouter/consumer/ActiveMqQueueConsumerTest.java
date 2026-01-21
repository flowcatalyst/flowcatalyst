package tech.flowcatalyst.messagerouter.consumer;

import jakarta.jms.*;
import org.apache.activemq.ActiveMQSession;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.AfterEach;
import org.mockito.ArgumentCaptor;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.manager.QueueManager;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.model.MediationType;

import java.util.Enumeration;
import java.util.List;

import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Comprehensive ActiveMqQueueConsumer tests covering:
 * - Message consumption and routing
 * - ACK behavior (INDIVIDUAL_ACKNOWLEDGE mode)
 * - NACK behavior (individual message redelivery without head-of-line blocking)
 * - Queue metrics polling using QueueBrowser
 * - Error handling
 * - Resource cleanup
 */
class ActiveMqQueueConsumerTest {

    private ActiveMqQueueConsumer activeMqConsumer;
    private ConnectionFactory mockConnectionFactory;
    private Connection mockConnection;
    private Session mockSession;
    private MessageConsumer mockMessageConsumer;
    private QueueBrowser mockQueueBrowser;
    private Queue mockQueue;
    private QueueManager mockQueueManager;
    private QueueMetricsService mockQueueMetrics;
    private tech.flowcatalyst.messagerouter.warning.WarningService mockWarningService;

    private final String queueUri = "test.queue";

    @BeforeEach
    void setUp() throws Exception {
        // Create mocks
        mockConnectionFactory = mock(ConnectionFactory.class);
        mockConnection = mock(Connection.class);
        mockSession = mock(Session.class);
        mockMessageConsumer = mock(MessageConsumer.class);
        mockQueueBrowser = mock(QueueBrowser.class);
        mockQueue = mock(Queue.class);
        mockQueueManager = mock(QueueManager.class);
        mockQueueMetrics = mock(QueueMetricsService.class);
        mockWarningService = mock(tech.flowcatalyst.messagerouter.warning.WarningService.class);

        // Setup mock chain for connection creation
        when(mockConnectionFactory.createConnection()).thenReturn(mockConnection);
        when(mockConnection.createSession(anyBoolean(), anyInt())).thenReturn(mockSession);
        when(mockSession.createQueue(queueUri)).thenReturn(mockQueue);
        when(mockSession.createConsumer(mockQueue)).thenReturn(mockMessageConsumer);
        when(mockSession.createBrowser(mockQueue)).thenReturn(mockQueueBrowser);

        activeMqConsumer = new ActiveMqQueueConsumer(
            mockConnectionFactory,
            queueUri,
            1, // 1 connection
            mockQueueManager,
            mockQueueMetrics,
            mockWarningService,
            1000, // receiveTimeoutMs
            5     // metricsPollIntervalSeconds
        );
    }

    @AfterEach
    void tearDown() {
        if (activeMqConsumer != null) {
            activeMqConsumer.stop();
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

        TextMessage textMessage = mock(TextMessage.class);
        when(textMessage.getText()).thenReturn(messageBody);

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(textMessage)
            .thenReturn(null); // Stop polling after first message

        doNothing().when(mockQueueManager).routeMessageBatch(anyList());

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
            verify(mockQueueMetrics).recordMessageReceived(queueUri);
        });
    }

    @Test
    void shouldAcknowledgeMessageOnAck() throws Exception {
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

        TextMessage textMessage = mock(TextMessage.class);
        when(textMessage.getText()).thenReturn(messageBody);

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(textMessage)
            .thenReturn(null);

        @SuppressWarnings("unchecked")
        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        activeMqConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-ack", "POOL-A", "token", MediationType.HTTP, "http://test.com", "test-group", null);
        callback.ack(testMessage);

        // Then
        verify(textMessage).acknowledge();
    }

    @Test
    void shouldRecoverSessionOnNack() throws Exception {
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

        TextMessage textMessage = mock(TextMessage.class);
        when(textMessage.getText()).thenReturn(messageBody);

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(textMessage)
            .thenReturn(null);

        @SuppressWarnings("unchecked")
        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        activeMqConsumer.start();

        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Extract callback from captured batch
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertFalse(batch.isEmpty());
        MessageCallback callback = batch.get(0).callback();
        MessagePointer testMessage = new MessagePointer("msg-nack", "POOL-A", "token", MediationType.HTTP, "http://test.com", "test-group", null);
        callback.nack(testMessage);

        // Then
        // With INDIVIDUAL_ACKNOWLEDGE mode, nack does NOT call session.recover()
        // The message is simply not acknowledged and will be redelivered when session closes
        verify(mockSession, never()).recover();
        verify(textMessage, never()).acknowledge();
    }

    @Test
    void shouldUseIndividualAcknowledgeMode() throws Exception {
        // Given
        when(mockMessageConsumer.receive(anyLong())).thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then
        // Verify INDIVIDUAL_ACKNOWLEDGE mode is used (prevents head-of-line blocking)
        await().untilAsserted(() -> {
            verify(mockConnection, atLeastOnce()).createSession(false, ActiveMQSession.INDIVIDUAL_ACKNOWLEDGE);
        });
    }

    @Test
    void shouldPollQueueMetricsUsingBrowser() throws Exception {
        // Given
        @SuppressWarnings("unchecked")
        Enumeration<Message> mockEnumeration = mock(Enumeration.class);
        when(mockEnumeration.hasMoreElements())
            .thenReturn(true)
            .thenReturn(true)
            .thenReturn(true)
            .thenReturn(false); // 3 messages in queue

        when(mockQueueBrowser.getEnumeration()).thenReturn(mockEnumeration);
        when(mockMessageConsumer.receive(anyLong())).thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockQueueMetrics, atLeastOnce()).recordQueueMetrics(queueUri, 3, 0);
        });
    }

    @Test
    void shouldStopGracefully() throws Exception {
        // Given
        when(mockMessageConsumer.receive(anyLong())).thenReturn(null);
        activeMqConsumer.start();

        // When
        activeMqConsumer.stop();

        // Then - wait for async cleanup to complete
        await().untilAsserted(() -> {
            verify(mockMessageConsumer, atLeastOnce()).close();
            verify(mockSession, atLeastOnce()).close();
            verify(mockConnection, atLeastOnce()).close();
        });
    }

    @Test
    void shouldGetQueueIdentifier() {
        assertEquals(queueUri, activeMqConsumer.getQueueIdentifier());
    }

    @Test
    void shouldHandleInvalidJson() throws Exception {
        // Given
        TextMessage invalidMessage = mock(TextMessage.class);
        when(invalidMessage.getText()).thenReturn("{ invalid json }");

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(invalidMessage)
            .thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            // Malformed messages should NOT be counted in recordMessageReceived
            // They're data quality issues that should be caught upstream
            verify(mockQueueMetrics, never()).recordMessageReceived(queueUri);
            verify(mockQueueMetrics).recordMessageProcessed(queueUri, false);
            verify(mockQueueManager, never()).routeMessageBatch(anyList());
        });
    }

    @Test
    void shouldHandleNonTextMessage() throws Exception {
        // Given
        Message nonTextMessage = mock(Message.class); // Not a TextMessage

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(nonTextMessage)
            .thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            // Should ignore non-text messages
            verify(mockQueueManager, never()).routeMessageBatch(anyList());
        });
    }

    @Test
    void shouldRouteBatchMessages() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "msg-batch",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8080/test",
                "messageGroupId": "test-group"
            }
            """;

        TextMessage textMessage = mock(TextMessage.class);
        when(textMessage.getText()).thenReturn(messageBody);

        when(mockMessageConsumer.receive(anyLong()))
            .thenReturn(textMessage)
            .thenReturn(null);

        @SuppressWarnings("unchecked")
        ArgumentCaptor<List<QueueManager.BatchMessage>> batchCaptor = ArgumentCaptor.forClass(List.class);
        doNothing().when(mockQueueManager).routeMessageBatch(batchCaptor.capture());

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockQueueManager).routeMessageBatch(anyList());
        });

        // Verify batch contains the message
        List<QueueManager.BatchMessage> batch = batchCaptor.getValue();
        assertEquals(1, batch.size());
        assertEquals("msg-batch", batch.get(0).message().id());
        assertEquals("POOL-A", batch.get(0).message().poolCode());
        assertEquals("test-group", batch.get(0).message().messageGroupId());
    }

    @Test
    void shouldHandleConnectionErrors() throws Exception {
        // Given - create a new connection factory that fails
        ConnectionFactory failingConnectionFactory = mock(ConnectionFactory.class);
        when(failingConnectionFactory.createConnection())
            .thenThrow(new JMSException("Connection failed"));

        // When/Then - constructor should throw when connection creation fails
        assertThrows(RuntimeException.class, () -> {
            new ActiveMqQueueConsumer(
                failingConnectionFactory,
                queueUri,
                1,
                mockQueueManager,
                mockQueueMetrics,
                mockWarningService,
                1000,
                5
            );
        });
    }

    @Test
    void shouldCreateBrowserForMetrics() throws Exception {
        // Given
        @SuppressWarnings("unchecked")
        Enumeration<Message> mockEnumeration = mock(Enumeration.class);
        when(mockEnumeration.hasMoreElements()).thenReturn(false);
        when(mockQueueBrowser.getEnumeration()).thenReturn(mockEnumeration);
        when(mockMessageConsumer.receive(anyLong())).thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then
        await().untilAsserted(() -> {
            verify(mockSession, atLeast(1)).createBrowser(mockQueue);
        });
    }

    @Test
    void shouldPollMetricsEvery5Seconds() throws Exception {
        // Given
        @SuppressWarnings("unchecked")
        Enumeration<Message> mockEnumeration = mock(Enumeration.class);
        when(mockEnumeration.hasMoreElements()).thenReturn(false);
        when(mockQueueBrowser.getEnumeration()).thenReturn(mockEnumeration);
        when(mockMessageConsumer.receive(anyLong())).thenReturn(null);

        // When
        activeMqConsumer.start();

        // Then - verify metrics are recorded multiple times (polling every 5 seconds)
        await().untilAsserted(() -> {
            verify(mockQueueMetrics, atLeast(2)).recordQueueMetrics(eq(queueUri), anyLong(), eq(0L));
        });
    }
}

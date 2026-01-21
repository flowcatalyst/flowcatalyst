package tech.flowcatalyst.messagerouter.integration;

import jakarta.jms.*;
import org.apache.activemq.ActiveMQConnectionFactory;
import org.apache.activemq.ActiveMQSession;
import org.junit.jupiter.api.*;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.containers.wait.strategy.Wait;
import org.testcontainers.utility.DockerImageName;

import java.time.Duration;
import java.util.UUID;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Integration test using real ActiveMQ Classic container.
 * This test verifies the complete ActiveMQ message flow with a real broker.
 */
@Tag("integration")
class ActiveMqClassicIntegrationTest {

    private static GenericContainer<?> activemqContainer;
    private static ConnectionFactory connectionFactory;

    private Connection connection;
    private Session session;
    private String testQueueName;
    private Queue testQueue;

    @BeforeAll
    static void startContainer() {
        activemqContainer = new GenericContainer<>(DockerImageName.parse("apache/activemq-classic:latest"))
            .withExposedPorts(61616)
            .waitingFor(Wait.forListeningPort().withStartupTimeout(Duration.ofMinutes(2)))
            .withReuse(true);

        activemqContainer.start();

        String brokerUrl = String.format("tcp://%s:%d",
            activemqContainer.getHost(),
            activemqContainer.getMappedPort(61616));

        connectionFactory = new ActiveMQConnectionFactory("admin", "admin", brokerUrl);
    }

    @AfterAll
    static void stopContainer() {
        if (activemqContainer != null) {
            activemqContainer.stop();
        }
    }

    @BeforeEach
    void setUp() throws Exception {
        // Create unique test queue name
        testQueueName = "test.queue." + UUID.randomUUID().toString();

        // Create connection and session
        connection = connectionFactory.createConnection();
        connection.start();
        session = connection.createSession(false, Session.AUTO_ACKNOWLEDGE);
        testQueue = session.createQueue(testQueueName);
    }

    @AfterEach
    void tearDown() throws Exception {
        if (session != null) {
            try {
                session.close();
            } catch (Exception e) {
                // Ignore
            }
        }

        if (connection != null) {
            try {
                connection.close();
            } catch (Exception e) {
                // Ignore
            }
        }
    }

    @Test
    void shouldConnectToActiveMqContainer() throws Exception {
        assertNotNull(connection);
        assertNotNull(session);
        assertNotNull(testQueue);
    }

    @Test
    void shouldSendAndReceiveMessageFromActiveMq() throws Exception {
        // Given
        String messageBody = """
            {
                "id": "activemq-msg-1",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8081/test/webhook"
            }
            """;

        // When - Send message
        MessageProducer producer = session.createProducer(testQueue);
        TextMessage message = session.createTextMessage(messageBody);
        producer.send(message);
        producer.close();

        // Then - Receive message
        MessageConsumer messageConsumer = session.createConsumer(testQueue);
        Message receivedMessage = messageConsumer.receive(5000);

        assertNotNull(receivedMessage);
        assertTrue(receivedMessage instanceof TextMessage);
        assertEquals(messageBody.trim(), ((TextMessage) receivedMessage).getText().trim());

        messageConsumer.close();
    }

    @Test
    void shouldAcknowledgeMessageWithIndividualMode() throws Exception {
        // Given - Session with INDIVIDUAL_ACKNOWLEDGE mode
        Session individualAckSession = connection.createSession(false, ActiveMQSession.INDIVIDUAL_ACKNOWLEDGE);
        Queue queue = individualAckSession.createQueue(testQueueName);

        String messageBody = """
            {
                "id": "ack-msg",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8081/test/webhook"
            }
            """;

        // Send message
        MessageProducer producer = individualAckSession.createProducer(queue);
        TextMessage message = individualAckSession.createTextMessage(messageBody);
        producer.send(message);
        producer.close();

        // When - Receive and acknowledge
        MessageConsumer messageConsumer = individualAckSession.createConsumer(queue);
        Message receivedMessage = messageConsumer.receive(5000);

        assertNotNull(receivedMessage);
        receivedMessage.acknowledge(); // Individual acknowledgment

        // Then - Message should not be redelivered
        messageConsumer.close();
        individualAckSession.close();

        // Verify message is gone
        Session verifySession = connection.createSession(false, Session.AUTO_ACKNOWLEDGE);
        MessageConsumer verifyConsumer = verifySession.createConsumer(verifySession.createQueue(testQueueName));
        Message shouldBeNull = verifyConsumer.receive(1000);

        assertNull(shouldBeNull, "Message should be acknowledged and removed");

        verifyConsumer.close();
        verifySession.close();
    }

    @Test
    void shouldRedeliverUnacknowledgedMessage() throws Exception {
        // Given - Session with CLIENT_ACKNOWLEDGE mode
        Session clientAckSession = connection.createSession(false, Session.CLIENT_ACKNOWLEDGE);
        Queue queue = clientAckSession.createQueue(testQueueName);

        String messageBody = """
            {
                "id": "redelivery-msg",
                "poolCode": "POOL-A",
                "authToken": "test-token",
                "mediationType": "HTTP",
                "mediationTarget": "http://localhost:8081/test/webhook"
            }
            """;

        // Send message
        MessageProducer producer = clientAckSession.createProducer(queue);
        TextMessage message = clientAckSession.createTextMessage(messageBody);
        producer.send(message);
        producer.close();

        // When - Receive but DON'T acknowledge (simulate nack)
        MessageConsumer firstConsumer = clientAckSession.createConsumer(queue);
        Message firstReceive = firstConsumer.receive(5000);

        assertNotNull(firstReceive);

        // Close session without acknowledging - triggers redelivery
        firstConsumer.close();
        clientAckSession.close();

        // Then - Message should be redelivered
        Session redeliverySession = connection.createSession(false, Session.AUTO_ACKNOWLEDGE);
        MessageConsumer redeliveryConsumer = redeliverySession.createConsumer(redeliverySession.createQueue(testQueueName));
        Message redeliveredMessage = redeliveryConsumer.receive(5000);

        assertNotNull(redeliveredMessage, "Message should be redelivered after unacknowledged close");
        assertTrue(redeliveredMessage.getJMSRedelivered(), "Message should be marked as redelivered");

        redeliveryConsumer.close();
        redeliverySession.close();
    }

    @Test
    void shouldHandleMultipleMessages() throws Exception {
        // Given - Send 5 messages
        MessageProducer producer = session.createProducer(testQueue);

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

            TextMessage message = session.createTextMessage(messageBody);
            producer.send(message);
        }
        producer.close();

        // When - Receive all messages
        MessageConsumer messageConsumer = session.createConsumer(testQueue);
        int receivedCount = 0;

        for (int i = 0; i < 5; i++) {
            Message receivedMessage = messageConsumer.receive(2000);
            if (receivedMessage != null) {
                receivedCount++;
            }
        }

        // Then
        assertEquals(5, receivedCount, "Should receive all 5 messages");

        messageConsumer.close();
    }

    @Test
    void shouldBrowseQueueWithoutConsuming() throws Exception {
        // Given - Send 3 messages
        MessageProducer producer = session.createProducer(testQueue);

        for (int i = 0; i < 3; i++) {
            String messageBody = String.format("""
                {
                    "id": "browse-msg-%d",
                    "poolCode": "POOL-A",
                    "authToken": "test-token",
                    "mediationType": "HTTP",
                    "mediationTarget": "http://localhost:8081/test/webhook"
                }
                """, i);

            TextMessage message = session.createTextMessage(messageBody);
            producer.send(message);
        }
        producer.close();

        // When - Browse queue (doesn't consume messages)
        QueueBrowser browser = session.createBrowser(testQueue);
        int browseCount = 0;

        var enumeration = browser.getEnumeration();
        while (enumeration.hasMoreElements()) {
            enumeration.nextElement();
            browseCount++;
        }
        browser.close();

        // Then - Should see 3 messages
        assertEquals(3, browseCount, "Should browse 3 messages");

        // Verify messages are still in queue
        MessageConsumer messageConsumer = session.createConsumer(testQueue);
        int consumeCount = 0;

        for (int i = 0; i < 3; i++) {
            Message receivedMessage = messageConsumer.receive(2000);
            if (receivedMessage != null) {
                consumeCount++;
            }
        }

        assertEquals(3, consumeCount, "Messages should still be in queue after browsing");

        messageConsumer.close();
    }

    @Test
    void shouldSupportDifferentMessageTypes() throws Exception {
        // Given
        MessageProducer producer = session.createProducer(testQueue);

        // Text message
        TextMessage textMessage = session.createTextMessage("test text");
        producer.send(textMessage);

        // Object message
        ObjectMessage objectMessage = session.createObjectMessage("test object");
        producer.send(objectMessage);

        producer.close();

        // When/Then
        MessageConsumer messageConsumer = session.createConsumer(testQueue);

        Message msg1 = messageConsumer.receive(2000);
        assertNotNull(msg1);
        assertTrue(msg1 instanceof TextMessage);

        Message msg2 = messageConsumer.receive(2000);
        assertNotNull(msg2);
        assertTrue(msg2 instanceof ObjectMessage);

        messageConsumer.close();
    }

    @Test
    void shouldSetMessageProperties() throws Exception {
        // Given
        MessageProducer producer = session.createProducer(testQueue);
        TextMessage message = session.createTextMessage("test");

        // Set custom properties
        message.setStringProperty("customKey", "customValue");
        message.setIntProperty("priority", 5);

        producer.send(message);
        producer.close();

        // When
        MessageConsumer messageConsumer = session.createConsumer(testQueue);
        Message receivedMessage = messageConsumer.receive(5000);

        // Then
        assertNotNull(receivedMessage);
        assertEquals("customValue", receivedMessage.getStringProperty("customKey"));
        assertEquals(5, receivedMessage.getIntProperty("priority"));

        messageConsumer.close();
    }

    @Test
    void shouldHandleTransactionalSession() throws Exception {
        // Given - Transactional session
        Session txSession = connection.createSession(true, Session.SESSION_TRANSACTED);
        Queue queue = txSession.createQueue(testQueueName);

        MessageProducer producer = txSession.createProducer(queue);
        TextMessage message = txSession.createTextMessage("transactional message");

        // When - Send but don't commit
        producer.send(message);

        // Then - Message should not be visible yet
        MessageConsumer messageConsumer = txSession.createConsumer(queue);
        Message shouldBeNull = messageConsumer.receive(1000);
        assertNull(shouldBeNull, "Message should not be visible before commit");

        // When - Commit transaction
        txSession.commit();

        // Then - Message should now be visible
        Message receivedMessage = messageConsumer.receive(2000);
        assertNotNull(receivedMessage, "Message should be visible after commit");

        producer.close();
        messageConsumer.close();
        txSession.close();
    }
}

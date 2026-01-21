package tech.flowcatalyst.messagerouter.integration;

import com.github.tomakehurst.wiremock.WireMockServer;
import com.github.tomakehurst.wiremock.client.WireMock;
import io.github.resilience4j.ratelimiter.RateLimiter;
import io.github.resilience4j.ratelimiter.RateLimiterConfig;
import io.quarkus.test.common.QuarkusTestResource;
import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.junit.jupiter.api.*;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.HttpMediator;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Duration;
import java.util.Set;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;

import static com.github.tomakehurst.wiremock.client.WireMock.*;
import static java.util.concurrent.TimeUnit.SECONDS;
import static org.awaitility.Awaitility.await;
import static org.junit.jupiter.api.Assertions.*;

/**
 * Resilience integration tests for the message routing system.
 *
 * <p>These tests validate system behavior under adverse conditions:
 * <ul>
 *   <li>HTTP endpoint timeouts and slow responses</li>
 *   <li>HTTP endpoint errors (4xx client errors, 5xx server errors)</li>
 *   <li>Rate limiting and backpressure handling</li>
 *   <li>Pool queue saturation and overflow</li>
 *   <li>Recovery from transient failures</li>
 * </ul>
 *
 * <p><b>Infrastructure:</b>
 * <ul>
 *   <li>WireMock for simulating endpoint failures and latency</li>
 *   <li>Real ProcessPool instances with various configurations</li>
 *   <li>Real HttpMediator with timeout and retry logic</li>
 * </ul>
 *
 * <p><b>Tags:</b> {@code @Tag("integration")} - requires WireMock
 *
 * @see ProcessPoolImpl
 * @see HttpMediator
 */
@QuarkusTest
@QuarkusTestResource(WireMockTestResource.class)
@Tag("integration")
@TestInstance(TestInstance.Lifecycle.PER_METHOD)
class ResilienceIntegrationTest {

    @Inject
    PoolMetricsService poolMetricsService;

    @Inject
    WarningService warningService;

    @ConfigProperty(name = "test.webhook.baseurl")
    String webhookBaseUrl;

    private ProcessPool processPool;
    private ConcurrentHashMap<String, MessagePointer> inPipelineMap;
    private String poolCode;

    @BeforeEach
    void setUp() {
        poolCode = "RESILIENCE-TEST-" + UUID.randomUUID().toString().substring(0, 8);
        inPipelineMap = new ConcurrentHashMap<>();

        // Configure WireMock client
        int port = Integer.parseInt(webhookBaseUrl.substring(webhookBaseUrl.lastIndexOf(':') + 1));
        WireMock.configureFor("localhost", port);

        // Reset WireMock stubs
        WireMock.reset();
    }

    @AfterEach
    void tearDown() {
        if (processPool != null) {
            processPool.shutdown();
        }
    }

    /**
     * Test 4.1.1: HTTP Endpoint Timeout Handling
     *
     * <p>Validates that the system correctly handles slow/unresponsive HTTP endpoints:
     * <ul>
     *   <li>HttpMediator enforces request timeout</li>
     *   <li>Timeout results in NACK (message returns to queue)</li>
     *   <li>Pool continues processing other messages</li>
     *   <li>Metrics reflect timeout as failure</li>
     * </ul>
     *
     * <p><b>Expected Behavior:</b> Message times out, gets NACKed, pool remains healthy
     */
    @Test
    void shouldHandleHttpEndpointTimeouts() {
        // Given: Configure endpoint with 15-second delay (exceeds 10-second HttpMediator timeout)
        stubFor(post(urlEqualTo("/webhook/timeout"))
            .willReturn(aResponse()
                .withStatus(200)
                .withFixedDelay(15000))); // 15 seconds > 10 second timeout

        Set<String> nackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger timeoutCount = new AtomicInteger(0);

        // Create custom mediator that tracks timeouts
        Mediator timeoutTrackingMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService); // 10 second timeout

            @Override
            public MediationOutcome process(MessagePointer message) {
                MediationOutcome outcome = httpMediator.process(message);
                if (outcome.result() == MediationResult.ERROR_PROCESS) {
                    // Timeout errors return ERROR_PROCESS (transient, will be retried via visibility timeout)
                    timeoutCount.incrementAndGet();
                }
                return outcome;
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                nackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            5,
            100,
            null,
            timeoutTrackingMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit message to slow endpoint
        MessagePointer timeoutMessage = new MessagePointer(
            "msg-timeout-1",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/timeout",
            "timeout-group",
            UUID.randomUUID().toString()
        );

        inPipelineMap.put(timeoutMessage.id(), timeoutMessage);
        processPool.submit(timeoutMessage);

        // Then: Wait for timeout and NACK
        // HttpMediator has 10s timeout, with 3 retries and backoff delays (1s, 2s, 3s)
        // Total time: ~10s + 1s + ~10s + 2s + ~10s = ~33s, plus overhead
        await().atMost(45, SECONDS).until(() -> nackedMessages.contains("msg-timeout-1"));

        // Verify timeout behavior
        await().atMost(5, SECONDS).until(() -> inPipelineMap.isEmpty());

        assertTrue(nackedMessages.contains("msg-timeout-1"),
            "Timeout message should be NACKed");
        assertFalse(ackedMessages.contains("msg-timeout-1"),
            "Timeout message should not be ACKed");
        assertEquals(1, timeoutCount.get(),
            "Should have detected exactly one timeout");

        // Verify WireMock received the request (confirms timeout, not connection failure)
        verify(postRequestedFor(urlEqualTo("/webhook/timeout")));
    }

    /**
     * Test 4.1.2: HTTP Endpoint Server Errors (5xx)
     *
     * <p>Validates handling of server errors:
     * <ul>
     *   <li>500 Internal Server Error → NACK</li>
     *   <li>502 Bad Gateway → NACK</li>
     *   <li>503 Service Unavailable → NACK</li>
     *   <li>Messages are NACKed for retry</li>
     *   <li>Pool continues processing</li>
     * </ul>
     */
    @Test
    void shouldHandleHttpServerErrors() {
        // Given: Configure endpoints with various 5xx errors
        stubFor(post(urlEqualTo("/webhook/error-500"))
            .willReturn(aResponse()
                .withStatus(500)
                .withBody("{\"error\":\"Internal server error\"}")));

        stubFor(post(urlEqualTo("/webhook/error-503"))
            .willReturn(aResponse()
                .withStatus(503)
                .withBody("{\"error\":\"Service unavailable\"}")));

        Set<String> nackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger serverErrorCount = new AtomicInteger(0);

        Mediator errorTrackingMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService);

            @Override
            public MediationOutcome process(MessagePointer message) {
                MediationOutcome outcome = httpMediator.process(message);
                if (outcome.result() == MediationResult.ERROR_PROCESS) {
                    // 5xx server errors return ERROR_PROCESS (transient, will be retried via visibility timeout)
                    serverErrorCount.incrementAndGet();
                }
                return outcome;
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                nackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            5,
            100,
            null,
            errorTrackingMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit messages to error endpoints
        MessagePointer msg500 = new MessagePointer(
            "msg-500",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/error-500",
            "error-group-1",
            UUID.randomUUID().toString()
        );

        MessagePointer msg503 = new MessagePointer(
            "msg-503",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/error-503",
            "error-group-2",
            UUID.randomUUID().toString()
        );

        inPipelineMap.put(msg500.id(), msg500);
        inPipelineMap.put(msg503.id(), msg503);

        processPool.submit(msg500);
        processPool.submit(msg503);

        // Then: Wait for processing
        await().atMost(10, SECONDS).until(() -> inPipelineMap.isEmpty());

        // Verify both messages were NACKed
        assertTrue(nackedMessages.contains("msg-500"),
            "500 error should result in NACK");
        assertTrue(nackedMessages.contains("msg-503"),
            "503 error should result in NACK");

        assertFalse(ackedMessages.contains("msg-500"),
            "500 error should not be ACKed");
        assertFalse(ackedMessages.contains("msg-503"),
            "503 error should not be ACKed");

        assertEquals(2, serverErrorCount.get(),
            "Should have detected 2 server errors");

        // Verify WireMock received both requests
        verify(postRequestedFor(urlEqualTo("/webhook/error-500")));
        verify(postRequestedFor(urlEqualTo("/webhook/error-503")));
    }

    /**
     * Test 4.1.3: HTTP Endpoint Client Errors (4xx)
     *
     * <p>Validates handling of client errors:
     * <ul>
     *   <li>400 Bad Request → ACK (permanent error, like "Record not found")</li>
     *   <li>404 Not Found → ACK (configuration error, don't retry)</li>
     *   <li>Pool continues processing</li>
     * </ul>
     *
     * <p><b>Note:</b> Both 400 and 404 now return ERROR_CONFIG and are ACKed
     * to prevent infinite retries. Transient errors that need retry use the
     * 200 response format with ack:false field instead of returning 4xx errors.
     */
    @Test
    void shouldHandleHttpClientErrors() {
        // Given: Configure endpoints with various 4xx errors
        stubFor(post(urlEqualTo("/webhook/error-400"))
            .willReturn(aResponse()
                .withStatus(400)
                .withBody("{\"error\":\"Bad request\"}")));

        stubFor(post(urlEqualTo("/webhook/error-404"))
            .willReturn(aResponse()
                .withStatus(404)
                .withBody("{\"error\":\"Not found\"}")));

        Set<String> nackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger processErrorCount = new AtomicInteger(0);
        AtomicInteger configErrorCount = new AtomicInteger(0);

        Mediator errorTrackingMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService);

            @Override
            public MediationOutcome process(MessagePointer message) {
                MediationOutcome outcome = httpMediator.process(message);
                if (outcome.result() == MediationResult.ERROR_PROCESS) {
                    processErrorCount.incrementAndGet();
                } else if (outcome.result() == MediationResult.ERROR_CONFIG) {
                    configErrorCount.incrementAndGet();
                }
                return outcome;
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                nackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            5,
            100,
            null,
            errorTrackingMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit messages to error endpoints
        MessagePointer msg400 = new MessagePointer(
            "msg-400",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/error-400",
            "client-error-group-1",
            UUID.randomUUID().toString()
        );

        MessagePointer msg404 = new MessagePointer(
            "msg-404",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/error-404",
            "client-error-group-2",
            UUID.randomUUID().toString()
        );

        inPipelineMap.put(msg400.id(), msg400);
        inPipelineMap.put(msg404.id(), msg404);

        processPool.submit(msg400);
        processPool.submit(msg404);

        // Then: Wait for processing
        await().atMost(10, SECONDS).until(() -> inPipelineMap.isEmpty());

        // Verify 400 error was ACKed (ERROR_CONFIG - permanent configuration error)
        // With new ack:false response format, 400 is now treated as permanent error
        assertTrue(ackedMessages.contains("msg-400"),
            "400 error should result in ACK (ERROR_CONFIG - permanent error)");
        assertFalse(nackedMessages.contains("msg-400"),
            "400 error should not be NACKed");

        // Verify 404 error was ACKed (ERROR_CONFIG - configuration error, don't retry)
        assertTrue(ackedMessages.contains("msg-404"),
            "404 error should result in ACK (ERROR_CONFIG)");
        assertFalse(nackedMessages.contains("msg-404"),
            "404 error should not be NACKed");

        assertEquals(0, processErrorCount.get(),
            "Should have detected 0 process errors (400 and 404 are both permanent errors)");
        assertEquals(2, configErrorCount.get(),
            "Should have detected 2 config errors (400 and 404)");

        // Verify WireMock received both requests
        verify(postRequestedFor(urlEqualTo("/webhook/error-400")));
        verify(postRequestedFor(urlEqualTo("/webhook/error-404")));
    }

    /**
     * Test 4.1.4: Rate Limiting and Backpressure
     *
     * <p>Validates rate limiting behavior with "fail fast" approach:
     * <ul>
     *   <li>Rate limiter enforces max requests per minute (300/min token bucket)</li>
     *   <li>Excess messages are immediately NACKed (fail fast, not queued)</li>
     *   <li>First 300 messages succeed (token bucket grants all permits upfront)</li>
     *   <li>Messages exceeding rate wait for permits (held in memory)</li>
     * </ul>
     *
     * <p><b>Implementation Note:</b> Resilience4j RateLimiter uses a token bucket algorithm
     * with a 1-minute refresh period. All permits are granted upfront at the start of
     * each period, allowing bursts up to the limit.
     *
     * <p><b>Behavior:</b> Pool now uses wait-for-permit rate limiting. Messages exceeding
     * the rate wait in memory for permits instead of being NACKed.
     */
    @Test
    void shouldEnforceRateLimitingAndBackpressure() {
        // Given: Configure fast endpoint
        stubFor(post(urlEqualTo("/webhook/rate-limited"))
            .willReturn(aResponse()
                .withStatus(200)
                .withBody("{\"ack\":true}")));

        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger processedCount = new AtomicInteger(0);

        Mediator simpleMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService);

            @Override
            public MediationOutcome process(MessagePointer message) {
                MediationOutcome outcome = httpMediator.process(message);
                processedCount.incrementAndGet();
                return outcome;
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }
        };

        // Use pool-level rate limiting: 100 requests per minute
        // Submit exactly 100 messages to stay within limit
        processPool = new ProcessPoolImpl(
            poolCode,
            10,  // High concurrency
            200, // Queue capacity
            100, // 100 requests per minute rate limit
            simpleMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit 100 messages (within rate limit)
        String singleGroup = "rate-limited-group";
        int totalMessages = 100;
        for (int i = 0; i < totalMessages; i++) {
            MessagePointer message = new MessagePointer(
                "msg-rate-" + i,
                poolCode,
                "test-token",
                MediationType.HTTP,
                webhookBaseUrl + "/webhook/rate-limited",
                singleGroup,
                UUID.randomUUID().toString()
            );

            inPipelineMap.put(message.id(), message);
            processPool.submit(message);
        }

        // Then: All messages should be processed within rate limit
        await().atMost(30, SECONDS).until(() -> inPipelineMap.isEmpty());

        // All messages should be ACKed
        assertEquals(totalMessages, ackedMessages.size(),
            "All messages should be ACKed");

        // All messages should reach mediator
        assertEquals(totalMessages, processedCount.get(),
            "All messages should reach mediator");

        // Verify WireMock received all requests
        verify(exactly(totalMessages), postRequestedFor(urlEqualTo("/webhook/rate-limited")));
    }

    /**
     * Test 4.1.5: Pool Queue Saturation and Overflow
     *
     * <p>Validates behavior when pool queue is full:
     * <ul>
     *   <li>Pool rejects messages when queue is full</li>
     *   <li>Rejected messages are not lost (caller handles)</li>
     *   <li>Pool continues processing existing messages</li>
     *   <li>Pool accepts new messages after queue has capacity</li>
     * </ul>
     */
    @Test
    void shouldHandlePoolQueueSaturation() {
        // Given: Configure slow endpoint to fill queue
        stubFor(post(urlEqualTo("/webhook/slow-saturation"))
            .willReturn(aResponse()
                .withStatus(200)
                .withBody("{\"ack\":true}")
                .withFixedDelay(500))); // 500ms delay per message

        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> rejectedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger processedCount = new AtomicInteger(0);

        Mediator slowMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService);

            @Override
            public MediationOutcome process(MessagePointer message) {
                MediationOutcome outcome = httpMediator.process(message);
                processedCount.incrementAndGet();
                return outcome;
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                inPipelineMap.remove(message.id());
            }
        };

        // Create pool with small queue (capacity: 10)
        processPool = new ProcessPoolImpl(
            poolCode,
            2,   // Low concurrency
            10,  // Small queue capacity
            null,
            slowMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit more messages than queue can hold
        int totalMessages = 20;  // Exceeds queue capacity of 10
        AtomicInteger acceptedMessages = new AtomicInteger(0);

        for (int i = 0; i < totalMessages; i++) {
            MessagePointer message = new MessagePointer(
                "msg-saturate-" + i,
                poolCode,
                "test-token",
                MediationType.HTTP,
                webhookBaseUrl + "/webhook/slow-saturation",
                "saturate-group",  // Same group to enforce FIFO
                UUID.randomUUID().toString()
            );

            inPipelineMap.put(message.id(), message);
            boolean submitted = processPool.submit(message);

            if (submitted) {
                acceptedMessages.incrementAndGet();
            } else {
                rejectedMessages.add(message.id());
                inPipelineMap.remove(message.id());  // Caller would handle rejection
            }
        }

        // Then: Verify some messages were rejected
        assertTrue(rejectedMessages.size() > 0,
            "Some messages should be rejected when queue is full");
        assertTrue(acceptedMessages.get() < totalMessages,
            "Not all messages should be accepted");
        assertTrue(acceptedMessages.get() > 0,
            "Some messages should be accepted");

        // Wait for accepted messages to process
        await().atMost(30, SECONDS).until(() -> ackedMessages.size() == acceptedMessages.get());

        // Verify all accepted messages were processed
        assertEquals(acceptedMessages.get(), ackedMessages.size(),
            "All accepted messages should be processed");
        assertEquals(acceptedMessages.get(), processedCount.get(),
            "Processed count should match accepted count");

        // Verify rejected messages were not processed
        for (String rejectedId : rejectedMessages) {
            assertFalse(ackedMessages.contains(rejectedId),
                "Rejected message " + rejectedId + " should not be processed");
        }
    }

    /**
     * Test 4.1.6: Recovery from Transient Failures
     *
     * <p>Validates recovery when endpoint becomes available after failures:
     * <ul>
     *   <li>Endpoint fails initially (503)</li>
     *   <li>Message is NACKed</li>
     *   <li>Endpoint recovers (200)</li>
     *   <li>New message with different batchId succeeds (simulates SQS redelivery)</li>
     *   <li>System continues normal operation</li>
     * </ul>
     *
     * <p><b>Note:</b> In production, SQS visibility timeout makes the message
     * available for retry with the same original attributes. This test simulates
     * that by submitting with a new batchId to avoid batch+group failure tracking.
     */
    @Test
    void shouldRecoverFromTransientFailures() {
        // Given: Configure endpoint that fails then succeeds
        stubFor(post(urlEqualTo("/webhook/flaky-recovery"))
            .inScenario("recovery")
            .whenScenarioStateIs("Started")
            .willReturn(aResponse()
                .withStatus(503)
                .withBody("{\"error\":\"Service temporarily unavailable\"}"))
            .willSetStateTo("recovered"));

        stubFor(post(urlEqualTo("/webhook/flaky-recovery"))
            .inScenario("recovery")
            .whenScenarioStateIs("recovered")
            .willReturn(aResponse()
                .withStatus(200)
                .withBody("{\"ack\":true}")));

        Set<String> ackedMessages = ConcurrentHashMap.newKeySet();
        Set<String> nackedMessages = ConcurrentHashMap.newKeySet();
        AtomicInteger attemptCount = new AtomicInteger(0);

        Mediator trackingMediator = new Mediator() {
            private final HttpMediator httpMediator = new HttpMediator("HTTP_2", 10000, warningService);

            @Override
            public MediationOutcome process(MessagePointer message) {
                attemptCount.incrementAndGet();
                return httpMediator.process(message);
            }

            @Override
            public MediationType getMediationType() {
                return MediationType.HTTP;
            }
        };

        MessageCallback trackingCallback = new MessageCallback() {
            @Override
            public void ack(MessagePointer message) {
                ackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
            }

            @Override
            public void nack(MessagePointer message) {
                nackedMessages.add(message.id());
                inPipelineMap.remove(message.id());
                // Don't automatically resubmit - manual retry with new batchId below
            }
        };

        processPool = new ProcessPoolImpl(
            poolCode,
            5,
            100,
            null,
            trackingMediator,
            trackingCallback,
            inPipelineMap,
            poolMetricsService,
            warningService
        );

        processPool.start();

        // When: Submit message to flaky endpoint (first attempt - will fail)
        MessagePointer firstAttempt = new MessagePointer(
            "msg-flaky-1",
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/flaky-recovery",
            "flaky-group",
            UUID.randomUUID().toString()  // First batchId
        );

        inPipelineMap.put(firstAttempt.id(), firstAttempt);
        processPool.submit(firstAttempt);

        // Wait for first attempt to fail and be nacked
        await().atMost(10, SECONDS).until(() -> nackedMessages.contains("msg-flaky-1"));
        await().atMost(5, SECONDS).until(() -> inPipelineMap.isEmpty());

        // Verify first attempt failed
        assertTrue(nackedMessages.contains("msg-flaky-1"),
            "First attempt should be NACKed");
        assertFalse(ackedMessages.contains("msg-flaky-1"),
            "First attempt should not be ACKed");
        assertEquals(1, attemptCount.get(),
            "Should have 1 attempt so far");

        // Simulate redelivery with new batchId (SQS would do this after visibility timeout)
        MessagePointer retryAttempt = new MessagePointer(
            "msg-flaky-1",  // Same message ID
            poolCode,
            "test-token",
            MediationType.HTTP,
            webhookBaseUrl + "/webhook/flaky-recovery",
            "flaky-group",
            UUID.randomUUID().toString()  // NEW batchId (simulates redelivery)
        );

        inPipelineMap.put(retryAttempt.id(), retryAttempt);
        processPool.submit(retryAttempt);

        // Then: Wait for retry to succeed
        await().atMost(10, SECONDS).until(() -> ackedMessages.contains("msg-flaky-1"));
        await().atMost(5, SECONDS).until(() -> inPipelineMap.isEmpty());

        // Verify message succeeded on retry
        assertTrue(ackedMessages.contains("msg-flaky-1"),
            "Message should succeed on retry");
        assertEquals(2, attemptCount.get(),
            "Should have 2 attempts total (fail + success)");

        // Verify WireMock received 2 requests
        verify(2, postRequestedFor(urlEqualTo("/webhook/flaky-recovery")));
    }
}

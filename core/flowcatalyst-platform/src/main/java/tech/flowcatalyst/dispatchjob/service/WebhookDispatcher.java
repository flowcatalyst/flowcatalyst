package tech.flowcatalyst.dispatchjob.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchAttempt;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchAttemptStatus;
import tech.flowcatalyst.dispatchjob.model.ErrorType;
import tech.flowcatalyst.dispatchjob.security.WebhookSigner;
import tech.flowcatalyst.dispatchjob.service.CredentialsService.ResolvedCredentials;

import java.io.PrintWriter;
import java.io.StringWriter;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.time.Instant;
import java.time.format.DateTimeFormatter;
import java.util.concurrent.Executors;

/**
 * Service for dispatching signed webhooks with HTTP/2.
 *
 * <h2>FlowCatalyst Headers</h2>
 * <p>The following headers are included in all webhook requests:</p>
 * <ul>
 *   <li>{@code X-FlowCatalyst-ID} - The dispatch job ID</li>
 *   <li>{@code X-FlowCatalyst-Causation-ID} - The source event ID (for EVENT kind)</li>
 *   <li>{@code X-FlowCatalyst-Kind} - The dispatch kind (EVENT or TASK)</li>
 *   <li>{@code X-FlowCatalyst-Code} - The event type or task code</li>
 *   <li>{@code X-FlowCatalyst-Subject} - The subject/aggregate reference</li>
 *   <li>{@code X-FlowCatalyst-Correlation-ID} - Correlation ID for distributed tracing</li>
 * </ul>
 *
 * <h2>Payload Format</h2>
 * <p>When {@code dataOnly = true} (default), the raw payload is sent.</p>
 * <p>When {@code dataOnly = false}, the payload is wrapped in a JSON envelope:</p>
 * <pre>{@code
 * {
 *   "id": "0HZXEQ5Y8JY5Z",
 *   "kind": "EVENT",
 *   "code": "order.created",
 *   "subject": "order:12345",
 *   "eventId": "0HZXEQ5Y8JY00",
 *   "correlationId": "abc-123",
 *   "timestamp": "2024-01-15T10:30:00Z",
 *   "data": { ... original payload ... }
 * }
 * }</pre>
 */
@ApplicationScoped
public class WebhookDispatcher {

    private static final Logger LOG = Logger.getLogger(WebhookDispatcher.class);
    private static final int MAX_RESPONSE_BODY_LENGTH = 5000;

    // FlowCatalyst headers
    public static final String HEADER_ID = "X-FlowCatalyst-ID";
    public static final String HEADER_CAUSATION_ID = "X-FlowCatalyst-Causation-ID";
    public static final String HEADER_KIND = "X-FlowCatalyst-Kind";
    public static final String HEADER_CODE = "X-FlowCatalyst-Code";
    public static final String HEADER_SUBJECT = "X-FlowCatalyst-Subject";
    public static final String HEADER_CORRELATION_ID = "X-FlowCatalyst-Correlation-ID";

    private final HttpClient httpClient;

    @Inject
    WebhookSigner webhookSigner;

    @Inject
    ObjectMapper objectMapper;

    public WebhookDispatcher() {
        this.httpClient = HttpClient.newBuilder()
            .version(HttpClient.Version.HTTP_2)
            .connectTimeout(Duration.ofSeconds(30))
            .executor(Executors.newVirtualThreadPerTaskExecutor())
            .build();
    }

    /**
     * Send a webhook using resolved credentials.
     *
     * @param job         The dispatch job
     * @param credentials The resolved credentials (auth token and signing secret)
     * @return The dispatch attempt result
     */
    public DispatchAttempt sendWebhook(DispatchJob job, ResolvedCredentials credentials) {
        return sendWebhook(job, credentials.authToken(), credentials.signingSecret());
    }

    /**
     * Send a webhook with explicit auth token and signing secret.
     *
     * @param job           The dispatch job
     * @param authToken     The bearer token for Authorization header
     * @param signingSecret The secret for HMAC signing
     * @return The dispatch attempt result
     */
    public DispatchAttempt sendWebhook(DispatchJob job, String authToken, String signingSecret) {
        Instant attemptStart = Instant.now();

        try {
            LOG.debugf("Sending webhook for dispatch job [%s] to [%s]", (Object) job.id, job.targetUrl);

            // Prepare payload (raw or envelope)
            String requestBody = preparePayload(job, attemptStart);

            // Sign the webhook
            WebhookSigner.SignedWebhookRequest signed = webhookSigner.sign(requestBody, authToken, signingSecret);

            // Build HTTP request
            HttpRequest.Builder requestBuilder = HttpRequest.newBuilder()
                .uri(URI.create(job.targetUrl))
                .header("Authorization", "Bearer " + signed.bearerToken())
                .header(WebhookSigner.SIGNATURE_HEADER, signed.signature())
                .header(WebhookSigner.TIMESTAMP_HEADER, signed.timestamp())
                .header("Content-Type", job.payloadContentType)
                .POST(HttpRequest.BodyPublishers.ofString(signed.payload()))
                .timeout(Duration.ofSeconds(job.timeoutSeconds));

            // Add FlowCatalyst headers
            addFlowCatalystHeaders(requestBuilder, job);

            HttpRequest request = requestBuilder.build();

            // Send request
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            // Build attempt record
            return buildAttempt(job, attemptStart, response, null);

        } catch (Exception e) {
            LOG.errorf(e, "Error sending webhook for dispatch job [%s]", (Object) job.id);
            return buildAttempt(job, attemptStart, null, e);
        }
    }

    /**
     * Prepare the request payload based on dataOnly flag.
     *
     * <p>When {@code dataOnly = true}, returns the raw payload.</p>
     * <p>When {@code dataOnly = false}, wraps the payload in a JSON envelope.</p>
     */
    private String preparePayload(DispatchJob job, Instant timestamp) {
        if (job.dataOnly) {
            // Raw payload
            return job.payload;
        }

        try {
            // Build JSON envelope
            ObjectNode envelope = objectMapper.createObjectNode();
            envelope.put("id", job.id);
            envelope.put("kind", job.kind != null ? job.kind.name() : null);
            envelope.put("code", job.code);
            envelope.put("subject", job.subject);
            envelope.put("eventId", job.eventId);
            envelope.put("correlationId", job.correlationId);
            envelope.put("timestamp", DateTimeFormatter.ISO_INSTANT.format(timestamp));

            // Parse payload as JSON and add as "data" field
            if (job.payload != null && !job.payload.isBlank()) {
                try {
                    envelope.set("data", objectMapper.readTree(job.payload));
                } catch (Exception e) {
                    // If payload is not valid JSON, add as raw string
                    envelope.put("data", job.payload);
                }
            }

            return objectMapper.writeValueAsString(envelope);

        } catch (Exception e) {
            LOG.warnf(e, "Failed to create envelope for dispatch job [%s], using raw payload", job.id);
            return job.payload;
        }
    }

    /**
     * Add FlowCatalyst headers to the HTTP request.
     */
    private void addFlowCatalystHeaders(HttpRequest.Builder requestBuilder, DispatchJob job) {
        // Always add job ID
        requestBuilder.header(HEADER_ID, job.id);

        // Add causation ID (event ID for EVENT kind)
        if (job.eventId != null) {
            requestBuilder.header(HEADER_CAUSATION_ID, job.eventId);
        }

        // Add kind
        if (job.kind != null) {
            requestBuilder.header(HEADER_KIND, job.kind.name());
        }

        // Add code
        if (job.code != null) {
            requestBuilder.header(HEADER_CODE, job.code);
        }

        // Add subject
        if (job.subject != null) {
            requestBuilder.header(HEADER_SUBJECT, job.subject);
        }

        // Add correlation ID
        if (job.correlationId != null) {
            requestBuilder.header(HEADER_CORRELATION_ID, job.correlationId);
        }
    }

    private DispatchAttempt buildAttempt(
        DispatchJob job,
        Instant attemptStart,
        HttpResponse<String> response,
        Throwable error) {

        Instant completedAt = Instant.now();
        long durationMillis = Duration.between(attemptStart, completedAt).toMillis();

        DispatchAttempt attempt = new DispatchAttempt();
        attempt.attemptNumber = job.attemptCount + 1;
        attempt.attemptedAt = attemptStart;
        attempt.completedAt = completedAt;
        attempt.durationMillis = durationMillis;

        if (error != null) {
            attempt.status = DispatchAttemptStatus.FAILURE;
            attempt.errorMessage = error.getMessage();
            attempt.errorStackTrace = getStackTrace(error);
            // Network/connection errors are typically transient
            attempt.errorType = classifyException(error);
        } else {
            attempt.responseCode = response.statusCode();
            attempt.responseBody = truncate(response.body(), MAX_RESPONSE_BODY_LENGTH);

            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                attempt.status = DispatchAttemptStatus.SUCCESS;
                attempt.errorType = null; // No error on success
                LOG.debugf("Webhook sent successfully for dispatch job [%s], status: %d", (Object) job.id, response.statusCode());
            } else {
                attempt.status = DispatchAttemptStatus.FAILURE;
                attempt.errorMessage = "HTTP " + response.statusCode();
                attempt.errorType = classifyHttpStatus(response.statusCode());
                LOG.warnf("Webhook failed for dispatch job [%s], status: %d", (Object) job.id, response.statusCode());
            }
        }

        return attempt;
    }

    /**
     * Classify HTTP status codes into error types.
     * - 4xx = NOT_TRANSIENT (client errors, won't succeed without changes)
     * - 5xx = TRANSIENT (server errors, may succeed on retry)
     */
    private ErrorType classifyHttpStatus(int statusCode) {
        if (statusCode >= 400 && statusCode < 500) {
            // 4xx client errors are permanent (bad request, unauthorized, forbidden, not found, etc.)
            return ErrorType.NOT_TRANSIENT;
        } else if (statusCode >= 500) {
            // 5xx server errors are transient (may succeed on retry)
            return ErrorType.TRANSIENT;
        }
        return ErrorType.UNKNOWN;
    }

    /**
     * Classify exceptions into error types.
     * Network/timeout errors are typically transient.
     */
    private ErrorType classifyException(Throwable error) {
        if (error instanceof java.net.ConnectException ||
            error instanceof java.net.SocketTimeoutException ||
            error instanceof java.net.http.HttpConnectTimeoutException ||
            error instanceof java.net.http.HttpTimeoutException) {
            return ErrorType.TRANSIENT;
        }
        // Unknown exceptions default to transient (safer to retry)
        return ErrorType.UNKNOWN;
    }

    private String truncate(String str, int maxLength) {
        if (str == null || str.length() <= maxLength) {
            return str;
        }
        return str.substring(0, maxLength) + "... (truncated)";
    }

    private String getStackTrace(Throwable t) {
        if (t == null) {
            return null;
        }
        StringWriter sw = new StringWriter();
        PrintWriter pw = new PrintWriter(sw);
        t.printStackTrace(pw);
        return truncate(sw.toString(), 10000);
    }
}

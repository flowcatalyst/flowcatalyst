package tech.flowcatalyst.dispatchjob.endpoint;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.service.WebhookDispatcher;

/**
 * Sample webhook receiver endpoint for testing dispatch job delivery.
 *
 * <p>This endpoint demonstrates how to receive webhooks from FlowCatalyst,
 * including verification of signatures and extraction of metadata headers.</p>
 *
 * <h2>Endpoint</h2>
 * <pre>POST /api/sample/webhook</pre>
 *
 * <h2>FlowCatalyst Headers (always sent)</h2>
 * <p>The following headers are sent with each webhook request:</p>
 * <ul>
 *   <li>{@code X-FlowCatalyst-ID} - The dispatch job ID</li>
 *   <li>{@code X-FlowCatalyst-Causation-ID} - The source event ID (for EVENT kind)</li>
 *   <li>{@code X-FlowCatalyst-Kind} - The dispatch kind (EVENT or TASK)</li>
 *   <li>{@code X-FlowCatalyst-Code} - The event type or task code</li>
 *   <li>{@code X-FlowCatalyst-Subject} - The subject/aggregate reference</li>
 *   <li>{@code X-FlowCatalyst-Correlation-ID} - Correlation ID for distributed tracing</li>
 *   <li>{@code X-FlowCatalyst-Signature} - HMAC-SHA256 signature</li>
 *   <li>{@code X-FlowCatalyst-Timestamp} - Signature timestamp</li>
 * </ul>
 *
 * <h2>Payload Formats</h2>
 *
 * <h3>When dataOnly = true (default)</h3>
 * <p>The raw payload is sent directly. Metadata is available only via headers.</p>
 * <pre>{@code
 * Headers:
 *   X-FlowCatalyst-ID: 0HZXEQ5Y8JY5Z
 *   X-FlowCatalyst-Kind: EVENT
 *   X-FlowCatalyst-Code: order.created
 *   X-FlowCatalyst-Subject: order:12345
 *   X-FlowCatalyst-Causation-ID: 0HZXEQ5Y8JY00
 *   X-FlowCatalyst-Correlation-ID: abc-123
 *
 * Body (raw payload):
 *   { "orderId": "12345", "amount": 99.99, "items": [...] }
 * }</pre>
 *
 * <h3>When dataOnly = false</h3>
 * <p>The payload is wrapped in a JSON envelope with metadata included in the body.</p>
 * <pre>{@code
 * Headers:
 *   X-FlowCatalyst-ID: 0HZXEQ5Y8JY5Z
 *   X-FlowCatalyst-Kind: EVENT
 *   ... (same headers as above)
 *
 * Body (envelope):
 *   {
 *     "id": "0HZXEQ5Y8JY5Z",
 *     "kind": "EVENT",
 *     "code": "order.created",
 *     "subject": "order:12345",
 *     "eventId": "0HZXEQ5Y8JY00",
 *     "correlationId": "abc-123",
 *     "timestamp": "2024-01-15T10:30:00Z",
 *     "data": { "orderId": "12345", "amount": 99.99, "items": [...] }
 *   }
 * }</pre>
 *
 * <h2>Usage</h2>
 * <p>Configure your subscription or dispatch job to use this endpoint as the target URL:</p>
 * <pre>{@code
 * targetUrl: "http://localhost:8080/api/sample/webhook"
 * }</pre>
 */
@Path("/api/sample/webhook")
@Consumes(MediaType.APPLICATION_JSON)
@Produces(MediaType.APPLICATION_JSON)
public class SampleWebhookReceiverResource {

    private static final Logger LOG = Logger.getLogger(SampleWebhookReceiverResource.class);

    @Inject
    ObjectMapper objectMapper;

    /**
     * Receive a webhook and log its contents.
     *
     * <p>This endpoint accepts any JSON payload and logs:</p>
     * <ul>
     *   <li>FlowCatalyst headers (ID, kind, code, subject, etc.)</li>
     *   <li>The request body (raw or envelope)</li>
     * </ul>
     *
     * <p>Returns 200 OK to acknowledge receipt. Return non-2xx to trigger retry.</p>
     *
     * @param body The webhook payload (raw or envelope)
     * @param dispatchJobId The dispatch job ID from X-FlowCatalyst-ID header
     * @param causationId The source event ID from X-FlowCatalyst-Causation-ID header
     * @param kind The dispatch kind from X-FlowCatalyst-Kind header
     * @param code The event type or task code from X-FlowCatalyst-Code header
     * @param subject The subject from X-FlowCatalyst-Subject header
     * @param correlationId The correlation ID from X-FlowCatalyst-Correlation-ID header
     * @param signature The webhook signature from X-FlowCatalyst-Signature header
     * @param timestamp The signature timestamp from X-FlowCatalyst-Timestamp header
     * @return 200 OK with a receipt confirmation showing received data
     */
    @POST
    public Response receiveWebhook(
        String body,
        @HeaderParam(WebhookDispatcher.HEADER_ID) String dispatchJobId,
        @HeaderParam(WebhookDispatcher.HEADER_CAUSATION_ID) String causationId,
        @HeaderParam(WebhookDispatcher.HEADER_KIND) String kind,
        @HeaderParam(WebhookDispatcher.HEADER_CODE) String code,
        @HeaderParam(WebhookDispatcher.HEADER_SUBJECT) String subject,
        @HeaderParam(WebhookDispatcher.HEADER_CORRELATION_ID) String correlationId,
        @HeaderParam("X-FlowCatalyst-Signature") String signature,
        @HeaderParam("X-FlowCatalyst-Timestamp") String timestamp
    ) {
        LOG.infof("=== Received Webhook ===");
        LOG.infof("  Dispatch Job ID: %s", dispatchJobId);
        LOG.infof("  Kind: %s", kind);
        LOG.infof("  Code: %s", code);
        LOG.infof("  Subject: %s", subject);
        LOG.infof("  Causation ID (Event ID): %s", causationId);
        LOG.infof("  Correlation ID: %s", correlationId);
        LOG.infof("  Signature: %s", signature != null ? signature.substring(0, Math.min(20, signature.length())) + "..." : null);
        LOG.infof("  Timestamp: %s", timestamp);

        // Try to parse as envelope to detect format
        WebhookEnvelope envelope = tryParseEnvelope(body);
        boolean isEnvelope = envelope != null && envelope.id() != null && envelope.data() != null;

        if (isEnvelope) {
            LOG.infof("  Format: ENVELOPE (dataOnly=false)");
            LOG.infof("  Envelope ID: %s", envelope.id());
            LOG.infof("  Envelope Kind: %s", envelope.kind());
            LOG.infof("  Envelope Code: %s", envelope.code());
            LOG.infof("  Envelope Subject: %s", envelope.subject());
            LOG.infof("  Envelope EventId: %s", envelope.eventId());
            LOG.infof("  Envelope CorrelationId: %s", envelope.correlationId());
            LOG.infof("  Envelope Timestamp: %s", envelope.timestamp());
            LOG.infof("  Envelope Data: %s", truncate(envelope.data() != null ? envelope.data().toString() : null, 500));

            return Response.ok(new EnvelopeReceipt(
                "received",
                "envelope",
                new ReceivedHeaders(dispatchJobId, kind, code, subject, causationId, correlationId),
                envelope
            )).build();
        } else {
            LOG.infof("  Format: RAW (dataOnly=true)");
            LOG.infof("  Raw Body: %s", truncate(body, 500));

            // Parse raw body as JSON for response
            JsonNode rawData = tryParseJson(body);

            return Response.ok(new RawReceipt(
                "received",
                "raw",
                new ReceivedHeaders(dispatchJobId, kind, code, subject, causationId, correlationId),
                rawData
            )).build();
        }
    }

    /**
     * Simulates a transient error (500) to test retry behavior.
     * Use this endpoint to test that dispatch jobs are retried.
     */
    @POST
    @Path("/fail-transient")
    public Response receiveWebhookTransientFailure(
        String body,
        @HeaderParam(WebhookDispatcher.HEADER_ID) String dispatchJobId
    ) {
        LOG.warnf("Simulating transient error for dispatch job [%s]", dispatchJobId);
        return Response.status(500)
            .entity(new ErrorResponse("simulated_transient_error", "This is a simulated transient error for testing"))
            .build();
    }

    /**
     * Simulates a permanent error (400) to test non-retry behavior.
     * Use this endpoint to test that dispatch jobs are NOT retried for 4xx errors.
     */
    @POST
    @Path("/fail-permanent")
    public Response receiveWebhookPermanentFailure(
        String body,
        @HeaderParam(WebhookDispatcher.HEADER_ID) String dispatchJobId
    ) {
        LOG.warnf("Simulating permanent error for dispatch job [%s]", dispatchJobId);
        return Response.status(400)
            .entity(new ErrorResponse("simulated_permanent_error", "This is a simulated permanent error for testing"))
            .build();
    }

    /**
     * Simulates a slow response to test timeout behavior.
     * Sleeps for 35 seconds (longer than default 30s timeout).
     */
    @POST
    @Path("/slow")
    public Response receiveWebhookSlow(
        String body,
        @HeaderParam(WebhookDispatcher.HEADER_ID) String dispatchJobId
    ) throws InterruptedException {
        LOG.warnf("Simulating slow response for dispatch job [%s]", dispatchJobId);
        Thread.sleep(35_000); // 35 seconds
        return Response.ok(new RawReceipt("received_after_delay", "raw", null, null)).build();
    }

    private WebhookEnvelope tryParseEnvelope(String body) {
        if (body == null || body.isBlank()) {
            return null;
        }
        try {
            return objectMapper.readValue(body, WebhookEnvelope.class);
        } catch (Exception e) {
            return null;
        }
    }

    private JsonNode tryParseJson(String body) {
        if (body == null || body.isBlank()) {
            return null;
        }
        try {
            return objectMapper.readTree(body);
        } catch (Exception e) {
            return null;
        }
    }

    private String truncate(String str, int maxLength) {
        if (str == null || str.length() <= maxLength) {
            return str;
        }
        return str.substring(0, maxLength) + "... (truncated)";
    }

    // ========================================================================
    // Response DTOs
    // ========================================================================

    /**
     * The FlowCatalyst envelope format (when dataOnly=false).
     * This is the structure of the request body when envelope mode is enabled.
     */
    @JsonIgnoreProperties(ignoreUnknown = true)
    public record WebhookEnvelope(
        /** Dispatch job ID */
        String id,
        /** Dispatch kind: EVENT or TASK */
        String kind,
        /** Event type or task code */
        String code,
        /** Subject/aggregate reference */
        String subject,
        /** Source event ID (for EVENT kind) */
        String eventId,
        /** Correlation ID for distributed tracing */
        String correlationId,
        /** ISO-8601 timestamp */
        String timestamp,
        /** The actual payload data */
        JsonNode data
    ) {}

    /**
     * Headers received from FlowCatalyst (always present regardless of dataOnly setting).
     */
    public record ReceivedHeaders(
        String dispatchJobId,
        String kind,
        String code,
        String subject,
        String causationId,
        String correlationId
    ) {}

    /**
     * Response for envelope format (dataOnly=false).
     */
    public record EnvelopeReceipt(
        String status,
        String format,
        ReceivedHeaders headers,
        WebhookEnvelope envelope
    ) {}

    /**
     * Response for raw format (dataOnly=true).
     */
    public record RawReceipt(
        String status,
        String format,
        ReceivedHeaders headers,
        JsonNode data
    ) {}

    /**
     * Error response for simulated failures.
     */
    public record ErrorResponse(
        String error,
        String message
    ) {}
}

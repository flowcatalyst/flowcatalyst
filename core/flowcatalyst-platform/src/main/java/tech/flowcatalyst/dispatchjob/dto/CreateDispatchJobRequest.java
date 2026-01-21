package tech.flowcatalyst.dispatchjob.dto;

import com.fasterxml.jackson.annotation.JsonProperty;
import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchProtocol;

import java.time.Instant;
import java.util.Map;

/**
 * Request to create a new dispatch job.
 *
 * <h2>Classification Fields</h2>
 * <p>The {@code kind} and {@code code} fields work together:</p>
 * <ul>
 *   <li>{@code kind = EVENT}: The {@code code} is an event type (e.g., order.created)</li>
 *   <li>{@code kind = TASK}: The {@code code} is a task identifier (e.g., send-welcome-email)</li>
 * </ul>
 *
 * <h2>Tracing Fields</h2>
 * <ul>
 *   <li>{@code eventId}: Source event ID (required for EVENT kind, optional for TASK)</li>
 *   <li>{@code correlationId}: Distributed tracing correlation ID</li>
 *   <li>{@code subject}: CloudEvents-style subject/aggregate reference</li>
 * </ul>
 */
public record CreateDispatchJobRequest(
    @JsonProperty("source")
    @NotBlank(message = "source is required")
    String source,

    /** The kind of dispatch job (EVENT or TASK) */
    @JsonProperty("kind")
    DispatchKind kind,

    /** The event type or task code (depends on kind) */
    @JsonProperty("code")
    @NotBlank(message = "code is required")
    String code,

    /** CloudEvents-style subject/aggregate reference */
    @JsonProperty("subject")
    String subject,

    /** Source event ID (required for EVENT kind) */
    @JsonProperty("eventId")
    String eventId,

    /** Correlation ID for distributed tracing */
    @JsonProperty("correlationId")
    String correlationId,

    @JsonProperty("metadata")
    Map<String, String> metadata,

    @JsonProperty("targetUrl")
    @NotBlank(message = "targetUrl is required")
    String targetUrl,

    @JsonProperty("protocol")
    DispatchProtocol protocol,

    @JsonProperty("headers")
    Map<String, String> headers,

    @JsonProperty("payload")
    @NotNull(message = "payload is required")
    String payload,

    @JsonProperty("payloadContentType")
    String payloadContentType,

    /** If true, send raw payload only; if false, wrap in JSON envelope */
    @JsonProperty("dataOnly")
    Boolean dataOnly,

    @JsonProperty("serviceAccountId")
    @NotNull(message = "serviceAccountId is required")
    String serviceAccountId,

    @JsonProperty("clientId")
    String clientId,

    @JsonProperty("subscriptionId")
    String subscriptionId,

    @JsonProperty("mode")
    DispatchMode mode,

    @JsonProperty("dispatchPoolId")
    String dispatchPoolId,

    @JsonProperty("messageGroup")
    String messageGroup,

    @JsonProperty("sequence")
    Integer sequence,

    @JsonProperty("timeoutSeconds")
    Integer timeoutSeconds,

    @JsonProperty("schemaId")
    String schemaId,

    @JsonProperty("maxRetries")
    Integer maxRetries,

    @JsonProperty("retryStrategy")
    String retryStrategy,

    @JsonProperty("scheduledFor")
    Instant scheduledFor,

    @JsonProperty("expiresAt")
    Instant expiresAt,

    @JsonProperty("idempotencyKey")
    String idempotencyKey,

    @JsonProperty("externalId")
    String externalId,

    @JsonProperty("queueUrl")
    @NotBlank(message = "queueUrl is required")
    String queueUrl
) {
}

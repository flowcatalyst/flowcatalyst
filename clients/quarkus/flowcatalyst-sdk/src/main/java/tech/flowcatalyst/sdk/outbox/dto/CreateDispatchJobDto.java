package tech.flowcatalyst.sdk.outbox.dto;

import lombok.Builder;
import lombok.With;

import java.time.Instant;
import java.util.Map;

/**
 * DTO for creating a dispatch job in the outbox.
 */
@Builder(toBuilder = true)
@With
public record CreateDispatchJobDto(
    String source,
    String code,
    String targetUrl,
    String payload,
    String dispatchPoolId,
    String partitionId,
    String subject,
    String correlationId,
    String eventId,
    Map<String, String> metadata,
    Map<String, String> headers,
    String payloadContentType,
    boolean dataOnly,
    String messageGroup,
    Integer sequence,
    int timeoutSeconds,
    int maxRetries,
    String retryStrategy,
    Instant scheduledFor,
    Instant expiresAt,
    String idempotencyKey,
    String externalId
) {
    /**
     * Create a new dispatch job DTO with required fields.
     */
    public static CreateDispatchJobDtoBuilder create(
        String source,
        String code,
        String targetUrl,
        String payload,
        String dispatchPoolId,
        String partitionId
    ) {
        return CreateDispatchJobDto.builder()
            .source(source)
            .code(code)
            .targetUrl(targetUrl)
            .payload(payload)
            .dispatchPoolId(dispatchPoolId)
            .partitionId(partitionId)
            .payloadContentType("application/json")
            .dataOnly(true)
            .timeoutSeconds(30)
            .maxRetries(5);
    }

    /**
     * Create a new dispatch job DTO with Map payload (will be JSON encoded).
     */
    public static CreateDispatchJobDtoBuilder create(
        String source,
        String code,
        String targetUrl,
        Map<String, Object> payload,
        String dispatchPoolId,
        String partitionId
    ) {
        String jsonPayload;
        try {
            jsonPayload = new com.fasterxml.jackson.databind.ObjectMapper().writeValueAsString(payload);
        } catch (Exception e) {
            jsonPayload = payload.toString();
        }
        return create(source, code, targetUrl, jsonPayload, dispatchPoolId, partitionId);
    }

    /**
     * Build the dispatch job payload for the outbox.
     */
    public Map<String, Object> toPayload() {
        var builder = new java.util.HashMap<String, Object>();
        builder.put("source", source);
        builder.put("code", code);
        builder.put("targetUrl", targetUrl);
        builder.put("payload", payload);
        builder.put("payloadContentType", payloadContentType != null ? payloadContentType : "application/json");
        builder.put("dispatchPoolId", dispatchPoolId);
        builder.put("dataOnly", dataOnly);
        builder.put("timeoutSeconds", timeoutSeconds);
        builder.put("maxRetries", maxRetries);

        if (subject != null) builder.put("subject", subject);
        if (correlationId != null) builder.put("correlationId", correlationId);
        if (eventId != null) builder.put("eventId", eventId);
        if (metadata != null && !metadata.isEmpty()) builder.put("metadata", metadata);
        if (headers != null && !headers.isEmpty()) builder.put("headers", headers);
        if (messageGroup != null) builder.put("messageGroup", messageGroup);
        if (sequence != null) builder.put("sequence", sequence);
        if (retryStrategy != null) builder.put("retryStrategy", retryStrategy);
        if (scheduledFor != null) builder.put("scheduledFor", scheduledFor.toString());
        if (expiresAt != null) builder.put("expiresAt", expiresAt.toString());
        if (idempotencyKey != null) builder.put("idempotencyKey", idempotencyKey);
        if (externalId != null) builder.put("externalId", externalId);

        return builder;
    }
}

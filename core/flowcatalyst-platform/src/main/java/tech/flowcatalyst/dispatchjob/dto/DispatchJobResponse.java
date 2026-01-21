package tech.flowcatalyst.dispatchjob.dto;

import com.fasterxml.jackson.annotation.JsonProperty;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchProtocol;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;

import java.time.Instant;
import java.util.Map;

public record DispatchJobResponse(
    @JsonProperty("id") String id,
    @JsonProperty("externalId") String externalId,
    @JsonProperty("source") String source,
    @JsonProperty("kind") DispatchKind kind,
    @JsonProperty("code") String code,
    @JsonProperty("subject") String subject,
    @JsonProperty("eventId") String eventId,
    @JsonProperty("correlationId") String correlationId,
    @JsonProperty("metadata") Map<String, String> metadata,
    @JsonProperty("targetUrl") String targetUrl,
    @JsonProperty("protocol") DispatchProtocol protocol,
    @JsonProperty("payloadContentType") String payloadContentType,
    @JsonProperty("dataOnly") boolean dataOnly,
    @JsonProperty("serviceAccountId") String serviceAccountId,
    @JsonProperty("clientId") String clientId,
    @JsonProperty("subscriptionId") String subscriptionId,
    @JsonProperty("mode") DispatchMode mode,
    @JsonProperty("dispatchPoolId") String dispatchPoolId,
    @JsonProperty("messageGroup") String messageGroup,
    @JsonProperty("sequence") int sequence,
    @JsonProperty("timeoutSeconds") int timeoutSeconds,
    @JsonProperty("schemaId") String schemaId,
    @JsonProperty("status") DispatchStatus status,
    @JsonProperty("maxRetries") Integer maxRetries,
    @JsonProperty("retryStrategy") String retryStrategy,
    @JsonProperty("scheduledFor") Instant scheduledFor,
    @JsonProperty("expiresAt") Instant expiresAt,
    @JsonProperty("attemptCount") Integer attemptCount,
    @JsonProperty("lastAttemptAt") Instant lastAttemptAt,
    @JsonProperty("completedAt") Instant completedAt,
    @JsonProperty("durationMillis") Long durationMillis,
    @JsonProperty("lastError") String lastError,
    @JsonProperty("idempotencyKey") String idempotencyKey,
    @JsonProperty("createdAt") Instant createdAt,
    @JsonProperty("updatedAt") Instant updatedAt
) {
    public static DispatchJobResponse from(DispatchJob job) {
        // Convert metadata entities to Map
        Map<String, String> metadataMap = new java.util.HashMap<>();
        if (job.metadata != null) {
            job.metadata.forEach(m -> metadataMap.put(m.key, m.value));
        }

        return new DispatchJobResponse(
            job.id,
            job.externalId,
            job.source,
            job.kind,
            job.code,
            job.subject,
            job.eventId,
            job.correlationId,
            metadataMap,
            job.targetUrl,
            job.protocol,
            job.payloadContentType,
            job.dataOnly,
            job.serviceAccountId,
            job.clientId,
            job.subscriptionId,
            job.mode,
            job.dispatchPoolId,
            job.messageGroup,
            job.sequence,
            job.timeoutSeconds,
            job.schemaId,
            job.status,
            job.maxRetries,
            job.retryStrategy,
            job.scheduledFor,
            job.expiresAt,
            job.attemptCount,
            job.lastAttemptAt,
            job.completedAt,
            job.durationMillis,
            job.lastError,
            job.idempotencyKey,
            job.createdAt,
            job.updatedAt
        );
    }
}

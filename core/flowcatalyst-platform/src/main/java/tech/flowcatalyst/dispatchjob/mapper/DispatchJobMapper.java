package tech.flowcatalyst.dispatchjob.mapper;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.dispatchjob.entity.DispatchAttempt;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.entity.DispatchJobMetadata;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobAttemptEntity;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobJpaEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between DispatchJob domain and JPA entities.
 */
public final class DispatchJobMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper();
    private static final TypeReference<List<DispatchJobMetadata>> METADATA_LIST_TYPE =
        new TypeReference<>() {};

    private DispatchJobMapper() {
    }

    /**
     * Convert JPA entity to domain object (without relations).
     * Call with relation data for complete conversion.
     */
    public static DispatchJob toDomain(DispatchJobJpaEntity entity) {
        if (entity == null) {
            return null;
        }

        DispatchJob job = new DispatchJob();
        job.id = entity.id;
        job.externalId = entity.externalId;
        job.source = entity.source;
        job.kind = entity.kind;
        job.code = entity.code;
        job.subject = entity.subject;
        job.eventId = entity.eventId;
        job.correlationId = entity.correlationId;
        job.metadata = parseMetadataJson(entity.metadataJson);
        job.targetUrl = entity.targetUrl;
        job.protocol = entity.protocol;
        job.payload = entity.payload;
        job.payloadContentType = entity.payloadContentType;
        job.dataOnly = entity.dataOnly;
        job.serviceAccountId = entity.serviceAccountId;
        job.clientId = entity.clientId;
        job.subscriptionId = entity.subscriptionId;
        job.mode = entity.mode;
        job.dispatchPoolId = entity.dispatchPoolId;
        job.messageGroup = entity.messageGroup;
        job.sequence = entity.sequence;
        job.timeoutSeconds = entity.timeoutSeconds;
        job.schemaId = entity.schemaId;
        job.status = entity.status;
        job.maxRetries = entity.maxRetries;
        job.retryStrategy = entity.retryStrategy;
        job.scheduledFor = entity.scheduledFor;
        job.expiresAt = entity.expiresAt;
        job.attemptCount = entity.attemptCount;
        job.lastAttemptAt = entity.lastAttemptAt;
        job.completedAt = entity.completedAt;
        job.durationMillis = entity.durationMillis;
        job.lastError = entity.lastError;
        job.idempotencyKey = entity.idempotencyKey;
        job.attempts = new ArrayList<>(); // loaded separately
        job.createdAt = entity.createdAt;
        job.updatedAt = entity.updatedAt;

        return job;
    }

    /**
     * Convert domain object to JPA entity.
     */
    public static DispatchJobJpaEntity toEntity(DispatchJob domain) {
        if (domain == null) {
            return null;
        }

        DispatchJobJpaEntity entity = new DispatchJobJpaEntity();
        entity.id = domain.id;
        entity.externalId = domain.externalId;
        entity.source = domain.source;
        entity.kind = domain.kind;
        entity.code = domain.code;
        entity.subject = domain.subject;
        entity.eventId = domain.eventId;
        entity.correlationId = domain.correlationId;
        entity.metadataJson = serializeMetadata(domain.metadata);
        entity.targetUrl = domain.targetUrl;
        entity.protocol = domain.protocol;
        entity.payload = domain.payload;
        entity.payloadContentType = domain.payloadContentType;
        entity.dataOnly = domain.dataOnly;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.clientId = domain.clientId;
        entity.subscriptionId = domain.subscriptionId;
        entity.mode = domain.mode;
        entity.dispatchPoolId = domain.dispatchPoolId;
        entity.messageGroup = domain.messageGroup;
        entity.sequence = domain.sequence;
        entity.timeoutSeconds = domain.timeoutSeconds;
        entity.schemaId = domain.schemaId;
        entity.status = domain.status;
        entity.maxRetries = domain.maxRetries;
        entity.retryStrategy = domain.retryStrategy;
        entity.scheduledFor = domain.scheduledFor;
        entity.expiresAt = domain.expiresAt;
        entity.attemptCount = domain.attemptCount;
        entity.lastAttemptAt = domain.lastAttemptAt;
        entity.completedAt = domain.completedAt;
        entity.durationMillis = domain.durationMillis;
        entity.lastError = domain.lastError;
        entity.idempotencyKey = domain.idempotencyKey;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;

        return entity;
    }

    /**
     * Update existing JPA entity with values from domain object.
     */
    public static void updateEntity(DispatchJobJpaEntity entity, DispatchJob domain) {
        entity.externalId = domain.externalId;
        entity.source = domain.source;
        entity.kind = domain.kind;
        entity.code = domain.code;
        entity.subject = domain.subject;
        entity.eventId = domain.eventId;
        entity.correlationId = domain.correlationId;
        entity.metadataJson = serializeMetadata(domain.metadata);
        entity.targetUrl = domain.targetUrl;
        entity.protocol = domain.protocol;
        entity.payload = domain.payload;
        entity.payloadContentType = domain.payloadContentType;
        entity.dataOnly = domain.dataOnly;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.clientId = domain.clientId;
        entity.subscriptionId = domain.subscriptionId;
        entity.mode = domain.mode;
        entity.dispatchPoolId = domain.dispatchPoolId;
        entity.messageGroup = domain.messageGroup;
        entity.sequence = domain.sequence;
        entity.timeoutSeconds = domain.timeoutSeconds;
        entity.schemaId = domain.schemaId;
        entity.status = domain.status;
        entity.maxRetries = domain.maxRetries;
        entity.retryStrategy = domain.retryStrategy;
        entity.scheduledFor = domain.scheduledFor;
        entity.expiresAt = domain.expiresAt;
        entity.attemptCount = domain.attemptCount;
        entity.lastAttemptAt = domain.lastAttemptAt;
        entity.completedAt = domain.completedAt;
        entity.durationMillis = domain.durationMillis;
        entity.lastError = domain.lastError;
        entity.idempotencyKey = domain.idempotencyKey;
        entity.updatedAt = domain.updatedAt;
    }

    // ========================================================================
    // Attempts Mapping
    // ========================================================================

    public static List<DispatchAttempt> toAttemptsList(List<DispatchJobAttemptEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> {
                DispatchAttempt a = new DispatchAttempt();
                a.id = e.id();
                a.attemptNumber = e.attemptNumber();
                a.attemptedAt = e.attemptedAt();
                a.completedAt = e.completedAt();
                a.durationMillis = e.durationMillis();
                a.status = e.status();
                a.responseCode = e.responseCode();
                a.responseBody = e.responseBody();
                a.errorMessage = e.errorMessage();
                a.errorStackTrace = e.errorStackTrace();
                a.errorType = e.errorType();
                a.createdAt = e.createdAt();
                return a;
            })
            .toList();
    }

    public static List<DispatchJobAttemptEntity> toAttemptEntities(String jobId, List<DispatchAttempt> attempts) {
        if (attempts == null) {
            return new ArrayList<>();
        }
        return attempts.stream()
            .map(a -> toAttemptEntity(jobId, a))
            .toList();
    }

    public static DispatchJobAttemptEntity toAttemptEntity(String jobId, DispatchAttempt attempt) {
        return new DispatchJobAttemptEntity(
            attempt.id,
            jobId,
            attempt.attemptNumber,
            attempt.attemptedAt,
            attempt.completedAt,
            attempt.durationMillis,
            attempt.status,
            attempt.responseCode,
            attempt.responseBody,
            attempt.errorMessage,
            attempt.errorStackTrace,
            attempt.errorType,
            attempt.createdAt
        );
    }

    // ========================================================================
    // Metadata JSON Helpers
    // ========================================================================

    private static List<DispatchJobMetadata> parseMetadataJson(String json) {
        if (json == null || json.isBlank()) {
            return new ArrayList<>();
        }
        try {
            return objectMapper.readValue(json, METADATA_LIST_TYPE);
        } catch (JsonProcessingException e) {
            return new ArrayList<>();
        }
    }

    private static String serializeMetadata(List<DispatchJobMetadata> metadata) {
        if (metadata == null || metadata.isEmpty()) {
            return "[]";
        }
        try {
            return objectMapper.writeValueAsString(metadata);
        } catch (JsonProcessingException e) {
            return "[]";
        }
    }
}

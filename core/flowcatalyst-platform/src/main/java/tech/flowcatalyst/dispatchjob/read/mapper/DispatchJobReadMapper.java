package tech.flowcatalyst.dispatchjob.read.mapper;

import tech.flowcatalyst.dispatchjob.read.DispatchJobRead;
import tech.flowcatalyst.dispatchjob.read.jpaentity.DispatchJobReadEntity;

/**
 * Mapper for converting between DispatchJobRead domain and JPA entity.
 */
public final class DispatchJobReadMapper {

    private DispatchJobReadMapper() {
    }

    /**
     * Convert JPA entity to domain object.
     */
    public static DispatchJobRead toDomain(DispatchJobReadEntity entity) {
        if (entity == null) {
            return null;
        }

        DispatchJobRead job = new DispatchJobRead();
        job.id = entity.id;
        // dispatchJobId removed - id IS the dispatch job ID
        job.externalId = entity.externalId;
        job.source = entity.source;
        job.kind = entity.kind;
        job.code = entity.code;
        job.subject = entity.subject;
        job.application = entity.application;
        job.subdomain = entity.subdomain;
        job.aggregate = entity.aggregate;
        job.eventId = entity.eventId;
        job.correlationId = entity.correlationId;
        job.targetUrl = entity.targetUrl;
        job.protocol = entity.protocol;
        job.clientId = entity.clientId;
        job.subscriptionId = entity.subscriptionId;
        job.serviceAccountId = entity.serviceAccountId;
        job.dispatchPoolId = entity.dispatchPoolId;
        job.messageGroup = entity.messageGroup;
        job.mode = entity.mode;
        job.sequence = entity.sequence;
        job.status = entity.status;
        job.attemptCount = entity.attemptCount;
        job.maxRetries = entity.maxRetries;
        job.lastError = entity.lastError;
        job.timeoutSeconds = entity.timeoutSeconds;
        job.retryStrategy = entity.retryStrategy;
        job.createdAt = entity.createdAt;
        job.updatedAt = entity.updatedAt;
        job.scheduledFor = entity.scheduledFor;
        job.expiresAt = entity.expiresAt;
        job.completedAt = entity.completedAt;
        job.lastAttemptAt = entity.lastAttemptAt;
        job.durationMillis = entity.durationMillis;
        job.idempotencyKey = entity.idempotencyKey;
        job.isCompleted = entity.isCompleted;
        job.isTerminal = entity.isTerminal;
        job.projectedAt = entity.projectedAt;

        return job;
    }

    /**
     * Convert domain object to JPA entity.
     */
    public static DispatchJobReadEntity toEntity(DispatchJobRead domain) {
        if (domain == null) {
            return null;
        }

        DispatchJobReadEntity entity = new DispatchJobReadEntity();
        entity.id = domain.id;
        // dispatchJobId removed - id IS the dispatch job ID
        entity.externalId = domain.externalId;
        entity.source = domain.source;
        entity.kind = domain.kind;
        entity.code = domain.code;
        entity.subject = domain.subject;
        entity.application = domain.application;
        entity.subdomain = domain.subdomain;
        entity.aggregate = domain.aggregate;
        entity.eventId = domain.eventId;
        entity.correlationId = domain.correlationId;
        entity.targetUrl = domain.targetUrl;
        entity.protocol = domain.protocol;
        entity.clientId = domain.clientId;
        entity.subscriptionId = domain.subscriptionId;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.dispatchPoolId = domain.dispatchPoolId;
        entity.messageGroup = domain.messageGroup;
        entity.mode = domain.mode;
        entity.sequence = domain.sequence;
        entity.status = domain.status;
        entity.attemptCount = domain.attemptCount;
        entity.maxRetries = domain.maxRetries;
        entity.lastError = domain.lastError;
        entity.timeoutSeconds = domain.timeoutSeconds;
        entity.retryStrategy = domain.retryStrategy;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        entity.scheduledFor = domain.scheduledFor;
        entity.expiresAt = domain.expiresAt;
        entity.completedAt = domain.completedAt;
        entity.lastAttemptAt = domain.lastAttemptAt;
        entity.durationMillis = domain.durationMillis;
        entity.idempotencyKey = domain.idempotencyKey;
        entity.isCompleted = domain.isCompleted;
        entity.isTerminal = domain.isTerminal;
        entity.projectedAt = domain.projectedAt;

        return entity;
    }

    /**
     * Update existing JPA entity with values from domain object.
     */
    public static void updateEntity(DispatchJobReadEntity entity, DispatchJobRead domain) {
        // dispatchJobId removed - id IS the dispatch job ID
        entity.externalId = domain.externalId;
        entity.source = domain.source;
        entity.kind = domain.kind;
        entity.code = domain.code;
        entity.subject = domain.subject;
        entity.application = domain.application;
        entity.subdomain = domain.subdomain;
        entity.aggregate = domain.aggregate;
        entity.eventId = domain.eventId;
        entity.correlationId = domain.correlationId;
        entity.targetUrl = domain.targetUrl;
        entity.protocol = domain.protocol;
        entity.clientId = domain.clientId;
        entity.subscriptionId = domain.subscriptionId;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.dispatchPoolId = domain.dispatchPoolId;
        entity.messageGroup = domain.messageGroup;
        entity.mode = domain.mode;
        entity.sequence = domain.sequence;
        entity.status = domain.status;
        entity.attemptCount = domain.attemptCount;
        entity.maxRetries = domain.maxRetries;
        entity.lastError = domain.lastError;
        entity.timeoutSeconds = domain.timeoutSeconds;
        entity.retryStrategy = domain.retryStrategy;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        entity.scheduledFor = domain.scheduledFor;
        entity.expiresAt = domain.expiresAt;
        entity.completedAt = domain.completedAt;
        entity.lastAttemptAt = domain.lastAttemptAt;
        entity.durationMillis = domain.durationMillis;
        entity.idempotencyKey = domain.idempotencyKey;
        entity.isCompleted = domain.isCompleted;
        entity.isTerminal = domain.isTerminal;
        entity.projectedAt = domain.projectedAt;
    }
}

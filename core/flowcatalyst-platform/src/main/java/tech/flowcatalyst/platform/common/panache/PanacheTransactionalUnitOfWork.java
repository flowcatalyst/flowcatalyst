package tech.flowcatalyst.platform.common.panache;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Synchronization;
import jakarta.transaction.TransactionSynchronizationRegistry;
import jakarta.transaction.Transactional;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.entity.DispatchJobMetadata;
import tech.flowcatalyst.event.ContextData;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.event.EventDispatchService;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Panache/JPA implementation of {@link UnitOfWork} using JTA transactions.
 *
 * <p>This implementation ensures atomic commits of:
 * <ul>
 *   <li>Aggregate entity (create/update/delete)</li>
 *   <li>Domain event (in the events table + event_projection_feed)</li>
 *   <li>Dispatch jobs (in dispatch_jobs table + dispatch_job_projection_feed)</li>
 *   <li>Audit log entry</li>
 * </ul>
 *
 * <p>All operations occur within a single JTA transaction managed by Quarkus,
 * ensuring consistency. If any operation fails, the entire transaction is rolled back.
 *
 * <p>After successful commit, dispatch jobs are queued to SQS for processing via
 * a post-commit callback registered with {@link TransactionSynchronizationRegistry}.
 */
@ApplicationScoped
public class PanacheTransactionalUnitOfWork implements UnitOfWork {

    private static final Logger LOG = Logger.getLogger(PanacheTransactionalUnitOfWork.class);

    @Inject
    EntityManager em;

    @Inject
    ObjectMapper objectMapper;

    @Inject
    PanacheAggregateRegistry aggregateRegistry;

    @Inject
    EventDispatchService eventDispatchService;

    @Inject
    TransactionSynchronizationRegistry txSyncRegistry;

    @Override
    @Transactional
    public <T extends DomainEvent> Result<T> commit(
            Object aggregate,
            T event,
            Object command
    ) {
        try {
            // 1. Persist/update aggregate
            aggregateRegistry.persist(aggregate);

            // 2. Create domain event + event_projection_feed
            Event eventEntity = createEvent(event);

            // 3. Build dispatch jobs for matching subscriptions
            List<DispatchJob> dispatchJobs = eventDispatchService.buildDispatchJobsForEvents(List.of(eventEntity));

            // 4. Persist dispatch jobs + dispatch_job_projection_feed
            if (!dispatchJobs.isEmpty()) {
                persistDispatchJobs(dispatchJobs);

                // 5. Register post-commit callback to queue dispatch jobs
                registerPostCommitQueueing(dispatchJobs);
            }

            // 6. Create audit log
            createAuditLog(event, command);

            LOG.debugf("Committed aggregate with event [%s] and %d dispatch jobs", event.eventId(), dispatchJobs.size());
            return Result.success(event);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to commit transaction");
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "COMMIT_FAILED",
                "Failed to commit transaction: " + e.getMessage(),
                Map.of("exception", e.getClass().getSimpleName())
            ));
        }
    }

    @Override
    @Transactional
    public <T extends DomainEvent> Result<T> commitDelete(
            Object aggregate,
            T event,
            Object command
    ) {
        try {
            // 1. Delete aggregate
            aggregateRegistry.delete(aggregate);

            // 2. Create domain event + event_projection_feed
            Event eventEntity = createEvent(event);

            // 3. Build dispatch jobs for matching subscriptions
            List<DispatchJob> dispatchJobs = eventDispatchService.buildDispatchJobsForEvents(List.of(eventEntity));

            // 4. Persist dispatch jobs + dispatch_job_projection_feed
            if (!dispatchJobs.isEmpty()) {
                persistDispatchJobs(dispatchJobs);
                registerPostCommitQueueing(dispatchJobs);
            }

            // 5. Create audit log
            createAuditLog(event, command);

            LOG.debugf("Committed delete with event [%s] and %d dispatch jobs", event.eventId(), dispatchJobs.size());
            return Result.success(event);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to commit delete transaction");
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "COMMIT_FAILED",
                "Failed to commit transaction: " + e.getMessage(),
                Map.of("exception", e.getClass().getSimpleName())
            ));
        }
    }

    @Override
    @Transactional
    public <T extends DomainEvent> Result<T> commitAll(
            List<Object> aggregates,
            T event,
            Object command
    ) {
        try {
            // 1. Persist/update all aggregates
            for (Object aggregate : aggregates) {
                aggregateRegistry.persist(aggregate);
            }

            // 2. Create domain event + event_projection_feed
            Event eventEntity = createEvent(event);

            // 3. Build dispatch jobs for matching subscriptions
            List<DispatchJob> dispatchJobs = eventDispatchService.buildDispatchJobsForEvents(List.of(eventEntity));

            // 4. Persist dispatch jobs + dispatch_job_projection_feed
            if (!dispatchJobs.isEmpty()) {
                persistDispatchJobs(dispatchJobs);
                registerPostCommitQueueing(dispatchJobs);
            }

            // 5. Create audit log
            createAuditLog(event, command);

            LOG.debugf("Committed %d aggregates with event [%s] and %d dispatch jobs",
                Integer.valueOf(aggregates.size()), event.eventId(), Integer.valueOf(dispatchJobs.size()));
            return Result.success(event);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to commit all transaction");
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "COMMIT_FAILED",
                "Failed to commit transaction: " + e.getMessage(),
                Map.of("exception", e.getClass().getSimpleName())
            ));
        }
    }

    @Override
    @Transactional
    public <T extends DomainEvent> Result<T> commitDeleteAll(
            List<Object> aggregates,
            T event,
            Object command
    ) {
        try {
            // 1. Delete all aggregates
            for (Object aggregate : aggregates) {
                aggregateRegistry.delete(aggregate);
            }

            // 2. Create domain event + event_projection_feed
            Event eventEntity = createEvent(event);

            // 3. Build dispatch jobs for matching subscriptions
            List<DispatchJob> dispatchJobs = eventDispatchService.buildDispatchJobsForEvents(List.of(eventEntity));

            // 4. Persist dispatch jobs + dispatch_job_projection_feed
            if (!dispatchJobs.isEmpty()) {
                persistDispatchJobs(dispatchJobs);
                registerPostCommitQueueing(dispatchJobs);
            }

            // 5. Create audit log
            createAuditLog(event, command);

            LOG.debugf("Committed delete of %d aggregates with event [%s] and %d dispatch jobs",
                Integer.valueOf(aggregates.size()), event.eventId(), Integer.valueOf(dispatchJobs.size()));
            return Result.success(event);

        } catch (Exception e) {
            LOG.errorf(e, "Failed to commit delete all transaction");
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "COMMIT_FAILED",
                "Failed to commit transaction: " + e.getMessage(),
                Map.of("exception", e.getClass().getSimpleName())
            ));
        }
    }

    // ========================================================================
    // Event Operations
    // ========================================================================

    /**
     * Creates an event in the events table and event_projection_feed for CQRS projection.
     *
     * <p>Uses a single writable CTE to insert into both tables atomically in one
     * database round-trip. The Event entity is still built in Java because it's
     * needed for dispatch job building and as the serialized projection feed payload.</p>
     *
     * @param domainEvent The domain event from the use case
     * @return The Event entity created (for dispatch job building)
     */
    private Event createEvent(DomainEvent domainEvent) throws JsonProcessingException {
        // Build context data for searchability
        List<ContextData> contextData = new ArrayList<>();
        contextData.add(new ContextData("principalId", String.valueOf(domainEvent.principalId())));
        contextData.add(new ContextData("aggregateType", extractAggregateType(domainEvent.subject())));

        String contextDataJson = objectMapper.writeValueAsString(contextData);
        Instant now = Instant.now();
        String deduplicationId = domainEvent.eventType() + "-" + domainEvent.eventId();

        // Build Event entity (needed for dispatch job building and projection feed payload)
        Event event = new Event();
        event.id = domainEvent.eventId();
        event.specVersion = domainEvent.specVersion();
        event.type = domainEvent.eventType();
        event.source = domainEvent.source();
        event.subject = domainEvent.subject();
        event.time = domainEvent.time();
        event.data = domainEvent.toDataJson();
        event.correlationId = domainEvent.correlationId();
        event.causationId = domainEvent.causationId();
        event.deduplicationId = deduplicationId;
        event.messageGroup = domainEvent.messageGroup();
        event.contextData = contextData;

        String payloadJson = objectMapper.writeValueAsString(event);

        // Single CTE: insert into events + event_projection_feed atomically
        String sql = """
            WITH event_insert AS (
                INSERT INTO events (id, spec_version, type, source, subject, time, data,
                    correlation_id, causation_id, deduplication_id, message_group, context_data, created_at)
                VALUES (:id, :specVersion, :type, :source, :subject, :time, CAST(:data AS jsonb),
                    :correlationId, :causationId, :deduplicationId, :messageGroup, CAST(:contextData AS jsonb), :createdAt)
            )
            INSERT INTO event_projection_feed (event_id, payload, created_at, processed)
            VALUES (:id, CAST(:payload AS jsonb), :createdAt, 0)
            """;

        em.createNativeQuery(sql)
            .setParameter("id", domainEvent.eventId())
            .setParameter("specVersion", domainEvent.specVersion())
            .setParameter("type", domainEvent.eventType())
            .setParameter("source", domainEvent.source())
            .setParameter("subject", domainEvent.subject())
            .setParameter("time", domainEvent.time())
            .setParameter("data", domainEvent.toDataJson())
            .setParameter("correlationId", domainEvent.correlationId())
            .setParameter("causationId", domainEvent.causationId())
            .setParameter("deduplicationId", deduplicationId)
            .setParameter("messageGroup", domainEvent.messageGroup())
            .setParameter("contextData", contextDataJson)
            .setParameter("createdAt", now)
            .setParameter("payload", payloadJson)
            .executeUpdate();

        return event;
    }

    // ========================================================================
    // Audit Log Operations
    // ========================================================================

    private void createAuditLog(DomainEvent event, Object command) throws JsonProcessingException {
        String operationJson = objectMapper.writeValueAsString(command);

        String sql = """
            INSERT INTO audit_logs (id, entity_type, entity_id, operation, operation_json, principal_id, performed_at)
            VALUES (:id, :entityType, :entityId, :operation, CAST(:operationJson AS jsonb), :principalId, :performedAt)
            """;

        em.createNativeQuery(sql)
            .setParameter("id", TsidGenerator.generate(EntityType.AUDIT_LOG))
            .setParameter("entityType", extractAggregateType(event.subject()))
            .setParameter("entityId", extractEntityIdFromSubject(event.subject()))
            .setParameter("operation", command.getClass().getSimpleName())
            .setParameter("operationJson", operationJson)
            .setParameter("principalId", event.principalId())
            .setParameter("performedAt", event.time())
            .executeUpdate();
    }

    // ========================================================================
    // Dispatch Job Operations
    // ========================================================================

    /**
     * Persist dispatch jobs using native SQL.
     *
     * <p>Writes to dispatch_jobs table (metadata stored as JSONB column).</p>
     *
     * @param jobs The dispatch jobs to persist
     */
    private void persistDispatchJobs(List<DispatchJob> jobs) throws JsonProcessingException {
        if (jobs.isEmpty()) {
            return;
        }

        Instant now = Instant.now();

        // Insert dispatch jobs
        for (DispatchJob job : jobs) {
            if (job.id == null) {
                job.id = TsidGenerator.generate(EntityType.DISPATCH_JOB);
            }
            if (job.createdAt == null) {
                job.createdAt = now;
            }
            job.updatedAt = now;

            String jobSql = """
                INSERT INTO dispatch_jobs (
                    id, external_id, source, kind, code, subject, event_id, correlation_id,
                    target_url, protocol, payload, payload_content_type, data_only,
                    service_account_id, client_id, subscription_id,
                    mode, dispatch_pool_id, message_group, sequence, timeout_seconds,
                    schema_id, status, max_retries, retry_strategy,
                    scheduled_for, expires_at, attempt_count, last_attempt_at,
                    completed_at, duration_millis, last_error, idempotency_key,
                    metadata, created_at, updated_at
                ) VALUES (
                    :id, :externalId, :source, :kind, :code, :subject, :eventId, :correlationId,
                    :targetUrl, :protocol, :payload, :payloadContentType, :dataOnly,
                    :serviceAccountId, :clientId, :subscriptionId,
                    :mode, :dispatchPoolId, :messageGroup, :sequence, :timeoutSeconds,
                    :schemaId, :status, :maxRetries, :retryStrategy,
                    :scheduledFor, :expiresAt, :attemptCount, :lastAttemptAt,
                    :completedAt, :durationMillis, :lastError, :idempotencyKey,
                    CAST(:metadata AS jsonb), :createdAt, :updatedAt
                )
                """;

            em.createNativeQuery(jobSql)
                .setParameter("id", job.id)
                .setParameter("externalId", job.externalId)
                .setParameter("source", job.source)
                .setParameter("kind", job.kind != null ? job.kind.name() : null)
                .setParameter("code", job.code)
                .setParameter("subject", job.subject)
                .setParameter("eventId", job.eventId)
                .setParameter("correlationId", job.correlationId)
                .setParameter("targetUrl", job.targetUrl)
                .setParameter("protocol", job.protocol != null ? job.protocol.name() : null)
                .setParameter("payload", job.payload)
                .setParameter("payloadContentType", job.payloadContentType)
                .setParameter("dataOnly", job.dataOnly)
                .setParameter("serviceAccountId", job.serviceAccountId)
                .setParameter("clientId", job.clientId)
                .setParameter("subscriptionId", job.subscriptionId)
                .setParameter("mode", job.mode != null ? job.mode.name() : null)
                .setParameter("dispatchPoolId", job.dispatchPoolId)
                .setParameter("messageGroup", job.messageGroup)
                .setParameter("sequence", job.sequence)
                .setParameter("timeoutSeconds", job.timeoutSeconds)
                .setParameter("schemaId", job.schemaId)
                .setParameter("status", job.status != null ? job.status.name() : null)
                .setParameter("maxRetries", job.maxRetries)
                .setParameter("retryStrategy", job.retryStrategy)
                .setParameter("scheduledFor", job.scheduledFor)
                .setParameter("expiresAt", job.expiresAt)
                .setParameter("attemptCount", job.attemptCount)
                .setParameter("lastAttemptAt", job.lastAttemptAt)
                .setParameter("completedAt", job.completedAt)
                .setParameter("durationMillis", job.durationMillis)
                .setParameter("lastError", job.lastError)
                .setParameter("idempotencyKey", job.idempotencyKey)
                .setParameter("metadata", objectMapper.writeValueAsString(job.metadata != null ? job.metadata : List.of()))
                .setParameter("createdAt", job.createdAt)
                .setParameter("updatedAt", job.updatedAt)
                .executeUpdate();

            // Insert change record for projection
            String changeSql = """
                INSERT INTO dispatch_job_projection_feed (dispatch_job_id, operation, changes, created_at)
                VALUES (:jobId, 'INSERT', CAST(:changes AS jsonb), :createdAt)
                """;

            em.createNativeQuery(changeSql)
                .setParameter("jobId", job.id)
                .setParameter("changes", objectMapper.writeValueAsString(job))
                .setParameter("createdAt", now)
                .executeUpdate();
        }

        LOG.debugf("Persisted %d dispatch jobs with change records", jobs.size());
    }

    /**
     * Register a post-commit callback to queue dispatch jobs to SQS.
     *
     * <p>This ensures dispatch jobs are only queued after the transaction
     * successfully commits. If the transaction rolls back, the callback
     * is not executed and no messages are sent.
     *
     * @param jobs The dispatch jobs to queue after commit
     */
    private void registerPostCommitQueueing(List<DispatchJob> jobs) {
        if (jobs.isEmpty()) {
            return;
        }

        // Create a copy of job IDs to avoid closure issues
        List<String> jobIds = jobs.stream().map(j -> j.id).toList();

        txSyncRegistry.registerInterposedSynchronization(new Synchronization() {
            @Override
            public void beforeCompletion() {
                // No action needed before completion
            }

            @Override
            public void afterCompletion(int status) {
                // STATUS_COMMITTED = 3
                if (status == 3) {
                    try {
                        eventDispatchService.queueDispatchJobs(jobs);
                        LOG.debugf("Queued %d dispatch jobs after transaction commit", jobs.size());
                    } catch (Exception e) {
                        // Log but don't fail - jobs are persisted with QUEUED status
                        // and will be picked up by the queue poller if queueing fails
                        LOG.warnf(e, "Failed to queue %d dispatch jobs after commit, " +
                            "jobs will be picked up by poller", jobs.size());
                    }
                } else {
                    LOG.debugf("Transaction rolled back (status=%d), skipping dispatch job queueing", status);
                }
            }
        });
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    /**
     * Extract the aggregate type from a subject string.
     *
     * <p>Subject format: "platform.eventtype.123456789"
     * <p>Returns: "EventType" (capitalized)
     */
    private String extractAggregateType(String subject) {
        if (subject == null) {
            return "Unknown";
        }
        String[] parts = subject.split("\\.");
        if (parts.length >= 2) {
            return capitalize(parts[1].replace("-", ""));
        }
        return "Unknown";
    }

    /**
     * Extract the entity ID from a subject string.
     *
     * <p>Subject format: "platform.eventtype.123456789"
     * <p>Returns: "123456789"
     */
    private String extractEntityIdFromSubject(String subject) {
        if (subject == null) {
            return null;
        }
        String[] parts = subject.split("\\.");
        if (parts.length >= 3) {
            return parts[2];
        }
        return null;
    }

    private String capitalize(String s) {
        if (s == null || s.isEmpty()) {
            return s;
        }
        return s.substring(0, 1).toUpperCase() + s.substring(1);
    }
}

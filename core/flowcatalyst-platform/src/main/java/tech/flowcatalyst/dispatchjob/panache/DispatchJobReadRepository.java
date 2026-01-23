package tech.flowcatalyst.dispatchjob.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.dispatchjob.dto.CreateDispatchJobRequest;
import tech.flowcatalyst.dispatchjob.dto.DispatchJobFilter;
import tech.flowcatalyst.dispatchjob.entity.DispatchAttempt;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.entity.DispatchJobMetadata;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobAttemptEntity;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobJpaEntity;
import tech.flowcatalyst.dispatchjob.mapper.DispatchJobMapper;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.platform.common.Page;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.*;
import java.util.stream.Collectors;

/**
 * Read-side repository for DispatchJob entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class DispatchJobReadRepository implements DispatchJobRepository {

    @Inject
    EntityManager em;

    @Inject
    DispatchJobWriteRepository writeRepo;

    @Override
    public DispatchJob findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<DispatchJob> findByIdOptional(String id) {
        DispatchJobJpaEntity entity = em.find(DispatchJobJpaEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        return Optional.of(toDomainWithRelations(entity));
    }

    @Override
    public List<DispatchJob> findWithFilter(DispatchJobFilter filter) {
        StringBuilder jpql = new StringBuilder("FROM DispatchJobJpaEntity WHERE 1=1");

        if (filter.status() != null) {
            jpql.append(" AND status = :status");
        }
        if (filter.source() != null) {
            jpql.append(" AND source = :source");
        }
        if (filter.kind() != null) {
            jpql.append(" AND kind = :kind");
        }
        if (filter.code() != null) {
            jpql.append(" AND code = :code");
        }
        if (filter.clientId() != null) {
            jpql.append(" AND clientId = :clientId");
        }
        if (filter.subscriptionId() != null) {
            jpql.append(" AND subscriptionId = :subscriptionId");
        }
        if (filter.dispatchPoolId() != null) {
            jpql.append(" AND dispatchPoolId = :dispatchPoolId");
        }
        if (filter.messageGroup() != null) {
            jpql.append(" AND messageGroup = :messageGroup");
        }
        if (filter.createdAfter() != null) {
            jpql.append(" AND createdAt >= :createdAfter");
        }
        if (filter.createdBefore() != null) {
            jpql.append(" AND createdAt <= :createdBefore");
        }

        jpql.append(" ORDER BY createdAt DESC");

        TypedQuery<DispatchJobJpaEntity> query = em.createQuery(jpql.toString(), DispatchJobJpaEntity.class);

        if (filter.status() != null) query.setParameter("status", filter.status());
        if (filter.source() != null) query.setParameter("source", filter.source());
        if (filter.kind() != null) query.setParameter("kind", filter.kind());
        if (filter.code() != null) query.setParameter("code", filter.code());
        if (filter.clientId() != null) query.setParameter("clientId", filter.clientId());
        if (filter.subscriptionId() != null) query.setParameter("subscriptionId", filter.subscriptionId());
        if (filter.dispatchPoolId() != null) query.setParameter("dispatchPoolId", filter.dispatchPoolId());
        if (filter.messageGroup() != null) query.setParameter("messageGroup", filter.messageGroup());
        if (filter.createdAfter() != null) query.setParameter("createdAfter", filter.createdAfter());
        if (filter.createdBefore() != null) query.setParameter("createdBefore", filter.createdBefore());

        query.setFirstResult(filter.page() * filter.size());
        query.setMaxResults(filter.size());

        return query.getResultList().stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<DispatchJob> findByMetadata(String key, String value) {
        @SuppressWarnings("unchecked")
        List<DispatchJobJpaEntity> results = em.createNativeQuery(
                "SELECT DISTINCT dj.* FROM dispatch_jobs dj " +
                "JOIN dispatch_job_metadata djm ON dj.id = djm.dispatch_job_id " +
                "WHERE djm.metadata_key = :key AND djm.metadata_value = :value",
                DispatchJobJpaEntity.class)
            .setParameter("key", key)
            .setParameter("value", value)
            .getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<DispatchJob> findByMetadataFilters(Map<String, String> metadataFilters) {
        if (metadataFilters == null || metadataFilters.isEmpty()) {
            return List.of();
        }

        // Build a query that finds jobs matching ALL metadata key-value pairs
        StringBuilder sql = new StringBuilder(
            "SELECT DISTINCT dj.* FROM dispatch_jobs dj ");

        int i = 0;
        for (String key : metadataFilters.keySet()) {
            sql.append("JOIN dispatch_job_metadata djm").append(i)
               .append(" ON dj.id = djm").append(i).append(".dispatch_job_id ")
               .append("AND djm").append(i).append(".metadata_key = :key").append(i)
               .append(" AND djm").append(i).append(".metadata_value = :val").append(i).append(" ");
            i++;
        }

        @SuppressWarnings("unchecked")
        var query = em.createNativeQuery(sql.toString(), DispatchJobJpaEntity.class);

        i = 0;
        for (var entry : metadataFilters.entrySet()) {
            query.setParameter("key" + i, entry.getKey());
            query.setParameter("val" + i, entry.getValue());
            i++;
        }

        List<DispatchJobJpaEntity> results = query.getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<DispatchJob> findRecentPaged(int page, int size) {
        return em.createQuery("FROM DispatchJobJpaEntity ORDER BY createdAt DESC", DispatchJobJpaEntity.class)
            .setFirstResult(page * size)
            .setMaxResults(size)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<DispatchJob> listAll() {
        return em.createQuery("FROM DispatchJobJpaEntity ORDER BY createdAt DESC", DispatchJobJpaEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    @Deprecated
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM DispatchJobJpaEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    @Deprecated
    public long countWithFilter(DispatchJobFilter filter) {
        StringBuilder jpql = new StringBuilder("SELECT COUNT(e) FROM DispatchJobJpaEntity e WHERE 1=1");

        if (filter.status() != null) jpql.append(" AND e.status = :status");
        if (filter.source() != null) jpql.append(" AND e.source = :source");
        if (filter.kind() != null) jpql.append(" AND e.kind = :kind");
        if (filter.code() != null) jpql.append(" AND e.code = :code");
        if (filter.clientId() != null) jpql.append(" AND e.clientId = :clientId");
        if (filter.subscriptionId() != null) jpql.append(" AND e.subscriptionId = :subscriptionId");
        if (filter.dispatchPoolId() != null) jpql.append(" AND e.dispatchPoolId = :dispatchPoolId");
        if (filter.messageGroup() != null) jpql.append(" AND e.messageGroup = :messageGroup");
        if (filter.createdAfter() != null) jpql.append(" AND e.createdAt >= :createdAfter");
        if (filter.createdBefore() != null) jpql.append(" AND e.createdAt <= :createdBefore");

        TypedQuery<Long> query = em.createQuery(jpql.toString(), Long.class);

        if (filter.status() != null) query.setParameter("status", filter.status());
        if (filter.source() != null) query.setParameter("source", filter.source());
        if (filter.kind() != null) query.setParameter("kind", filter.kind());
        if (filter.code() != null) query.setParameter("code", filter.code());
        if (filter.clientId() != null) query.setParameter("clientId", filter.clientId());
        if (filter.subscriptionId() != null) query.setParameter("subscriptionId", filter.subscriptionId());
        if (filter.dispatchPoolId() != null) query.setParameter("dispatchPoolId", filter.dispatchPoolId());
        if (filter.messageGroup() != null) query.setParameter("messageGroup", filter.messageGroup());
        if (filter.createdAfter() != null) query.setParameter("createdAfter", filter.createdAfter());
        if (filter.createdBefore() != null) query.setParameter("createdBefore", filter.createdBefore());

        return query.getSingleResult();
    }

    @Override
    public Page<DispatchJob> findPage(String afterCursor, int limit) {
        TypedQuery<DispatchJobJpaEntity> query;
        if (afterCursor == null) {
            query = em.createQuery(
                "FROM DispatchJobJpaEntity ORDER BY id DESC",
                DispatchJobJpaEntity.class);
        } else {
            query = em.createQuery(
                "FROM DispatchJobJpaEntity WHERE id < :cursor ORDER BY id DESC",
                DispatchJobJpaEntity.class)
                .setParameter("cursor", afterCursor);
        }

        List<DispatchJobJpaEntity> results = query.setMaxResults(limit + 1).getResultList();
        List<DispatchJob> items = results.stream()
            .limit(limit)
            .map(this::toDomainWithRelations)
            .toList();

        return Page.of(items, limit, job -> job.id);
    }

    // ========================================================================
    // Scheduler Query Methods
    // ========================================================================

    @Override
    public List<DispatchJob> findPendingJobs(int limit) {
        return em.createQuery(
                "FROM DispatchJobJpaEntity WHERE status = :status ORDER BY scheduledFor ASC NULLS FIRST, createdAt ASC",
                DispatchJobJpaEntity.class)
            .setParameter("status", DispatchStatus.PENDING)
            .setMaxResults(limit)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public long countByMessageGroupAndStatus(String messageGroup, DispatchStatus status) {
        return em.createQuery(
                "SELECT COUNT(e) FROM DispatchJobJpaEntity e WHERE e.messageGroup = :messageGroup AND e.status = :status",
                Long.class)
            .setParameter("messageGroup", messageGroup)
            .setParameter("status", status)
            .getSingleResult();
    }

    @Override
    public Set<String> findGroupsWithErrors(Set<String> messageGroups) {
        if (messageGroups == null || messageGroups.isEmpty()) {
            return Set.of();
        }
        List<String> results = em.createQuery(
                "SELECT DISTINCT e.messageGroup FROM DispatchJobJpaEntity e WHERE e.messageGroup IN :groups AND e.status = :status",
                String.class)
            .setParameter("groups", messageGroups)
            .setParameter("status", DispatchStatus.ERROR)
            .getResultList();
        return new HashSet<>(results);
    }

    // ========================================================================
    // Stale Job Queries
    // ========================================================================

    @Override
    public List<DispatchJob> findStaleQueued(Instant threshold) {
        return em.createQuery(
                "FROM DispatchJobJpaEntity WHERE status = :status AND updatedAt < :threshold",
                DispatchJobJpaEntity.class)
            .setParameter("status", DispatchStatus.QUEUED)
            .setParameter("threshold", threshold)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<DispatchJob> findStaleQueued(Instant threshold, int limit) {
        return em.createQuery(
                "FROM DispatchJobJpaEntity WHERE status = :status AND updatedAt < :threshold",
                DispatchJobJpaEntity.class)
            .setParameter("status", DispatchStatus.QUEUED)
            .setParameter("threshold", threshold)
            .setMaxResults(limit)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    // ========================================================================
    // Write Operations (delegate to WriteRepository)
    // ========================================================================

    @Override
    public DispatchJob create(CreateDispatchJobRequest request) {
        DispatchJob job = new DispatchJob();
        job.id = TsidGenerator.generate(EntityType.DISPATCH_JOB);
        job.externalId = request.externalId();
        job.source = request.source();
        job.kind = request.kind() != null ? request.kind() : tech.flowcatalyst.dispatchjob.model.DispatchKind.EVENT;
        job.code = request.code();
        job.subject = request.subject();
        job.eventId = request.eventId();
        job.correlationId = request.correlationId();
        job.targetUrl = request.targetUrl();
        job.protocol = request.protocol() != null ? request.protocol() : tech.flowcatalyst.dispatchjob.model.DispatchProtocol.HTTP_WEBHOOK;
        job.payload = request.payload();
        job.payloadContentType = request.payloadContentType() != null ? request.payloadContentType() : "application/json";
        job.dataOnly = request.dataOnly() != null ? request.dataOnly() : true;
        job.serviceAccountId = request.serviceAccountId();
        job.clientId = request.clientId();
        job.subscriptionId = request.subscriptionId();
        job.mode = request.mode() != null ? request.mode() : tech.flowcatalyst.dispatch.DispatchMode.IMMEDIATE;
        job.dispatchPoolId = request.dispatchPoolId();
        job.messageGroup = request.messageGroup();
        job.sequence = request.sequence() != null ? request.sequence() : 99;
        job.timeoutSeconds = request.timeoutSeconds() != null ? request.timeoutSeconds() : 30;
        job.schemaId = request.schemaId();
        job.maxRetries = request.maxRetries() != null ? request.maxRetries() : 3;
        job.retryStrategy = request.retryStrategy() != null ? request.retryStrategy() : "exponential";
        job.scheduledFor = request.scheduledFor();
        job.expiresAt = request.expiresAt();
        job.idempotencyKey = request.idempotencyKey();

        // Convert metadata
        if (request.metadata() != null) {
            job.metadata = request.metadata().entrySet().stream()
                .map(e -> new DispatchJobMetadata(e.getKey(), e.getValue()))
                .toList();
        }

        writeRepo.persistDispatchJob(job);
        return job;
    }

    @Override
    public void addAttempt(String jobId, DispatchAttempt attempt) {
        writeRepo.addAttemptToJob(jobId, attempt);
    }

    @Override
    public void updateStatus(String jobId, DispatchStatus status, Instant completedAt, Long durationMillis, String lastError) {
        writeRepo.updateJobStatus(jobId, status, completedAt, durationMillis, lastError);
    }

    @Override
    public void updateStatusBatch(List<String> ids, DispatchStatus status) {
        writeRepo.updateStatusBatch(ids, status);
    }

    @Override
    public void persist(DispatchJob job) {
        writeRepo.persistDispatchJob(job);
    }

    @Override
    public void persistAll(List<DispatchJob> jobs) {
        writeRepo.persistAllDispatchJobs(jobs);
    }

    @Override
    public void update(DispatchJob job) {
        writeRepo.updateDispatchJob(job);
    }

    @Override
    public void delete(DispatchJob job) {
        writeRepo.deleteDispatchJob(job.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteDispatchJob(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private DispatchJob toDomainWithRelations(DispatchJobJpaEntity entity) {
        DispatchJob base = DispatchJobMapper.toDomain(entity);

        // Load attempts
        List<DispatchJobAttemptEntity> attemptEntities = em.createQuery(
                "FROM DispatchJobAttemptEntity WHERE dispatchJobId = :id ORDER BY attemptNumber ASC", DispatchJobAttemptEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        base.attempts = DispatchJobMapper.toAttemptsList(attemptEntities);

        return base;
    }
}

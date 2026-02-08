package tech.flowcatalyst.dispatchjob.panache;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.dispatchjob.entity.DispatchAttempt;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobAttemptEntity;
import tech.flowcatalyst.dispatchjob.jpaentity.DispatchJobJpaEntity;
import tech.flowcatalyst.dispatchjob.mapper.DispatchJobMapper;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for DispatchJob entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
@Transactional
public class DispatchJobWriteRepository implements PanacheRepositoryBase<DispatchJobJpaEntity, String> {

    private static final ObjectMapper objectMapper = new ObjectMapper()
        .registerModule(new JavaTimeModule());

    @Inject
    EntityManager em;

    /**
     * Persist a new dispatch job with its attempts.
     */
    public void persistDispatchJob(DispatchJob job) {
        if (job.createdAt == null) {
            job.createdAt = Instant.now();
        }
        job.updatedAt = Instant.now();

        DispatchJobJpaEntity entity = DispatchJobMapper.toEntity(job);
        persist(entity);

        // Save attempts
        saveAttempts(job.id, job.attempts);
    }

    /**
     * Persist multiple dispatch jobs.
     */
    public void persistAllDispatchJobs(List<DispatchJob> jobs) {
        for (DispatchJob job : jobs) {
            persistDispatchJob(job);
        }
    }

    /**
     * Update an existing dispatch job with its attempts.
     */
    public void updateDispatchJob(DispatchJob job) {
        job.updatedAt = Instant.now();

        DispatchJobJpaEntity entity = findById(job.id);
        if (entity != null) {
            DispatchJobMapper.updateEntity(entity, job);
        }

        // Update attempts
        saveAttempts(job.id, job.attempts);
    }

    /**
     * Delete a dispatch job and its related entities.
     */
    public boolean deleteDispatchJob(String id) {
        // Delete attempts first
        em.createQuery("DELETE FROM DispatchJobAttemptEntity WHERE dispatchJobId = :id")
            .setParameter("id", id)
            .executeUpdate();

        return deleteById(id);
    }

    /**
     * Add an attempt to a dispatch job.
     */
    public void addAttemptToJob(String jobId, DispatchAttempt attempt) {
        DispatchJobAttemptEntity entity = DispatchJobMapper.toAttemptEntity(jobId, attempt);
        em.persist(entity);

        Instant now = Instant.now();

        // Update attempt count and last attempt on the job
        em.createQuery("UPDATE DispatchJobJpaEntity SET attemptCount = attemptCount + 1, lastAttemptAt = :now, updatedAt = :now WHERE id = :id")
            .setParameter("now", now)
            .setParameter("id", jobId)
            .executeUpdate();

        // Get current attempt count for change record
        Integer attemptCount = em.createQuery("SELECT attemptCount FROM DispatchJobJpaEntity WHERE id = :id", Integer.class)
            .setParameter("id", jobId)
            .getSingleResult();

        // Write change record
        writeChangeRecord(jobId, createAttemptChangeJson(attemptCount, now));
    }

    /**
     * Update just the status fields of a dispatch job.
     */
    public void updateJobStatus(String jobId, DispatchStatus status, Instant completedAt, Long durationMillis, String lastError) {
        Instant now = Instant.now();
        StringBuilder jpql = new StringBuilder("UPDATE DispatchJobJpaEntity SET status = :status, updatedAt = :now");

        if (completedAt != null) {
            jpql.append(", completedAt = :completedAt");
        }
        if (durationMillis != null) {
            jpql.append(", durationMillis = :durationMillis");
        }
        if (lastError != null) {
            jpql.append(", lastError = :lastError");
        }
        jpql.append(" WHERE id = :id");

        var query = em.createQuery(jpql.toString())
            .setParameter("status", status)
            .setParameter("now", now)
            .setParameter("id", jobId);

        if (completedAt != null) {
            query.setParameter("completedAt", completedAt);
        }
        if (durationMillis != null) {
            query.setParameter("durationMillis", durationMillis);
        }
        if (lastError != null) {
            query.setParameter("lastError", lastError);
        }

        query.executeUpdate();

        // Write change record
        writeChangeRecord(jobId, createStatusChangeJson(status, completedAt, durationMillis, lastError, now));
    }

    /**
     * Batch update status for multiple jobs.
     */
    public void updateStatusBatch(List<String> ids, DispatchStatus status) {
        if (ids == null || ids.isEmpty()) {
            return;
        }

        Instant now = Instant.now();
        em.createQuery("UPDATE DispatchJobJpaEntity SET status = :status, updatedAt = :now WHERE id IN :ids")
            .setParameter("status", status)
            .setParameter("now", now)
            .setParameter("ids", ids)
            .executeUpdate();

        // Write change records for each job
        String changeJson = createStatusChangeJson(status, null, null, null, now);
        for (String jobId : ids) {
            writeChangeRecord(jobId, changeJson);
        }
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private void saveAttempts(String jobId, List<DispatchAttempt> attempts) {
        // Delete existing
        em.createQuery("DELETE FROM DispatchJobAttemptEntity WHERE dispatchJobId = :id")
            .setParameter("id", jobId)
            .executeUpdate();

        // Insert new
        if (attempts != null) {
            List<DispatchJobAttemptEntity> entities = DispatchJobMapper.toAttemptEntities(jobId, attempts);
            for (DispatchJobAttemptEntity entity : entities) {
                em.persist(entity);
            }
        }
    }

    // ========================================================================
    // Change Record Methods
    // ========================================================================

    private void writeChangeRecord(String jobId, String changesJson) {
        em.createNativeQuery("""
            INSERT INTO dispatch_job_projection_feed (dispatch_job_id, operation, changes, created_at)
            VALUES (:jobId, 'UPDATE', CAST(:changes AS jsonb), NOW())
            """)
            .setParameter("jobId", jobId)
            .setParameter("changes", changesJson)
            .executeUpdate();
    }

    private String createStatusChangeJson(DispatchStatus status, Instant completedAt, Long durationMillis, String lastError, Instant updatedAt) {
        ObjectNode node = objectMapper.createObjectNode();
        node.put("status", status.name());
        node.put("updatedAt", updatedAt.toString());
        if (completedAt != null) {
            node.put("completedAt", completedAt.toString());
        }
        if (durationMillis != null) {
            node.put("durationMillis", durationMillis);
        }
        if (lastError != null) {
            node.put("lastError", lastError);
        }
        return node.toString();
    }

    private String createAttemptChangeJson(Integer attemptCount, Instant lastAttemptAt) {
        ObjectNode node = objectMapper.createObjectNode();
        node.put("attemptCount", attemptCount);
        node.put("lastAttemptAt", lastAttemptAt.toString());
        node.put("updatedAt", lastAttemptAt.toString());
        return node.toString();
    }
}

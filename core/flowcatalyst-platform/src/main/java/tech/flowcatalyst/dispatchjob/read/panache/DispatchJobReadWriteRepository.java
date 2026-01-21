package tech.flowcatalyst.dispatchjob.read.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.dispatchjob.read.DispatchJobRead;
import tech.flowcatalyst.dispatchjob.read.jpaentity.DispatchJobReadEntity;
import tech.flowcatalyst.dispatchjob.read.mapper.DispatchJobReadMapper;

import java.time.Instant;

/**
 * Write-side repository for DispatchJobRead entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class DispatchJobReadWriteRepository implements PanacheRepositoryBase<DispatchJobReadEntity, String> {

    @Inject
    EntityManager em;

    /**
     * Persist a new dispatch job read projection.
     */
    public void persistDispatchJobRead(DispatchJobRead job) {
        if (job.projectedAt == null) {
            job.projectedAt = Instant.now();
        }

        DispatchJobReadEntity entity = DispatchJobReadMapper.toEntity(job);
        persist(entity);
    }

    /**
     * Update an existing dispatch job read projection.
     */
    public void updateDispatchJobRead(DispatchJobRead job) {
        job.projectedAt = Instant.now();

        DispatchJobReadEntity entity = findById(job.id);
        if (entity != null) {
            DispatchJobReadMapper.updateEntity(entity, job);
        }
    }

    /**
     * Delete a dispatch job read projection.
     */
    public boolean deleteDispatchJobRead(String id) {
        return deleteById(id);
    }
}

package tech.flowcatalyst.dispatchpool.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.entity.DispatchPoolEntity;
import tech.flowcatalyst.dispatchpool.mapper.DispatchPoolMapper;

import java.time.Instant;

/**
 * Write-side repository for DispatchPool entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class DispatchPoolWriteRepository implements PanacheRepositoryBase<DispatchPoolEntity, String> {

    /**
     * Persist a new dispatch pool.
     */
    public void persistPool(DispatchPool pool) {
        var updated = pool.toBuilder()
            .createdAt(pool.createdAt() != null ? pool.createdAt() : Instant.now())
            .updatedAt(Instant.now())
            .build();
        DispatchPoolEntity entity = DispatchPoolMapper.toEntity(updated);
        persist(entity);
    }

    /**
     * Update an existing dispatch pool.
     */
    public void updatePool(DispatchPool pool) {
        var updated = pool.toBuilder()
            .updatedAt(Instant.now())
            .build();
        DispatchPoolEntity entity = findById(pool.id());
        if (entity != null) {
            DispatchPoolMapper.updateEntity(entity, updated);
        }
    }

    /**
     * Delete a dispatch pool by ID.
     */
    public boolean deletePoolById(String id) {
        return deleteById(id);
    }
}

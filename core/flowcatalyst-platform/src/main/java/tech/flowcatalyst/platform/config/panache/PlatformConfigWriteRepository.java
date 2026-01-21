package tech.flowcatalyst.platform.config.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.config.PlatformConfig;
import tech.flowcatalyst.platform.config.entity.PlatformConfigEntity;
import tech.flowcatalyst.platform.config.mapper.PlatformConfigMapper;

import java.time.Instant;

/**
 * Write-side repository for PlatformConfig entities.
 */
@ApplicationScoped
public class PlatformConfigWriteRepository implements PanacheRepositoryBase<PlatformConfigEntity, String> {

    /**
     * Persist a new platform config.
     */
    public void persistConfig(PlatformConfig config) {
        if (config.createdAt == null) {
            config.createdAt = Instant.now();
        }
        config.updatedAt = Instant.now();
        PlatformConfigEntity entity = PlatformConfigMapper.toEntity(config);
        persist(entity);
    }

    /**
     * Update an existing platform config.
     */
    public void updateConfig(PlatformConfig config) {
        config.updatedAt = Instant.now();
        PlatformConfigEntity entity = findById(config.id);
        if (entity != null) {
            PlatformConfigMapper.updateEntity(entity, config);
        }
    }

    /**
     * Delete a platform config by ID.
     */
    public boolean deleteConfig(String id) {
        return deleteById(id);
    }
}

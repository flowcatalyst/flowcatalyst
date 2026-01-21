package tech.flowcatalyst.platform.application.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.entity.ApplicationClientConfigEntity;
import tech.flowcatalyst.platform.application.mapper.ApplicationClientConfigMapper;

import java.time.Instant;

/**
 * Write-side repository for ApplicationClientConfig entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class ApplicationClientConfigWriteRepository implements PanacheRepositoryBase<ApplicationClientConfigEntity, String> {

    /**
     * Persist a new application client config.
     */
    public void persistConfig(ApplicationClientConfig config) {
        if (config.createdAt == null) {
            config.createdAt = Instant.now();
        }
        config.updatedAt = Instant.now();
        ApplicationClientConfigEntity entity = ApplicationClientConfigMapper.toEntity(config);
        persist(entity);
    }

    /**
     * Update an existing application client config.
     */
    public void updateConfig(ApplicationClientConfig config) {
        config.updatedAt = Instant.now();
        ApplicationClientConfigEntity entity = findById(config.id);
        if (entity != null) {
            ApplicationClientConfigMapper.updateEntity(entity, config);
        }
    }

    /**
     * Delete an application client config by ID.
     */
    public boolean deleteConfigById(String id) {
        return deleteById(id);
    }

    /**
     * Delete by application and client IDs.
     */
    public long deleteByApplicationAndClient(String applicationId, String clientId) {
        return delete("applicationId = ?1 and clientId = ?2", applicationId, clientId);
    }
}

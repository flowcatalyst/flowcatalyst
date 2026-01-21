package tech.flowcatalyst.platform.client.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.entity.ClientAuthConfigEntity;
import tech.flowcatalyst.platform.client.mapper.ClientAuthConfigMapper;

import java.time.Instant;

/**
 * Write-side repository for ClientAuthConfig entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class ClientAuthConfigWriteRepository implements PanacheRepositoryBase<ClientAuthConfigEntity, String> {

    /**
     * Persist a new client auth config.
     */
    public void persistConfig(ClientAuthConfig config) {
        if (config.createdAt == null) {
            config.createdAt = Instant.now();
        }
        config.updatedAt = Instant.now();
        ClientAuthConfigEntity entity = ClientAuthConfigMapper.toEntity(config);
        persist(entity);
    }

    /**
     * Update an existing client auth config.
     */
    public void updateConfig(ClientAuthConfig config) {
        config.updatedAt = Instant.now();
        ClientAuthConfigEntity entity = findById(config.id);
        if (entity != null) {
            ClientAuthConfigMapper.updateEntity(entity, config);
        }
    }

    /**
     * Delete a client auth config by ID.
     */
    public boolean deleteConfig(String id) {
        return deleteById(id);
    }
}

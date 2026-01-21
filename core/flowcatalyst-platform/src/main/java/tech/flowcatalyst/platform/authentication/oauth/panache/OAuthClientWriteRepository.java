package tech.flowcatalyst.platform.authentication.oauth.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.entity.OAuthClientEntity;
import tech.flowcatalyst.platform.authentication.oauth.mapper.OAuthClientMapper;

import java.time.Instant;

/**
 * Write-side repository for OAuthClient entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class OAuthClientWriteRepository implements PanacheRepositoryBase<OAuthClientEntity, String> {

    /**
     * Persist a new OAuth client.
     */
    public void persistClient(OAuthClient client) {
        if (client.createdAt == null) {
            client.createdAt = Instant.now();
        }
        client.updatedAt = Instant.now();
        OAuthClientEntity entity = OAuthClientMapper.toEntity(client);
        persist(entity);
    }

    /**
     * Update an existing OAuth client.
     */
    public void updateClient(OAuthClient client) {
        client.updatedAt = Instant.now();
        OAuthClientEntity entity = findById(client.id);
        if (entity != null) {
            OAuthClientMapper.updateEntity(entity, client);
        }
    }

    /**
     * Delete an OAuth client by ID.
     */
    public boolean deleteClientById(String id) {
        return deleteById(id);
    }

    /**
     * Delete by service account principal ID.
     */
    public long deleteByServiceAccountPrincipalId(String principalId) {
        return delete("serviceAccountPrincipalId", principalId);
    }
}

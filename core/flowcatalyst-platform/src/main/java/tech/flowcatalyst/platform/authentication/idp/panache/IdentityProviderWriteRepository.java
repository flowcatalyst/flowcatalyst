package tech.flowcatalyst.platform.authentication.idp.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.entity.IdentityProviderEntity;
import tech.flowcatalyst.platform.authentication.idp.mapper.IdentityProviderMapper;

import java.time.Instant;

/**
 * Write-side repository for IdentityProvider entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class IdentityProviderWriteRepository implements PanacheRepositoryBase<IdentityProviderEntity, String> {

    /**
     * Persist a new identity provider.
     */
    public void persistIdp(IdentityProvider idp) {
        if (idp.createdAt == null) {
            idp.createdAt = Instant.now();
        }
        idp.updatedAt = Instant.now();
        var entity = IdentityProviderMapper.toEntity(idp);
        persist(entity);
    }

    /**
     * Update an existing identity provider.
     */
    public void updateIdp(IdentityProvider idp) {
        idp.updatedAt = Instant.now();
        var entity = findById(idp.id);
        if (entity != null) {
            IdentityProviderMapper.updateEntity(entity, idp);
        }
    }

    /**
     * Delete an identity provider by ID.
     */
    public boolean deleteIdp(String id) {
        return deleteById(id);
    }
}

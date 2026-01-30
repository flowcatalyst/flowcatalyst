package tech.flowcatalyst.platform.authentication.domain.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.entity.EmailDomainMappingEntity;
import tech.flowcatalyst.platform.authentication.domain.mapper.EmailDomainMappingMapper;

import java.time.Instant;

/**
 * Write-side repository for EmailDomainMapping entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class EmailDomainMappingWriteRepository implements PanacheRepositoryBase<EmailDomainMappingEntity, String> {

    /**
     * Persist a new email domain mapping.
     */
    public void persistMapping(EmailDomainMapping mapping) {
        if (mapping.createdAt == null) {
            mapping.createdAt = Instant.now();
        }
        mapping.updatedAt = Instant.now();
        var entity = EmailDomainMappingMapper.toEntity(mapping);
        persist(entity);
    }

    /**
     * Update an existing email domain mapping.
     */
    public void updateMapping(EmailDomainMapping mapping) {
        mapping.updatedAt = Instant.now();
        var entity = findById(mapping.id);
        if (entity != null) {
            EmailDomainMappingMapper.updateEntity(entity, mapping);
        }
    }

    /**
     * Delete an email domain mapping by ID.
     */
    public boolean deleteMapping(String id) {
        return deleteById(id);
    }
}

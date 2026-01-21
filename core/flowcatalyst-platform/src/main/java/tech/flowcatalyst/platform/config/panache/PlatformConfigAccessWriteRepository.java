package tech.flowcatalyst.platform.config.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.config.PlatformConfigAccess;
import tech.flowcatalyst.platform.config.entity.PlatformConfigAccessEntity;
import tech.flowcatalyst.platform.config.mapper.PlatformConfigAccessMapper;

import java.time.Instant;

/**
 * Write-side repository for PlatformConfigAccess entities.
 */
@ApplicationScoped
public class PlatformConfigAccessWriteRepository implements PanacheRepositoryBase<PlatformConfigAccessEntity, String> {

    /**
     * Persist a new config access grant.
     */
    public void persistAccess(PlatformConfigAccess access) {
        if (access.createdAt == null) {
            access.createdAt = Instant.now();
        }
        PlatformConfigAccessEntity entity = PlatformConfigAccessMapper.toEntity(access);
        persist(entity);
    }

    /**
     * Update an existing config access grant.
     */
    public void updateAccess(PlatformConfigAccess access) {
        PlatformConfigAccessEntity entity = findById(access.id);
        if (entity != null) {
            PlatformConfigAccessMapper.updateEntity(entity, access);
        }
    }

    /**
     * Delete a config access grant by ID.
     */
    public boolean deleteAccess(String id) {
        return deleteById(id);
    }

    /**
     * Delete by application code and role code.
     */
    public boolean deleteByApplicationAndRole(String applicationCode, String roleCode) {
        return delete("applicationCode = ?1 AND roleCode = ?2", applicationCode, roleCode) > 0;
    }
}

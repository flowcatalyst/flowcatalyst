package tech.flowcatalyst.platform.authorization.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authorization.AuthPermission;
import tech.flowcatalyst.platform.authorization.entity.AuthPermissionEntity;
import tech.flowcatalyst.platform.authorization.mapper.AuthPermissionMapper;

/**
 * Write-side repository for AuthPermission entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class AuthPermissionWriteRepository implements PanacheRepositoryBase<AuthPermissionEntity, String> {

    /**
     * Persist a new auth permission.
     */
    public void persistPermission(AuthPermission permission) {
        AuthPermissionEntity entity = AuthPermissionMapper.toEntity(permission);
        persist(entity);
    }

    /**
     * Update an existing auth permission.
     */
    public void updatePermission(AuthPermission permission) {
        AuthPermissionEntity entity = findById(permission.id);
        if (entity != null) {
            AuthPermissionMapper.updateEntity(entity, permission);
        }
    }

    /**
     * Delete an auth permission by ID.
     */
    public boolean deletePermissionById(String id) {
        return deleteById(id);
    }

    /**
     * Delete by application ID.
     */
    public long deleteByApplicationId(String applicationId) {
        return delete("applicationId", applicationId);
    }
}

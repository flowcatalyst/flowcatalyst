package tech.flowcatalyst.platform.authorization.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.entity.AuthRoleEntity;
import tech.flowcatalyst.platform.authorization.mapper.AuthRoleMapper;

import java.time.Instant;

/**
 * Write-side repository for AuthRole entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class AuthRoleWriteRepository implements PanacheRepositoryBase<AuthRoleEntity, String> {

    /**
     * Persist a new auth role.
     */
    public void persistRole(AuthRole role) {
        if (role.createdAt == null) {
            role.createdAt = Instant.now();
        }
        role.updatedAt = Instant.now();
        AuthRoleEntity entity = AuthRoleMapper.toEntity(role);
        persist(entity);
    }

    /**
     * Update an existing auth role.
     */
    public void updateRole(AuthRole role) {
        role.updatedAt = Instant.now();
        AuthRoleEntity entity = findById(role.id);
        if (entity != null) {
            AuthRoleMapper.updateEntity(entity, role);
        }
    }

    /**
     * Delete an auth role by ID.
     */
    public boolean deleteRoleById(String id) {
        return deleteById(id);
    }
}

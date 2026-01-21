package tech.flowcatalyst.platform.principal.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.entity.PrincipalEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalManagedApplicationEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalRoleEntity;
import tech.flowcatalyst.platform.principal.mapper.PrincipalMapper;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for Principal entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
@Transactional
public class PrincipalWriteRepository implements PanacheRepositoryBase<PrincipalEntity, String> {

    @Inject
    EntityManager em;

    /**
     * Persist a new principal with its roles and managed applications.
     */
    public void persistPrincipal(Principal principal) {
        if (principal.createdAt == null) {
            principal.createdAt = Instant.now();
        }
        principal.updatedAt = Instant.now();

        PrincipalEntity entity = PrincipalMapper.toEntity(principal);
        persist(entity);

        // Save roles to normalized table
        saveRoles(principal.id, principal.roles);

        // Save managed applications to normalized table
        saveManagedApplications(principal.id, principal.managedApplicationIds);
    }

    /**
     * Update an existing principal with its roles and managed applications.
     */
    public void updatePrincipal(Principal principal) {
        principal.updatedAt = Instant.now();

        PrincipalEntity entity = findById(principal.id);
        if (entity != null) {
            PrincipalMapper.updateEntity(entity, principal);
        }

        // Update roles in normalized table
        saveRoles(principal.id, principal.roles);

        // Update managed applications in normalized table
        saveManagedApplications(principal.id, principal.managedApplicationIds);
    }

    /**
     * Update an existing principal WITHOUT updating roles.
     * Use this for simple field updates like lastLoginAt.
     */
    public void updatePrincipalOnly(Principal principal) {
        principal.updatedAt = Instant.now();

        PrincipalEntity entity = findById(principal.id);
        if (entity != null) {
            PrincipalMapper.updateEntity(entity, principal);
        }
        // Note: Does NOT update roles
    }

    /**
     * Delete a principal and its roles and managed applications.
     */
    public boolean deletePrincipal(String id) {
        // Delete roles first (or use ON DELETE CASCADE)
        em.createQuery("DELETE FROM PrincipalRoleEntity WHERE principalId = :id")
            .setParameter("id", id)
            .executeUpdate();

        // Delete managed applications
        em.createQuery("DELETE FROM PrincipalManagedApplicationEntity WHERE principalId = :id")
            .setParameter("id", id)
            .executeUpdate();

        return deleteById(id);
    }

    /**
     * Save roles to the normalized principal_roles table.
     * Replaces all existing roles for the principal.
     */
    private void saveRoles(String principalId, List<Principal.RoleAssignment> roles) {
        // Delete existing roles
        em.createQuery("DELETE FROM PrincipalRoleEntity WHERE principalId = :id")
            .setParameter("id", principalId)
            .executeUpdate();

        // Flush to synchronize with database
        em.flush();

        // Insert new roles using merge to handle any stale references
        if (roles != null) {
            List<PrincipalRoleEntity> entities = PrincipalMapper.toRoleEntities(principalId, roles);
            for (PrincipalRoleEntity entity : entities) {
                em.merge(entity);
            }
        }
    }

    /**
     * Save managed applications to the normalized principal_managed_applications table.
     * Replaces all existing managed applications for the principal.
     */
    private void saveManagedApplications(String principalId, List<String> applicationIds) {
        // Delete existing managed applications
        em.createQuery("DELETE FROM PrincipalManagedApplicationEntity WHERE principalId = :id")
            .setParameter("id", principalId)
            .executeUpdate();

        // Flush to synchronize with database
        em.flush();

        // Insert new managed applications using merge to handle any stale references
        if (applicationIds != null) {
            List<PrincipalManagedApplicationEntity> entities =
                PrincipalMapper.toManagedApplicationEntities(principalId, applicationIds);
            for (PrincipalManagedApplicationEntity entity : entities) {
                em.merge(entity);
            }
        }
    }
}

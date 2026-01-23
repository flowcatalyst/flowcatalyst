package tech.flowcatalyst.serviceaccount.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountClientIdEntity;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountJpaEntity;
import tech.flowcatalyst.serviceaccount.mapper.ServiceAccountMapper;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for ServiceAccount entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
@Transactional
public class ServiceAccountWriteRepository implements PanacheRepositoryBase<ServiceAccountJpaEntity, String> {

    @Inject
    EntityManager em;

    /**
     * Persist a new service account with its client IDs.
     * Note: Roles are stored on the Principal entity, not on the ServiceAccount.
     */
    public void persistServiceAccount(ServiceAccount serviceAccount) {
        ServiceAccount toSave = serviceAccount;
        if (toSave.createdAt == null) {
            toSave.createdAt = Instant.now();
        }
        toSave.updatedAt = Instant.now();

        ServiceAccountJpaEntity entity = ServiceAccountMapper.toEntity(toSave);
        persist(entity);

        // Save client IDs
        saveClientIds(toSave.id, toSave.clientIds);
    }

    /**
     * Update an existing service account with its client IDs.
     * Note: Roles are stored on the Principal entity, not on the ServiceAccount.
     */
    public void updateServiceAccount(ServiceAccount serviceAccount) {
        serviceAccount.updatedAt = Instant.now();

        ServiceAccountJpaEntity entity = findById(serviceAccount.id);
        if (entity != null) {
            ServiceAccountMapper.updateEntity(entity, serviceAccount);
        }

        // Update client IDs
        saveClientIds(serviceAccount.id, serviceAccount.clientIds);
    }

    /**
     * Delete a service account and its related entities.
     * Note: Roles are stored on the Principal entity, not on the ServiceAccount.
     */
    public boolean deleteServiceAccount(String id) {
        // Delete client IDs first
        em.createQuery("DELETE FROM ServiceAccountClientIdEntity WHERE serviceAccountId = :id")
            .setParameter("id", id)
            .executeUpdate();

        return deleteById(id);
    }

    /**
     * Save client IDs to the normalized table.
     * Replaces all existing client IDs for the service account.
     */
    private void saveClientIds(String serviceAccountId, List<String> clientIds) {
        // Delete existing
        em.createQuery("DELETE FROM ServiceAccountClientIdEntity WHERE serviceAccountId = :id")
            .setParameter("id", serviceAccountId)
            .executeUpdate();

        // Insert new
        if (clientIds != null) {
            List<ServiceAccountClientIdEntity> entities = ServiceAccountMapper.toClientIdEntities(serviceAccountId, clientIds);
            for (ServiceAccountClientIdEntity entity : entities) {
                em.persist(entity);
            }
        }
    }
}

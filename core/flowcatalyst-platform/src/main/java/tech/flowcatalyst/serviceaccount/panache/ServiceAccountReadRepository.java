package tech.flowcatalyst.serviceaccount.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountClientIdEntity;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountJpaEntity;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountRoleEntity;
import tech.flowcatalyst.serviceaccount.mapper.ServiceAccountMapper;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountFilter;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for ServiceAccount entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class ServiceAccountReadRepository implements ServiceAccountRepository {

    @Inject
    EntityManager em;

    @Inject
    ServiceAccountWriteRepository writeRepo;

    @Override
    public ServiceAccount findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<ServiceAccount> findByIdOptional(String id) {
        ServiceAccountJpaEntity entity = em.find(ServiceAccountJpaEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        return Optional.of(toDomainWithRelations(entity));
    }

    @Override
    public Optional<ServiceAccount> findByCode(String code) {
        List<ServiceAccountJpaEntity> results = em.createQuery(
                "FROM ServiceAccountJpaEntity WHERE code = :code", ServiceAccountJpaEntity.class)
            .setParameter("code", code)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(toDomainWithRelations(results.get(0)));
    }

    @Override
    public Optional<ServiceAccount> findByApplicationId(String applicationId) {
        List<ServiceAccountJpaEntity> results = em.createQuery(
                "FROM ServiceAccountJpaEntity WHERE applicationId = :applicationId", ServiceAccountJpaEntity.class)
            .setParameter("applicationId", applicationId)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(toDomainWithRelations(results.get(0)));
    }

    @Override
    public List<ServiceAccount> findByClientId(String clientId) {
        // Service accounts that have the given clientId in their clientIds list
        @SuppressWarnings("unchecked")
        List<ServiceAccountJpaEntity> results = em.createNativeQuery(
                "SELECT sa.* FROM service_accounts sa " +
                "JOIN service_account_client_ids saci ON sa.id = saci.service_account_id " +
                "WHERE saci.client_id = :clientId",
                ServiceAccountJpaEntity.class)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<ServiceAccount> findActive() {
        return em.createQuery("FROM ServiceAccountJpaEntity WHERE active = true", ServiceAccountJpaEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<ServiceAccount> findWithFilter(ServiceAccountFilter filter) {
        StringBuilder jpql = new StringBuilder("FROM ServiceAccountJpaEntity WHERE 1=1");

        if (filter.active() != null) {
            jpql.append(" AND active = :active");
        }
        if (filter.applicationId() != null) {
            jpql.append(" AND applicationId = :applicationId");
        }

        TypedQuery<ServiceAccountJpaEntity> query = em.createQuery(jpql.toString(), ServiceAccountJpaEntity.class);

        if (filter.active() != null) {
            query.setParameter("active", filter.active());
        }
        if (filter.applicationId() != null) {
            query.setParameter("applicationId", filter.applicationId());
        }

        List<ServiceAccountJpaEntity> results = query.getResultList();

        // If filtering by clientId, do post-filtering since it's in a separate table
        if (filter.clientId() != null) {
            return results.stream()
                .map(this::toDomainWithRelations)
                .filter(sa -> sa.clientIds.contains(filter.clientId()))
                .toList();
        }

        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<ServiceAccount> listAll() {
        return em.createQuery("FROM ServiceAccountJpaEntity", ServiceAccountJpaEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM ServiceAccountJpaEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public long countWithFilter(ServiceAccountFilter filter) {
        // Simplified count - for more accurate count with clientId filter,
        // would need subquery
        StringBuilder jpql = new StringBuilder("SELECT COUNT(e) FROM ServiceAccountJpaEntity e WHERE 1=1");

        if (filter.active() != null) {
            jpql.append(" AND e.active = :active");
        }
        if (filter.applicationId() != null) {
            jpql.append(" AND e.applicationId = :applicationId");
        }

        TypedQuery<Long> query = em.createQuery(jpql.toString(), Long.class);

        if (filter.active() != null) {
            query.setParameter("active", filter.active());
        }
        if (filter.applicationId() != null) {
            query.setParameter("applicationId", filter.applicationId());
        }

        return query.getSingleResult();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(ServiceAccount serviceAccount) {
        writeRepo.persistServiceAccount(serviceAccount);
    }

    @Override
    public void update(ServiceAccount serviceAccount) {
        writeRepo.updateServiceAccount(serviceAccount);
    }

    @Override
    public void delete(ServiceAccount serviceAccount) {
        writeRepo.deleteServiceAccount(serviceAccount.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteServiceAccount(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private ServiceAccount toDomainWithRelations(ServiceAccountJpaEntity entity) {
        ServiceAccount base = ServiceAccountMapper.toDomain(entity);

        // Load client IDs
        List<ServiceAccountClientIdEntity> clientIdEntities = em.createQuery(
                "FROM ServiceAccountClientIdEntity WHERE serviceAccountId = :id", ServiceAccountClientIdEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        List<String> clientIds = ServiceAccountMapper.toClientIds(clientIdEntities);

        // Load roles
        List<ServiceAccountRoleEntity> roleEntities = em.createQuery(
                "FROM ServiceAccountRoleEntity WHERE serviceAccountId = :id", ServiceAccountRoleEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        List<Principal.RoleAssignment> roles = ServiceAccountMapper.toRoleAssignments(roleEntities);

        base.clientIds = clientIds;
        base.roles = roles;

        return base;
    }
}

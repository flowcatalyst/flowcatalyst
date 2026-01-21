package tech.flowcatalyst.dispatchpool.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.entity.DispatchPoolEntity;
import tech.flowcatalyst.dispatchpool.mapper.DispatchPoolMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for DispatchPool entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class DispatchPoolReadRepository implements DispatchPoolRepository {

    @Inject
    EntityManager em;

    @Inject
    DispatchPoolWriteRepository writeRepo;

    @Override
    public DispatchPool findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<DispatchPool> findByIdOptional(String id) {
        DispatchPoolEntity entity = em.find(DispatchPoolEntity.class, id);
        return Optional.ofNullable(entity).map(DispatchPoolMapper::toDomain);
    }

    @Override
    public Optional<DispatchPool> findByCodeAndClientId(String code, String clientId) {
        String jpql;
        TypedQuery<DispatchPoolEntity> query;
        if (clientId == null) {
            jpql = "FROM DispatchPoolEntity WHERE code = :code AND clientId IS NULL";
            query = em.createQuery(jpql, DispatchPoolEntity.class)
                .setParameter("code", code);
        } else {
            jpql = "FROM DispatchPoolEntity WHERE code = :code AND clientId = :clientId";
            query = em.createQuery(jpql, DispatchPoolEntity.class)
                .setParameter("code", code)
                .setParameter("clientId", clientId);
        }
        var results = query.getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(DispatchPoolMapper.toDomain(results.get(0)));
    }

    @Override
    public List<DispatchPool> findByClientId(String clientId) {
        return em.createQuery("FROM DispatchPoolEntity WHERE clientId = :clientId", DispatchPoolEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> findAnchorLevel() {
        return em.createQuery("FROM DispatchPoolEntity WHERE clientId IS NULL", DispatchPoolEntity.class)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> findByStatus(DispatchPoolStatus status) {
        return em.createQuery("FROM DispatchPoolEntity WHERE status = :status", DispatchPoolEntity.class)
            .setParameter("status", status)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> findActive() {
        return em.createQuery("FROM DispatchPoolEntity WHERE status = :status", DispatchPoolEntity.class)
            .setParameter("status", DispatchPoolStatus.ACTIVE)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> findAllNonArchived() {
        return em.createQuery("FROM DispatchPoolEntity WHERE status != :status", DispatchPoolEntity.class)
            .setParameter("status", DispatchPoolStatus.ARCHIVED)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> findWithFilters(String clientId, DispatchPoolStatus status, boolean includeArchived) {
        StringBuilder jpql = new StringBuilder("FROM DispatchPoolEntity WHERE 1=1");

        if (clientId != null) {
            jpql.append(" AND clientId = :clientId");
        }
        if (status != null) {
            jpql.append(" AND status = :status");
        }
        if (!includeArchived && status == null) {
            jpql.append(" AND status != :archivedStatus");
        }

        TypedQuery<DispatchPoolEntity> query = em.createQuery(jpql.toString(), DispatchPoolEntity.class);

        if (clientId != null) {
            query.setParameter("clientId", clientId);
        }
        if (status != null) {
            query.setParameter("status", status);
        }
        if (!includeArchived && status == null) {
            query.setParameter("archivedStatus", DispatchPoolStatus.ARCHIVED);
        }

        return query.getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchPool> listAll() {
        return em.createQuery("FROM DispatchPoolEntity", DispatchPoolEntity.class)
            .getResultList()
            .stream()
            .map(DispatchPoolMapper::toDomain)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM DispatchPoolEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public boolean existsByCodeAndClientId(String code, String clientId) {
        String jpql;
        TypedQuery<Long> query;
        if (clientId == null) {
            jpql = "SELECT COUNT(e) FROM DispatchPoolEntity e WHERE e.code = :code AND e.clientId IS NULL";
            query = em.createQuery(jpql, Long.class)
                .setParameter("code", code);
        } else {
            jpql = "SELECT COUNT(e) FROM DispatchPoolEntity e WHERE e.code = :code AND e.clientId = :clientId";
            query = em.createQuery(jpql, Long.class)
                .setParameter("code", code)
                .setParameter("clientId", clientId);
        }
        return query.getSingleResult() > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(DispatchPool pool) {
        writeRepo.persistPool(pool);
    }

    @Override
    public void update(DispatchPool pool) {
        writeRepo.updatePool(pool);
    }

    @Override
    public void delete(DispatchPool pool) {
        writeRepo.deletePoolById(pool.id());
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deletePoolById(id);
    }
}

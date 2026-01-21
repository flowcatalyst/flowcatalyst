package tech.flowcatalyst.platform.authorization.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.authorization.AuthPermission;
import tech.flowcatalyst.platform.authorization.AuthPermissionRepository;
import tech.flowcatalyst.platform.authorization.entity.AuthPermissionEntity;
import tech.flowcatalyst.platform.authorization.mapper.AuthPermissionMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for AuthPermission entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class AuthPermissionReadRepository implements AuthPermissionRepository {

    @Inject
    EntityManager em;

    @Inject
    AuthPermissionWriteRepository writeRepo;

    @Override
    public Optional<AuthPermission> findByName(String name) {
        var results = em.createQuery("FROM AuthPermissionEntity WHERE name = :name", AuthPermissionEntity.class)
            .setParameter("name", name)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(AuthPermissionMapper.toDomain(results.get(0)));
    }

    @Override
    public List<AuthPermission> findByApplicationId(String applicationId) {
        return em.createQuery(
                "FROM AuthPermissionEntity WHERE applicationId = :appId", AuthPermissionEntity.class)
            .setParameter("appId", applicationId)
            .getResultList()
            .stream()
            .map(AuthPermissionMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuthPermission> listAll() {
        return em.createQuery("FROM AuthPermissionEntity", AuthPermissionEntity.class)
            .getResultList()
            .stream()
            .map(AuthPermissionMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByName(String name) {
        Long count = em.createQuery("SELECT COUNT(e) FROM AuthPermissionEntity e WHERE e.name = :name", Long.class)
            .setParameter("name", name)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(AuthPermission permission) {
        writeRepo.persistPermission(permission);
    }

    @Override
    public void update(AuthPermission permission) {
        writeRepo.updatePermission(permission);
    }

    @Override
    public void delete(AuthPermission permission) {
        writeRepo.deletePermissionById(permission.id);
    }

    @Override
    public long deleteByApplicationId(String applicationId) {
        return writeRepo.deleteByApplicationId(applicationId);
    }
}

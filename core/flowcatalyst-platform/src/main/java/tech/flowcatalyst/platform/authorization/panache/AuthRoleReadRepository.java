package tech.flowcatalyst.platform.authorization.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.entity.AuthRoleEntity;
import tech.flowcatalyst.platform.authorization.mapper.AuthRoleMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for AuthRole entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class AuthRoleReadRepository implements AuthRoleRepository {

    @Inject
    EntityManager em;

    @Inject
    AuthRoleWriteRepository writeRepo;

    @Override
    public Optional<AuthRole> findByName(String name) {
        var results = em.createQuery("FROM AuthRoleEntity WHERE name = :name", AuthRoleEntity.class)
            .setParameter("name", name)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(AuthRoleMapper.toDomain(results.get(0)));
    }

    @Override
    public List<AuthRole> findByApplicationCode(String applicationCode) {
        return em.createQuery(
                "FROM AuthRoleEntity WHERE applicationCode = :appCode", AuthRoleEntity.class)
            .setParameter("appCode", applicationCode)
            .getResultList()
            .stream()
            .map(AuthRoleMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuthRole> findBySource(AuthRole.RoleSource source) {
        return em.createQuery("FROM AuthRoleEntity WHERE source = :source", AuthRoleEntity.class)
            .setParameter("source", source)
            .getResultList()
            .stream()
            .map(AuthRoleMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuthRole> listAll() {
        return em.createQuery("FROM AuthRoleEntity", AuthRoleEntity.class)
            .getResultList()
            .stream()
            .map(AuthRoleMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByName(String name) {
        Long count = em.createQuery("SELECT COUNT(e) FROM AuthRoleEntity e WHERE e.name = :name", Long.class)
            .setParameter("name", name)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(AuthRole role) {
        writeRepo.persistRole(role);
    }

    @Override
    public void update(AuthRole role) {
        writeRepo.updateRole(role);
    }

    @Override
    public void delete(AuthRole role) {
        writeRepo.deleteRoleById(role.id);
    }
}

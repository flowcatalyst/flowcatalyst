package tech.flowcatalyst.platform.config.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.config.PlatformConfigAccess;
import tech.flowcatalyst.platform.config.PlatformConfigAccessRepository;
import tech.flowcatalyst.platform.config.entity.PlatformConfigAccessEntity;
import tech.flowcatalyst.platform.config.mapper.PlatformConfigAccessMapper;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Read-side repository for PlatformConfigAccess entities.
 */
@ApplicationScoped
public class PlatformConfigAccessReadRepository implements PlatformConfigAccessRepository {

    @Inject
    EntityManager em;

    @Inject
    PlatformConfigAccessWriteRepository writeRepo;

    @Override
    public Optional<PlatformConfigAccess> findByIdOptional(String id) {
        PlatformConfigAccessEntity entity = em.find(PlatformConfigAccessEntity.class, id);
        return Optional.ofNullable(entity).map(PlatformConfigAccessMapper::toDomain);
    }

    @Override
    public List<PlatformConfigAccess> findByApplicationCode(String applicationCode) {
        return em.createQuery(
                "FROM PlatformConfigAccessEntity WHERE applicationCode = :appCode ORDER BY roleCode",
                PlatformConfigAccessEntity.class)
            .setParameter("appCode", applicationCode)
            .getResultList()
            .stream()
            .map(PlatformConfigAccessMapper::toDomain)
            .toList();
    }

    @Override
    public List<PlatformConfigAccess> findByRoleCodes(Set<String> roleCodes) {
        if (roleCodes == null || roleCodes.isEmpty()) {
            return List.of();
        }
        return em.createQuery(
                "FROM PlatformConfigAccessEntity WHERE roleCode IN :roleCodes ORDER BY applicationCode, roleCode",
                PlatformConfigAccessEntity.class)
            .setParameter("roleCodes", roleCodes)
            .getResultList()
            .stream()
            .map(PlatformConfigAccessMapper::toDomain)
            .toList();
    }

    @Override
    public Optional<PlatformConfigAccess> findByApplicationAndRole(String applicationCode, String roleCode) {
        var results = em.createQuery(
                "FROM PlatformConfigAccessEntity WHERE applicationCode = :appCode AND roleCode = :roleCode",
                PlatformConfigAccessEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("roleCode", roleCode)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(PlatformConfigAccessMapper.toDomain(results.get(0)));
    }

    @Override
    public boolean hasReadAccess(String applicationCode, Set<String> roleCodes) {
        if (roleCodes == null || roleCodes.isEmpty()) {
            return false;
        }
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM PlatformConfigAccessEntity e " +
                "WHERE e.applicationCode = :appCode AND e.roleCode IN :roleCodes AND e.canRead = true",
                Long.class)
            .setParameter("appCode", applicationCode)
            .setParameter("roleCodes", roleCodes)
            .getSingleResult();
        return count > 0;
    }

    @Override
    public boolean hasWriteAccess(String applicationCode, Set<String> roleCodes) {
        if (roleCodes == null || roleCodes.isEmpty()) {
            return false;
        }
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM PlatformConfigAccessEntity e " +
                "WHERE e.applicationCode = :appCode AND e.roleCode IN :roleCodes AND e.canWrite = true",
                Long.class)
            .setParameter("appCode", applicationCode)
            .setParameter("roleCodes", roleCodes)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(PlatformConfigAccess access) {
        writeRepo.persistAccess(access);
    }

    @Override
    public void update(PlatformConfigAccess access) {
        writeRepo.updateAccess(access);
    }

    @Override
    public void delete(PlatformConfigAccess access) {
        writeRepo.deleteAccess(access.id);
    }

    @Override
    public boolean deleteByApplicationAndRole(String applicationCode, String roleCode) {
        return writeRepo.deleteByApplicationAndRole(applicationCode, roleCode);
    }
}

package tech.flowcatalyst.platform.config.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.config.ConfigScope;
import tech.flowcatalyst.platform.config.PlatformConfig;
import tech.flowcatalyst.platform.config.PlatformConfigRepository;
import tech.flowcatalyst.platform.config.entity.PlatformConfigEntity;
import tech.flowcatalyst.platform.config.mapper.PlatformConfigMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for PlatformConfig entities.
 */
@ApplicationScoped
public class PlatformConfigReadRepository implements PlatformConfigRepository {

    @Inject
    EntityManager em;

    @Inject
    PlatformConfigWriteRepository writeRepo;

    @Override
    public Optional<PlatformConfig> findByIdOptional(String id) {
        PlatformConfigEntity entity = em.find(PlatformConfigEntity.class, id);
        return Optional.ofNullable(entity).map(PlatformConfigMapper::toDomain);
    }

    @Override
    public Optional<PlatformConfig> findByKey(String applicationCode, String section, String property,
                                               ConfigScope scope, String clientId) {
        String query;
        if (scope == ConfigScope.GLOBAL) {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND section = :section AND property = :property " +
                    "AND scope = :scope AND clientId IS NULL";
        } else {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND section = :section AND property = :property " +
                    "AND scope = :scope AND clientId = :clientId";
        }

        var typedQuery = em.createQuery(query, PlatformConfigEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("section", section)
            .setParameter("property", property)
            .setParameter("scope", scope);

        if (scope == ConfigScope.CLIENT) {
            typedQuery.setParameter("clientId", clientId);
        }

        var results = typedQuery.getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(PlatformConfigMapper.toDomain(results.get(0)));
    }

    @Override
    public List<PlatformConfig> findByApplicationAndSection(String applicationCode, String section,
                                                             ConfigScope scope, String clientId) {
        String query;
        if (scope == ConfigScope.GLOBAL) {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND section = :section AND scope = :scope AND clientId IS NULL " +
                    "ORDER BY property";
        } else {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND section = :section AND scope = :scope AND clientId = :clientId " +
                    "ORDER BY property";
        }

        var typedQuery = em.createQuery(query, PlatformConfigEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("section", section)
            .setParameter("scope", scope);

        if (scope == ConfigScope.CLIENT) {
            typedQuery.setParameter("clientId", clientId);
        }

        return typedQuery.getResultList()
            .stream()
            .map(PlatformConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<PlatformConfig> findByApplication(String applicationCode, ConfigScope scope, String clientId) {
        String query;
        if (scope == ConfigScope.GLOBAL) {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND scope = :scope AND clientId IS NULL " +
                    "ORDER BY section, property";
        } else {
            query = "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                    "AND scope = :scope AND clientId = :clientId " +
                    "ORDER BY section, property";
        }

        var typedQuery = em.createQuery(query, PlatformConfigEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("scope", scope);

        if (scope == ConfigScope.CLIENT) {
            typedQuery.setParameter("clientId", clientId);
        }

        return typedQuery.getResultList()
            .stream()
            .map(PlatformConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<PlatformConfig> findAllGlobal(String applicationCode) {
        return em.createQuery(
                "FROM PlatformConfigEntity WHERE applicationCode = :appCode AND scope = :scope " +
                "ORDER BY section, property",
                PlatformConfigEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("scope", ConfigScope.GLOBAL)
            .getResultList()
            .stream()
            .map(PlatformConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<PlatformConfig> findAllGlobalBySection(String applicationCode, String section) {
        return em.createQuery(
                "FROM PlatformConfigEntity WHERE applicationCode = :appCode " +
                "AND section = :section AND scope = :scope " +
                "ORDER BY property",
                PlatformConfigEntity.class)
            .setParameter("appCode", applicationCode)
            .setParameter("section", section)
            .setParameter("scope", ConfigScope.GLOBAL)
            .getResultList()
            .stream()
            .map(PlatformConfigMapper::toDomain)
            .toList();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(PlatformConfig config) {
        writeRepo.persistConfig(config);
    }

    @Override
    public void update(PlatformConfig config) {
        writeRepo.updateConfig(config);
    }

    @Override
    public void delete(PlatformConfig config) {
        writeRepo.deleteConfig(config.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteConfig(id);
    }
}

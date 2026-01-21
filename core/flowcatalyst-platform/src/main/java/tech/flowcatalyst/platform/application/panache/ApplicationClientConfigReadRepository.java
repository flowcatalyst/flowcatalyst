package tech.flowcatalyst.platform.application.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.application.ApplicationClientConfig;
import tech.flowcatalyst.platform.application.ApplicationClientConfigRepository;
import tech.flowcatalyst.platform.application.entity.ApplicationClientConfigEntity;
import tech.flowcatalyst.platform.application.mapper.ApplicationClientConfigMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for ApplicationClientConfig entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class ApplicationClientConfigReadRepository implements ApplicationClientConfigRepository {

    @Inject
    EntityManager em;

    @Inject
    ApplicationClientConfigWriteRepository writeRepo;

    @Override
    public Optional<ApplicationClientConfig> findByIdOptional(String id) {
        ApplicationClientConfigEntity entity = em.find(ApplicationClientConfigEntity.class, id);
        return Optional.ofNullable(entity).map(ApplicationClientConfigMapper::toDomain);
    }

    @Override
    public Optional<ApplicationClientConfig> findByApplicationAndClient(String applicationId, String clientId) {
        var results = em.createQuery(
                "FROM ApplicationClientConfigEntity WHERE applicationId = :appId AND clientId = :clientId",
                ApplicationClientConfigEntity.class)
            .setParameter("appId", applicationId)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(ApplicationClientConfigMapper.toDomain(results.get(0)));
    }

    @Override
    public List<ApplicationClientConfig> findByApplication(String applicationId) {
        return em.createQuery(
                "FROM ApplicationClientConfigEntity WHERE applicationId = :appId", ApplicationClientConfigEntity.class)
            .setParameter("appId", applicationId)
            .getResultList()
            .stream()
            .map(ApplicationClientConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<ApplicationClientConfig> findByClient(String clientId) {
        return em.createQuery(
                "FROM ApplicationClientConfigEntity WHERE clientId = :clientId", ApplicationClientConfigEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(ApplicationClientConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<ApplicationClientConfig> findEnabledByClient(String clientId) {
        return em.createQuery(
                "FROM ApplicationClientConfigEntity WHERE clientId = :clientId AND enabled = true",
                ApplicationClientConfigEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(ApplicationClientConfigMapper::toDomain)
            .toList();
    }

    @Override
    public boolean isApplicationEnabledForClient(String applicationId, String clientId) {
        return findByApplicationAndClient(applicationId, clientId)
            .map(config -> config.enabled)
            .orElse(false);
    }

    @Override
    public long countByApplication(String applicationId) {
        return em.createQuery(
                "SELECT COUNT(e) FROM ApplicationClientConfigEntity e WHERE e.applicationId = :appId", Long.class)
            .setParameter("appId", applicationId)
            .getSingleResult();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(ApplicationClientConfig config) {
        writeRepo.persistConfig(config);
    }

    @Override
    public void update(ApplicationClientConfig config) {
        writeRepo.updateConfig(config);
    }

    @Override
    public void delete(ApplicationClientConfig config) {
        writeRepo.deleteConfigById(config.id);
    }

    @Override
    public void deleteByApplicationAndClient(String applicationId, String clientId) {
        writeRepo.deleteByApplicationAndClient(applicationId, clientId);
    }
}

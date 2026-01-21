package tech.flowcatalyst.platform.client.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.client.AuthConfigType;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.ClientAuthConfigRepository;
import tech.flowcatalyst.platform.client.entity.ClientAuthConfigEntity;
import tech.flowcatalyst.platform.client.mapper.ClientAuthConfigMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for ClientAuthConfig entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class ClientAuthConfigReadRepository implements ClientAuthConfigRepository {

    @Inject
    EntityManager em;

    @Inject
    ClientAuthConfigWriteRepository writeRepo;

    @Override
    public Optional<ClientAuthConfig> findByIdOptional(String id) {
        ClientAuthConfigEntity entity = em.find(ClientAuthConfigEntity.class, id);
        return Optional.ofNullable(entity).map(ClientAuthConfigMapper::toDomain);
    }

    @Override
    public Optional<ClientAuthConfig> findByEmailDomain(String emailDomain) {
        var results = em.createQuery(
                "FROM ClientAuthConfigEntity WHERE emailDomain = :emailDomain", ClientAuthConfigEntity.class)
            .setParameter("emailDomain", emailDomain)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(ClientAuthConfigMapper.toDomain(results.get(0)));
    }

    @Override
    public List<ClientAuthConfig> findByClientId(String clientId) {
        return em.createQuery(
                "FROM ClientAuthConfigEntity WHERE clientId = :clientId OR primaryClientId = :clientId",
                ClientAuthConfigEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(ClientAuthConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<ClientAuthConfig> findByConfigType(AuthConfigType configType) {
        return em.createQuery(
                "FROM ClientAuthConfigEntity WHERE configType = :configType", ClientAuthConfigEntity.class)
            .setParameter("configType", configType)
            .getResultList()
            .stream()
            .map(ClientAuthConfigMapper::toDomain)
            .toList();
    }

    @Override
    public List<ClientAuthConfig> listAll() {
        return em.createQuery("FROM ClientAuthConfigEntity", ClientAuthConfigEntity.class)
            .getResultList()
            .stream()
            .map(ClientAuthConfigMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByEmailDomain(String emailDomain) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM ClientAuthConfigEntity e WHERE e.emailDomain = :emailDomain", Long.class)
            .setParameter("emailDomain", emailDomain)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(ClientAuthConfig config) {
        writeRepo.persistConfig(config);
    }

    @Override
    public void update(ClientAuthConfig config) {
        writeRepo.updateConfig(config);
    }

    @Override
    public void delete(ClientAuthConfig config) {
        writeRepo.deleteConfig(config.id);
    }
}

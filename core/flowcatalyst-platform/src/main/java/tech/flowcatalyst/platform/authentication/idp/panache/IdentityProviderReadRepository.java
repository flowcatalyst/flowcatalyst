package tech.flowcatalyst.platform.authentication.idp.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.authentication.idp.entity.IdentityProviderEntity;
import tech.flowcatalyst.platform.authentication.idp.mapper.IdentityProviderMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for IdentityProvider entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class IdentityProviderReadRepository implements IdentityProviderRepository {

    @Inject
    EntityManager em;

    @Inject
    IdentityProviderWriteRepository writeRepo;

    @Override
    public Optional<IdentityProvider> findByIdOptional(String id) {
        var entity = em.find(IdentityProviderEntity.class, id);
        return Optional.ofNullable(entity).map(IdentityProviderMapper::toDomain);
    }

    @Override
    public Optional<IdentityProvider> findByCode(String code) {
        var results = em.createQuery(
                "FROM IdentityProviderEntity WHERE code = :code", IdentityProviderEntity.class)
            .setParameter("code", code)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(IdentityProviderMapper.toDomain(results.get(0)));
    }

    @Override
    public List<IdentityProvider> findByType(IdentityProviderType type) {
        return em.createQuery(
                "FROM IdentityProviderEntity WHERE type = :type", IdentityProviderEntity.class)
            .setParameter("type", type)
            .getResultList()
            .stream()
            .map(IdentityProviderMapper::toDomain)
            .toList();
    }

    @Override
    public List<IdentityProvider> listAll() {
        return em.createQuery("FROM IdentityProviderEntity ORDER BY name", IdentityProviderEntity.class)
            .getResultList()
            .stream()
            .map(IdentityProviderMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByCode(String code) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM IdentityProviderEntity e WHERE e.code = :code", Long.class)
            .setParameter("code", code)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(IdentityProvider idp) {
        writeRepo.persistIdp(idp);
    }

    @Override
    public void update(IdentityProvider idp) {
        writeRepo.updateIdp(idp);
    }

    @Override
    public void delete(IdentityProvider idp) {
        writeRepo.deleteIdp(idp.id);
    }
}

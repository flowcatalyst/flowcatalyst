package tech.flowcatalyst.platform.principal.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.principal.AnchorDomain;
import tech.flowcatalyst.platform.principal.AnchorDomainRepository;
import tech.flowcatalyst.platform.principal.entity.AnchorDomainEntity;
import tech.flowcatalyst.platform.principal.mapper.AnchorDomainMapper;

import java.util.List;
import java.util.Optional;

/**
 * Panache-based implementation of AnchorDomainRepository.
 */
@ApplicationScoped
public class PanacheAnchorDomainRepository implements AnchorDomainRepository {

    @Inject
    EntityManager em;

    @Override
    public Optional<AnchorDomain> findByIdOptional(String id) {
        AnchorDomainEntity entity = em.find(AnchorDomainEntity.class, id);
        return Optional.ofNullable(AnchorDomainMapper.toDomain(entity));
    }

    @Override
    public List<AnchorDomain> listAll() {
        return em.createQuery("FROM AnchorDomainEntity ORDER BY domain", AnchorDomainEntity.class)
            .getResultList()
            .stream()
            .map(AnchorDomainMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByDomain(String domain) {
        Long count = em.createQuery("SELECT COUNT(e) FROM AnchorDomainEntity e WHERE e.domain = :domain", Long.class)
            .setParameter("domain", domain)
            .getSingleResult();
        return count > 0;
    }

    @Override
    public void persist(AnchorDomain domain) {
        AnchorDomainEntity entity = AnchorDomainMapper.toEntity(domain);
        em.persist(entity);
    }

    @Override
    public void delete(AnchorDomain domain) {
        AnchorDomainEntity entity = em.find(AnchorDomainEntity.class, domain.id);
        if (entity != null) {
            em.remove(entity);
        }
    }

    public Optional<AnchorDomain> findByDomain(String domain) {
        var results = em.createQuery("FROM AnchorDomainEntity WHERE domain = :domain", AnchorDomainEntity.class)
            .setParameter("domain", domain)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(AnchorDomainMapper.toDomain(results.get(0)));
    }
}

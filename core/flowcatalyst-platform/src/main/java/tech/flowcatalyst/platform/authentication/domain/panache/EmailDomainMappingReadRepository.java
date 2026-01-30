package tech.flowcatalyst.platform.authentication.domain.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.domain.entity.EmailDomainMappingEntity;
import tech.flowcatalyst.platform.authentication.domain.mapper.EmailDomainMappingMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for EmailDomainMapping entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class EmailDomainMappingReadRepository implements EmailDomainMappingRepository {

    @Inject
    EntityManager em;

    @Inject
    EmailDomainMappingWriteRepository writeRepo;

    @Override
    public Optional<EmailDomainMapping> findByIdOptional(String id) {
        var entity = em.find(EmailDomainMappingEntity.class, id);
        return Optional.ofNullable(entity).map(EmailDomainMappingMapper::toDomain);
    }

    @Override
    public Optional<EmailDomainMapping> findByEmailDomain(String emailDomain) {
        var results = em.createQuery(
                "FROM EmailDomainMappingEntity WHERE emailDomain = :emailDomain", EmailDomainMappingEntity.class)
            .setParameter("emailDomain", emailDomain.toLowerCase())
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(EmailDomainMappingMapper.toDomain(results.get(0)));
    }

    @Override
    public List<EmailDomainMapping> findByIdentityProviderId(String identityProviderId) {
        return em.createQuery(
                "FROM EmailDomainMappingEntity WHERE identityProviderId = :idpId ORDER BY emailDomain",
                EmailDomainMappingEntity.class)
            .setParameter("idpId", identityProviderId)
            .getResultList()
            .stream()
            .map(EmailDomainMappingMapper::toDomain)
            .toList();
    }

    @Override
    public List<EmailDomainMapping> findByScopeType(ScopeType scopeType) {
        return em.createQuery(
                "FROM EmailDomainMappingEntity WHERE scopeType = :scopeType ORDER BY emailDomain",
                EmailDomainMappingEntity.class)
            .setParameter("scopeType", scopeType)
            .getResultList()
            .stream()
            .map(EmailDomainMappingMapper::toDomain)
            .toList();
    }

    @Override
    public List<EmailDomainMapping> findByPrimaryClientId(String primaryClientId) {
        return em.createQuery(
                "FROM EmailDomainMappingEntity WHERE primaryClientId = :clientId ORDER BY emailDomain",
                EmailDomainMappingEntity.class)
            .setParameter("clientId", primaryClientId)
            .getResultList()
            .stream()
            .map(EmailDomainMappingMapper::toDomain)
            .toList();
    }

    @Override
    public List<EmailDomainMapping> listAll() {
        return em.createQuery("FROM EmailDomainMappingEntity ORDER BY emailDomain", EmailDomainMappingEntity.class)
            .getResultList()
            .stream()
            .map(EmailDomainMappingMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByEmailDomain(String emailDomain) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM EmailDomainMappingEntity e WHERE e.emailDomain = :emailDomain", Long.class)
            .setParameter("emailDomain", emailDomain.toLowerCase())
            .getSingleResult();
        return count > 0;
    }

    @Override
    public boolean isAnchorDomain(String emailDomain) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM EmailDomainMappingEntity e WHERE e.emailDomain = :emailDomain AND e.scopeType = :scopeType", Long.class)
            .setParameter("emailDomain", emailDomain.toLowerCase())
            .setParameter("scopeType", ScopeType.ANCHOR)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(EmailDomainMapping mapping) {
        writeRepo.persistMapping(mapping);
    }

    @Override
    public void update(EmailDomainMapping mapping) {
        writeRepo.updateMapping(mapping);
    }

    @Override
    public void delete(EmailDomainMapping mapping) {
        writeRepo.deleteMapping(mapping.id);
    }
}

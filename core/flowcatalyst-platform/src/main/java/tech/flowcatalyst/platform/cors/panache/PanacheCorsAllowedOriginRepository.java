package tech.flowcatalyst.platform.cors.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.cors.CorsAllowedOrigin;
import tech.flowcatalyst.platform.cors.CorsAllowedOriginRepository;
import tech.flowcatalyst.platform.cors.entity.CorsAllowedOriginEntity;
import tech.flowcatalyst.platform.cors.mapper.CorsAllowedOriginMapper;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Panache-based implementation of CorsAllowedOriginRepository.
 */
@ApplicationScoped
public class PanacheCorsAllowedOriginRepository implements CorsAllowedOriginRepository {

    @Inject
    EntityManager em;

    @Override
    public Optional<CorsAllowedOrigin> findById(String id) {
        CorsAllowedOriginEntity entity = em.find(CorsAllowedOriginEntity.class, id);
        return Optional.ofNullable(CorsAllowedOriginMapper.toDomain(entity));
    }

    @Override
    public Optional<CorsAllowedOrigin> findByOrigin(String origin) {
        var results = em.createQuery(
                "FROM CorsAllowedOriginEntity WHERE origin = :origin", CorsAllowedOriginEntity.class)
            .setParameter("origin", origin)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(CorsAllowedOriginMapper.toDomain(results.get(0)));
    }

    @Override
    public List<CorsAllowedOrigin> listAll() {
        return em.createQuery("FROM CorsAllowedOriginEntity ORDER BY origin", CorsAllowedOriginEntity.class)
            .getResultList()
            .stream()
            .map(CorsAllowedOriginMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByOrigin(String origin) {
        Long count = em.createQuery("SELECT COUNT(e) FROM CorsAllowedOriginEntity e WHERE e.origin = :origin", Long.class)
            .setParameter("origin", origin)
            .getSingleResult();
        return count > 0;
    }

    @Override
    public void persist(CorsAllowedOrigin corsOrigin) {
        if (corsOrigin.createdAt == null) {
            corsOrigin.createdAt = Instant.now();
        }
        CorsAllowedOriginEntity entity = CorsAllowedOriginMapper.toEntity(corsOrigin);
        em.persist(entity);
    }

    @Override
    public void delete(CorsAllowedOrigin corsOrigin) {
        CorsAllowedOriginEntity entity = em.find(CorsAllowedOriginEntity.class, corsOrigin.id);
        if (entity != null) {
            em.remove(entity);
        }
    }
}

package tech.flowcatalyst.platform.application.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.entity.ApplicationEntity;
import tech.flowcatalyst.platform.application.mapper.ApplicationMapper;

import java.util.Collection;
import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for Application entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class ApplicationReadRepository implements ApplicationRepository {

    @Inject
    EntityManager em;

    @Inject
    ApplicationWriteRepository writeRepo;

    @Override
    public Optional<Application> findByIdOptional(String id) {
        ApplicationEntity entity = em.find(ApplicationEntity.class, id);
        return Optional.ofNullable(entity).map(ApplicationMapper::toDomain);
    }

    @Override
    public Optional<Application> findByCode(String code) {
        var results = em.createQuery("FROM ApplicationEntity WHERE code = :code", ApplicationEntity.class)
            .setParameter("code", code)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(ApplicationMapper.toDomain(results.get(0)));
    }

    @Override
    public List<Application> findAllActive() {
        return em.createQuery("FROM ApplicationEntity WHERE active = true", ApplicationEntity.class)
            .getResultList()
            .stream()
            .map(ApplicationMapper::toDomain)
            .toList();
    }

    @Override
    public List<Application> findByType(Application.ApplicationType type, boolean activeOnly) {
        if (activeOnly) {
            return em.createQuery(
                    "FROM ApplicationEntity WHERE type = :type AND active = true", ApplicationEntity.class)
                .setParameter("type", type)
                .getResultList()
                .stream()
                .map(ApplicationMapper::toDomain)
                .toList();
        }
        return em.createQuery("FROM ApplicationEntity WHERE type = :type", ApplicationEntity.class)
            .setParameter("type", type)
            .getResultList()
            .stream()
            .map(ApplicationMapper::toDomain)
            .toList();
    }

    @Override
    public List<Application> findByCodes(Collection<String> codes) {
        if (codes == null || codes.isEmpty()) {
            return List.of();
        }
        return em.createQuery("FROM ApplicationEntity WHERE code IN :codes", ApplicationEntity.class)
            .setParameter("codes", codes)
            .getResultList()
            .stream()
            .map(ApplicationMapper::toDomain)
            .toList();
    }

    @Override
    public List<Application> findByIds(Collection<String> ids) {
        if (ids == null || ids.isEmpty()) {
            return List.of();
        }
        return em.createQuery("FROM ApplicationEntity WHERE id IN :ids", ApplicationEntity.class)
            .setParameter("ids", ids)
            .getResultList()
            .stream()
            .map(ApplicationMapper::toDomain)
            .toList();
    }

    @Override
    public List<Application> listAll() {
        return em.createQuery("FROM ApplicationEntity", ApplicationEntity.class)
            .getResultList()
            .stream()
            .map(ApplicationMapper::toDomain)
            .toList();
    }

    @Override
    public boolean existsByCode(String code) {
        Long count = em.createQuery("SELECT COUNT(e) FROM ApplicationEntity e WHERE e.code = :code", Long.class)
            .setParameter("code", code)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(Application application) {
        writeRepo.persistApplication(application);
    }

    @Override
    public void update(Application application) {
        writeRepo.updateApplication(application);
    }

    @Override
    public void delete(Application application) {
        writeRepo.deleteApplicationById(application.id);
    }
}

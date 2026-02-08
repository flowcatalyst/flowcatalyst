package tech.flowcatalyst.eventtype.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.EventTypeStatus;
import tech.flowcatalyst.eventtype.entity.EventTypeEntity;
import tech.flowcatalyst.eventtype.mapper.EventTypeMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for EventType entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class EventTypeReadRepository implements EventTypeRepository {

    @Inject
    EntityManager em;

    @Inject
    EventTypeWriteRepository writeRepo;

    @Override
    public EventType findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<EventType> findByIdOptional(String id) {
        EventTypeEntity entity = em.find(EventTypeEntity.class, id);
        return Optional.ofNullable(entity).map(EventTypeMapper::toDomain);
    }

    @Override
    public Optional<EventType> findByCode(String code) {
        var results = em.createQuery("FROM EventTypeEntity WHERE code = :code", EventTypeEntity.class)
            .setParameter("code", code)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(EventTypeMapper.toDomain(results.get(0)));
    }

    @Override
    public List<EventType> findAllOrdered() {
        return em.createQuery("FROM EventTypeEntity ORDER BY code", EventTypeEntity.class)
            .getResultList()
            .stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    @Override
    public List<EventType> findCurrent() {
        return em.createQuery("FROM EventTypeEntity WHERE status = :status", EventTypeEntity.class)
            .setParameter("status", EventTypeStatus.CURRENT)
            .getResultList()
            .stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    @Override
    public List<EventType> findArchived() {
        return em.createQuery("FROM EventTypeEntity WHERE status = :status", EventTypeEntity.class)
            .setParameter("status", EventTypeStatus.ARCHIVE)
            .getResultList()
            .stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    @Override
    public List<EventType> findByCodePrefix(String prefix) {
        return em.createQuery("FROM EventTypeEntity WHERE code LIKE :prefix", EventTypeEntity.class)
            .setParameter("prefix", prefix + "%")
            .getResultList()
            .stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    @Override
    public List<EventType> listAll() {
        return em.createQuery("FROM EventTypeEntity", EventTypeEntity.class)
            .getResultList()
            .stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM EventTypeEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public boolean existsByCode(String code) {
        Long count = em.createQuery("SELECT COUNT(e) FROM EventTypeEntity e WHERE e.code = :code", Long.class)
            .setParameter("code", code)
            .getSingleResult();
        return count > 0;
    }

    @Override
    public List<String> findDistinctApplications() {
        return em.createQuery(
                "SELECT DISTINCT e.application FROM EventTypeEntity e ORDER BY e.application", String.class)
            .getResultList();
    }

    @Override
    public List<String> findDistinctSubdomains(String application) {
        return em.createQuery(
                "SELECT DISTINCT e.subdomain FROM EventTypeEntity e WHERE e.application = :app ORDER BY e.subdomain", String.class)
            .setParameter("app", application)
            .getResultList();
    }

    @Override
    public List<String> findAllDistinctSubdomains() {
        return em.createQuery(
                "SELECT DISTINCT e.subdomain FROM EventTypeEntity e ORDER BY e.subdomain", String.class)
            .getResultList();
    }

    @Override
    public List<String> findDistinctSubdomains(List<String> applications) {
        if (applications == null || applications.isEmpty()) {
            return findAllDistinctSubdomains();
        }
        return em.createQuery(
                "SELECT DISTINCT e.subdomain FROM EventTypeEntity e WHERE e.application IN :apps ORDER BY e.subdomain", String.class)
            .setParameter("apps", applications)
            .getResultList();
    }

    @Override
    public List<String> findDistinctAggregates(String application, String subdomain) {
        return em.createQuery(
                "SELECT DISTINCT e.aggregate FROM EventTypeEntity e WHERE e.application = :app AND e.subdomain = :sub ORDER BY e.aggregate", String.class)
            .setParameter("app", application)
            .setParameter("sub", subdomain)
            .getResultList();
    }

    @Override
    public List<String> findAllDistinctAggregates() {
        return em.createQuery(
                "SELECT DISTINCT e.aggregate FROM EventTypeEntity e ORDER BY e.aggregate", String.class)
            .getResultList();
    }

    @Override
    public List<String> findDistinctAggregates(List<String> applications, List<String> subdomains) {
        var hql = new StringBuilder("SELECT DISTINCT e.aggregate FROM EventTypeEntity e WHERE 1=1");

        if (applications != null && !applications.isEmpty()) {
            hql.append(" AND e.application IN :apps");
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            hql.append(" AND e.subdomain IN :subdomains");
        }
        hql.append(" ORDER BY e.aggregate");

        var query = em.createQuery(hql.toString(), String.class);

        if (applications != null && !applications.isEmpty()) {
            query.setParameter("apps", applications);
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            query.setParameter("subdomains", subdomains);
        }

        return query.getResultList();
    }

    @Override
    public List<EventType> findWithFilters(
            List<String> applications,
            List<String> subdomains,
            List<String> aggregates,
            EventTypeStatus status) {

        var hql = new StringBuilder("FROM EventTypeEntity e WHERE 1=1");

        if (status != null) {
            hql.append(" AND e.status = :status");
        }
        if (applications != null && !applications.isEmpty()) {
            hql.append(" AND e.application IN :apps");
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            hql.append(" AND e.subdomain IN :subdomains");
        }
        if (aggregates != null && !aggregates.isEmpty()) {
            hql.append(" AND e.aggregate IN :aggregates");
        }
        hql.append(" ORDER BY e.code");

        var query = em.createQuery(hql.toString(), EventTypeEntity.class);

        if (status != null) {
            query.setParameter("status", status);
        }
        if (applications != null && !applications.isEmpty()) {
            query.setParameter("apps", applications);
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            query.setParameter("subdomains", subdomains);
        }
        if (aggregates != null && !aggregates.isEmpty()) {
            query.setParameter("aggregates", aggregates);
        }

        return query.getResultList().stream()
            .map(EventTypeMapper::toDomain)
            .toList();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(EventType eventType) {
        writeRepo.persistEventType(eventType);
    }

    @Override
    public void update(EventType eventType) {
        writeRepo.updateEventType(eventType);
    }

    @Override
    public void delete(EventType eventType) {
        writeRepo.deleteEventTypeById(eventType.id());
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteEventTypeById(id);
    }
}

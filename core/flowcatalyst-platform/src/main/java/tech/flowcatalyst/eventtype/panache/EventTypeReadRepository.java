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
                "SELECT DISTINCT SUBSTRING(e.code, 1, LOCATE(':', e.code) - 1) " +
                "FROM EventTypeEntity e " +
                "WHERE e.code LIKE '%:%'", String.class)
            .getResultList();
    }

    @Override
    public List<String> findDistinctSubdomains(String application) {
        @SuppressWarnings("unchecked")
        List<String> results = em.createNativeQuery(
                "SELECT DISTINCT split_part(code, ':', 2) " +
                "FROM event_types " +
                "WHERE split_part(code, ':', 1) = :app " +
                "AND code LIKE '%:%:%'")
            .setParameter("app", application)
            .getResultList();
        return results;
    }

    @Override
    public List<String> findAllDistinctSubdomains() {
        @SuppressWarnings("unchecked")
        List<String> results = em.createNativeQuery(
                "SELECT DISTINCT split_part(code, ':', 2) " +
                "FROM event_types " +
                "WHERE code LIKE '%:%:%'")
            .getResultList();
        return results;
    }

    @Override
    public List<String> findDistinctSubdomains(List<String> applications) {
        if (applications == null || applications.isEmpty()) {
            return findAllDistinctSubdomains();
        }
        @SuppressWarnings("unchecked")
        List<String> results = em.createNativeQuery(
                "SELECT DISTINCT split_part(code, ':', 2) " +
                "FROM event_types " +
                "WHERE split_part(code, ':', 1) IN :apps " +
                "AND code LIKE '%:%:%'")
            .setParameter("apps", applications)
            .getResultList();
        return results;
    }

    @Override
    public List<String> findDistinctAggregates(String application, String subdomain) {
        @SuppressWarnings("unchecked")
        List<String> results = em.createNativeQuery(
                "SELECT DISTINCT split_part(code, ':', 3) " +
                "FROM event_types " +
                "WHERE split_part(code, ':', 1) = :app " +
                "AND split_part(code, ':', 2) = :subdomain " +
                "AND code LIKE '%:%:%:%'")
            .setParameter("app", application)
            .setParameter("subdomain", subdomain)
            .getResultList();
        return results;
    }

    @Override
    public List<String> findAllDistinctAggregates() {
        @SuppressWarnings("unchecked")
        List<String> results = em.createNativeQuery(
                "SELECT DISTINCT split_part(code, ':', 3) " +
                "FROM event_types " +
                "WHERE code LIKE '%:%:%:%'")
            .getResultList();
        return results;
    }

    @Override
    public List<String> findDistinctAggregates(List<String> applications, List<String> subdomains) {
        StringBuilder sql = new StringBuilder(
            "SELECT DISTINCT split_part(code, ':', 3) FROM event_types WHERE code LIKE '%:%:%:%'");

        if (applications != null && !applications.isEmpty()) {
            sql.append(" AND split_part(code, ':', 1) IN :apps");
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            sql.append(" AND split_part(code, ':', 2) IN :subdomains");
        }

        var query = em.createNativeQuery(sql.toString());

        if (applications != null && !applications.isEmpty()) {
            query.setParameter("apps", applications);
        }
        if (subdomains != null && !subdomains.isEmpty()) {
            query.setParameter("subdomains", subdomains);
        }

        @SuppressWarnings("unchecked")
        List<String> results = query.getResultList();
        return results;
    }

    @Override
    public List<EventType> findWithFilters(
            List<String> applications,
            List<String> subdomains,
            List<String> aggregates,
            EventTypeStatus status) {

        // Build code filter conditions using native query for split_part
        if ((applications != null && !applications.isEmpty()) ||
            (subdomains != null && !subdomains.isEmpty()) ||
            (aggregates != null && !aggregates.isEmpty())) {

            StringBuilder sql = new StringBuilder("SELECT * FROM event_types WHERE 1=1");

            if (status != null) {
                sql.append(" AND status = :status");
            }
            if (applications != null && !applications.isEmpty()) {
                sql.append(" AND split_part(code, ':', 1) IN :apps");
            }
            if (subdomains != null && !subdomains.isEmpty()) {
                sql.append(" AND split_part(code, ':', 2) IN :subdomains");
            }
            if (aggregates != null && !aggregates.isEmpty()) {
                sql.append(" AND split_part(code, ':', 3) IN :aggregates");
            }

            var query = em.createNativeQuery(sql.toString(), EventTypeEntity.class);

            if (status != null) {
                query.setParameter("status", status.name());
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

            @SuppressWarnings("unchecked")
            List<EventTypeEntity> results = query.getResultList();
            return results.stream()
                .map(EventTypeMapper::toDomain)
                .toList();
        }

        // Simple case - only status filter or no filter
        if (status != null) {
            return em.createQuery("FROM EventTypeEntity WHERE status = :status", EventTypeEntity.class)
                .setParameter("status", status)
                .getResultList()
                .stream()
                .map(EventTypeMapper::toDomain)
                .toList();
        }

        return listAll();
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

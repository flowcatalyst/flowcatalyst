package tech.flowcatalyst.event.read.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.event.read.EventRead;
import tech.flowcatalyst.event.read.EventReadRepository;
import tech.flowcatalyst.event.read.jpaentity.EventReadContextDataEntity;
import tech.flowcatalyst.event.read.jpaentity.EventReadEntity;
import tech.flowcatalyst.event.read.mapper.EventReadMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for EventRead entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class EventReadReadRepository implements EventReadRepository {

    @Inject
    EntityManager em;

    @Inject
    EventReadWriteRepository writeRepo;

    @Override
    public EventRead findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<EventRead> findByIdOptional(String id) {
        EventReadEntity entity = em.find(EventReadEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        return Optional.of(toDomainWithRelations(entity));
    }

    @Override
    public List<EventRead> findWithFilter(EventFilter filter) {
        StringBuilder jpql = new StringBuilder("FROM EventReadEntity WHERE 1=1");

        // Handle clientIds with special "null" marker for IS NULL
        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            boolean hasNull = filter.clientIds().contains("null");
            List<String> nonNullIds = filter.clientIds().stream().filter(id -> !"null".equals(id)).toList();
            if (hasNull && nonNullIds.isEmpty()) {
                jpql.append(" AND clientId IS NULL");
            } else if (hasNull) {
                jpql.append(" AND (clientId IS NULL OR clientId IN :clientIds)");
            } else {
                jpql.append(" AND clientId IN :clientIds");
            }
        }
        if (filter.applications() != null && !filter.applications().isEmpty()) {
            jpql.append(" AND application IN :applications");
        }
        if (filter.subdomains() != null && !filter.subdomains().isEmpty()) {
            jpql.append(" AND subdomain IN :subdomains");
        }
        if (filter.aggregates() != null && !filter.aggregates().isEmpty()) {
            jpql.append(" AND aggregate IN :aggregates");
        }
        if (filter.types() != null && !filter.types().isEmpty()) {
            jpql.append(" AND type IN :types");
        }
        if (filter.source() != null) {
            jpql.append(" AND source = :source");
        }
        if (filter.subject() != null) {
            jpql.append(" AND subject = :subject");
        }
        if (filter.correlationId() != null) {
            jpql.append(" AND correlationId = :correlationId");
        }
        if (filter.messageGroup() != null) {
            jpql.append(" AND messageGroup = :messageGroup");
        }
        if (filter.timeAfter() != null) {
            jpql.append(" AND time >= :timeAfter");
        }
        if (filter.timeBefore() != null) {
            jpql.append(" AND time <= :timeBefore");
        }

        jpql.append(" ORDER BY time DESC");

        TypedQuery<EventReadEntity> query = em.createQuery(jpql.toString(), EventReadEntity.class);

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            List<String> nonNullIds = filter.clientIds().stream().filter(id -> !"null".equals(id)).toList();
            if (!nonNullIds.isEmpty()) {
                query.setParameter("clientIds", nonNullIds);
            }
        }
        if (filter.applications() != null && !filter.applications().isEmpty()) {
            query.setParameter("applications", filter.applications());
        }
        if (filter.subdomains() != null && !filter.subdomains().isEmpty()) {
            query.setParameter("subdomains", filter.subdomains());
        }
        if (filter.aggregates() != null && !filter.aggregates().isEmpty()) {
            query.setParameter("aggregates", filter.aggregates());
        }
        if (filter.types() != null && !filter.types().isEmpty()) {
            query.setParameter("types", filter.types());
        }
        if (filter.source() != null) {
            query.setParameter("source", filter.source());
        }
        if (filter.subject() != null) {
            query.setParameter("subject", filter.subject());
        }
        if (filter.correlationId() != null) {
            query.setParameter("correlationId", filter.correlationId());
        }
        if (filter.messageGroup() != null) {
            query.setParameter("messageGroup", filter.messageGroup());
        }
        if (filter.timeAfter() != null) {
            query.setParameter("timeAfter", filter.timeAfter());
        }
        if (filter.timeBefore() != null) {
            query.setParameter("timeBefore", filter.timeBefore());
        }

        query.setFirstResult(filter.page() * filter.size());
        query.setMaxResults(filter.size());

        return query.getResultList().stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<EventRead> listAll() {
        return em.createQuery("FROM EventReadEntity ORDER BY time DESC", EventReadEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM EventReadEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public long countWithFilter(EventFilter filter) {
        StringBuilder jpql = new StringBuilder("SELECT COUNT(e) FROM EventReadEntity e WHERE 1=1");

        // Handle clientIds with special "null" marker for IS NULL
        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            boolean hasNull = filter.clientIds().contains("null");
            List<String> nonNullIds = filter.clientIds().stream().filter(id -> !"null".equals(id)).toList();
            if (hasNull && nonNullIds.isEmpty()) {
                jpql.append(" AND e.clientId IS NULL");
            } else if (hasNull) {
                jpql.append(" AND (e.clientId IS NULL OR e.clientId IN :clientIds)");
            } else {
                jpql.append(" AND e.clientId IN :clientIds");
            }
        }
        if (filter.applications() != null && !filter.applications().isEmpty()) {
            jpql.append(" AND e.application IN :applications");
        }
        if (filter.subdomains() != null && !filter.subdomains().isEmpty()) {
            jpql.append(" AND e.subdomain IN :subdomains");
        }
        if (filter.aggregates() != null && !filter.aggregates().isEmpty()) {
            jpql.append(" AND e.aggregate IN :aggregates");
        }
        if (filter.types() != null && !filter.types().isEmpty()) {
            jpql.append(" AND e.type IN :types");
        }
        if (filter.source() != null) {
            jpql.append(" AND e.source = :source");
        }
        if (filter.subject() != null) {
            jpql.append(" AND e.subject = :subject");
        }
        if (filter.correlationId() != null) {
            jpql.append(" AND e.correlationId = :correlationId");
        }
        if (filter.messageGroup() != null) {
            jpql.append(" AND e.messageGroup = :messageGroup");
        }
        if (filter.timeAfter() != null) {
            jpql.append(" AND e.time >= :timeAfter");
        }
        if (filter.timeBefore() != null) {
            jpql.append(" AND e.time <= :timeBefore");
        }

        TypedQuery<Long> query = em.createQuery(jpql.toString(), Long.class);

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            List<String> nonNullIds = filter.clientIds().stream().filter(id -> !"null".equals(id)).toList();
            if (!nonNullIds.isEmpty()) {
                query.setParameter("clientIds", nonNullIds);
            }
        }
        if (filter.applications() != null && !filter.applications().isEmpty()) {
            query.setParameter("applications", filter.applications());
        }
        if (filter.subdomains() != null && !filter.subdomains().isEmpty()) {
            query.setParameter("subdomains", filter.subdomains());
        }
        if (filter.aggregates() != null && !filter.aggregates().isEmpty()) {
            query.setParameter("aggregates", filter.aggregates());
        }
        if (filter.types() != null && !filter.types().isEmpty()) {
            query.setParameter("types", filter.types());
        }
        if (filter.source() != null) {
            query.setParameter("source", filter.source());
        }
        if (filter.subject() != null) {
            query.setParameter("subject", filter.subject());
        }
        if (filter.correlationId() != null) {
            query.setParameter("correlationId", filter.correlationId());
        }
        if (filter.messageGroup() != null) {
            query.setParameter("messageGroup", filter.messageGroup());
        }
        if (filter.timeAfter() != null) {
            query.setParameter("timeAfter", filter.timeAfter());
        }
        if (filter.timeBefore() != null) {
            query.setParameter("timeBefore", filter.timeBefore());
        }

        return query.getSingleResult();
    }

    @Override
    public FilterOptions getFilterOptions(FilterOptionsRequest request) {
        // Parse clientIds for null handling
        boolean hasNullClient = request.clientIds() != null && request.clientIds().contains("null");
        List<String> nonNullClientIds = request.clientIds() != null
            ? request.clientIds().stream().filter(id -> !"null".equals(id)).toList()
            : null;

        // Get distinct clients
        List<String> clients = em.createQuery(
                "SELECT DISTINCT e.clientId FROM EventReadEntity e WHERE e.clientId IS NOT NULL ORDER BY e.clientId",
                String.class)
            .getResultList();

        // Get distinct applications (optionally filtered by clients)
        StringBuilder appQuery = new StringBuilder(
            "SELECT DISTINCT e.application FROM EventReadEntity e WHERE e.application IS NOT NULL");
        appendClientIdFilter(appQuery, "e", hasNullClient, nonNullClientIds);
        appQuery.append(" ORDER BY e.application");
        TypedQuery<String> appQ = em.createQuery(appQuery.toString(), String.class);
        setClientIdParameter(appQ, nonNullClientIds);
        List<String> applications = appQ.getResultList();

        // Get distinct subdomains (optionally filtered)
        StringBuilder subQuery = new StringBuilder(
            "SELECT DISTINCT e.subdomain FROM EventReadEntity e WHERE e.subdomain IS NOT NULL");
        appendClientIdFilter(subQuery, "e", hasNullClient, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            subQuery.append(" AND e.application IN :applications");
        }
        subQuery.append(" ORDER BY e.subdomain");
        TypedQuery<String> subQ = em.createQuery(subQuery.toString(), String.class);
        setClientIdParameter(subQ, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            subQ.setParameter("applications", request.applications());
        }
        List<String> subdomains = subQ.getResultList();

        // Get distinct aggregates (optionally filtered)
        StringBuilder aggQuery = new StringBuilder(
            "SELECT DISTINCT e.aggregate FROM EventReadEntity e WHERE e.aggregate IS NOT NULL");
        appendClientIdFilter(aggQuery, "e", hasNullClient, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            aggQuery.append(" AND e.application IN :applications");
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            aggQuery.append(" AND e.subdomain IN :subdomains");
        }
        aggQuery.append(" ORDER BY e.aggregate");
        TypedQuery<String> aggQ = em.createQuery(aggQuery.toString(), String.class);
        setClientIdParameter(aggQ, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            aggQ.setParameter("applications", request.applications());
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            aggQ.setParameter("subdomains", request.subdomains());
        }
        List<String> aggregates = aggQ.getResultList();

        // Get distinct types (optionally filtered)
        StringBuilder typeQuery = new StringBuilder(
            "SELECT DISTINCT e.type FROM EventReadEntity e WHERE e.type IS NOT NULL");
        appendClientIdFilter(typeQuery, "e", hasNullClient, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            typeQuery.append(" AND e.application IN :applications");
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            typeQuery.append(" AND e.subdomain IN :subdomains");
        }
        if (request.aggregates() != null && !request.aggregates().isEmpty()) {
            typeQuery.append(" AND e.aggregate IN :aggregates");
        }
        typeQuery.append(" ORDER BY e.type");
        TypedQuery<String> typeQ = em.createQuery(typeQuery.toString(), String.class);
        setClientIdParameter(typeQ, nonNullClientIds);
        if (request.applications() != null && !request.applications().isEmpty()) {
            typeQ.setParameter("applications", request.applications());
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            typeQ.setParameter("subdomains", request.subdomains());
        }
        if (request.aggregates() != null && !request.aggregates().isEmpty()) {
            typeQ.setParameter("aggregates", request.aggregates());
        }
        List<String> types = typeQ.getResultList();

        return new FilterOptions(clients, applications, subdomains, aggregates, types);
    }

    /**
     * Append client ID filter clause to query, handling "null" marker for IS NULL.
     */
    private void appendClientIdFilter(StringBuilder query, String alias, boolean hasNull, List<String> nonNullIds) {
        if (hasNull || (nonNullIds != null && !nonNullIds.isEmpty())) {
            if (hasNull && (nonNullIds == null || nonNullIds.isEmpty())) {
                query.append(" AND ").append(alias).append(".clientId IS NULL");
            } else if (hasNull) {
                query.append(" AND (").append(alias).append(".clientId IS NULL OR ").append(alias).append(".clientId IN :clientIds)");
            } else {
                query.append(" AND ").append(alias).append(".clientId IN :clientIds");
            }
        }
    }

    /**
     * Set clientIds parameter if there are non-null IDs to bind.
     */
    private void setClientIdParameter(TypedQuery<?> query, List<String> nonNullIds) {
        if (nonNullIds != null && !nonNullIds.isEmpty()) {
            query.setParameter("clientIds", nonNullIds);
        }
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(EventRead event) {
        writeRepo.persistEventRead(event);
    }

    @Override
    public void update(EventRead event) {
        writeRepo.updateEventRead(event);
    }

    @Override
    public void delete(EventRead event) {
        writeRepo.deleteEventRead(event.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteEventRead(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private EventRead toDomainWithRelations(EventReadEntity entity) {
        EventRead base = EventReadMapper.toDomain(entity);

        // Load context data
        List<EventReadContextDataEntity> contextDataEntities = em.createQuery(
                "FROM EventReadContextDataEntity WHERE eventReadId = :id", EventReadContextDataEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        base.contextData = EventReadMapper.toContextDataList(contextDataEntities);

        return base;
    }
}

package tech.flowcatalyst.dispatchjob.read.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.dispatchjob.read.DispatchJobRead;
import tech.flowcatalyst.dispatchjob.read.DispatchJobReadRepository;
import tech.flowcatalyst.dispatchjob.read.jpaentity.DispatchJobReadEntity;
import tech.flowcatalyst.dispatchjob.read.mapper.DispatchJobReadMapper;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for DispatchJobRead entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class DispatchJobReadReadRepository implements DispatchJobReadRepository {

    @Inject
    EntityManager em;

    @Inject
    DispatchJobReadWriteRepository writeRepo;

    @Override
    public DispatchJobRead findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<DispatchJobRead> findByIdOptional(String id) {
        DispatchJobReadEntity entity = em.find(DispatchJobReadEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        return Optional.of(DispatchJobReadMapper.toDomain(entity));
    }

    @Override
    public List<DispatchJobRead> findWithFilter(DispatchJobReadFilter filter) {
        StringBuilder jpql = new StringBuilder("FROM DispatchJobReadEntity WHERE 1=1");

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            jpql.append(" AND clientId IN :clientIds");
        }
        if (filter.statuses() != null && !filter.statuses().isEmpty()) {
            jpql.append(" AND status IN :statuses");
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
        if (filter.codes() != null && !filter.codes().isEmpty()) {
            jpql.append(" AND code IN :codes");
        }
        if (filter.source() != null) {
            jpql.append(" AND source = :source");
        }
        if (filter.kind() != null) {
            jpql.append(" AND kind = :kind");
        }
        if (filter.subscriptionId() != null) {
            jpql.append(" AND subscriptionId = :subscriptionId");
        }
        if (filter.dispatchPoolId() != null) {
            jpql.append(" AND dispatchPoolId = :dispatchPoolId");
        }
        if (filter.messageGroup() != null) {
            jpql.append(" AND messageGroup = :messageGroup");
        }
        if (filter.createdAfter() != null) {
            jpql.append(" AND createdAt >= :createdAfter");
        }
        if (filter.createdBefore() != null) {
            jpql.append(" AND createdAt <= :createdBefore");
        }

        jpql.append(" ORDER BY createdAt DESC");

        TypedQuery<DispatchJobReadEntity> query = em.createQuery(jpql.toString(), DispatchJobReadEntity.class);

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            query.setParameter("clientIds", filter.clientIds());
        }
        if (filter.statuses() != null && !filter.statuses().isEmpty()) {
            query.setParameter("statuses", filter.statuses());
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
        if (filter.codes() != null && !filter.codes().isEmpty()) {
            query.setParameter("codes", filter.codes());
        }
        if (filter.source() != null) {
            query.setParameter("source", filter.source());
        }
        if (filter.kind() != null) {
            query.setParameter("kind", filter.kind());
        }
        if (filter.subscriptionId() != null) {
            query.setParameter("subscriptionId", filter.subscriptionId());
        }
        if (filter.dispatchPoolId() != null) {
            query.setParameter("dispatchPoolId", filter.dispatchPoolId());
        }
        if (filter.messageGroup() != null) {
            query.setParameter("messageGroup", filter.messageGroup());
        }
        if (filter.createdAfter() != null) {
            query.setParameter("createdAfter", filter.createdAfter());
        }
        if (filter.createdBefore() != null) {
            query.setParameter("createdBefore", filter.createdBefore());
        }

        query.setFirstResult(filter.page() * filter.size());
        query.setMaxResults(filter.size());

        return query.getResultList().stream()
            .map(DispatchJobReadMapper::toDomain)
            .toList();
    }

    @Override
    public List<DispatchJobRead> listAll() {
        return em.createQuery("FROM DispatchJobReadEntity ORDER BY createdAt DESC", DispatchJobReadEntity.class)
            .getResultList()
            .stream()
            .map(DispatchJobReadMapper::toDomain)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM DispatchJobReadEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public long countWithFilter(DispatchJobReadFilter filter) {
        StringBuilder jpql = new StringBuilder("SELECT COUNT(e) FROM DispatchJobReadEntity e WHERE 1=1");

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            jpql.append(" AND e.clientId IN :clientIds");
        }
        if (filter.statuses() != null && !filter.statuses().isEmpty()) {
            jpql.append(" AND e.status IN :statuses");
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
        if (filter.codes() != null && !filter.codes().isEmpty()) {
            jpql.append(" AND e.code IN :codes");
        }
        if (filter.source() != null) {
            jpql.append(" AND e.source = :source");
        }
        if (filter.kind() != null) {
            jpql.append(" AND e.kind = :kind");
        }
        if (filter.subscriptionId() != null) {
            jpql.append(" AND e.subscriptionId = :subscriptionId");
        }
        if (filter.dispatchPoolId() != null) {
            jpql.append(" AND e.dispatchPoolId = :dispatchPoolId");
        }
        if (filter.messageGroup() != null) {
            jpql.append(" AND e.messageGroup = :messageGroup");
        }
        if (filter.createdAfter() != null) {
            jpql.append(" AND e.createdAt >= :createdAfter");
        }
        if (filter.createdBefore() != null) {
            jpql.append(" AND e.createdAt <= :createdBefore");
        }

        TypedQuery<Long> query = em.createQuery(jpql.toString(), Long.class);

        if (filter.clientIds() != null && !filter.clientIds().isEmpty()) {
            query.setParameter("clientIds", filter.clientIds());
        }
        if (filter.statuses() != null && !filter.statuses().isEmpty()) {
            query.setParameter("statuses", filter.statuses());
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
        if (filter.codes() != null && !filter.codes().isEmpty()) {
            query.setParameter("codes", filter.codes());
        }
        if (filter.source() != null) {
            query.setParameter("source", filter.source());
        }
        if (filter.kind() != null) {
            query.setParameter("kind", filter.kind());
        }
        if (filter.subscriptionId() != null) {
            query.setParameter("subscriptionId", filter.subscriptionId());
        }
        if (filter.dispatchPoolId() != null) {
            query.setParameter("dispatchPoolId", filter.dispatchPoolId());
        }
        if (filter.messageGroup() != null) {
            query.setParameter("messageGroup", filter.messageGroup());
        }
        if (filter.createdAfter() != null) {
            query.setParameter("createdAfter", filter.createdAfter());
        }
        if (filter.createdBefore() != null) {
            query.setParameter("createdBefore", filter.createdBefore());
        }

        return query.getSingleResult();
    }

    @Override
    public FilterOptions getFilterOptions(FilterOptionsRequest request) {
        // Get distinct clients
        List<String> clients = em.createQuery(
                "SELECT DISTINCT e.clientId FROM DispatchJobReadEntity e WHERE e.clientId IS NOT NULL ORDER BY e.clientId",
                String.class)
            .getResultList();

        // Get distinct applications (optionally filtered by clients)
        StringBuilder appQuery = new StringBuilder(
            "SELECT DISTINCT e.application FROM DispatchJobReadEntity e WHERE e.application IS NOT NULL");
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            appQuery.append(" AND e.clientId IN :clientIds");
        }
        appQuery.append(" ORDER BY e.application");
        TypedQuery<String> appQ = em.createQuery(appQuery.toString(), String.class);
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            appQ.setParameter("clientIds", request.clientIds());
        }
        List<String> applications = appQ.getResultList();

        // Get distinct subdomains (optionally filtered)
        StringBuilder subQuery = new StringBuilder(
            "SELECT DISTINCT e.subdomain FROM DispatchJobReadEntity e WHERE e.subdomain IS NOT NULL");
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            subQuery.append(" AND e.clientId IN :clientIds");
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            subQuery.append(" AND e.application IN :applications");
        }
        subQuery.append(" ORDER BY e.subdomain");
        TypedQuery<String> subQ = em.createQuery(subQuery.toString(), String.class);
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            subQ.setParameter("clientIds", request.clientIds());
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            subQ.setParameter("applications", request.applications());
        }
        List<String> subdomains = subQ.getResultList();

        // Get distinct aggregates (optionally filtered)
        StringBuilder aggQuery = new StringBuilder(
            "SELECT DISTINCT e.aggregate FROM DispatchJobReadEntity e WHERE e.aggregate IS NOT NULL");
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            aggQuery.append(" AND e.clientId IN :clientIds");
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            aggQuery.append(" AND e.application IN :applications");
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            aggQuery.append(" AND e.subdomain IN :subdomains");
        }
        aggQuery.append(" ORDER BY e.aggregate");
        TypedQuery<String> aggQ = em.createQuery(aggQuery.toString(), String.class);
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            aggQ.setParameter("clientIds", request.clientIds());
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            aggQ.setParameter("applications", request.applications());
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            aggQ.setParameter("subdomains", request.subdomains());
        }
        List<String> aggregates = aggQ.getResultList();

        // Get distinct codes (optionally filtered)
        StringBuilder codeQuery = new StringBuilder(
            "SELECT DISTINCT e.code FROM DispatchJobReadEntity e WHERE e.code IS NOT NULL");
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            codeQuery.append(" AND e.clientId IN :clientIds");
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            codeQuery.append(" AND e.application IN :applications");
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            codeQuery.append(" AND e.subdomain IN :subdomains");
        }
        if (request.aggregates() != null && !request.aggregates().isEmpty()) {
            codeQuery.append(" AND e.aggregate IN :aggregates");
        }
        codeQuery.append(" ORDER BY e.code");
        TypedQuery<String> codeQ = em.createQuery(codeQuery.toString(), String.class);
        if (request.clientIds() != null && !request.clientIds().isEmpty()) {
            codeQ.setParameter("clientIds", request.clientIds());
        }
        if (request.applications() != null && !request.applications().isEmpty()) {
            codeQ.setParameter("applications", request.applications());
        }
        if (request.subdomains() != null && !request.subdomains().isEmpty()) {
            codeQ.setParameter("subdomains", request.subdomains());
        }
        if (request.aggregates() != null && !request.aggregates().isEmpty()) {
            codeQ.setParameter("aggregates", request.aggregates());
        }
        List<String> codes = codeQ.getResultList();

        // Get distinct statuses
        List<String> statuses = em.createQuery(
                "SELECT DISTINCT e.status FROM DispatchJobReadEntity e WHERE e.status IS NOT NULL ORDER BY e.status",
                String.class)
            .getResultList();

        return new FilterOptions(clients, applications, subdomains, aggregates, codes, statuses);
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(DispatchJobRead job) {
        writeRepo.persistDispatchJobRead(job);
    }

    @Override
    public void update(DispatchJobRead job) {
        writeRepo.updateDispatchJobRead(job);
    }

    @Override
    public void delete(DispatchJobRead job) {
        writeRepo.deleteDispatchJobRead(job.id);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteDispatchJobRead(id);
    }
}

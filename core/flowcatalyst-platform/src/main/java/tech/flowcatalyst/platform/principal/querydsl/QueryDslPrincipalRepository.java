package tech.flowcatalyst.platform.principal.querydsl;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import jakarta.persistence.criteria.CriteriaBuilder;
import jakarta.persistence.criteria.CriteriaQuery;
import jakarta.persistence.criteria.Predicate;
import jakarta.persistence.criteria.Root;
import tech.flowcatalyst.platform.common.OffsetPage;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.principal.entity.PrincipalEntity;
import tech.flowcatalyst.platform.principal.mapper.PrincipalMapper;

import java.util.ArrayList;
import java.util.List;

/**
 * QueryDSL-style repository for Principal with dynamic filtering.
 *
 * <p>Uses JPA Criteria API for type-safe dynamic queries.
 * This provides the same capability as QueryDSL BooleanBuilder patterns
 * without requiring compile-time Q-class generation.
 *
 * <p>Once QueryDSL annotation processor is configured, this can be refactored
 * to use QPrincipalEntity for even better type safety.
 */
@ApplicationScoped
public class QueryDslPrincipalRepository {

    @Inject
    EntityManager em;

    /**
     * Filter criteria for principal queries.
     */
    public record PrincipalFilter(
        String clientId,
        PrincipalType type,
        UserScope scope,
        Boolean active,
        String emailDomain,
        String searchTerm,
        Integer offset,
        Integer limit
    ) {
        public static PrincipalFilterBuilder builder() {
            return new PrincipalFilterBuilder();
        }

        public static class PrincipalFilterBuilder {
            private String clientId;
            private PrincipalType type;
            private UserScope scope;
            private Boolean active;
            private String emailDomain;
            private String searchTerm;
            private Integer offset;
            private Integer limit;

            public PrincipalFilterBuilder clientId(String clientId) {
                this.clientId = clientId;
                return this;
            }

            public PrincipalFilterBuilder type(PrincipalType type) {
                this.type = type;
                return this;
            }

            public PrincipalFilterBuilder scope(UserScope scope) {
                this.scope = scope;
                return this;
            }

            public PrincipalFilterBuilder active(Boolean active) {
                this.active = active;
                return this;
            }

            public PrincipalFilterBuilder emailDomain(String emailDomain) {
                this.emailDomain = emailDomain;
                return this;
            }

            public PrincipalFilterBuilder searchTerm(String searchTerm) {
                this.searchTerm = searchTerm;
                return this;
            }

            public PrincipalFilterBuilder offset(Integer offset) {
                this.offset = offset;
                return this;
            }

            public PrincipalFilterBuilder limit(Integer limit) {
                this.limit = limit;
                return this;
            }

            public PrincipalFilter build() {
                return new PrincipalFilter(clientId, type, scope, active, emailDomain, searchTerm, offset, limit);
            }
        }
    }

    /**
     * Find principals matching dynamic filter criteria.
     *
     * <p>Example usage:
     * <pre>{@code
     * var filter = PrincipalFilter.builder()
     *     .clientId("clt_123")
     *     .type(PrincipalType.USER)
     *     .active(true)
     *     .limit(20)
     *     .build();
     *
     * List<Principal> results = repo.findByFilter(filter);
     * }</pre>
     */
    public List<Principal> findByFilter(PrincipalFilter filter) {
        CriteriaBuilder cb = em.getCriteriaBuilder();
        CriteriaQuery<PrincipalEntity> query = cb.createQuery(PrincipalEntity.class);
        Root<PrincipalEntity> root = query.from(PrincipalEntity.class);

        List<Predicate> predicates = buildPredicates(cb, root, filter);

        query.select(root).where(predicates.toArray(new Predicate[0]));
        query.orderBy(cb.desc(root.get("createdAt")));

        TypedQuery<PrincipalEntity> typedQuery = em.createQuery(query);

        if (filter.offset() != null) {
            typedQuery.setFirstResult(filter.offset());
        }
        if (filter.limit() != null) {
            typedQuery.setMaxResults(filter.limit());
        }

        return typedQuery.getResultList()
            .stream()
            .map(PrincipalMapper::toDomain)
            .toList();
    }

    /**
     * Find principals with pagination.
     */
    public OffsetPage<Principal> findByFilterPaged(PrincipalFilter filter) {
        // Count total
        long total = countByFilter(filter);

        // Fetch results
        List<Principal> results = findByFilter(filter);

        int offset = filter.offset() != null ? filter.offset() : 0;
        int limit = filter.limit() != null ? filter.limit() : 20;

        return new OffsetPage<>(results, total, offset, limit);
    }

    /**
     * Count principals matching filter criteria.
     */
    public long countByFilter(PrincipalFilter filter) {
        CriteriaBuilder cb = em.getCriteriaBuilder();
        CriteriaQuery<Long> query = cb.createQuery(Long.class);
        Root<PrincipalEntity> root = query.from(PrincipalEntity.class);

        List<Predicate> predicates = buildPredicates(cb, root, filter);

        query.select(cb.count(root)).where(predicates.toArray(new Predicate[0]));

        return em.createQuery(query).getSingleResult();
    }

    /**
     * Find principals by role name using the normalized principal_roles table.
     */
    public List<Principal> findByRoleName(String roleName) {
        List<PrincipalEntity> results = em.createQuery("""
            SELECT DISTINCT p FROM PrincipalEntity p
            JOIN PrincipalRoleEntity r ON r.principalId = p.id
            WHERE r.roleName = :roleName
            """, PrincipalEntity.class)
            .setParameter("roleName", roleName)
            .getResultList();

        return results.stream()
            .map(PrincipalMapper::toDomain)
            .toList();
    }

    /**
     * Find principals by client ID and role name.
     */
    public List<Principal> findByClientIdAndRoleName(String clientId, String roleName) {
        List<PrincipalEntity> results = em.createQuery("""
            SELECT DISTINCT p FROM PrincipalEntity p
            JOIN PrincipalRoleEntity r ON r.principalId = p.id
            WHERE p.clientId = :clientId AND r.roleName = :roleName
            """, PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("roleName", roleName)
            .getResultList();

        return results.stream()
            .map(PrincipalMapper::toDomain)
            .toList();
    }

    // ========================================================================
    // Private Helpers
    // ========================================================================

    private List<Predicate> buildPredicates(CriteriaBuilder cb, Root<PrincipalEntity> root, PrincipalFilter filter) {
        List<Predicate> predicates = new ArrayList<>();

        if (filter.clientId() != null) {
            predicates.add(cb.equal(root.get("clientId"), filter.clientId()));
        }

        if (filter.type() != null) {
            predicates.add(cb.equal(root.get("type"), filter.type()));
        }

        if (filter.scope() != null) {
            predicates.add(cb.equal(root.get("scope"), filter.scope()));
        }

        if (filter.active() != null) {
            predicates.add(cb.equal(root.get("active"), filter.active()));
        }

        if (filter.emailDomain() != null) {
            predicates.add(cb.equal(root.get("emailDomain"), filter.emailDomain()));
        }

        if (filter.searchTerm() != null && !filter.searchTerm().isBlank()) {
            String searchPattern = "%" + filter.searchTerm().toLowerCase() + "%";
            Predicate nameLike = cb.like(cb.lower(root.get("name")), searchPattern);
            Predicate emailLike = cb.like(cb.lower(root.get("email")), searchPattern);
            predicates.add(cb.or(nameLike, emailLike));
        }

        return predicates;
    }
}

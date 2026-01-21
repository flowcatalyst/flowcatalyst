package tech.flowcatalyst.event.read;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Repository interface for EventRead entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 *
 * This collection is optimized for read operations with rich indexes.
 */
public interface EventReadRepository {

    // Read operations
    EventRead findById(String id);
    Optional<EventRead> findByIdOptional(String id);
    List<EventRead> findWithFilter(EventFilter filter);
    List<EventRead> listAll();
    long count();
    long countWithFilter(EventFilter filter);
    FilterOptions getFilterOptions(FilterOptionsRequest request);

    // Write operations
    void persist(EventRead event);
    void update(EventRead event);
    void delete(EventRead event);
    boolean deleteById(String id);

    /**
     * Request for filter options.
     */
    record FilterOptionsRequest(
        List<String> clientIds,
        List<String> applications,
        List<String> subdomains,
        List<String> aggregates
    ) {}

    /**
     * Available options for each filter level.
     */
    record FilterOptions(
        List<String> clients,
        List<String> applications,
        List<String> subdomains,
        List<String> aggregates,
        List<String> types
    ) {}

    /**
     * Filter criteria for event queries.
     */
    record EventFilter(
        List<String> clientIds,
        List<String> applications,
        List<String> subdomains,
        List<String> aggregates,
        List<String> types,
        String source,
        String subject,
        String correlationId,
        String messageGroup,
        Instant timeAfter,
        Instant timeBefore,
        Integer page,
        Integer size
    ) {
        public EventFilter {
            if (page == null || page < 0) {
                page = 0;
            }
            if (size == null || size < 1 || size > 100) {
                size = 20;
            }
        }

        public static EventFilter of(
            List<String> clientIds,
            List<String> applications,
            List<String> subdomains,
            List<String> aggregates,
            List<String> types,
            String source,
            String subject,
            String correlationId,
            String messageGroup,
            Instant timeAfter,
            Instant timeBefore,
            Integer page,
            Integer size
        ) {
            return new EventFilter(
                clientIds, applications, subdomains, aggregates, types,
                source, subject, correlationId, messageGroup,
                timeAfter, timeBefore, page, size
            );
        }
    }
}

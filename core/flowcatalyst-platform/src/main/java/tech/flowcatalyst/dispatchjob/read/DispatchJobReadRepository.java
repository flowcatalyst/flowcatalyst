package tech.flowcatalyst.dispatchjob.read;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Repository interface for DispatchJobRead entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 *
 * This collection is optimized for read operations with rich indexes.
 */
public interface DispatchJobReadRepository {

    // Read operations
    DispatchJobRead findById(String id);
    Optional<DispatchJobRead> findByIdOptional(String id);
    List<DispatchJobRead> findWithFilter(DispatchJobReadFilter filter);
    List<DispatchJobRead> listAll();
    long count();
    long countWithFilter(DispatchJobReadFilter filter);
    FilterOptions getFilterOptions(FilterOptionsRequest request);

    // Write operations
    void persist(DispatchJobRead job);
    void update(DispatchJobRead job);
    void delete(DispatchJobRead job);
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
        List<String> codes,
        List<String> statuses
    ) {}

    /**
     * Filter criteria for dispatch job queries.
     */
    record DispatchJobReadFilter(
        List<String> clientIds,
        List<String> statuses,
        List<String> applications,
        List<String> subdomains,
        List<String> aggregates,
        List<String> codes,
        String source,
        String kind,
        String subscriptionId,
        String dispatchPoolId,
        String messageGroup,
        Instant createdAfter,
        Instant createdBefore,
        Integer page,
        Integer size
    ) {
        public DispatchJobReadFilter {
            if (page == null || page < 0) {
                page = 0;
            }
            if (size == null || size < 1 || size > 100) {
                size = 20;
            }
        }
    }
}

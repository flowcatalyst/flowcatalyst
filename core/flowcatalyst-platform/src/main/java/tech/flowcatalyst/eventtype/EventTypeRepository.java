package tech.flowcatalyst.eventtype;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for EventType entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 *
 * Event type codes follow the format: application:subdomain:aggregate:action
 */
public interface EventTypeRepository {

    // Read operations
    EventType findById(String id);
    Optional<EventType> findByIdOptional(String id);
    Optional<EventType> findByCode(String code);
    List<EventType> findAllOrdered();
    List<EventType> findCurrent();
    List<EventType> findArchived();
    List<EventType> findByCodePrefix(String prefix);
    List<EventType> listAll();
    long count();
    boolean existsByCode(String code);

    // Aggregation queries for code segments
    List<String> findDistinctApplications();
    List<String> findDistinctSubdomains(String application);
    List<String> findAllDistinctSubdomains();
    List<String> findDistinctSubdomains(List<String> applications);
    List<String> findDistinctAggregates(String application, String subdomain);
    List<String> findAllDistinctAggregates();
    List<String> findDistinctAggregates(List<String> applications, List<String> subdomains);

    // Filtered queries
    List<EventType> findWithFilters(
        List<String> applications,
        List<String> subdomains,
        List<String> aggregates,
        EventTypeStatus status
    );

    // Write operations
    void persist(EventType eventType);
    void update(EventType eventType);
    void delete(EventType eventType);
    boolean deleteById(String id);
}

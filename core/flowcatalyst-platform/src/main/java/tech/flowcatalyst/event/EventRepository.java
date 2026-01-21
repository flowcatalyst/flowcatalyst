package tech.flowcatalyst.event;

import tech.flowcatalyst.platform.common.Page;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for Event entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 *
 * Indexes are created by MongoIndexInitializer on startup.
 */
public interface EventRepository {

    // Read operations
    Event findById(String id);
    Optional<Event> findByIdOptional(String id);
    Optional<Event> findByDeduplicationId(String deduplicationId);
    List<Event> listAll();
    List<Event> findRecentPaged(int page, int size);

    /**
     * Count all events.
     *
     * @deprecated Use cursor-based pagination with {@link #findPage(String, int)} instead.
     *             COUNT(*) on large tables can be slow and resource-intensive.
     */
    @Deprecated(since = "1.0", forRemoval = false)
    long count();

    boolean existsByDeduplicationId(String deduplicationId);

    /**
     * Find events using cursor-based pagination.
     * More efficient than offset pagination for large datasets.
     *
     * @param afterCursor The ID of the last event from the previous page, or null for the first page
     * @param limit Maximum number of events to return
     * @return A Page containing the events and cursor for the next page
     */
    default Page<Event> findPage(String afterCursor, int limit) {
        // Default implementation for backwards compatibility
        throw new UnsupportedOperationException("Cursor-based pagination not implemented");
    }

    // Write operations
    void insert(Event event);
    void persist(Event event);
    void persistAll(List<Event> events);
    void update(Event event);
    void delete(Event event);
    boolean deleteById(String id);
}

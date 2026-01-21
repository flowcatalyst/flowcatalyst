package tech.flowcatalyst.platform.audit;

import tech.flowcatalyst.platform.common.Page;

import java.util.List;

/**
 * Repository interface for AuditLog entities.
 * Provides audit log access methods for compliance and debugging.
 */
public interface AuditLogRepository {

    // Read operations
    AuditLog findById(String id);
    List<AuditLog> findByEntity(String entityType, String entityId);
    List<AuditLog> findByPrincipal(String principalId);
    List<AuditLog> findByOperation(String operation);
    List<AuditLog> findPaged(int page, int pageSize);
    List<AuditLog> findByEntityTypePaged(String entityType, int page, int pageSize);

    /**
     * Count all audit logs.
     *
     * @deprecated Use cursor-based pagination with {@link #findPage(String, int)} instead.
     *             COUNT(*) on large tables can be slow and resource-intensive.
     */
    @Deprecated(since = "1.0", forRemoval = false)
    long count();

    /**
     * Count audit logs by entity type.
     *
     * @deprecated Consider using cursor-based pagination instead.
     */
    @Deprecated(since = "1.0", forRemoval = false)
    long countByEntityType(String entityType);

    /**
     * Find audit logs using cursor-based pagination.
     * More efficient than offset pagination for large datasets.
     *
     * @param afterCursor The ID of the last log from the previous page, or null for the first page
     * @param limit Maximum number of logs to return
     * @return A Page containing the logs and cursor for the next page
     */
    default Page<AuditLog> findPage(String afterCursor, int limit) {
        throw new UnsupportedOperationException("Cursor-based pagination not implemented");
    }

    // Aggregation operations
    List<String> findDistinctEntityTypes();
    List<String> findDistinctOperations();

    // Write operations
    void persist(AuditLog log);
}

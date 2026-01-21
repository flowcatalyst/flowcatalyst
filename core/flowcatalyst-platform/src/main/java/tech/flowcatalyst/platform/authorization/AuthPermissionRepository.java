package tech.flowcatalyst.platform.authorization;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for AuthPermission entities.
 * Used for SDK-registered permissions.
 */
public interface AuthPermissionRepository {

    // Read operations
    Optional<AuthPermission> findByName(String name);
    List<AuthPermission> findByApplicationId(String applicationId);
    List<AuthPermission> listAll();
    boolean existsByName(String name);

    // Write operations
    void persist(AuthPermission permission);
    void update(AuthPermission permission);
    void delete(AuthPermission permission);
    long deleteByApplicationId(String applicationId);
}

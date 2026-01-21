package tech.flowcatalyst.platform.authorization;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for AuthRole entities.
 */
public interface AuthRoleRepository {

    // Read operations
    Optional<AuthRole> findByName(String name);
    List<AuthRole> findByApplicationCode(String applicationCode);
    List<AuthRole> findBySource(AuthRole.RoleSource source);
    List<AuthRole> listAll();
    boolean existsByName(String name);

    // Write operations
    void persist(AuthRole role);
    void update(AuthRole role);
    void delete(AuthRole role);
}

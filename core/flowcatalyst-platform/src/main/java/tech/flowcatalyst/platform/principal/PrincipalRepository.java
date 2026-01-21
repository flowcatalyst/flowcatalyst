package tech.flowcatalyst.platform.principal;

import java.util.Collection;
import java.util.List;
import java.util.Optional;

/**
 * Repository interface for Principal entities.
 */
public interface PrincipalRepository {

    // Read operations - single entity
    Principal findById(String id);
    Optional<Principal> findByIdOptional(String id);
    Optional<Principal> findByEmail(String email);
    Optional<Principal> findByServiceAccountCode(String code);

    // Read operations - lists
    List<Principal> findByType(PrincipalType type);
    List<Principal> findByClientId(String clientId);
    List<Principal> findByIds(Collection<String> ids);
    List<Principal> findUsersByClientId(String clientId);
    List<Principal> findActiveUsersByClientId(String clientId);
    List<Principal> findByClientIdAndTypeAndActive(String clientId, PrincipalType type, Boolean active);
    List<Principal> findByClientIdAndType(String clientId, PrincipalType type);
    List<Principal> findByClientIdAndActive(String clientId, Boolean active);
    List<Principal> findByActive(Boolean active);
    List<Principal> listAll();
    Optional<Principal> findByServiceAccountClientId(String clientId);

    // Count operations
    long countByEmailDomain(String domain);

    // Write operations
    void persist(Principal principal);
    void update(Principal principal);
    void updateOnly(Principal principal);  // Update without touching roles
    boolean deleteById(String id);
}

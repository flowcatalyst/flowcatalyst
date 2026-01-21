package tech.flowcatalyst.platform.client;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for ClientAccessGrant entities.
 */
public interface ClientAccessGrantRepository {

    // Read operations
    List<ClientAccessGrant> findByPrincipalId(String principalId);
    List<ClientAccessGrant> findByClientId(String clientId);
    Optional<ClientAccessGrant> findByPrincipalIdAndClientId(String principalId, String clientId);
    boolean existsByPrincipalIdAndClientId(String principalId, String clientId);

    // Write operations
    void persist(ClientAccessGrant grant);
    void delete(ClientAccessGrant grant);
    void deleteByPrincipalId(String principalId);
    long deleteByPrincipalIdAndClientId(String principalId, String clientId);
}

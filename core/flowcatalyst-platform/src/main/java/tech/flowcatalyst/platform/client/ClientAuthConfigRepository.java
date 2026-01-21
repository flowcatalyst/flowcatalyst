package tech.flowcatalyst.platform.client;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for ClientAuthConfig entities.
 * Used to look up authentication configuration by email domain.
 */
public interface ClientAuthConfigRepository {

    // Read operations
    Optional<ClientAuthConfig> findByIdOptional(String id);
    Optional<ClientAuthConfig> findByEmailDomain(String emailDomain);
    List<ClientAuthConfig> findByClientId(String clientId);
    List<ClientAuthConfig> findByConfigType(AuthConfigType configType);
    List<ClientAuthConfig> listAll();
    boolean existsByEmailDomain(String emailDomain);

    // Write operations
    void persist(ClientAuthConfig config);
    void update(ClientAuthConfig config);
    void delete(ClientAuthConfig config);
}

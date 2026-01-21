package tech.flowcatalyst.platform.application;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for ApplicationClientConfig entities.
 */
public interface ApplicationClientConfigRepository {

    // Read operations
    Optional<ApplicationClientConfig> findByIdOptional(String id);
    Optional<ApplicationClientConfig> findByApplicationAndClient(String applicationId, String clientId);
    List<ApplicationClientConfig> findByApplication(String applicationId);
    List<ApplicationClientConfig> findByClient(String clientId);
    List<ApplicationClientConfig> findEnabledByClient(String clientId);
    boolean isApplicationEnabledForClient(String applicationId, String clientId);
    long countByApplication(String applicationId);

    // Write operations
    void persist(ApplicationClientConfig config);
    void update(ApplicationClientConfig config);
    void delete(ApplicationClientConfig config);
    void deleteByApplicationAndClient(String applicationId, String clientId);
}

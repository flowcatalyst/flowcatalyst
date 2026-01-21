package tech.flowcatalyst.platform.config;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for PlatformConfig entities.
 */
public interface PlatformConfigRepository {

    // Read operations
    Optional<PlatformConfig> findByIdOptional(String id);

    Optional<PlatformConfig> findByKey(String applicationCode, String section, String property,
                                        ConfigScope scope, String clientId);

    List<PlatformConfig> findByApplicationAndSection(String applicationCode, String section,
                                                      ConfigScope scope, String clientId);

    List<PlatformConfig> findByApplication(String applicationCode, ConfigScope scope, String clientId);

    List<PlatformConfig> findAllGlobal(String applicationCode);

    List<PlatformConfig> findAllGlobalBySection(String applicationCode, String section);

    // Write operations
    void persist(PlatformConfig config);

    void update(PlatformConfig config);

    void delete(PlatformConfig config);

    boolean deleteById(String id);
}

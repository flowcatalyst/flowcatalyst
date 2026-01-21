package tech.flowcatalyst.platform.config;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Repository interface for PlatformConfigAccess entities.
 */
public interface PlatformConfigAccessRepository {

    // Read operations
    Optional<PlatformConfigAccess> findByIdOptional(String id);

    List<PlatformConfigAccess> findByApplicationCode(String applicationCode);

    List<PlatformConfigAccess> findByRoleCodes(Set<String> roleCodes);

    Optional<PlatformConfigAccess> findByApplicationAndRole(String applicationCode, String roleCode);

    boolean hasReadAccess(String applicationCode, Set<String> roleCodes);

    boolean hasWriteAccess(String applicationCode, Set<String> roleCodes);

    // Write operations
    void persist(PlatformConfigAccess access);

    void update(PlatformConfigAccess access);

    void delete(PlatformConfigAccess access);

    boolean deleteByApplicationAndRole(String applicationCode, String roleCode);
}

package tech.flowcatalyst.platform.authentication;

import java.util.Optional;

/**
 * Repository interface for IdpRoleMapping entities.
 * SECURITY: Only explicitly authorized IDP roles should exist in this table.
 */
public interface IdpRoleMappingRepository {

    // Read operations
    Optional<IdpRoleMapping> findByIdpRoleName(String idpRoleName);

    // Write operations
    void persist(IdpRoleMapping mapping);
    void delete(IdpRoleMapping mapping);
}

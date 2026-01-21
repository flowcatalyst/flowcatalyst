package tech.flowcatalyst.platform.authentication.oidc;

import java.util.Optional;

/**
 * Repository interface for OidcLoginState entities.
 * Provides OIDC login state access methods.
 */
public interface OidcLoginStateRepository {

    // Read operations
    Optional<OidcLoginState> findValidState(String state);

    // Write operations
    void persist(OidcLoginState state);
    void deleteByState(String state);
}

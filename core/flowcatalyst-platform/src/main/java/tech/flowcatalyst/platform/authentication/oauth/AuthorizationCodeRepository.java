package tech.flowcatalyst.platform.authentication.oauth;

import java.util.Optional;

/**
 * Repository interface for AuthorizationCode entities.
 * Provides OAuth authorization code access methods.
 */
public interface AuthorizationCodeRepository {

    // Read operations
    Optional<AuthorizationCode> findValidCode(String code);

    // Write operations
    void persist(AuthorizationCode code);
    void markAsUsed(String code);
}

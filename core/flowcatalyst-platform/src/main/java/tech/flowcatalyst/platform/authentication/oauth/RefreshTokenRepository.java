package tech.flowcatalyst.platform.authentication.oauth;

import java.util.Optional;

/**
 * Repository interface for RefreshToken entities.
 * Provides OAuth refresh token access methods.
 */
public interface RefreshTokenRepository {

    // Read operations
    Optional<RefreshToken> findValidToken(String tokenHash);
    Optional<RefreshToken> findByTokenHash(String tokenHash);

    // Write operations
    void persist(RefreshToken token);
    void update(RefreshToken token);
    void revokeToken(String tokenHash, String replacedBy);
    void revokeTokenFamily(String tokenFamily);
}

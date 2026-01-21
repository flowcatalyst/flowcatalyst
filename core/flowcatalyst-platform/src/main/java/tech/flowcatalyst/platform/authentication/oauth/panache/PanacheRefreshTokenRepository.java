package tech.flowcatalyst.platform.authentication.oauth.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.oauth.RefreshToken;
import tech.flowcatalyst.platform.authentication.oauth.RefreshTokenRepository;
import tech.flowcatalyst.platform.authentication.oauth.entity.RefreshTokenEntity;
import tech.flowcatalyst.platform.authentication.oauth.mapper.RefreshTokenMapper;

import java.time.Instant;
import java.util.Optional;

/**
 * Panache-based implementation of RefreshTokenRepository.
 */
@ApplicationScoped
public class PanacheRefreshTokenRepository
    implements RefreshTokenRepository, PanacheRepositoryBase<RefreshTokenEntity, String> {

    @Override
    public Optional<RefreshToken> findValidToken(String tokenHash) {
        return find("tokenHash = ?1 and revoked = false and expiresAt > ?2", tokenHash, Instant.now())
            .firstResultOptional()
            .map(RefreshTokenMapper::toDomain);
    }

    @Override
    public Optional<RefreshToken> findByTokenHash(String tokenHash) {
        return find("tokenHash", tokenHash)
            .firstResultOptional()
            .map(RefreshTokenMapper::toDomain);
    }

    @Override
    public void persist(RefreshToken token) {
        RefreshTokenEntity entity = RefreshTokenMapper.toEntity(token);
        persist(entity);
    }

    @Override
    public void update(RefreshToken token) {
        RefreshTokenEntity entity = findById(token.tokenHash);
        if (entity != null) {
            RefreshTokenMapper.updateEntity(entity, token);
        }
    }

    @Override
    public void revokeToken(String tokenHash, String replacedBy) {
        update("revoked = true, revokedAt = ?1, replacedBy = ?2 where tokenHash = ?3",
            Instant.now(), replacedBy, tokenHash);
    }

    @Override
    public void revokeTokenFamily(String tokenFamily) {
        update("revoked = true, revokedAt = ?1 where tokenFamily = ?2 and revoked = false",
            Instant.now(), tokenFamily);
    }
}

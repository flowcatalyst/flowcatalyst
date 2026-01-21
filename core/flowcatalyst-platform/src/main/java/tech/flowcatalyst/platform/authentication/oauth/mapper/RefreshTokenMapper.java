package tech.flowcatalyst.platform.authentication.oauth.mapper;

import tech.flowcatalyst.platform.authentication.oauth.RefreshToken;
import tech.flowcatalyst.platform.authentication.oauth.entity.RefreshTokenEntity;

/**
 * Mapper for converting between RefreshToken domain model and JPA entity.
 */
public final class RefreshTokenMapper {

    private RefreshTokenMapper() {
    }

    public static RefreshToken toDomain(RefreshTokenEntity entity) {
        if (entity == null) {
            return null;
        }

        RefreshToken domain = new RefreshToken();
        domain.tokenHash = entity.tokenHash;
        domain.principalId = entity.principalId;
        domain.clientId = entity.clientId;
        domain.contextClientId = entity.contextClientId;
        domain.scope = entity.scope;
        domain.tokenFamily = entity.tokenFamily;
        domain.revoked = entity.revoked;
        domain.revokedAt = entity.revokedAt;
        domain.replacedBy = entity.replacedBy;
        domain.createdAt = entity.createdAt;
        domain.expiresAt = entity.expiresAt;
        return domain;
    }

    public static RefreshTokenEntity toEntity(RefreshToken domain) {
        if (domain == null) {
            return null;
        }

        RefreshTokenEntity entity = new RefreshTokenEntity();
        entity.tokenHash = domain.tokenHash;
        entity.principalId = domain.principalId;
        entity.clientId = domain.clientId;
        entity.contextClientId = domain.contextClientId;
        entity.scope = domain.scope;
        entity.tokenFamily = domain.tokenFamily;
        entity.revoked = domain.revoked;
        entity.revokedAt = domain.revokedAt;
        entity.replacedBy = domain.replacedBy;
        entity.createdAt = domain.createdAt;
        entity.expiresAt = domain.expiresAt;
        return entity;
    }

    public static void updateEntity(RefreshTokenEntity entity, RefreshToken domain) {
        entity.principalId = domain.principalId;
        entity.clientId = domain.clientId;
        entity.contextClientId = domain.contextClientId;
        entity.scope = domain.scope;
        entity.tokenFamily = domain.tokenFamily;
        entity.revoked = domain.revoked;
        entity.revokedAt = domain.revokedAt;
        entity.replacedBy = domain.replacedBy;
        entity.expiresAt = domain.expiresAt;
    }
}

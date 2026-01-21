package tech.flowcatalyst.platform.authentication.oauth.mapper;

import tech.flowcatalyst.platform.authentication.oauth.AuthorizationCode;
import tech.flowcatalyst.platform.authentication.oauth.entity.AuthorizationCodeEntity;

import java.time.Instant;

/**
 * Mapper for converting between AuthorizationCode domain model and JPA entity.
 */
public final class AuthorizationCodeMapper {

    private AuthorizationCodeMapper() {
    }

    public static AuthorizationCode toDomain(AuthorizationCodeEntity entity) {
        if (entity == null) {
            return null;
        }

        AuthorizationCode domain = new AuthorizationCode();
        domain.id = entity.id;
        domain.code = entity.code;
        domain.clientId = entity.clientId;
        domain.principalId = entity.principalId;
        domain.redirectUri = entity.redirectUri;
        domain.scope = entity.scope;
        domain.codeChallenge = entity.codeChallenge;
        domain.codeChallengeMethod = entity.codeChallengeMethod;
        domain.nonce = entity.nonce;
        domain.state = entity.state;
        domain.contextClientId = entity.contextClientId;
        domain.createdAt = entity.createdAt;
        domain.expiresAt = entity.expiresAt;
        domain.used = entity.used;
        return domain;
    }

    public static AuthorizationCodeEntity toEntity(AuthorizationCode domain) {
        if (domain == null) {
            return null;
        }

        AuthorizationCodeEntity entity = new AuthorizationCodeEntity();
        entity.id = domain.id;
        entity.code = domain.code;
        entity.clientId = domain.clientId;
        entity.principalId = domain.principalId;
        entity.redirectUri = domain.redirectUri;
        entity.scope = domain.scope;
        entity.codeChallenge = domain.codeChallenge;
        entity.codeChallengeMethod = domain.codeChallengeMethod;
        entity.nonce = domain.nonce;
        entity.state = domain.state;
        entity.contextClientId = domain.contextClientId;
        entity.createdAt = domain.createdAt != null ? domain.createdAt : Instant.now();
        entity.expiresAt = domain.expiresAt;
        entity.used = domain.used;
        return entity;
    }

    public static void updateEntity(AuthorizationCodeEntity entity, AuthorizationCode domain) {
        entity.code = domain.code;
        entity.clientId = domain.clientId;
        entity.principalId = domain.principalId;
        entity.redirectUri = domain.redirectUri;
        entity.scope = domain.scope;
        entity.codeChallenge = domain.codeChallenge;
        entity.codeChallengeMethod = domain.codeChallengeMethod;
        entity.nonce = domain.nonce;
        entity.state = domain.state;
        entity.contextClientId = domain.contextClientId;
        entity.expiresAt = domain.expiresAt;
        entity.used = domain.used;
    }
}

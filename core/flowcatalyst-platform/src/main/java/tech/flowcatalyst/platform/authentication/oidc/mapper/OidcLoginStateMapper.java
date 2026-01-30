package tech.flowcatalyst.platform.authentication.oidc.mapper;

import tech.flowcatalyst.platform.authentication.oidc.OidcLoginState;
import tech.flowcatalyst.platform.authentication.oidc.entity.OidcLoginStateEntity;

import java.time.Instant;

/**
 * Mapper for converting between OidcLoginState domain model and JPA entity.
 */
public final class OidcLoginStateMapper {

    private OidcLoginStateMapper() {
    }

    public static OidcLoginState toDomain(OidcLoginStateEntity entity) {
        if (entity == null) {
            return null;
        }

        OidcLoginState domain = new OidcLoginState();
        domain.state = entity.state;
        domain.emailDomain = entity.emailDomain;
        domain.identityProviderId = entity.identityProviderId;
        domain.emailDomainMappingId = entity.emailDomainMappingId;
        domain.nonce = entity.nonce;
        domain.codeVerifier = entity.codeVerifier;
        domain.returnUrl = entity.returnUrl;
        domain.oauthClientId = entity.oauthClientId;
        domain.oauthRedirectUri = entity.oauthRedirectUri;
        domain.oauthScope = entity.oauthScope;
        domain.oauthState = entity.oauthState;
        domain.oauthCodeChallenge = entity.oauthCodeChallenge;
        domain.oauthCodeChallengeMethod = entity.oauthCodeChallengeMethod;
        domain.oauthNonce = entity.oauthNonce;
        domain.createdAt = entity.createdAt;
        domain.expiresAt = entity.expiresAt;
        return domain;
    }

    public static OidcLoginStateEntity toEntity(OidcLoginState domain) {
        if (domain == null) {
            return null;
        }

        OidcLoginStateEntity entity = new OidcLoginStateEntity();
        entity.state = domain.state;
        entity.emailDomain = domain.emailDomain;
        entity.identityProviderId = domain.identityProviderId;
        entity.emailDomainMappingId = domain.emailDomainMappingId;
        entity.nonce = domain.nonce;
        entity.codeVerifier = domain.codeVerifier;
        entity.returnUrl = domain.returnUrl;
        entity.oauthClientId = domain.oauthClientId;
        entity.oauthRedirectUri = domain.oauthRedirectUri;
        entity.oauthScope = domain.oauthScope;
        entity.oauthState = domain.oauthState;
        entity.oauthCodeChallenge = domain.oauthCodeChallenge;
        entity.oauthCodeChallengeMethod = domain.oauthCodeChallengeMethod;
        entity.oauthNonce = domain.oauthNonce;
        entity.createdAt = domain.createdAt != null ? domain.createdAt : Instant.now();
        entity.expiresAt = domain.expiresAt != null ? domain.expiresAt : Instant.now().plusSeconds(600);
        return entity;
    }
}

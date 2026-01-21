package tech.flowcatalyst.platform.authentication.oauth.mapper;

import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.entity.OAuthClientEntity;

import java.util.ArrayList;

/**
 * Mapper for converting between OAuthClient domain and entity.
 */
public final class OAuthClientMapper {

    private OAuthClientMapper() {
    }

    /**
     * Convert entity to domain object.
     */
    public static OAuthClient toDomain(OAuthClientEntity entity) {
        if (entity == null) {
            return null;
        }

        OAuthClient domain = new OAuthClient();
        domain.id = entity.id;
        domain.clientId = entity.clientId;
        domain.clientName = entity.clientName;
        domain.clientType = entity.clientType;
        domain.clientSecretRef = entity.clientSecretRef;
        domain.redirectUris = entity.redirectUris != null ? new ArrayList<>(entity.redirectUris) : new ArrayList<>();
        domain.allowedOrigins = entity.allowedOrigins != null ? new ArrayList<>(entity.allowedOrigins) : new ArrayList<>();
        domain.grantTypes = entity.grantTypes != null ? new ArrayList<>(entity.grantTypes) : new ArrayList<>();
        domain.defaultScopes = entity.defaultScopes;
        domain.pkceRequired = entity.pkceRequired;
        domain.applicationIds = entity.applicationIds != null ? new ArrayList<>(entity.applicationIds) : new ArrayList<>();
        domain.serviceAccountPrincipalId = entity.serviceAccountPrincipalId;
        domain.active = entity.active;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;

        return domain;
    }

    /**
     * Convert domain object to entity.
     */
    public static OAuthClientEntity toEntity(OAuthClient domain) {
        if (domain == null) {
            return null;
        }

        OAuthClientEntity entity = new OAuthClientEntity();
        entity.id = domain.id;
        entity.clientId = domain.clientId;
        entity.clientName = domain.clientName;
        entity.clientType = domain.clientType;
        entity.clientSecretRef = domain.clientSecretRef;
        entity.redirectUris = domain.redirectUris != null ? new ArrayList<>(domain.redirectUris) : new ArrayList<>();
        entity.allowedOrigins = domain.allowedOrigins != null ? new ArrayList<>(domain.allowedOrigins) : new ArrayList<>();
        entity.grantTypes = domain.grantTypes != null ? new ArrayList<>(domain.grantTypes) : new ArrayList<>();
        entity.defaultScopes = domain.defaultScopes;
        entity.pkceRequired = domain.pkceRequired;
        entity.applicationIds = domain.applicationIds != null ? new ArrayList<>(domain.applicationIds) : new ArrayList<>();
        entity.serviceAccountPrincipalId = domain.serviceAccountPrincipalId;
        entity.active = domain.active;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;

        return entity;
    }

    /**
     * Update existing entity with values from domain object.
     */
    public static void updateEntity(OAuthClientEntity entity, OAuthClient domain) {
        entity.clientId = domain.clientId;
        entity.clientName = domain.clientName;
        entity.clientType = domain.clientType;
        entity.clientSecretRef = domain.clientSecretRef;
        entity.redirectUris = domain.redirectUris != null ? new ArrayList<>(domain.redirectUris) : new ArrayList<>();
        entity.allowedOrigins = domain.allowedOrigins != null ? new ArrayList<>(domain.allowedOrigins) : new ArrayList<>();
        entity.grantTypes = domain.grantTypes != null ? new ArrayList<>(domain.grantTypes) : new ArrayList<>();
        entity.defaultScopes = domain.defaultScopes;
        entity.pkceRequired = domain.pkceRequired;
        entity.applicationIds = domain.applicationIds != null ? new ArrayList<>(domain.applicationIds) : new ArrayList<>();
        entity.serviceAccountPrincipalId = domain.serviceAccountPrincipalId;
        entity.active = domain.active;
        entity.updatedAt = domain.updatedAt;
    }
}

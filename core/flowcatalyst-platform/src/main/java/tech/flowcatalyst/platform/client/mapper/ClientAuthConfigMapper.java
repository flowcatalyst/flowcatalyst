package tech.flowcatalyst.platform.client.mapper;

import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.client.AuthConfigType;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.entity.ClientAuthConfigEntity;

import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/**
 * Mapper for converting between ClientAuthConfig domain model and JPA entity.
 */
public final class ClientAuthConfigMapper {

    private ClientAuthConfigMapper() {
    }

    public static ClientAuthConfig toDomain(ClientAuthConfigEntity entity) {
        if (entity == null) {
            return null;
        }

        ClientAuthConfig domain = new ClientAuthConfig();
        domain.id = entity.id;
        domain.emailDomain = entity.emailDomain;
        domain.configType = entity.configType != null ? entity.configType : AuthConfigType.CLIENT;
        domain.clientId = entity.clientId;
        domain.primaryClientId = entity.primaryClientId;
        domain.additionalClientIds = entity.additionalClientIds != null
            ? new ArrayList<>(Arrays.asList(entity.additionalClientIds))
            : new ArrayList<>();
        domain.grantedClientIds = entity.grantedClientIds != null
            ? new ArrayList<>(Arrays.asList(entity.grantedClientIds))
            : new ArrayList<>();
        domain.authProvider = entity.authProvider != null ? entity.authProvider : AuthProvider.INTERNAL;
        domain.oidcIssuerUrl = entity.oidcIssuerUrl;
        domain.oidcClientId = entity.oidcClientId;
        domain.oidcClientSecretRef = entity.oidcClientSecretRef;
        domain.oidcMultiTenant = entity.oidcMultiTenant;
        domain.oidcIssuerPattern = entity.oidcIssuerPattern;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static ClientAuthConfigEntity toEntity(ClientAuthConfig domain) {
        if (domain == null) {
            return null;
        }

        ClientAuthConfigEntity entity = new ClientAuthConfigEntity();
        entity.id = domain.id;
        entity.emailDomain = domain.emailDomain;
        entity.configType = domain.configType != null ? domain.configType : AuthConfigType.CLIENT;
        entity.clientId = domain.clientId;
        entity.primaryClientId = domain.primaryClientId;
        entity.additionalClientIds = domain.additionalClientIds != null
            ? domain.additionalClientIds.toArray(new String[0])
            : new String[0];
        entity.grantedClientIds = domain.grantedClientIds != null
            ? domain.grantedClientIds.toArray(new String[0])
            : new String[0];
        entity.authProvider = domain.authProvider != null ? domain.authProvider : AuthProvider.INTERNAL;
        entity.oidcIssuerUrl = domain.oidcIssuerUrl;
        entity.oidcClientId = domain.oidcClientId;
        entity.oidcClientSecretRef = domain.oidcClientSecretRef;
        entity.oidcMultiTenant = domain.oidcMultiTenant;
        entity.oidcIssuerPattern = domain.oidcIssuerPattern;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(ClientAuthConfigEntity entity, ClientAuthConfig domain) {
        entity.emailDomain = domain.emailDomain;
        entity.configType = domain.configType != null ? domain.configType : AuthConfigType.CLIENT;
        entity.clientId = domain.clientId;
        entity.primaryClientId = domain.primaryClientId;
        entity.additionalClientIds = domain.additionalClientIds != null
            ? domain.additionalClientIds.toArray(new String[0])
            : new String[0];
        entity.grantedClientIds = domain.grantedClientIds != null
            ? domain.grantedClientIds.toArray(new String[0])
            : new String[0];
        entity.authProvider = domain.authProvider != null ? domain.authProvider : AuthProvider.INTERNAL;
        entity.oidcIssuerUrl = domain.oidcIssuerUrl;
        entity.oidcClientId = domain.oidcClientId;
        entity.oidcClientSecretRef = domain.oidcClientSecretRef;
        entity.oidcMultiTenant = domain.oidcMultiTenant;
        entity.oidcIssuerPattern = domain.oidcIssuerPattern;
        entity.updatedAt = domain.updatedAt;
    }
}

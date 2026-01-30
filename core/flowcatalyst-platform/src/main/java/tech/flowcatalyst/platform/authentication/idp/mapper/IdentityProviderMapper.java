package tech.flowcatalyst.platform.authentication.idp.mapper;

import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.authentication.idp.entity.IdentityProviderEntity;

import java.util.ArrayList;

/**
 * Mapper for converting between IdentityProvider domain model and JPA entity.
 */
public final class IdentityProviderMapper {

    private IdentityProviderMapper() {
    }

    public static IdentityProvider toDomain(IdentityProviderEntity entity) {
        if (entity == null) {
            return null;
        }

        var domain = new IdentityProvider();
        domain.id = entity.id;
        domain.code = entity.code;
        domain.name = entity.name;
        domain.type = entity.type != null ? entity.type : IdentityProviderType.INTERNAL;
        domain.oidcIssuerUrl = entity.oidcIssuerUrl;
        domain.oidcClientId = entity.oidcClientId;
        domain.oidcClientSecretRef = entity.oidcClientSecretRef;
        domain.oidcMultiTenant = entity.oidcMultiTenant;
        domain.oidcIssuerPattern = entity.oidcIssuerPattern;
        domain.allowedEmailDomains = entity.allowedEmailDomains != null
            ? new ArrayList<>(entity.allowedEmailDomains)
            : new ArrayList<>();
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static IdentityProviderEntity toEntity(IdentityProvider domain) {
        if (domain == null) {
            return null;
        }

        var entity = new IdentityProviderEntity();
        entity.id = domain.id;
        entity.code = domain.code;
        entity.name = domain.name;
        entity.type = domain.type != null ? domain.type : IdentityProviderType.INTERNAL;
        entity.oidcIssuerUrl = domain.oidcIssuerUrl;
        entity.oidcClientId = domain.oidcClientId;
        entity.oidcClientSecretRef = domain.oidcClientSecretRef;
        entity.oidcMultiTenant = domain.oidcMultiTenant;
        entity.oidcIssuerPattern = domain.oidcIssuerPattern;
        entity.allowedEmailDomains = domain.allowedEmailDomains != null
            ? new ArrayList<>(domain.allowedEmailDomains)
            : new ArrayList<>();
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(IdentityProviderEntity entity, IdentityProvider domain) {
        entity.code = domain.code;
        entity.name = domain.name;
        entity.type = domain.type != null ? domain.type : IdentityProviderType.INTERNAL;
        entity.oidcIssuerUrl = domain.oidcIssuerUrl;
        entity.oidcClientId = domain.oidcClientId;
        entity.oidcClientSecretRef = domain.oidcClientSecretRef;
        entity.oidcMultiTenant = domain.oidcMultiTenant;
        entity.oidcIssuerPattern = domain.oidcIssuerPattern;
        entity.allowedEmailDomains.clear();
        if (domain.allowedEmailDomains != null) {
            entity.allowedEmailDomains.addAll(domain.allowedEmailDomains);
        }
        entity.updatedAt = domain.updatedAt;
    }
}

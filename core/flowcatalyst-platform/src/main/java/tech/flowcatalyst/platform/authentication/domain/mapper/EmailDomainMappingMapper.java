package tech.flowcatalyst.platform.authentication.domain.mapper;

import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.domain.entity.EmailDomainMappingEntity;

import java.util.ArrayList;

/**
 * Mapper for converting between EmailDomainMapping domain model and JPA entity.
 */
public final class EmailDomainMappingMapper {

    private EmailDomainMappingMapper() {
    }

    public static EmailDomainMapping toDomain(EmailDomainMappingEntity entity) {
        if (entity == null) {
            return null;
        }

        var domain = new EmailDomainMapping();
        domain.id = entity.id;
        domain.emailDomain = entity.emailDomain;
        domain.identityProviderId = entity.identityProviderId;
        domain.scopeType = entity.scopeType != null ? entity.scopeType : ScopeType.CLIENT;
        domain.primaryClientId = entity.primaryClientId;
        domain.additionalClientIds = entity.additionalClientIds != null
            ? new ArrayList<>(entity.additionalClientIds)
            : new ArrayList<>();
        domain.grantedClientIds = entity.grantedClientIds != null
            ? new ArrayList<>(entity.grantedClientIds)
            : new ArrayList<>();
        domain.requiredOidcTenantId = entity.requiredOidcTenantId;
        domain.allowedRoleIds = entity.allowedRoleIds != null
            ? new ArrayList<>(entity.allowedRoleIds)
            : new ArrayList<>();
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static EmailDomainMappingEntity toEntity(EmailDomainMapping domain) {
        if (domain == null) {
            return null;
        }

        var entity = new EmailDomainMappingEntity();
        entity.id = domain.id;
        entity.emailDomain = domain.emailDomain;
        entity.identityProviderId = domain.identityProviderId;
        entity.scopeType = domain.scopeType != null ? domain.scopeType : ScopeType.CLIENT;
        entity.primaryClientId = domain.primaryClientId;
        entity.additionalClientIds = domain.additionalClientIds != null
            ? new ArrayList<>(domain.additionalClientIds)
            : new ArrayList<>();
        entity.grantedClientIds = domain.grantedClientIds != null
            ? new ArrayList<>(domain.grantedClientIds)
            : new ArrayList<>();
        entity.requiredOidcTenantId = domain.requiredOidcTenantId;
        entity.allowedRoleIds = domain.allowedRoleIds != null
            ? new ArrayList<>(domain.allowedRoleIds)
            : new ArrayList<>();
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(EmailDomainMappingEntity entity, EmailDomainMapping domain) {
        entity.emailDomain = domain.emailDomain;
        entity.identityProviderId = domain.identityProviderId;
        entity.scopeType = domain.scopeType != null ? domain.scopeType : ScopeType.CLIENT;
        entity.primaryClientId = domain.primaryClientId;
        entity.additionalClientIds.clear();
        if (domain.additionalClientIds != null) {
            entity.additionalClientIds.addAll(domain.additionalClientIds);
        }
        entity.grantedClientIds.clear();
        if (domain.grantedClientIds != null) {
            entity.grantedClientIds.addAll(domain.grantedClientIds);
        }
        entity.requiredOidcTenantId = domain.requiredOidcTenantId;
        entity.allowedRoleIds.clear();
        if (domain.allowedRoleIds != null) {
            entity.allowedRoleIds.addAll(domain.allowedRoleIds);
        }
        entity.updatedAt = domain.updatedAt;
    }
}

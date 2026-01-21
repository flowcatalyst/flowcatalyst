package tech.flowcatalyst.serviceaccount.mapper;

import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.entity.WebhookCredentials;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountClientIdEntity;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountJpaEntity;
import tech.flowcatalyst.serviceaccount.jpaentity.ServiceAccountRoleEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between ServiceAccount domain and JPA entities.
 */
public final class ServiceAccountMapper {

    private ServiceAccountMapper() {
    }

    /**
     * Convert JPA entity to domain object (without relations).
     * Call with relation data for complete conversion.
     */
    public static ServiceAccount toDomain(ServiceAccountJpaEntity entity) {
        if (entity == null) {
            return null;
        }

        ServiceAccount sa = new ServiceAccount();
        sa.id = entity.id;
        sa.code = entity.code;
        sa.name = entity.name;
        sa.description = entity.description;
        sa.applicationId = entity.applicationId;
        sa.active = entity.active;
        sa.clientIds = new ArrayList<>(); // loaded separately
        sa.roles = new ArrayList<>(); // loaded separately

        // Map embedded webhook credentials
        sa.webhookCredentials = toWebhookCredentials(entity);

        sa.lastUsedAt = entity.lastUsedAt;
        sa.createdAt = entity.createdAt;
        sa.updatedAt = entity.updatedAt;

        return sa;
    }

    /**
     * Convert domain object to JPA entity.
     */
    public static ServiceAccountJpaEntity toEntity(ServiceAccount domain) {
        if (domain == null) {
            return null;
        }

        ServiceAccountJpaEntity entity = new ServiceAccountJpaEntity();
        entity.id = domain.id;
        entity.code = domain.code;
        entity.name = domain.name;
        entity.description = domain.description;
        entity.applicationId = domain.applicationId;
        entity.active = domain.active;

        // Map webhook credentials to embedded columns
        if (domain.webhookCredentials != null) {
            entity.webhookAuthType = domain.webhookCredentials.authType;
            entity.webhookAuthTokenRef = domain.webhookCredentials.authTokenRef;
            entity.webhookSigningSecretRef = domain.webhookCredentials.signingSecretRef;
            entity.webhookSigningAlgorithm = domain.webhookCredentials.signingAlgorithm;
            entity.webhookCredentialsCreatedAt = domain.webhookCredentials.createdAt;
            entity.webhookCredentialsRegeneratedAt = domain.webhookCredentials.regeneratedAt;
        }

        entity.lastUsedAt = domain.lastUsedAt;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;

        return entity;
    }

    /**
     * Update existing JPA entity with values from domain object.
     */
    public static void updateEntity(ServiceAccountJpaEntity entity, ServiceAccount domain) {
        entity.code = domain.code;
        entity.name = domain.name;
        entity.description = domain.description;
        entity.applicationId = domain.applicationId;
        entity.active = domain.active;

        // Update webhook credentials
        if (domain.webhookCredentials != null) {
            entity.webhookAuthType = domain.webhookCredentials.authType;
            entity.webhookAuthTokenRef = domain.webhookCredentials.authTokenRef;
            entity.webhookSigningSecretRef = domain.webhookCredentials.signingSecretRef;
            entity.webhookSigningAlgorithm = domain.webhookCredentials.signingAlgorithm;
            entity.webhookCredentialsCreatedAt = domain.webhookCredentials.createdAt;
            entity.webhookCredentialsRegeneratedAt = domain.webhookCredentials.regeneratedAt;
        }

        entity.lastUsedAt = domain.lastUsedAt;
        entity.updatedAt = domain.updatedAt;
    }

    /**
     * Extract WebhookCredentials from entity embedded columns.
     */
    private static WebhookCredentials toWebhookCredentials(ServiceAccountJpaEntity entity) {
        WebhookCredentials creds = new WebhookCredentials();
        creds.authType = entity.webhookAuthType;
        creds.authTokenRef = entity.webhookAuthTokenRef;
        creds.signingSecretRef = entity.webhookSigningSecretRef;
        creds.signingAlgorithm = entity.webhookSigningAlgorithm;
        creds.createdAt = entity.webhookCredentialsCreatedAt;
        creds.regeneratedAt = entity.webhookCredentialsRegeneratedAt;
        return creds;
    }

    /**
     * Convert client ID entities to domain list.
     */
    public static List<String> toClientIds(List<ServiceAccountClientIdEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> e.clientId)
            .toList();
    }

    /**
     * Convert domain client IDs to entities.
     */
    public static List<ServiceAccountClientIdEntity> toClientIdEntities(String serviceAccountId, List<String> clientIds) {
        if (clientIds == null) {
            return new ArrayList<>();
        }
        return clientIds.stream()
            .map(clientId -> new ServiceAccountClientIdEntity(serviceAccountId, clientId))
            .toList();
    }

    /**
     * Convert role entities to domain role assignments.
     */
    public static List<Principal.RoleAssignment> toRoleAssignments(List<ServiceAccountRoleEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> {
                Principal.RoleAssignment ra = new Principal.RoleAssignment();
                ra.roleName = e.roleName;
                ra.assignmentSource = e.assignmentSource;
                ra.assignedAt = e.assignedAt;
                return ra;
            })
            .toList();
    }

    /**
     * Convert domain role assignments to entities.
     */
    public static List<ServiceAccountRoleEntity> toRoleEntities(String serviceAccountId, List<Principal.RoleAssignment> roles) {
        if (roles == null) {
            return new ArrayList<>();
        }
        return roles.stream()
            .map(r -> new ServiceAccountRoleEntity(serviceAccountId, r.roleName, r.assignmentSource, r.assignedAt))
            .toList();
    }
}

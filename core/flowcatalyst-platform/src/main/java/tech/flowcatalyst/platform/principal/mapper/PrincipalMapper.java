package tech.flowcatalyst.platform.principal.mapper;

import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.platform.principal.ManagedApplicationScope;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.ServiceAccount;
import tech.flowcatalyst.platform.principal.UserIdentity;
import tech.flowcatalyst.platform.principal.entity.PrincipalEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalManagedApplicationEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalRoleEntity;

import java.util.ArrayList;
import java.util.List;
import java.util.stream.Collectors;

/**
 * Mapper for converting between Principal domain model and JPA entity.
 */
public final class PrincipalMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper();

    private PrincipalMapper() {
    }

    /**
     * Convert JPA entity to domain model.
     * Note: Roles are loaded separately from principal_roles table.
     */
    public static Principal toDomain(PrincipalEntity entity) {
        if (entity == null) {
            return null;
        }

        Principal domain = new Principal();
        domain.id = entity.id;
        domain.type = entity.type;
        domain.scope = entity.scope;
        domain.clientId = entity.clientId;
        domain.applicationId = entity.applicationId;
        domain.managedApplicationScope = entity.managedApplicationScope != null
            ? entity.managedApplicationScope
            : ManagedApplicationScope.NONE;
        domain.name = entity.name;
        domain.active = entity.active;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        // Note: managedApplicationIds is loaded separately from principal_managed_applications table

        // UserIdentity (from flat columns)
        if (entity.email != null) {
            UserIdentity ui = new UserIdentity();
            ui.email = entity.email;
            ui.emailDomain = entity.emailDomain;
            ui.idpType = entity.idpType;
            ui.externalIdpId = entity.externalIdpId;
            ui.passwordHash = entity.passwordHash;
            ui.lastLoginAt = entity.lastLoginAt;
            domain.userIdentity = ui;
        }

        // ServiceAccount (from JSONB)
        if (entity.serviceAccount != null && !entity.serviceAccount.isBlank()) {
            domain.serviceAccount = parseJson(entity.serviceAccount, ServiceAccount.class);
        }

        // Note: Roles are loaded separately from principal_roles table via toRoleAssignments()
        // The roles column has been dropped from the principals table

        return domain;
    }

    /**
     * Convert domain model to JPA entity.
     */
    public static PrincipalEntity toEntity(Principal domain) {
        if (domain == null) {
            return null;
        }

        PrincipalEntity entity = new PrincipalEntity();
        entity.id = domain.id;
        entity.type = domain.type;
        entity.scope = domain.scope;
        entity.clientId = domain.clientId;
        entity.applicationId = domain.applicationId;
        entity.managedApplicationScope = domain.managedApplicationScope;
        entity.name = domain.name;
        entity.active = domain.active;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        // Note: managedApplicationIds is persisted separately to principal_managed_applications table

        // UserIdentity (to flat columns)
        if (domain.userIdentity != null) {
            entity.email = domain.userIdentity.email;
            entity.emailDomain = domain.userIdentity.emailDomain;
            entity.idpType = domain.userIdentity.idpType;
            entity.externalIdpId = domain.userIdentity.externalIdpId;
            entity.passwordHash = domain.userIdentity.passwordHash;
            entity.lastLoginAt = domain.userIdentity.lastLoginAt;
        }

        // ServiceAccount (to JSONB)
        entity.serviceAccount = toJson(domain.serviceAccount);

        // Note: Roles are persisted separately to principal_roles table via toRoleEntities()
        // The roles column has been dropped from the principals table

        return entity;
    }

    /**
     * Update existing entity from domain model.
     */
    public static void updateEntity(PrincipalEntity entity, Principal domain) {
        entity.type = domain.type;
        entity.scope = domain.scope;
        entity.clientId = domain.clientId;
        entity.applicationId = domain.applicationId;
        entity.managedApplicationScope = domain.managedApplicationScope;
        entity.name = domain.name;
        entity.active = domain.active;
        entity.updatedAt = domain.updatedAt;

        if (domain.userIdentity != null) {
            entity.email = domain.userIdentity.email;
            entity.emailDomain = domain.userIdentity.emailDomain;
            entity.idpType = domain.userIdentity.idpType;
            entity.externalIdpId = domain.userIdentity.externalIdpId;
            entity.passwordHash = domain.userIdentity.passwordHash;
            entity.lastLoginAt = domain.userIdentity.lastLoginAt;
        }

        entity.serviceAccount = toJson(domain.serviceAccount);
        // Note: Roles are updated separately via principal_roles table
    }

    /**
     * Convert role entities to domain role assignments.
     */
    public static List<Principal.RoleAssignment> toRoleAssignments(List<PrincipalRoleEntity> roleEntities) {
        if (roleEntities == null) {
            return new ArrayList<>();
        }
        return roleEntities.stream()
            .map(re -> new Principal.RoleAssignment(re.roleName, re.assignmentSource, re.assignedAt))
            .collect(Collectors.toList());
    }

    /**
     * Convert domain role assignments to role entities.
     */
    public static List<PrincipalRoleEntity> toRoleEntities(String principalId, List<Principal.RoleAssignment> roles) {
        if (roles == null) {
            return new ArrayList<>();
        }
        return roles.stream()
            .map(r -> new PrincipalRoleEntity(principalId, r.roleName, r.assignmentSource, r.assignedAt))
            .collect(Collectors.toList());
    }

    // ========================================================================
    // Managed Applications Mapping
    // ========================================================================

    /**
     * Convert managed application entities to list of application IDs.
     */
    public static List<String> toManagedApplicationIds(List<PrincipalManagedApplicationEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> e.applicationId)
            .collect(Collectors.toList());
    }

    /**
     * Convert list of application IDs to managed application entities.
     */
    public static List<PrincipalManagedApplicationEntity> toManagedApplicationEntities(
            String principalId, List<String> applicationIds) {
        if (applicationIds == null) {
            return new ArrayList<>();
        }
        return applicationIds.stream()
            .map(appId -> new PrincipalManagedApplicationEntity(principalId, appId))
            .collect(Collectors.toList());
    }

    // ========================================================================
    // JSON Utilities
    // ========================================================================

    private static <T> T parseJson(String json, Class<T> clazz) {
        if (json == null || json.isBlank()) return null;
        try {
            return objectMapper.readValue(json, clazz);
        } catch (Exception e) {
            return null;
        }
    }

    private static String toJson(Object obj) {
        if (obj == null) return null;
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (Exception e) {
            return null;
        }
    }
}

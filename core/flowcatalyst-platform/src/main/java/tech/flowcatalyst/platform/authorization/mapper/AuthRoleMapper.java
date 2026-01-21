package tech.flowcatalyst.platform.authorization.mapper;

import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.entity.AuthRoleEntity;

import java.util.Arrays;
import java.util.HashSet;

/**
 * Mapper for converting between AuthRole domain model and JPA entity.
 */
public final class AuthRoleMapper {

    private AuthRoleMapper() {
    }

    public static AuthRole toDomain(AuthRoleEntity entity) {
        if (entity == null) {
            return null;
        }

        AuthRole domain = new AuthRole();
        domain.id = entity.id;
        domain.applicationId = entity.applicationId;
        domain.applicationCode = entity.applicationCode;
        domain.name = entity.name;
        domain.displayName = entity.displayName;
        domain.description = entity.description;
        domain.permissions = entity.permissions != null
            ? new HashSet<>(Arrays.asList(entity.permissions))
            : new HashSet<>();
        domain.source = entity.source != null ? entity.source : AuthRole.RoleSource.DATABASE;
        domain.clientManaged = entity.clientManaged;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static AuthRoleEntity toEntity(AuthRole domain) {
        if (domain == null) {
            return null;
        }

        AuthRoleEntity entity = new AuthRoleEntity();
        entity.id = domain.id;
        entity.applicationId = domain.applicationId;
        entity.applicationCode = domain.applicationCode;
        entity.name = domain.name;
        entity.displayName = domain.displayName;
        entity.description = domain.description;
        entity.permissions = domain.permissions != null
            ? domain.permissions.toArray(new String[0])
            : new String[0];
        entity.source = domain.source != null ? domain.source : AuthRole.RoleSource.DATABASE;
        entity.clientManaged = domain.clientManaged;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(AuthRoleEntity entity, AuthRole domain) {
        entity.applicationId = domain.applicationId;
        entity.applicationCode = domain.applicationCode;
        entity.name = domain.name;
        entity.displayName = domain.displayName;
        entity.description = domain.description;
        entity.permissions = domain.permissions != null
            ? domain.permissions.toArray(new String[0])
            : new String[0];
        entity.source = domain.source != null ? domain.source : AuthRole.RoleSource.DATABASE;
        entity.clientManaged = domain.clientManaged;
        entity.updatedAt = domain.updatedAt;
    }
}

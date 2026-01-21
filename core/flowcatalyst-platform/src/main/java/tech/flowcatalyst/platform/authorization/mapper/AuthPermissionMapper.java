package tech.flowcatalyst.platform.authorization.mapper;

import tech.flowcatalyst.platform.authorization.AuthPermission;
import tech.flowcatalyst.platform.authorization.entity.AuthPermissionEntity;

/**
 * Mapper for converting between AuthPermission domain model and JPA entity.
 */
public final class AuthPermissionMapper {

    private AuthPermissionMapper() {
    }

    public static AuthPermission toDomain(AuthPermissionEntity entity) {
        if (entity == null) {
            return null;
        }

        AuthPermission domain = new AuthPermission();
        domain.id = entity.id;
        domain.applicationId = entity.applicationId;
        domain.name = entity.name;
        domain.displayName = entity.displayName;
        domain.description = entity.description;
        domain.source = entity.source;
        domain.createdAt = entity.createdAt;
        return domain;
    }

    public static AuthPermissionEntity toEntity(AuthPermission domain) {
        if (domain == null) {
            return null;
        }

        return new AuthPermissionEntity(
            domain.id,
            domain.applicationId,
            domain.name,
            domain.displayName,
            domain.description,
            domain.source != null ? domain.source : AuthPermission.PermissionSource.SDK,
            domain.createdAt
        );
    }

    public static void updateEntity(AuthPermissionEntity entity, AuthPermission domain) {
        entity.applicationId = domain.applicationId;
        entity.name = domain.name;
        entity.displayName = domain.displayName;
        entity.description = domain.description;
        entity.source = domain.source != null ? domain.source : AuthPermission.PermissionSource.SDK;
    }
}

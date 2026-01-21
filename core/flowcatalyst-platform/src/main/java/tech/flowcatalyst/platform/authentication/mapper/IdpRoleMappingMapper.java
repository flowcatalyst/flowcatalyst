package tech.flowcatalyst.platform.authentication.mapper;

import tech.flowcatalyst.platform.authentication.IdpRoleMapping;
import tech.flowcatalyst.platform.authentication.entity.IdpRoleMappingEntity;

/**
 * Mapper for converting between IdpRoleMapping domain model and JPA entity.
 */
public final class IdpRoleMappingMapper {

    private IdpRoleMappingMapper() {
    }

    public static IdpRoleMapping toDomain(IdpRoleMappingEntity entity) {
        if (entity == null) {
            return null;
        }

        IdpRoleMapping domain = new IdpRoleMapping();
        domain.id = entity.id;
        domain.idpRoleName = entity.idpRoleName;
        domain.internalRoleName = entity.internalRoleName;
        domain.createdAt = entity.createdAt;
        return domain;
    }

    public static IdpRoleMappingEntity toEntity(IdpRoleMapping domain) {
        if (domain == null) {
            return null;
        }

        return new IdpRoleMappingEntity(
            domain.id,
            domain.idpRoleName,
            domain.internalRoleName,
            domain.createdAt
        );
    }
}

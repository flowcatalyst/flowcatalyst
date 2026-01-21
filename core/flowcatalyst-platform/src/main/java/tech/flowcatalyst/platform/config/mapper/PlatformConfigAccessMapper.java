package tech.flowcatalyst.platform.config.mapper;

import tech.flowcatalyst.platform.config.PlatformConfigAccess;
import tech.flowcatalyst.platform.config.entity.PlatformConfigAccessEntity;

/**
 * Mapper for converting between PlatformConfigAccess domain model and JPA entity.
 */
public final class PlatformConfigAccessMapper {

    private PlatformConfigAccessMapper() {
    }

    /**
     * Convert JPA entity to domain model.
     */
    public static PlatformConfigAccess toDomain(PlatformConfigAccessEntity entity) {
        if (entity == null) {
            return null;
        }

        PlatformConfigAccess domain = new PlatformConfigAccess();
        domain.id = entity.id;
        domain.applicationCode = entity.applicationCode;
        domain.roleCode = entity.roleCode;
        domain.canRead = entity.canRead;
        domain.canWrite = entity.canWrite;
        domain.createdAt = entity.createdAt;
        return domain;
    }

    /**
     * Convert domain model to JPA entity.
     */
    public static PlatformConfigAccessEntity toEntity(PlatformConfigAccess domain) {
        if (domain == null) {
            return null;
        }

        PlatformConfigAccessEntity entity = new PlatformConfigAccessEntity();
        entity.id = domain.id;
        entity.applicationCode = domain.applicationCode;
        entity.roleCode = domain.roleCode;
        entity.canRead = domain.canRead;
        entity.canWrite = domain.canWrite;
        entity.createdAt = domain.createdAt;
        return entity;
    }

    /**
     * Update existing entity from domain model.
     */
    public static void updateEntity(PlatformConfigAccessEntity entity, PlatformConfigAccess domain) {
        entity.applicationCode = domain.applicationCode;
        entity.roleCode = domain.roleCode;
        entity.canRead = domain.canRead;
        entity.canWrite = domain.canWrite;
    }
}

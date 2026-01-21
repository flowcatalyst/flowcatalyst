package tech.flowcatalyst.platform.config.mapper;

import tech.flowcatalyst.platform.config.PlatformConfig;
import tech.flowcatalyst.platform.config.entity.PlatformConfigEntity;

/**
 * Mapper for converting between PlatformConfig domain model and JPA entity.
 */
public final class PlatformConfigMapper {

    private PlatformConfigMapper() {
    }

    /**
     * Convert JPA entity to domain model.
     */
    public static PlatformConfig toDomain(PlatformConfigEntity entity) {
        if (entity == null) {
            return null;
        }

        PlatformConfig domain = new PlatformConfig();
        domain.id = entity.id;
        domain.applicationCode = entity.applicationCode;
        domain.section = entity.section;
        domain.property = entity.property;
        domain.scope = entity.scope;
        domain.clientId = entity.clientId;
        domain.valueType = entity.valueType;
        domain.value = entity.value;
        domain.description = entity.description;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    /**
     * Convert domain model to JPA entity.
     */
    public static PlatformConfigEntity toEntity(PlatformConfig domain) {
        if (domain == null) {
            return null;
        }

        PlatformConfigEntity entity = new PlatformConfigEntity();
        entity.id = domain.id;
        entity.applicationCode = domain.applicationCode;
        entity.section = domain.section;
        entity.property = domain.property;
        entity.scope = domain.scope;
        entity.clientId = domain.clientId;
        entity.valueType = domain.valueType;
        entity.value = domain.value;
        entity.description = domain.description;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    /**
     * Update existing entity from domain model.
     */
    public static void updateEntity(PlatformConfigEntity entity, PlatformConfig domain) {
        entity.applicationCode = domain.applicationCode;
        entity.section = domain.section;
        entity.property = domain.property;
        entity.scope = domain.scope;
        entity.clientId = domain.clientId;
        entity.valueType = domain.valueType;
        entity.value = domain.value;
        entity.description = domain.description;
        entity.updatedAt = domain.updatedAt;
    }
}

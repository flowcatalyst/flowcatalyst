package tech.flowcatalyst.platform.application.mapper;

import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.entity.ApplicationEntity;

/**
 * Mapper for converting between Application domain model and JPA entity.
 */
public final class ApplicationMapper {

    private ApplicationMapper() {
    }

    public static Application toDomain(ApplicationEntity entity) {
        if (entity == null) {
            return null;
        }

        Application domain = new Application();
        domain.id = entity.id;
        domain.code = entity.code;
        domain.name = entity.name;
        domain.description = entity.description;
        domain.type = entity.type;
        domain.defaultBaseUrl = entity.defaultBaseUrl;
        domain.serviceAccountId = entity.serviceAccountId;
        domain.active = entity.active;
        domain.iconUrl = entity.iconUrl;
        domain.website = entity.website;
        domain.logo = entity.logo;
        domain.logoMimeType = entity.logoMimeType;
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static ApplicationEntity toEntity(Application domain) {
        if (domain == null) {
            return null;
        }

        ApplicationEntity entity = new ApplicationEntity();
        entity.id = domain.id;
        entity.code = domain.code;
        entity.name = domain.name;
        entity.description = domain.description;
        entity.type = domain.type != null ? domain.type : Application.ApplicationType.APPLICATION;
        entity.defaultBaseUrl = domain.defaultBaseUrl;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.active = domain.active;
        entity.iconUrl = domain.iconUrl;
        entity.website = domain.website;
        entity.logo = domain.logo;
        entity.logoMimeType = domain.logoMimeType;
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(ApplicationEntity entity, Application domain) {
        entity.code = domain.code;
        entity.name = domain.name;
        entity.description = domain.description;
        entity.type = domain.type != null ? domain.type : Application.ApplicationType.APPLICATION;
        entity.defaultBaseUrl = domain.defaultBaseUrl;
        entity.serviceAccountId = domain.serviceAccountId;
        entity.active = domain.active;
        entity.iconUrl = domain.iconUrl;
        entity.website = domain.website;
        entity.logo = domain.logo;
        entity.logoMimeType = domain.logoMimeType;
        entity.updatedAt = domain.updatedAt;
    }
}

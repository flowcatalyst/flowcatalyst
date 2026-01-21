package tech.flowcatalyst.platform.cors.mapper;

import tech.flowcatalyst.platform.cors.CorsAllowedOrigin;
import tech.flowcatalyst.platform.cors.entity.CorsAllowedOriginEntity;

/**
 * Mapper for converting between CorsAllowedOrigin domain model and JPA entity.
 */
public final class CorsAllowedOriginMapper {

    private CorsAllowedOriginMapper() {
    }

    public static CorsAllowedOrigin toDomain(CorsAllowedOriginEntity entity) {
        if (entity == null) {
            return null;
        }

        CorsAllowedOrigin domain = new CorsAllowedOrigin();
        domain.id = entity.id;
        domain.origin = entity.origin;
        domain.description = entity.description;
        domain.createdBy = entity.createdBy;
        domain.createdAt = entity.createdAt;
        return domain;
    }

    public static CorsAllowedOriginEntity toEntity(CorsAllowedOrigin domain) {
        if (domain == null) {
            return null;
        }

        return new CorsAllowedOriginEntity(
            domain.id,
            domain.origin,
            domain.description,
            domain.createdBy,
            domain.createdAt
        );
    }
}

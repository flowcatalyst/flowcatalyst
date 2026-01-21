package tech.flowcatalyst.platform.principal.mapper;

import tech.flowcatalyst.platform.principal.AnchorDomain;
import tech.flowcatalyst.platform.principal.entity.AnchorDomainEntity;

/**
 * Mapper for converting between AnchorDomain domain model and JPA entity.
 */
public final class AnchorDomainMapper {

    private AnchorDomainMapper() {
    }

    /**
     * Convert JPA entity to domain model.
     */
    public static AnchorDomain toDomain(AnchorDomainEntity entity) {
        if (entity == null) {
            return null;
        }

        AnchorDomain domain = new AnchorDomain();
        domain.id = entity.id;
        domain.domain = entity.domain;
        domain.createdAt = entity.createdAt;
        return domain;
    }

    /**
     * Convert domain model to JPA entity.
     */
    public static AnchorDomainEntity toEntity(AnchorDomain domain) {
        if (domain == null) {
            return null;
        }

        return new AnchorDomainEntity(
            domain.id,
            domain.domain,
            domain.createdAt
        );
    }

    /**
     * Update existing entity from domain model.
     */
    public static void updateEntity(AnchorDomainEntity entity, AnchorDomain domain) {
        entity.domain = domain.domain;
        entity.createdAt = domain.createdAt;
    }
}

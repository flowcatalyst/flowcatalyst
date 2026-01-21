package tech.flowcatalyst.dispatchpool.mapper;

import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.dispatchpool.entity.DispatchPoolEntity;

/**
 * Mapper for converting between DispatchPool domain model and JPA entity.
 */
public final class DispatchPoolMapper {

    private DispatchPoolMapper() {
    }

    public static DispatchPool toDomain(DispatchPoolEntity entity) {
        if (entity == null) {
            return null;
        }

        return DispatchPool.builder()
            .id(entity.id)
            .code(entity.code)
            .name(entity.name)
            .description(entity.description)
            .rateLimit(entity.rateLimit)
            .concurrency(entity.concurrency)
            .clientId(entity.clientId)
            .clientIdentifier(entity.clientIdentifier)
            .status(entity.status != null ? entity.status : DispatchPoolStatus.ACTIVE)
            .createdAt(entity.createdAt)
            .updatedAt(entity.updatedAt)
            .build();
    }

    public static DispatchPoolEntity toEntity(DispatchPool domain) {
        if (domain == null) {
            return null;
        }

        DispatchPoolEntity entity = new DispatchPoolEntity();
        entity.id = domain.id();
        entity.code = domain.code();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.rateLimit = domain.rateLimit();
        entity.concurrency = domain.concurrency();
        entity.clientId = domain.clientId();
        entity.clientIdentifier = domain.clientIdentifier();
        entity.status = domain.status() != null ? domain.status() : DispatchPoolStatus.ACTIVE;
        entity.createdAt = domain.createdAt();
        entity.updatedAt = domain.updatedAt();
        return entity;
    }

    public static void updateEntity(DispatchPoolEntity entity, DispatchPool domain) {
        entity.code = domain.code();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.rateLimit = domain.rateLimit();
        entity.concurrency = domain.concurrency();
        entity.clientId = domain.clientId();
        entity.clientIdentifier = domain.clientIdentifier();
        entity.status = domain.status() != null ? domain.status() : DispatchPoolStatus.ACTIVE;
        entity.updatedAt = domain.updatedAt();
    }
}

package tech.flowcatalyst.platform.audit.mapper;

import tech.flowcatalyst.platform.audit.AuditLog;
import tech.flowcatalyst.platform.audit.entity.AuditLogEntity;

import java.time.Instant;

/**
 * Mapper for converting between AuditLog domain model and JPA entity.
 */
public final class AuditLogMapper {

    private AuditLogMapper() {
    }

    public static AuditLog toDomain(AuditLogEntity entity) {
        if (entity == null) {
            return null;
        }

        AuditLog domain = new AuditLog();
        domain.id = entity.id;
        domain.entityType = entity.entityType;
        domain.entityId = entity.entityId;
        domain.operation = entity.operation;
        domain.operationJson = entity.operationJson;
        domain.principalId = entity.principalId;
        domain.performedAt = entity.performedAt;
        return domain;
    }

    public static AuditLogEntity toEntity(AuditLog domain) {
        if (domain == null) {
            return null;
        }

        AuditLogEntity entity = new AuditLogEntity();
        entity.id = domain.id;
        entity.entityType = domain.entityType;
        entity.entityId = domain.entityId;
        entity.operation = domain.operation;
        entity.operationJson = domain.operationJson;
        entity.principalId = domain.principalId;
        entity.performedAt = domain.performedAt != null ? domain.performedAt : Instant.now();
        return entity;
    }
}

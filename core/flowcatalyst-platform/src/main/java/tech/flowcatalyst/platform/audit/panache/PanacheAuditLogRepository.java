package tech.flowcatalyst.platform.audit.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.audit.AuditLog;
import tech.flowcatalyst.platform.audit.AuditLogRepository;
import tech.flowcatalyst.platform.audit.entity.AuditLogEntity;
import tech.flowcatalyst.platform.audit.mapper.AuditLogMapper;
import tech.flowcatalyst.platform.common.Page;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;

/**
 * Panache-based implementation of AuditLogRepository.
 */
@ApplicationScoped
public class PanacheAuditLogRepository implements AuditLogRepository {

    @Inject
    EntityManager em;

    @Override
    public AuditLog findById(String id) {
        AuditLogEntity entity = em.find(AuditLogEntity.class, id);
        return AuditLogMapper.toDomain(entity);
    }

    @Override
    public List<AuditLog> findByEntity(String entityType, String entityId) {
        return em.createQuery(
                "FROM AuditLogEntity WHERE entityType = :entityType AND entityId = :entityId ORDER BY performedAt DESC",
                AuditLogEntity.class)
            .setParameter("entityType", entityType)
            .setParameter("entityId", entityId)
            .getResultList()
            .stream()
            .map(AuditLogMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuditLog> findByPrincipal(String principalId) {
        return em.createQuery(
                "FROM AuditLogEntity WHERE principalId = :principalId ORDER BY performedAt DESC",
                AuditLogEntity.class)
            .setParameter("principalId", principalId)
            .getResultList()
            .stream()
            .map(AuditLogMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuditLog> findByOperation(String operation) {
        return em.createQuery(
                "FROM AuditLogEntity WHERE operation = :operation ORDER BY performedAt DESC",
                AuditLogEntity.class)
            .setParameter("operation", operation)
            .getResultList()
            .stream()
            .map(AuditLogMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuditLog> findPaged(int page, int pageSize) {
        return em.createQuery("FROM AuditLogEntity ORDER BY performedAt DESC", AuditLogEntity.class)
            .setFirstResult(page * pageSize)
            .setMaxResults(pageSize)
            .getResultList()
            .stream()
            .map(AuditLogMapper::toDomain)
            .toList();
    }

    @Override
    public List<AuditLog> findByEntityTypePaged(String entityType, int page, int pageSize) {
        return em.createQuery(
                "FROM AuditLogEntity WHERE entityType = :entityType ORDER BY performedAt DESC",
                AuditLogEntity.class)
            .setParameter("entityType", entityType)
            .setFirstResult(page * pageSize)
            .setMaxResults(pageSize)
            .getResultList()
            .stream()
            .map(AuditLogMapper::toDomain)
            .toList();
    }

    @Override
    @Deprecated
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM AuditLogEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    @Deprecated
    public long countByEntityType(String entityType) {
        return em.createQuery("SELECT COUNT(e) FROM AuditLogEntity e WHERE e.entityType = :entityType", Long.class)
            .setParameter("entityType", entityType)
            .getSingleResult();
    }

    @Override
    public Page<AuditLog> findPage(String afterCursor, int limit) {
        List<AuditLogEntity> entities;

        if (afterCursor == null || afterCursor.isBlank()) {
            entities = em.createQuery("FROM AuditLogEntity ORDER BY id DESC", AuditLogEntity.class)
                .setMaxResults(limit + 1)
                .getResultList();
        } else {
            entities = em.createQuery("FROM AuditLogEntity WHERE id < :cursor ORDER BY id DESC", AuditLogEntity.class)
                .setParameter("cursor", afterCursor)
                .setMaxResults(limit + 1)
                .getResultList();
        }

        List<AuditLog> logs = entities.stream()
            .map(AuditLogMapper::toDomain)
            .toList();

        return Page.of(logs, limit, log -> log.id);
    }

    @Override
    public List<String> findDistinctEntityTypes() {
        return em.createQuery(
                "SELECT DISTINCT a.entityType FROM AuditLogEntity a ORDER BY a.entityType",
                String.class)
            .getResultList();
    }

    @Override
    public List<String> findDistinctOperations() {
        return em.createQuery(
                "SELECT DISTINCT a.operation FROM AuditLogEntity a ORDER BY a.operation",
                String.class)
            .getResultList();
    }

    @Override
    public void persist(AuditLog log) {
        if (log.id == null) {
            log.id = TsidGenerator.generate(EntityType.AUDIT_LOG);
        }
        if (log.performedAt == null) {
            log.performedAt = Instant.now();
        }
        AuditLogEntity entity = AuditLogMapper.toEntity(log);
        em.persist(entity);
    }
}

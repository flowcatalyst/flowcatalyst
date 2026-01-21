package tech.flowcatalyst.platform.audit.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.audit.entity.AuditLogEntity;

/**
 * Base Panache repository for AuditLog entities.
 * Used internally by the TransactionalUnitOfWork for atomic audit log persistence.
 */
@ApplicationScoped
public class PanacheAuditLogBaseRepository implements PanacheRepositoryBase<AuditLogEntity, String> {
}

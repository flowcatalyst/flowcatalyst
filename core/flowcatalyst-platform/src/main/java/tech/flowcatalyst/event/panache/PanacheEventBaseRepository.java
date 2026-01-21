package tech.flowcatalyst.event.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.event.entity.EventEntity;

/**
 * Base Panache repository for Event entities.
 * Used internally by the TransactionalUnitOfWork for atomic event persistence.
 */
@ApplicationScoped
public class PanacheEventBaseRepository implements PanacheRepositoryBase<EventEntity, String> {
}

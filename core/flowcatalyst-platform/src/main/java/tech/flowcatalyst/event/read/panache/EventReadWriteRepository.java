package tech.flowcatalyst.event.read.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.event.read.EventRead;
import tech.flowcatalyst.event.read.jpaentity.EventReadContextDataEntity;
import tech.flowcatalyst.event.read.jpaentity.EventReadEntity;
import tech.flowcatalyst.event.read.mapper.EventReadMapper;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for EventRead entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
@Transactional
public class EventReadWriteRepository implements PanacheRepositoryBase<EventReadEntity, String> {

    @Inject
    EntityManager em;

    /**
     * Persist a new event read projection with its context data.
     */
    public void persistEventRead(EventRead event) {
        if (event.projectedAt == null) {
            event.projectedAt = Instant.now();
        }

        EventReadEntity entity = EventReadMapper.toEntity(event);
        persist(entity);

        // Save context data
        saveContextData(event.id, event.contextData);
    }

    /**
     * Update an existing event read projection with its context data.
     */
    public void updateEventRead(EventRead event) {
        event.projectedAt = Instant.now();

        EventReadEntity entity = findById(event.id);
        if (entity != null) {
            EventReadMapper.updateEntity(entity, event);
        }

        // Update context data
        saveContextData(event.id, event.contextData);
    }

    /**
     * Delete an event read projection and its context data.
     */
    public boolean deleteEventRead(String id) {
        // Delete context data first
        em.createQuery("DELETE FROM EventReadContextDataEntity WHERE eventReadId = :id")
            .setParameter("id", id)
            .executeUpdate();

        return deleteById(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private void saveContextData(String eventReadId, List<EventRead.ContextDataRead> contextData) {
        // Delete existing
        em.createQuery("DELETE FROM EventReadContextDataEntity WHERE eventReadId = :id")
            .setParameter("id", eventReadId)
            .executeUpdate();

        // Insert new
        if (contextData != null) {
            List<EventReadContextDataEntity> entities = EventReadMapper.toContextDataEntities(eventReadId, contextData);
            for (EventReadContextDataEntity entity : entities) {
                em.persist(entity);
            }
        }
    }
}

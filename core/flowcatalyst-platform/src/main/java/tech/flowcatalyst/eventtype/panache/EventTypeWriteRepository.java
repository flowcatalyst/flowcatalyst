package tech.flowcatalyst.eventtype.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.entity.EventTypeEntity;
import tech.flowcatalyst.eventtype.mapper.EventTypeMapper;

import java.time.Instant;

/**
 * Write-side repository for EventType entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class EventTypeWriteRepository implements PanacheRepositoryBase<EventTypeEntity, String> {

    /**
     * Persist a new event type.
     */
    public void persistEventType(EventType eventType) {
        EventType toSave = eventType;
        if (toSave.createdAt() == null) {
            toSave = toSave.toBuilder().createdAt(Instant.now()).build();
        }
        toSave = toSave.toBuilder().updatedAt(Instant.now()).build();
        EventTypeEntity entity = EventTypeMapper.toEntity(toSave);
        persist(entity);
    }

    /**
     * Update an existing event type.
     */
    public void updateEventType(EventType eventType) {
        EventType toUpdate = eventType.toBuilder().updatedAt(Instant.now()).build();
        EventTypeEntity entity = findById(eventType.id());
        if (entity != null) {
            EventTypeMapper.updateEntity(entity, toUpdate);
        }
    }

    /**
     * Delete an event type by ID.
     */
    public boolean deleteEventTypeById(String id) {
        return deleteById(id);
    }
}

package tech.flowcatalyst.event.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.event.entity.EventEntity;
import tech.flowcatalyst.event.mapper.EventMapper;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for Event entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class EventWriteRepository implements PanacheRepositoryBase<EventEntity, String> {

    /**
     * Persist a new event.
     */
    public void persistEvent(Event event) {
        if (event.id == null) {
            event.id = TsidGenerator.generate(EntityType.EVENT);
        }
        if (event.time == null) {
            event.time = Instant.now();
        }
        EventEntity entity = EventMapper.toEntity(event);
        persist(entity);
    }

    /**
     * Persist multiple events.
     */
    public void persistAllEvents(List<Event> events) {
        List<EventEntity> entities = events.stream()
            .map(event -> {
                if (event.id == null) {
                    event.id = TsidGenerator.generate(EntityType.EVENT);
                }
                if (event.time == null) {
                    event.time = Instant.now();
                }
                return EventMapper.toEntity(event);
            })
            .toList();
        persist(entities);
    }

    /**
     * Update an existing event.
     */
    public void updateEvent(Event event) {
        EventEntity entity = findById(event.id);
        if (entity != null) {
            EventMapper.updateEntity(entity, event);
        }
    }

    /**
     * Delete an event by ID.
     */
    public boolean deleteEventById(String id) {
        return deleteById(id);
    }
}

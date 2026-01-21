package tech.flowcatalyst.event.read.mapper;

import tech.flowcatalyst.event.read.EventRead;
import tech.flowcatalyst.event.read.jpaentity.EventReadContextDataEntity;
import tech.flowcatalyst.event.read.jpaentity.EventReadEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between EventRead domain and JPA entities.
 */
public final class EventReadMapper {

    private EventReadMapper() {
    }

    /**
     * Convert JPA entity to domain object (without context data).
     */
    public static EventRead toDomain(EventReadEntity entity) {
        if (entity == null) {
            return null;
        }

        EventRead event = new EventRead();
        event.id = entity.id;
        event.specVersion = entity.specVersion;
        event.type = entity.type;
        event.application = entity.application;
        event.subdomain = entity.subdomain;
        event.aggregate = entity.aggregate;
        event.source = entity.source;
        event.subject = entity.subject;
        event.time = entity.time;
        event.data = entity.data;
        event.messageGroup = entity.messageGroup;
        event.correlationId = entity.correlationId;
        event.causationId = entity.causationId;
        event.deduplicationId = entity.deduplicationId;
        event.clientId = entity.clientId;
        event.projectedAt = entity.projectedAt;
        event.contextData = new ArrayList<>(); // loaded separately

        return event;
    }

    /**
     * Convert domain object to JPA entity.
     */
    public static EventReadEntity toEntity(EventRead domain) {
        if (domain == null) {
            return null;
        }

        EventReadEntity entity = new EventReadEntity();
        entity.id = domain.id;
        entity.specVersion = domain.specVersion;
        entity.type = domain.type;
        entity.application = domain.application;
        entity.subdomain = domain.subdomain;
        entity.aggregate = domain.aggregate;
        entity.source = domain.source;
        entity.subject = domain.subject;
        entity.time = domain.time;
        entity.data = domain.data;
        entity.messageGroup = domain.messageGroup;
        entity.correlationId = domain.correlationId;
        entity.causationId = domain.causationId;
        entity.deduplicationId = domain.deduplicationId;
        entity.clientId = domain.clientId;
        entity.projectedAt = domain.projectedAt;

        return entity;
    }

    /**
     * Update existing JPA entity with values from domain object.
     */
    public static void updateEntity(EventReadEntity entity, EventRead domain) {
        entity.specVersion = domain.specVersion;
        entity.type = domain.type;
        entity.application = domain.application;
        entity.subdomain = domain.subdomain;
        entity.aggregate = domain.aggregate;
        entity.source = domain.source;
        entity.subject = domain.subject;
        entity.time = domain.time;
        entity.data = domain.data;
        entity.messageGroup = domain.messageGroup;
        entity.correlationId = domain.correlationId;
        entity.causationId = domain.causationId;
        entity.deduplicationId = domain.deduplicationId;
        entity.clientId = domain.clientId;
        entity.projectedAt = domain.projectedAt;
    }

    // ========================================================================
    // Context Data Mapping
    // ========================================================================

    public static List<EventRead.ContextDataRead> toContextDataList(List<EventReadContextDataEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> new EventRead.ContextDataRead(e.key, e.value))
            .toList();
    }

    public static List<EventReadContextDataEntity> toContextDataEntities(String eventReadId, List<EventRead.ContextDataRead> contextData) {
        if (contextData == null) {
            return new ArrayList<>();
        }
        return contextData.stream()
            .map(cd -> new EventReadContextDataEntity(eventReadId, cd.key, cd.value))
            .toList();
    }
}

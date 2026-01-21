package tech.flowcatalyst.event.mapper;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.event.ContextData;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.event.entity.EventEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between Event domain model and JPA entity.
 */
public final class EventMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper();

    private EventMapper() {
    }

    public static Event toDomain(EventEntity entity) {
        if (entity == null) {
            return null;
        }

        Event domain = new Event();
        domain.id = entity.id;
        domain.specVersion = entity.specVersion;
        domain.type = entity.type;
        domain.source = entity.source;
        domain.subject = entity.subject;
        domain.time = entity.time;
        domain.data = entity.data;
        domain.correlationId = entity.correlationId;
        domain.causationId = entity.causationId;
        domain.deduplicationId = entity.deduplicationId;
        domain.messageGroup = entity.messageGroup;
        domain.contextData = parseContextData(entity.contextDataJson);
        domain.clientId = entity.clientId;
        return domain;
    }

    public static EventEntity toEntity(Event domain) {
        if (domain == null) {
            return null;
        }

        EventEntity entity = new EventEntity();
        entity.id = domain.id;
        entity.specVersion = domain.specVersion;
        entity.type = domain.type;
        entity.source = domain.source;
        entity.subject = domain.subject;
        entity.time = domain.time;
        entity.data = domain.data;
        entity.correlationId = domain.correlationId;
        entity.causationId = domain.causationId;
        entity.deduplicationId = domain.deduplicationId;
        entity.messageGroup = domain.messageGroup;
        entity.contextDataJson = toJson(domain.contextData);
        entity.clientId = domain.clientId;
        return entity;
    }

    public static void updateEntity(EventEntity entity, Event domain) {
        entity.specVersion = domain.specVersion;
        entity.type = domain.type;
        entity.source = domain.source;
        entity.subject = domain.subject;
        entity.time = domain.time;
        entity.data = domain.data;
        entity.correlationId = domain.correlationId;
        entity.causationId = domain.causationId;
        entity.deduplicationId = domain.deduplicationId;
        entity.messageGroup = domain.messageGroup;
        entity.contextDataJson = toJson(domain.contextData);
        entity.clientId = domain.clientId;
    }

    private static List<ContextData> parseContextData(String json) {
        if (json == null || json.isBlank()) {
            return new ArrayList<>();
        }
        try {
            return objectMapper.readValue(json, new TypeReference<List<ContextData>>() {});
        } catch (Exception e) {
            return new ArrayList<>();
        }
    }

    private static String toJson(Object obj) {
        if (obj == null) {
            return null;
        }
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (Exception e) {
            return null;
        }
    }
}

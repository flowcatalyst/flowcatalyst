package tech.flowcatalyst.eventtype.mapper;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.EventTypeStatus;
import tech.flowcatalyst.eventtype.SpecVersion;
import tech.flowcatalyst.eventtype.entity.EventTypeEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between EventType domain model and JPA entity.
 */
public final class EventTypeMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper()
        .registerModule(new JavaTimeModule());

    private EventTypeMapper() {
    }

    public static EventType toDomain(EventTypeEntity entity) {
        if (entity == null) {
            return null;
        }

        return EventType.builder()
            .id(entity.id)
            .code(entity.code)
            .name(entity.name)
            .description(entity.description)
            .specVersions(parseSpecVersions(entity.specVersionsJson))
            .status(entity.status != null ? entity.status : EventTypeStatus.CURRENT)
            .source(entity.source != null ? entity.source : EventTypeSource.UI)
            .clientScoped(entity.clientScoped)
            .application(entity.application)
            .subdomain(entity.subdomain)
            .aggregate(entity.aggregate)
            .createdAt(entity.createdAt)
            .updatedAt(entity.updatedAt)
            .build();
    }

    public static EventTypeEntity toEntity(EventType domain) {
        if (domain == null) {
            return null;
        }

        EventTypeEntity entity = new EventTypeEntity();
        entity.id = domain.id();
        entity.code = domain.code();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.specVersionsJson = toJson(domain.specVersions());
        entity.status = domain.status() != null ? domain.status() : EventTypeStatus.CURRENT;
        entity.source = domain.source() != null ? domain.source() : EventTypeSource.UI;
        entity.clientScoped = domain.clientScoped();
        entity.application = domain.application();
        entity.subdomain = domain.subdomain();
        entity.aggregate = domain.aggregate();
        entity.createdAt = domain.createdAt();
        entity.updatedAt = domain.updatedAt();
        return entity;
    }

    public static void updateEntity(EventTypeEntity entity, EventType domain) {
        entity.code = domain.code();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.specVersionsJson = toJson(domain.specVersions());
        entity.status = domain.status() != null ? domain.status() : EventTypeStatus.CURRENT;
        entity.source = domain.source() != null ? domain.source() : entity.source;
        entity.updatedAt = domain.updatedAt();
    }

    private static List<SpecVersion> parseSpecVersions(String json) {
        if (json == null || json.isBlank()) {
            return new ArrayList<>();
        }
        try {
            return objectMapper.readValue(json, new TypeReference<List<SpecVersion>>() {});
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

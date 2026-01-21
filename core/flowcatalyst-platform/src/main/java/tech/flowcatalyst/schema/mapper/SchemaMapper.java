package tech.flowcatalyst.schema.mapper;

import tech.flowcatalyst.eventtype.SchemaType;
import tech.flowcatalyst.schema.Schema;
import tech.flowcatalyst.schema.entity.SchemaEntity;

/**
 * Mapper for converting between Schema domain model and JPA entity.
 */
public final class SchemaMapper {

    private SchemaMapper() {
    }

    public static Schema toDomain(SchemaEntity entity) {
        if (entity == null) {
            return null;
        }

        return Schema.builder()
            .id(entity.id)
            .name(entity.name)
            .description(entity.description)
            .mimeType(entity.mimeType)
            .schemaType(entity.schemaType != null ? entity.schemaType : SchemaType.JSON_SCHEMA)
            .content(entity.content)
            .eventTypeId(entity.eventTypeId)
            .version(entity.version)
            .createdAt(entity.createdAt)
            .updatedAt(entity.updatedAt)
            .build();
    }

    public static SchemaEntity toEntity(Schema domain) {
        if (domain == null) {
            return null;
        }

        SchemaEntity entity = new SchemaEntity();
        entity.id = domain.id();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.mimeType = domain.mimeType();
        entity.schemaType = domain.schemaType() != null ? domain.schemaType() : SchemaType.JSON_SCHEMA;
        entity.content = domain.content();
        entity.eventTypeId = domain.eventTypeId();
        entity.version = domain.version();
        entity.createdAt = domain.createdAt();
        entity.updatedAt = domain.updatedAt();
        return entity;
    }

    public static void updateEntity(SchemaEntity entity, Schema domain) {
        entity.name = domain.name();
        entity.description = domain.description();
        entity.mimeType = domain.mimeType();
        entity.schemaType = domain.schemaType() != null ? domain.schemaType() : SchemaType.JSON_SCHEMA;
        entity.content = domain.content();
        entity.eventTypeId = domain.eventTypeId();
        entity.version = domain.version();
        entity.updatedAt = domain.updatedAt();
    }
}

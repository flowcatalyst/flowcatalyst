package tech.flowcatalyst.schema.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import tech.flowcatalyst.eventtype.SchemaType;

import java.time.Instant;

/**
 * JPA entity for schemas table.
 */
@Entity
@Table(name = "schemas")
public class SchemaEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "name")
    public String name;

    @Column(name = "description")
    public String description;

    @Column(name = "mime_type")
    public String mimeType;

    @Column(name = "schema_type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public SchemaType schemaType;

    @Column(name = "content", columnDefinition = "TEXT")
    public String content;

    @Column(name = "event_type_id", length = 17)
    public String eventTypeId;

    @Column(name = "version", length = 20)
    public String version;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public SchemaEntity() {
    }
}

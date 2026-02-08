package tech.flowcatalyst.eventtype.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.eventtype.EventTypeSource;
import tech.flowcatalyst.eventtype.EventTypeStatus;

import java.time.Instant;

/**
 * JPA entity for event_types table.
 */
@Entity
@Table(name = "event_types")
public class EventTypeEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, unique = true)
    public String code;

    @Column(name = "name", nullable = false)
    public String name;

    @Column(name = "description")
    public String description;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "spec_versions", columnDefinition = "jsonb")
    public String specVersionsJson;

    @Column(name = "status", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public EventTypeStatus status;

    @Column(name = "source", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public EventTypeSource source;

    @Column(name = "client_scoped", nullable = false)
    public boolean clientScoped;

    @Column(name = "application", nullable = false, length = 100)
    public String application;

    @Column(name = "subdomain", nullable = false, length = 100)
    public String subdomain;

    @Column(name = "aggregate", nullable = false, length = 100)
    public String aggregate;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public EventTypeEntity() {
    }
}

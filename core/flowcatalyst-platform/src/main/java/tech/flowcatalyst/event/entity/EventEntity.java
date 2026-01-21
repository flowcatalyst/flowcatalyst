package tech.flowcatalyst.event.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import java.time.Instant;

/**
 * JPA entity for events table.
 */
@Entity
@Table(name = "events")
public class EventEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "spec_version")
    public String specVersion;

    @Column(name = "type", nullable = false)
    public String type;

    @Column(name = "source", nullable = false)
    public String source;

    @Column(name = "subject")
    public String subject;

    @Column(name = "time")
    public Instant time;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "data", columnDefinition = "jsonb")
    public String data;

    @Column(name = "correlation_id")
    public String correlationId;

    @Column(name = "causation_id")
    public String causationId;

    @Column(name = "deduplication_id")
    public String deduplicationId;

    @Column(name = "message_group")
    public String messageGroup;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "context_data", columnDefinition = "jsonb")
    public String contextDataJson;

    @Column(name = "client_id", length = 17)
    public String clientId;


    public EventEntity() {
    }
}

package tech.flowcatalyst.event.read.jpaentity;

import jakarta.persistence.*;

import java.time.Instant;

/**
 * JPA Entity for events_read table (read-optimized projection).
 */
@Entity
@Table(name = "events_read")
public class EventReadEntity {

    @Id
    @Column(name = "id", length = 13)
    public String id;

    @Column(name = "spec_version", length = 10)
    public String specVersion;

    @Column(name = "type", length = 200)
    public String type;

    // Parsed type segments for filtering
    @Column(name = "application", length = 100)
    public String application;

    @Column(name = "subdomain", length = 100)
    public String subdomain;

    @Column(name = "aggregate", length = 100)
    public String aggregate;

    @Column(name = "source", length = 500)
    public String source;

    @Column(name = "subject", length = 200)
    public String subject;

    @Column(name = "time")
    public Instant time;

    @Column(name = "data", columnDefinition = "TEXT")
    public String data;

    @Column(name = "message_group", length = 200)
    public String messageGroup;

    @Column(name = "correlation_id", length = 100)
    public String correlationId;

    @Column(name = "causation_id", length = 17)
    public String causationId;

    @Column(name = "deduplication_id", length = 100)
    public String deduplicationId;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "projected_at")
    public Instant projectedAt;

    public EventReadEntity() {
    }
}

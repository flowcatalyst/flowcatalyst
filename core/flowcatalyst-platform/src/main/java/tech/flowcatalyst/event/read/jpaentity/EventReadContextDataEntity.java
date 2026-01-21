package tech.flowcatalyst.event.read.jpaentity;

import jakarta.persistence.*;

/**
 * JPA Entity for event_read_context_data table (normalized from contextData array).
 */
@Entity
@Table(name = "event_read_context_data")
public class EventReadContextDataEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    @Column(name = "id")
    public Long id;

    @Column(name = "event_read_id", nullable = false, length = 17)
    public String eventReadId;

    @Column(name = "context_key", nullable = false, length = 100)
    public String key;

    @Column(name = "context_value", length = 1000)
    public String value;

    public EventReadContextDataEntity() {
    }

    public EventReadContextDataEntity(String eventReadId, String key, String value) {
        this.eventReadId = eventReadId;
        this.key = key;
        this.value = value;
    }
}

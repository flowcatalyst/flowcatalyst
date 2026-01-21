package tech.flowcatalyst.subscription.entity;

import jakarta.persistence.*;

/**
 * JPA Entity for subscription event type bindings (normalized from embedded array).
 */
@Entity
@Table(name = "subscription_event_types")
public class SubscriptionEventTypeEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    @Column(name = "id")
    public Long id;

    @Column(name = "subscription_id", nullable = false, length = 17)
    public String subscriptionId;

    @Column(name = "event_type_id", length = 17)
    public String eventTypeId;

    @Column(name = "event_type_code", nullable = false, length = 200)
    public String eventTypeCode;

    @Column(name = "spec_version", length = 50)
    public String specVersion;

    public SubscriptionEventTypeEntity() {
    }

    public SubscriptionEventTypeEntity(String subscriptionId, String eventTypeId, String eventTypeCode, String specVersion) {
        this.subscriptionId = subscriptionId;
        this.eventTypeId = eventTypeId;
        this.eventTypeCode = eventTypeCode;
        this.specVersion = specVersion;
    }
}

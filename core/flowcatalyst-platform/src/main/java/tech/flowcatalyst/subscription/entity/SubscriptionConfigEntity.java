package tech.flowcatalyst.subscription.entity;

import jakarta.persistence.*;

/**
 * JPA Entity for subscription custom config entries (normalized from embedded array).
 */
@Entity
@Table(name = "subscription_configs")
public class SubscriptionConfigEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    @Column(name = "id")
    public Long id;

    @Column(name = "subscription_id", nullable = false, length = 17)
    public String subscriptionId;

    @Column(name = "config_key", nullable = false, length = 100)
    public String key;

    @Column(name = "config_value", length = 1000)
    public String value;

    public SubscriptionConfigEntity() {
    }

    public SubscriptionConfigEntity(String subscriptionId, String key, String value) {
        this.subscriptionId = subscriptionId;
        this.key = key;
        this.value = value;
    }
}

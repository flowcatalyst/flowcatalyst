package tech.flowcatalyst.subscription.entity;

import jakarta.persistence.*;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.subscription.SubscriptionSource;
import tech.flowcatalyst.subscription.SubscriptionStatus;

import java.time.Instant;

/**
 * JPA Entity for Subscription.
 */
@Entity
@Table(name = "subscriptions")
public class SubscriptionEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, length = 100)
    public String code;

    @Column(name = "name", nullable = false, length = 200)
    public String name;

    @Column(name = "description", length = 1000)
    public String description;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "client_identifier", length = 100)
    public String clientIdentifier;

    @Column(name = "client_scoped", nullable = false)
    public boolean clientScoped;

    @Column(name = "target", nullable = false, length = 500)
    public String target;

    @Column(name = "queue", length = 200)
    public String queue;

    @Enumerated(EnumType.STRING)
    @Column(name = "source", nullable = false, length = 20)
    public SubscriptionSource source;

    @Enumerated(EnumType.STRING)
    @Column(name = "status", nullable = false, length = 20)
    public SubscriptionStatus status;

    @Column(name = "max_age_seconds", nullable = false)
    public int maxAgeSeconds;

    @Column(name = "dispatch_pool_id", length = 17)
    public String dispatchPoolId;

    @Column(name = "dispatch_pool_code", length = 100)
    public String dispatchPoolCode;

    @Column(name = "delay_seconds", nullable = false)
    public int delaySeconds;

    @Column(name = "sequence", nullable = false)
    public int sequence;

    @Enumerated(EnumType.STRING)
    @Column(name = "mode", nullable = false, length = 30)
    public DispatchMode mode;

    @Column(name = "timeout_seconds", nullable = false)
    public int timeoutSeconds;

    @Column(name = "max_retries", nullable = false)
    public int maxRetries;

    @Column(name = "service_account_id", length = 17)
    public String serviceAccountId;

    @Column(name = "data_only", nullable = false)
    public boolean dataOnly;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public SubscriptionEntity() {
    }
}

package tech.flowcatalyst.dispatchpool.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import jakarta.persistence.UniqueConstraint;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import java.time.Instant;

/**
 * JPA entity for dispatch_pools table.
 */
@Entity
@Table(name = "dispatch_pools",
    uniqueConstraints = @UniqueConstraint(columnNames = {"client_id", "code"}))
public class DispatchPoolEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, length = 100)
    public String code;

    @Column(name = "name")
    public String name;

    @Column(name = "description", columnDefinition = "TEXT")
    public String description;

    @Column(name = "rate_limit", nullable = false)
    public int rateLimit;

    @Column(name = "concurrency", nullable = false)
    public int concurrency;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "client_identifier", length = 100)
    public String clientIdentifier;

    @Column(name = "status", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public DispatchPoolStatus status;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public DispatchPoolEntity() {
    }
}

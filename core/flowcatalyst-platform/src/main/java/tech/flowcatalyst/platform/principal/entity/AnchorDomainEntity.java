package tech.flowcatalyst.platform.principal.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;

/**
 * JPA entity for anchor_domains table.
 * Separate from domain model to keep persistence concerns isolated.
 */
@Entity
@Table(name = "anchor_domains")
public class AnchorDomainEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "domain", nullable = false, unique = true)
    public String domain;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    public AnchorDomainEntity() {
    }

    public AnchorDomainEntity(String id, String domain, Instant createdAt) {
        this.id = id;
        this.domain = domain;
        this.createdAt = createdAt;
    }
}

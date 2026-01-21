package tech.flowcatalyst.platform.cors.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;

/**
 * JPA entity for cors_allowed_origins table.
 */
@Entity
@Table(name = "cors_allowed_origins")
public class CorsAllowedOriginEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "origin", nullable = false, unique = true)
    public String origin;

    @Column(name = "description")
    public String description;

    @Column(name = "created_by")
    public String createdBy;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    public CorsAllowedOriginEntity() {
    }

    public CorsAllowedOriginEntity(String id, String origin, String description, String createdBy, Instant createdAt) {
        this.id = id;
        this.origin = origin;
        this.description = description;
        this.createdBy = createdBy;
        this.createdAt = createdAt;
    }
}

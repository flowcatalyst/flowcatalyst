package tech.flowcatalyst.platform.config.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;

import java.time.Instant;

/**
 * JPA entity for platform_config_access table.
 */
@Entity
@Table(name = "platform_config_access")
public class PlatformConfigAccessEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "application_code", nullable = false, length = 100)
    public String applicationCode;

    @Column(name = "role_code", nullable = false, length = 200)
    public String roleCode;

    @Column(name = "can_read", nullable = false)
    public boolean canRead;

    @Column(name = "can_write", nullable = false)
    public boolean canWrite;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    public PlatformConfigAccessEntity() {
    }
}

package tech.flowcatalyst.platform.config.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import tech.flowcatalyst.platform.config.ConfigScope;
import tech.flowcatalyst.platform.config.ConfigValueType;

import java.time.Instant;

/**
 * JPA entity for platform_configs table.
 */
@Entity
@Table(name = "platform_configs")
public class PlatformConfigEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "application_code", nullable = false, length = 100)
    public String applicationCode;

    @Column(name = "section", nullable = false, length = 100)
    public String section;

    @Column(name = "property", nullable = false, length = 100)
    public String property;

    @Column(name = "scope", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public ConfigScope scope;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "value_type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public ConfigValueType valueType;

    @Column(name = "value", nullable = false, columnDefinition = "TEXT")
    public String value;

    @Column(name = "description", columnDefinition = "TEXT")
    public String description;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public PlatformConfigEntity() {
    }
}

package tech.flowcatalyst.platform.application.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import jakarta.persistence.UniqueConstraint;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import java.time.Instant;

/**
 * JPA entity for application_client_configs table.
 */
@Entity
@Table(name = "application_client_configs",
    uniqueConstraints = @UniqueConstraint(columnNames = {"application_id", "client_id"}))
public class ApplicationClientConfigEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "application_id", nullable = false, length = 17)
    public String applicationId;

    @Column(name = "client_id", nullable = false, length = 17)
    public String clientId;

    @Column(name = "enabled", nullable = false)
    public boolean enabled;

    @Column(name = "base_url_override", length = 500)
    public String baseUrlOverride;

    @Column(name = "website_override", length = 500)
    public String websiteOverride;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "config_json", columnDefinition = "jsonb")
    public String configJson;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public ApplicationClientConfigEntity() {
    }
}

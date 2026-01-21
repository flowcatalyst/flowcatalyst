package tech.flowcatalyst.platform.application.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import tech.flowcatalyst.platform.application.Application;
import java.time.Instant;

/**
 * JPA entity for applications table.
 */
@Entity
@Table(name = "applications")
public class ApplicationEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, unique = true, length = 100)
    public String code;

    @Column(name = "name", nullable = false)
    public String name;

    @Column(name = "description", columnDefinition = "TEXT")
    public String description;

    @Column(name = "type", nullable = false, length = 50)
    @Enumerated(EnumType.STRING)
    public Application.ApplicationType type;

    @Column(name = "default_base_url", length = 500)
    public String defaultBaseUrl;

    @Column(name = "service_account_id", length = 17)
    public String serviceAccountId;

    @Column(name = "active", nullable = false)
    public boolean active;

    @Column(name = "icon_url", length = 500)
    public String iconUrl;

    @Column(name = "website", length = 500)
    public String website;

    @Column(name = "logo", columnDefinition = "TEXT")
    public String logo;

    @Column(name = "logo_mime_type", length = 100)
    public String logoMimeType;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public ApplicationEntity() {
    }
}

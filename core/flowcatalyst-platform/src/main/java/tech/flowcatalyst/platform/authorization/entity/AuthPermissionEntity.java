package tech.flowcatalyst.platform.authorization.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import tech.flowcatalyst.platform.authorization.AuthPermission;
import java.time.Instant;

/**
 * JPA entity for auth_permissions table.
 */
@Entity
@Table(name = "auth_permissions")
public class AuthPermissionEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "name", nullable = false, unique = true)
    public String name;

    @Column(name = "display_name")
    public String displayName;

    @Column(name = "description")
    public String description;

    @Column(name = "source", nullable = false)
    @Enumerated(EnumType.STRING)
    public AuthPermission.PermissionSource source;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    public AuthPermissionEntity() {
    }

    public AuthPermissionEntity(String id, String applicationId, String name, String displayName,
                                 String description, AuthPermission.PermissionSource source, Instant createdAt) {
        this.id = id;
        this.applicationId = applicationId;
        this.name = name;
        this.displayName = displayName;
        this.description = description;
        this.source = source;
        this.createdAt = createdAt;
    }
}

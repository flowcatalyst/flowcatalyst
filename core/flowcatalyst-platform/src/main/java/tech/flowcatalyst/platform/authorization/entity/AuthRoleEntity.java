package tech.flowcatalyst.platform.authorization.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.platform.authorization.AuthRole;
import java.time.Instant;
import java.util.Set;

/**
 * JPA entity for auth_roles table.
 */
@Entity
@Table(name = "auth_roles")
public class AuthRoleEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "application_code", length = 100)
    public String applicationCode;

    @Column(name = "name", nullable = false, length = 100)
    public String name;

    @Column(name = "display_name")
    public String displayName;

    @Column(name = "description", columnDefinition = "TEXT")
    public String description;

    @Column(name = "permissions", columnDefinition = "TEXT[]")
    @JdbcTypeCode(SqlTypes.ARRAY)
    public String[] permissions;

    @Column(name = "source", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public AuthRole.RoleSource source;

    @Column(name = "client_managed", nullable = false)
    public boolean clientManaged;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public AuthRoleEntity() {
    }
}

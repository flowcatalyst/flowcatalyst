package tech.flowcatalyst.platform.principal.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.IdClass;
import jakarta.persistence.Table;
import java.io.Serializable;
import java.time.Instant;
import java.util.Objects;

/**
 * JPA entity for principal_roles junction table.
 *
 * <p>This is the normalized table created in V8__normalize_principal_roles.sql.
 * It replaces the JSONB roles column in principals table.
 */
@Entity
@Table(name = "principal_roles")
@IdClass(PrincipalRoleEntity.PrincipalRoleId.class)
public class PrincipalRoleEntity {

    @Id
    @Column(name = "principal_id", length = 17)
    public String principalId;

    @Id
    @Column(name = "role_name", length = 100)
    public String roleName;

    @Column(name = "assignment_source", length = 50)
    public String assignmentSource;

    @Column(name = "assigned_at", nullable = false)
    public Instant assignedAt;

    public PrincipalRoleEntity() {
    }

    public PrincipalRoleEntity(String principalId, String roleName, String assignmentSource, Instant assignedAt) {
        this.principalId = principalId;
        this.roleName = roleName;
        this.assignmentSource = assignmentSource;
        this.assignedAt = assignedAt != null ? assignedAt : Instant.now();
    }

    /**
     * Composite primary key for principal_roles.
     */
    public static class PrincipalRoleId implements Serializable {
        public String principalId;
        public String roleName;

        public PrincipalRoleId() {
        }

        public PrincipalRoleId(String principalId, String roleName) {
            this.principalId = principalId;
            this.roleName = roleName;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (o == null || getClass() != o.getClass()) return false;
            PrincipalRoleId that = (PrincipalRoleId) o;
            return Objects.equals(principalId, that.principalId) &&
                   Objects.equals(roleName, that.roleName);
        }

        @Override
        public int hashCode() {
            return Objects.hash(principalId, roleName);
        }
    }
}

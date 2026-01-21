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
 * JPA entity for principal_managed_applications junction table.
 *
 * <p>Maps principals to the applications they can manage.
 * Used in conjunction with ManagedApplicationScope.SPECIFIC to determine
 * which applications a principal can create roles, permissions, event types, etc. for.
 */
@Entity
@Table(name = "principal_managed_applications")
@IdClass(PrincipalManagedApplicationEntity.PrincipalManagedApplicationId.class)
public class PrincipalManagedApplicationEntity {

    @Id
    @Column(name = "principal_id", length = 17)
    public String principalId;

    @Id
    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "granted_at", nullable = false)
    public Instant grantedAt;

    public PrincipalManagedApplicationEntity() {
    }

    public PrincipalManagedApplicationEntity(String principalId, String applicationId) {
        this.principalId = principalId;
        this.applicationId = applicationId;
        this.grantedAt = Instant.now();
    }

    public PrincipalManagedApplicationEntity(String principalId, String applicationId, Instant grantedAt) {
        this.principalId = principalId;
        this.applicationId = applicationId;
        this.grantedAt = grantedAt != null ? grantedAt : Instant.now();
    }

    /**
     * Composite primary key for principal_managed_applications.
     */
    public static class PrincipalManagedApplicationId implements Serializable {
        public String principalId;
        public String applicationId;

        public PrincipalManagedApplicationId() {
        }

        public PrincipalManagedApplicationId(String principalId, String applicationId) {
            this.principalId = principalId;
            this.applicationId = applicationId;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (o == null || getClass() != o.getClass()) return false;
            PrincipalManagedApplicationId that = (PrincipalManagedApplicationId) o;
            return Objects.equals(principalId, that.principalId) &&
                   Objects.equals(applicationId, that.applicationId);
        }

        @Override
        public int hashCode() {
            return Objects.hash(principalId, applicationId);
        }
    }
}

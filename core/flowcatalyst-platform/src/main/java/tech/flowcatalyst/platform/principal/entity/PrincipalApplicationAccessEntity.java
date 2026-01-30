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
 * JPA entity for principal_application_access junction table.
 *
 * <p>Maps principals (users) to the applications they can access.
 * This is the new model replacing the managed application scope.
 * Users get no applications by default - each must be explicitly granted.
 */
@Entity
@Table(name = "principal_application_access")
@IdClass(PrincipalApplicationAccessEntity.PrincipalApplicationAccessId.class)
public class PrincipalApplicationAccessEntity {

    @Id
    @Column(name = "principal_id", length = 17)
    public String principalId;

    @Id
    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "granted_at", nullable = false)
    public Instant grantedAt;

    public PrincipalApplicationAccessEntity() {
    }

    public PrincipalApplicationAccessEntity(String principalId, String applicationId) {
        this.principalId = principalId;
        this.applicationId = applicationId;
        this.grantedAt = Instant.now();
    }

    public PrincipalApplicationAccessEntity(String principalId, String applicationId, Instant grantedAt) {
        this.principalId = principalId;
        this.applicationId = applicationId;
        this.grantedAt = grantedAt != null ? grantedAt : Instant.now();
    }

    /**
     * Composite primary key for principal_application_access.
     */
    public static class PrincipalApplicationAccessId implements Serializable {
        public String principalId;
        public String applicationId;

        public PrincipalApplicationAccessId() {
        }

        public PrincipalApplicationAccessId(String principalId, String applicationId) {
            this.principalId = principalId;
            this.applicationId = applicationId;
        }

        @Override
        public boolean equals(Object o) {
            if (this == o) return true;
            if (o == null || getClass() != o.getClass()) return false;
            PrincipalApplicationAccessId that = (PrincipalApplicationAccessId) o;
            return Objects.equals(principalId, that.principalId) &&
                   Objects.equals(applicationId, that.applicationId);
        }

        @Override
        public int hashCode() {
            return Objects.hash(principalId, applicationId);
        }
    }
}

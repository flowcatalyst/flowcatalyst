package tech.flowcatalyst.serviceaccount.jpaentity;

import jakarta.persistence.*;

import java.time.Instant;

/**
 * JPA Entity for service_account_roles table (normalized from roles array).
 */
@Entity
@Table(name = "service_account_roles")
public class ServiceAccountRoleEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    @Column(name = "id")
    public Long id;

    @Column(name = "service_account_id", nullable = false, length = 17)
    public String serviceAccountId;

    @Column(name = "role_name", nullable = false, length = 100)
    public String roleName;

    @Column(name = "assignment_source", length = 50)
    public String assignmentSource;

    @Column(name = "assigned_at", nullable = false)
    public Instant assignedAt;

    public ServiceAccountRoleEntity() {
    }

    public ServiceAccountRoleEntity(String serviceAccountId, String roleName, String assignmentSource, Instant assignedAt) {
        this.serviceAccountId = serviceAccountId;
        this.roleName = roleName;
        this.assignmentSource = assignmentSource;
        this.assignedAt = assignedAt;
    }
}

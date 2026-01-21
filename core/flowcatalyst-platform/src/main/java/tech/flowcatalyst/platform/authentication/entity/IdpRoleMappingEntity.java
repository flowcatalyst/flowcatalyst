package tech.flowcatalyst.platform.authentication.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;

/**
 * JPA entity for idp_role_mappings table.
 */
@Entity
@Table(name = "idp_role_mappings")
public class IdpRoleMappingEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "idp_role_name", nullable = false, unique = true)
    public String idpRoleName;

    @Column(name = "internal_role_name", nullable = false)
    public String internalRoleName;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    public IdpRoleMappingEntity() {
    }

    public IdpRoleMappingEntity(String id, String idpRoleName, String internalRoleName, Instant createdAt) {
        this.id = id;
        this.idpRoleName = idpRoleName;
        this.internalRoleName = internalRoleName;
        this.createdAt = createdAt;
    }
}

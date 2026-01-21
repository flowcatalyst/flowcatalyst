package tech.flowcatalyst.platform.principal.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.principal.ManagedApplicationScope;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserScope;
import java.time.Instant;

/**
 * JPA entity for principals table.
 *
 * <p>Used for both Panache simple CRUD and QueryDSL complex queries.
 * Note: The roles column is JSONB in the DB but we store it as String here.
 * The normalized principal_roles table is the source of truth for roles.
 */
@Entity
@Table(name = "principals")
public class PrincipalEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public PrincipalType type;

    @Column(name = "scope", length = 20)
    @Enumerated(EnumType.STRING)
    public UserScope scope;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "managed_application_scope", length = 20)
    @Enumerated(EnumType.STRING)
    public ManagedApplicationScope managedApplicationScope;

    @Column(name = "name", nullable = false)
    public String name;

    @Column(name = "active", nullable = false)
    public boolean active;

    // UserIdentity fields (embedded as flat columns)
    @Column(name = "email")
    public String email;

    @Column(name = "email_domain", length = 100)
    public String emailDomain;

    @Column(name = "idp_type", length = 50)
    @Enumerated(EnumType.STRING)
    public IdpType idpType;

    @Column(name = "external_idp_id")
    public String externalIdpId;

    @Column(name = "password_hash")
    public String passwordHash;

    @Column(name = "last_login_at")
    public Instant lastLoginAt;

    // ServiceAccount embedded as JSONB
    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "service_account", columnDefinition = "jsonb")
    public String serviceAccount;

    // Note: roles column has been dropped - use principal_roles table via PrincipalRoleEntity

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public PrincipalEntity() {
    }
}

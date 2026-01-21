package tech.flowcatalyst.platform.client.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.client.AuthConfigType;
import java.time.Instant;

/**
 * JPA entity for client_auth_configs table.
 */
@Entity
@Table(name = "client_auth_configs")
public class ClientAuthConfigEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "email_domain", nullable = false, unique = true)
    public String emailDomain;

    @Column(name = "config_type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public AuthConfigType configType;

    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "primary_client_id", length = 17)
    public String primaryClientId;

    @Column(name = "additional_client_ids", columnDefinition = "TEXT[]")
    @JdbcTypeCode(SqlTypes.ARRAY)
    public String[] additionalClientIds;

    @Column(name = "granted_client_ids", columnDefinition = "TEXT[]")
    @JdbcTypeCode(SqlTypes.ARRAY)
    public String[] grantedClientIds;

    @Column(name = "auth_provider", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public AuthProvider authProvider;

    @Column(name = "oidc_issuer_url", length = 500)
    public String oidcIssuerUrl;

    @Column(name = "oidc_client_id", length = 200)
    public String oidcClientId;

    @Column(name = "oidc_client_secret_ref", length = 500)
    public String oidcClientSecretRef;

    @Column(name = "oidc_multi_tenant")
    public boolean oidcMultiTenant;

    @Column(name = "oidc_issuer_pattern", length = 500)
    public String oidcIssuerPattern;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public ClientAuthConfigEntity() {
    }
}

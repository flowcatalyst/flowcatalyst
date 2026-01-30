package tech.flowcatalyst.platform.authentication.idp.entity;

import jakarta.persistence.CollectionTable;
import jakarta.persistence.Column;
import jakarta.persistence.ElementCollection;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.Table;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * JPA entity for identity_providers table.
 */
@Entity
@Table(name = "identity_providers")
public class IdentityProviderEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, unique = true, length = 50)
    public String code;

    @Column(name = "name", nullable = false, length = 200)
    public String name;

    @Column(name = "type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public IdentityProviderType type;

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

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(
        name = "identity_provider_allowed_domains",
        joinColumns = @JoinColumn(name = "identity_provider_id")
    )
    @Column(name = "email_domain", length = 255)
    public List<String> allowedEmailDomains = new ArrayList<>();

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public IdentityProviderEntity() {
    }
}

package tech.flowcatalyst.platform.authentication.domain.entity;

import jakarta.persistence.CollectionTable;
import jakarta.persistence.Column;
import jakarta.persistence.ElementCollection;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.FetchType;
import jakarta.persistence.Id;
import jakarta.persistence.Index;
import jakarta.persistence.JoinColumn;
import jakarta.persistence.Table;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * JPA entity for email_domain_mappings table.
 */
@Entity
@Table(name = "email_domain_mappings", indexes = {
    @Index(name = "idx_edm_idp", columnList = "identity_provider_id")
})
public class EmailDomainMappingEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "email_domain", nullable = false, unique = true, length = 255)
    public String emailDomain;

    @Column(name = "identity_provider_id", nullable = false, length = 17)
    public String identityProviderId;

    @Column(name = "scope_type", nullable = false, length = 20)
    @Enumerated(EnumType.STRING)
    public ScopeType scopeType;

    @Column(name = "primary_client_id", length = 17)
    public String primaryClientId;

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(
        name = "email_domain_mapping_additional_clients",
        joinColumns = @JoinColumn(name = "email_domain_mapping_id")
    )
    @Column(name = "client_id", length = 17)
    public List<String> additionalClientIds = new ArrayList<>();

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(
        name = "email_domain_mapping_granted_clients",
        joinColumns = @JoinColumn(name = "email_domain_mapping_id")
    )
    @Column(name = "client_id", length = 17)
    public List<String> grantedClientIds = new ArrayList<>();

    @Column(name = "required_oidc_tenant_id", length = 100)
    public String requiredOidcTenantId;

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(
        name = "email_domain_mapping_allowed_roles",
        joinColumns = @JoinColumn(name = "email_domain_mapping_id")
    )
    @Column(name = "role_id", length = 17)
    public List<String> allowedRoleIds = new ArrayList<>();

    @Column(name = "sync_roles_from_idp", nullable = false)
    public boolean syncRolesFromIdp = false;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public EmailDomainMappingEntity() {
    }
}

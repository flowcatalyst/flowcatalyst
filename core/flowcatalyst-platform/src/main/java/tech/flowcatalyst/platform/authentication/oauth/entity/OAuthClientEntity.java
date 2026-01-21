package tech.flowcatalyst.platform.authentication.oauth.entity;

import jakarta.persistence.*;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient.ClientType;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * JPA Entity for OAuth clients.
 */
@Entity
@Table(name = "oauth_clients")
public class OAuthClientEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "client_id", nullable = false, unique = true, length = 50)
    public String clientId;

    @Column(name = "client_name", nullable = false, length = 200)
    public String clientName;

    @Enumerated(EnumType.STRING)
    @Column(name = "client_type", nullable = false, length = 20)
    public ClientType clientType;

    @Column(name = "client_secret_ref", length = 500)
    public String clientSecretRef;

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(name = "oauth_client_redirect_uris", joinColumns = @JoinColumn(name = "oauth_client_id"))
    @Column(name = "redirect_uri", length = 500)
    public List<String> redirectUris = new ArrayList<>();

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(name = "oauth_client_allowed_origins", joinColumns = @JoinColumn(name = "oauth_client_id"))
    @Column(name = "allowed_origin", length = 200)
    public List<String> allowedOrigins = new ArrayList<>();

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(name = "oauth_client_grant_types", joinColumns = @JoinColumn(name = "oauth_client_id"))
    @Column(name = "grant_type", length = 50)
    public List<String> grantTypes = new ArrayList<>();

    @Column(name = "default_scopes", length = 500)
    public String defaultScopes;

    @Column(name = "pkce_required", nullable = false)
    public boolean pkceRequired = true;

    @ElementCollection(fetch = FetchType.EAGER)
    @CollectionTable(name = "oauth_client_application_ids", joinColumns = @JoinColumn(name = "oauth_client_id"))
    @Column(name = "application_id", length = 17)
    public List<String> applicationIds = new ArrayList<>();

    @Column(name = "service_account_principal_id", length = 17)
    public String serviceAccountPrincipalId;

    @Column(name = "active", nullable = false)
    public boolean active = true;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public OAuthClientEntity() {
    }
}

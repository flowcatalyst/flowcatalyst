package tech.flowcatalyst.platform.authentication.oidc.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;

import java.time.Instant;

/**
 * JPA entity for oidc_login_states table.
 */
@Entity
@Table(name = "oidc_login_states")
public class OidcLoginStateEntity {

    @Id
    @Column(name = "state", length = 64)
    public String state;

    @Column(name = "email_domain", nullable = false)
    public String emailDomain;

    @Column(name = "identity_provider_id", length = 17)
    public String identityProviderId;

    @Column(name = "email_domain_mapping_id", length = 17)
    public String emailDomainMappingId;

    @Column(name = "nonce", length = 64)
    public String nonce;

    @Column(name = "code_verifier", length = 128)
    public String codeVerifier;

    @Column(name = "return_url", length = 2000)
    public String returnUrl;

    @Column(name = "oauth_client_id", length = 17)
    public String oauthClientId;

    @Column(name = "oauth_redirect_uri", length = 2000)
    public String oauthRedirectUri;

    @Column(name = "oauth_scope", length = 1000)
    public String oauthScope;

    @Column(name = "oauth_state", length = 256)
    public String oauthState;

    @Column(name = "oauth_code_challenge", length = 128)
    public String oauthCodeChallenge;

    @Column(name = "oauth_code_challenge_method", length = 10)
    public String oauthCodeChallengeMethod;

    @Column(name = "oauth_nonce", length = 256)
    public String oauthNonce;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "expires_at", nullable = false)
    public Instant expiresAt;

    public OidcLoginStateEntity() {
    }
}

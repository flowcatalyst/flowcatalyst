package tech.flowcatalyst.platform.authentication.oauth.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;

import java.time.Instant;

/**
 * JPA entity for authorization_codes table.
 */
@Entity
@Table(name = "authorization_codes")
public class AuthorizationCodeEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, unique = true, length = 64)
    public String code;

    @Column(name = "client_id", nullable = false, length = 17)
    public String clientId;

    @Column(name = "principal_id", nullable = false, length = 17)
    public String principalId;

    @Column(name = "redirect_uri", nullable = false, length = 2000)
    public String redirectUri;

    @Column(name = "scope", length = 1000)
    public String scope;

    @Column(name = "code_challenge", length = 128)
    public String codeChallenge;

    @Column(name = "code_challenge_method", length = 10)
    public String codeChallengeMethod;

    @Column(name = "nonce", length = 256)
    public String nonce;

    @Column(name = "state", length = 256)
    public String state;

    @Column(name = "context_client_id", length = 17)
    public String contextClientId;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "expires_at", nullable = false)
    public Instant expiresAt;

    @Column(name = "used", nullable = false)
    public boolean used;

    public AuthorizationCodeEntity() {
    }
}

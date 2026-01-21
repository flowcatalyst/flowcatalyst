package tech.flowcatalyst.platform.authentication.oauth.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import java.time.Instant;

/**
 * JPA entity for refresh_tokens table.
 */
@Entity
@Table(name = "refresh_tokens")
public class RefreshTokenEntity {

    @Id
    @Column(name = "token_hash", length = 64)
    public String tokenHash;

    @Column(name = "principal_id", nullable = false, length = 17)
    public String principalId;

    @Column(name = "client_id", nullable = false)
    public String clientId;

    @Column(name = "context_client_id", length = 17)
    public String contextClientId;

    @Column(name = "scope")
    public String scope;

    @Column(name = "token_family", nullable = false)
    public String tokenFamily;

    @Column(name = "revoked", nullable = false)
    public boolean revoked;

    @Column(name = "revoked_at")
    public Instant revokedAt;

    @Column(name = "replaced_by", length = 64)
    public String replacedBy;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "expires_at", nullable = false)
    public Instant expiresAt;

    public RefreshTokenEntity() {
    }
}

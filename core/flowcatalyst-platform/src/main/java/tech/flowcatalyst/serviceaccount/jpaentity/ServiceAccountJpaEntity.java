package tech.flowcatalyst.serviceaccount.jpaentity;

import jakarta.persistence.*;
import tech.flowcatalyst.dispatchjob.model.SignatureAlgorithm;
import tech.flowcatalyst.serviceaccount.entity.WebhookAuthType;

import java.time.Instant;

/**
 * JPA Entity for service_accounts table.
 * Webhook credentials are stored as embedded columns.
 */
@Entity
@Table(name = "service_accounts")
public class ServiceAccountJpaEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "code", nullable = false, unique = true, length = 100)
    public String code;

    @Column(name = "name", nullable = false, length = 200)
    public String name;

    @Column(name = "description", length = 500)
    public String description;

    @Column(name = "application_id", length = 17)
    public String applicationId;

    @Column(name = "active", nullable = false)
    public boolean active = true;

    // Embedded webhook credentials (wh_ prefix in DB)
    @Enumerated(EnumType.STRING)
    @Column(name = "wh_auth_type", length = 50)
    public WebhookAuthType webhookAuthType = WebhookAuthType.BEARER_TOKEN;

    @Column(name = "wh_auth_token_ref", length = 500)
    public String webhookAuthTokenRef;

    @Column(name = "wh_signing_secret_ref", length = 500)
    public String webhookSigningSecretRef;

    @Enumerated(EnumType.STRING)
    @Column(name = "wh_signing_algorithm", length = 50)
    public SignatureAlgorithm webhookSigningAlgorithm = SignatureAlgorithm.HMAC_SHA256;

    @Column(name = "wh_credentials_created_at")
    public Instant webhookCredentialsCreatedAt;

    @Column(name = "wh_credentials_regenerated_at")
    public Instant webhookCredentialsRegeneratedAt;

    @Column(name = "last_used_at")
    public Instant lastUsedAt;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public ServiceAccountJpaEntity() {
    }
}

package tech.flowcatalyst.dispatchjob.jpaentity;

import jakarta.persistence.*;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchProtocol;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;

import java.time.Instant;

/**
 * JPA Entity for dispatch_jobs table.
 */
@Entity
@Table(name = "dispatch_jobs")
public class DispatchJobJpaEntity {

    @Id
    @Column(name = "id", length = 13)
    public String id;

    @Column(name = "external_id", length = 100)
    public String externalId;

    // Classification Fields
    @Column(name = "source", length = 100)
    public String source;

    @Enumerated(EnumType.STRING)
    @Column(name = "kind", nullable = false, length = 20)
    public DispatchKind kind = DispatchKind.EVENT;

    @Column(name = "code", nullable = false, length = 200)
    public String code;

    @Column(name = "subject", length = 200)
    public String subject;

    @Column(name = "event_id", length = 13)
    public String eventId;

    @Column(name = "correlation_id", length = 100)
    public String correlationId;

    // Metadata as JSONB
    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "metadata", columnDefinition = "jsonb")
    public String metadataJson;

    // Target Information
    @Column(name = "target_url", nullable = false, length = 2048)
    public String targetUrl;

    @Enumerated(EnumType.STRING)
    @Column(name = "protocol", nullable = false, length = 30)
    public DispatchProtocol protocol = DispatchProtocol.HTTP_WEBHOOK;

    // Payload
    @Column(name = "payload", columnDefinition = "TEXT")
    public String payload;

    @Column(name = "payload_content_type", length = 100)
    public String payloadContentType = "application/json";

    @Column(name = "data_only", nullable = false)
    public boolean dataOnly = true;

    // Credentials Reference
    @Column(name = "service_account_id", length = 17)
    public String serviceAccountId;

    // Context
    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "subscription_id", length = 17)
    public String subscriptionId;

    // Dispatch Behavior
    @Enumerated(EnumType.STRING)
    @Column(name = "mode", nullable = false, length = 30)
    public DispatchMode mode = DispatchMode.IMMEDIATE;

    @Column(name = "dispatch_pool_id", length = 17)
    public String dispatchPoolId;

    @Column(name = "message_group", length = 200)
    public String messageGroup;

    @Column(name = "sequence", nullable = false)
    public int sequence = 99;

    @Column(name = "timeout_seconds", nullable = false)
    public int timeoutSeconds = 30;

    // Schema Reference
    @Column(name = "schema_id", length = 17)
    public String schemaId;

    // Execution Control
    @Enumerated(EnumType.STRING)
    @Column(name = "status", nullable = false, length = 20)
    public DispatchStatus status = DispatchStatus.PENDING;

    @Column(name = "max_retries")
    public Integer maxRetries = 3;

    @Column(name = "retry_strategy", length = 50)
    public String retryStrategy = "exponential";

    @Column(name = "scheduled_for")
    public Instant scheduledFor;

    @Column(name = "expires_at")
    public Instant expiresAt;

    // Tracking
    @Column(name = "attempt_count")
    public Integer attemptCount = 0;

    @Column(name = "last_attempt_at")
    public Instant lastAttemptAt;

    @Column(name = "completed_at")
    public Instant completedAt;

    @Column(name = "duration_millis")
    public Long durationMillis;

    @Column(name = "last_error", columnDefinition = "TEXT")
    public String lastError;

    // Idempotency
    @Column(name = "idempotency_key", length = 100)
    public String idempotencyKey;

    // Timestamps
    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public DispatchJobJpaEntity() {
    }
}

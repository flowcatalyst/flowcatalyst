package tech.flowcatalyst.dispatchjob.read.jpaentity;

import jakarta.persistence.*;

import java.time.Instant;

/**
 * JPA Entity for dispatch_jobs_read table (read-optimized projection).
 */
@Entity
@Table(name = "dispatch_jobs_read")
public class DispatchJobReadEntity {

    @Id
    @Column(name = "id", length = 13)
    public String id;

    // Note: dispatch_job_id was removed - the id field IS the dispatch job ID (no prefix for performance)

    // Core identifiers
    @Column(name = "external_id", length = 100)
    public String externalId;

    @Column(name = "source", length = 100)
    public String source;

    @Column(name = "kind", length = 20)
    public String kind;

    @Column(name = "code", length = 200)
    public String code;

    @Column(name = "subject", length = 200)
    public String subject;

    // Parsed code segments for filtering
    @Column(name = "application", length = 100)
    public String application;

    @Column(name = "subdomain", length = 100)
    public String subdomain;

    @Column(name = "aggregate", length = 100)
    public String aggregate;

    @Column(name = "event_id", length = 17)
    public String eventId;

    @Column(name = "correlation_id", length = 100)
    public String correlationId;

    // Target
    @Column(name = "target_url", length = 2048)
    public String targetUrl;

    @Column(name = "protocol", length = 30)
    public String protocol;

    // Context
    @Column(name = "client_id", length = 17)
    public String clientId;

    @Column(name = "subscription_id", length = 17)
    public String subscriptionId;

    @Column(name = "service_account_id", length = 17)
    public String serviceAccountId;

    @Column(name = "dispatch_pool_id", length = 17)
    public String dispatchPoolId;

    @Column(name = "message_group", length = 200)
    public String messageGroup;

    @Column(name = "mode", length = 30)
    public String mode;

    @Column(name = "sequence")
    public Integer sequence;

    // Status tracking
    @Column(name = "status", length = 20)
    public String status;

    @Column(name = "attempt_count")
    public Integer attemptCount;

    @Column(name = "max_retries")
    public Integer maxRetries;

    @Column(name = "last_error", columnDefinition = "TEXT")
    public String lastError;

    // Timing
    @Column(name = "timeout_seconds")
    public Integer timeoutSeconds;

    @Column(name = "retry_strategy", length = 50)
    public String retryStrategy;

    // Timestamps
    @Column(name = "created_at")
    public Instant createdAt;

    @Column(name = "updated_at")
    public Instant updatedAt;

    @Column(name = "scheduled_for")
    public Instant scheduledFor;

    @Column(name = "expires_at")
    public Instant expiresAt;

    @Column(name = "completed_at")
    public Instant completedAt;

    @Column(name = "last_attempt_at")
    public Instant lastAttemptAt;

    @Column(name = "duration_millis")
    public Long durationMillis;

    // Idempotency
    @Column(name = "idempotency_key", length = 100)
    public String idempotencyKey;

    // Computed fields
    @Column(name = "is_completed")
    public Boolean isCompleted;

    @Column(name = "is_terminal")
    public Boolean isTerminal;

    @Column(name = "projected_at")
    public Instant projectedAt;

    public DispatchJobReadEntity() {
    }
}

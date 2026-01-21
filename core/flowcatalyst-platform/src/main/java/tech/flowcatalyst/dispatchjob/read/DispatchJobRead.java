package tech.flowcatalyst.dispatchjob.read;

import java.time.Instant;

/**
 * Read-optimized projection of dispatch jobs from the dispatch_jobs_read collection.
 *
 * This collection is populated by the stream processor and has rich indexes
 * for efficient querying. The raw dispatch_jobs collection is write-optimized.
 *
 * This is a "light" projection that excludes large fields:
 * - payload (can be large)
 * - headers (variable size map)
 * - attempts (list that grows over time)
 * - metadata (variable size list)
 */

public class DispatchJobRead {

    public String id;

    // Note: dispatchJobId was removed - the id field IS the dispatch job ID

    // Core identifiers
    public String externalId;
    public String source;
    public String kind;
    public String code;
    public String subject;

    /**
     * Application code - first segment of code.
     * Parsed from code (format: app:subdomain:aggregate:event) for efficient filtering.
     */
    public String application;

    /**
     * Subdomain - second segment of code.
     * Parsed from code for efficient filtering.
     */
    public String subdomain;

    /**
     * Aggregate - third segment of code.
     * Parsed from code for efficient filtering.
     */
    public String aggregate;
    public String eventId;
    public String correlationId;

    // Target (URL only, no headers)
    public String targetUrl;
    public String protocol;

    // Context
    public String clientId;
    public String subscriptionId;
    public String serviceAccountId;
    public String dispatchPoolId;
    public String messageGroup;
    public String mode;
    public Integer sequence;

    // Status tracking
    public String status;
    public Integer attemptCount;
    public Integer maxRetries;
    public String lastError;

    // Timing
    public Integer timeoutSeconds;
    public String retryStrategy;

    // Timestamps
    public Instant createdAt;
    public Instant updatedAt;
    public Instant scheduledFor;
    public Instant expiresAt;
    public Instant completedAt;
    public Instant lastAttemptAt;
    public Long durationMillis;

    // Idempotency
    public String idempotencyKey;

    // Computed fields
    public Boolean isCompleted;
    public Boolean isTerminal;
    public Instant projectedAt;

    public DispatchJobRead() {
    }
}

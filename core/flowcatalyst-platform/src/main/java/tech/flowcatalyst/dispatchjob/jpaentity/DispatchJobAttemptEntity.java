package tech.flowcatalyst.dispatchjob.jpaentity;

import jakarta.persistence.*;
import tech.flowcatalyst.dispatchjob.model.DispatchAttemptStatus;
import tech.flowcatalyst.dispatchjob.model.ErrorType;

import java.time.Instant;

/**
 * JPA Entity for dispatch_job_attempts table (normalized from attempts array).
 *
 * <p>This is an immutable record entity - attempts are insert-only (never updated),
 * making records a good fit. Hibernate 6+ supports records as entities.
 */
@Entity
@Table(name = "dispatch_job_attempts")
public record DispatchJobAttemptEntity(
    @Id
    @Column(name = "id", length = 17)
    String id,

    @Column(name = "dispatch_job_id", nullable = false, length = 13)
    String dispatchJobId,

    @Column(name = "attempt_number")
    Integer attemptNumber,

    @Column(name = "attempted_at")
    Instant attemptedAt,

    @Column(name = "completed_at")
    Instant completedAt,

    @Column(name = "duration_millis")
    Long durationMillis,

    @Enumerated(EnumType.STRING)
    @Column(name = "status", length = 20)
    DispatchAttemptStatus status,

    @Column(name = "response_code")
    Integer responseCode,

    @Column(name = "response_body", columnDefinition = "TEXT")
    String responseBody,

    @Column(name = "error_message", columnDefinition = "TEXT")
    String errorMessage,

    @Column(name = "error_stack_trace", columnDefinition = "TEXT")
    String errorStackTrace,

    @Enumerated(EnumType.STRING)
    @Column(name = "error_type", length = 20)
    ErrorType errorType,

    @Column(name = "created_at")
    Instant createdAt
) {
    /**
     * Convenience constructor with just required fields.
     * Other fields can be set using the with* methods pattern or full constructor.
     */
    public DispatchJobAttemptEntity(String id, String dispatchJobId) {
        this(id, dispatchJobId, null, null, null, null, null, null, null, null, null, null, null);
    }
}

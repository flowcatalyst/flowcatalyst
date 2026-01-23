package tech.flowcatalyst.platform.batch;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import io.agroal.api.AgroalDataSource;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.event.Event;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.sql.*;
import java.time.Instant;
import java.util.List;

/**
 * High-performance batch writer for events and dispatch jobs.
 *
 * <p>Uses JDBC batch inserts to minimize database round trips.
 * All writes happen in a single transaction for atomicity.</p>
 *
 * <h2>Tables Written</h2>
 * <ul>
 *   <li>events - Main event storage</li>
 *   <li>dispatch_jobs - Main dispatch job storage (metadata as JSONB)</li>
 *   <li>dispatch_job_changes - Change log for projection (INSERT with full job)</li>
 * </ul>
 */
@ApplicationScoped
public class BatchEventWriter {

    private static final ObjectMapper objectMapper = new ObjectMapper()
        .registerModule(new JavaTimeModule());

    @Inject
    AgroalDataSource dataSource;

    /**
     * Write events and their dispatch jobs in a single transaction.
     *
     * @param events List of events to persist
     * @param dispatchJobs List of dispatch jobs created from these events
     */
    public void writeBatch(List<Event> events, List<DispatchJob> dispatchJobs) {
        try (Connection conn = dataSource.getConnection()) {
            conn.setAutoCommit(false);
            try {
                // Assign IDs and timestamps to events
                Instant now = Instant.now();
                for (Event event : events) {
                    if (event.id == null) {
                        event.id = TsidGenerator.generate(EntityType.EVENT);
                    }
                    if (event.time == null) {
                        event.time = now;
                    }
                }

                // Assign IDs and timestamps to dispatch jobs
                for (DispatchJob job : dispatchJobs) {
                    if (job.id == null) {
                        job.id = TsidGenerator.generate(EntityType.DISPATCH_JOB);
                    }
                    if (job.createdAt == null) {
                        job.createdAt = now;
                    }
                    job.updatedAt = now;
                }

                // Batch insert all tables
                insertEventsBatch(conn, events);
                insertDispatchJobsBatch(conn, dispatchJobs);
                insertDispatchJobChangesBatch(conn, dispatchJobs);

                conn.commit();
            } catch (Exception e) {
                conn.rollback();
                throw e;
            }
        } catch (SQLException e) {
            throw new RuntimeException("Failed to write batch", e);
        }
    }

    // ========================================================================
    // Event Batch Inserts
    // ========================================================================

    private void insertEventsBatch(Connection conn, List<Event> events) throws SQLException {
        if (events.isEmpty()) return;

        String sql = """
            INSERT INTO events (
                id, spec_version, type, source, subject, time, data,
                correlation_id, causation_id, deduplication_id, message_group,
                context_data, client_id
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?)
            """;

        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            for (Event event : events) {
                ps.setString(1, event.id);
                ps.setString(2, event.specVersion);
                ps.setString(3, event.type);
                ps.setString(4, event.source);
                ps.setString(5, event.subject);
                ps.setTimestamp(6, event.time != null ? Timestamp.from(event.time) : null);
                ps.setString(7, event.data);
                ps.setString(8, event.correlationId);
                ps.setString(9, event.causationId);
                ps.setString(10, event.deduplicationId);
                ps.setString(11, event.messageGroup);
                ps.setString(12, toJson(event.contextData));
                ps.setString(13, event.clientId);
                ps.addBatch();
            }
            ps.executeBatch();
        }
    }

    // ========================================================================
    // Dispatch Job Batch Inserts
    // ========================================================================

    private void insertDispatchJobsBatch(Connection conn, List<DispatchJob> jobs) throws SQLException {
        if (jobs.isEmpty()) return;

        String sql = """
            INSERT INTO dispatch_jobs (
                id, external_id, source, kind, code, subject, event_id, correlation_id,
                target_url, protocol, payload, payload_content_type, data_only,
                service_account_id, client_id, subscription_id,
                mode, dispatch_pool_id, message_group, sequence, timeout_seconds,
                schema_id, status, max_retries, retry_strategy,
                scheduled_for, expires_at, attempt_count, last_attempt_at,
                completed_at, duration_millis, last_error, idempotency_key,
                metadata, created_at, updated_at
            ) VALUES (
                ?, ?, ?, ?, ?, ?, ?, ?,
                ?, ?, ?, ?, ?,
                ?, ?, ?,
                ?, ?, ?, ?, ?,
                ?, ?, ?, ?,
                ?, ?, ?, ?,
                ?, ?, ?, ?,
                ?::jsonb, ?, ?
            )
            """;

        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            for (DispatchJob job : jobs) {
                int i = 1;
                ps.setString(i++, job.id);
                ps.setString(i++, job.externalId);
                ps.setString(i++, job.source);
                ps.setString(i++, job.kind != null ? job.kind.name() : null);
                ps.setString(i++, job.code);
                ps.setString(i++, job.subject);
                ps.setString(i++, job.eventId);
                ps.setString(i++, job.correlationId);
                ps.setString(i++, job.targetUrl);
                ps.setString(i++, job.protocol != null ? job.protocol.name() : null);
                ps.setString(i++, job.payload);
                ps.setString(i++, job.payloadContentType);
                ps.setBoolean(i++, job.dataOnly);
                ps.setString(i++, job.serviceAccountId);
                ps.setString(i++, job.clientId);
                ps.setString(i++, job.subscriptionId);
                ps.setString(i++, job.mode != null ? job.mode.name() : null);
                ps.setString(i++, job.dispatchPoolId);
                ps.setString(i++, job.messageGroup);
                ps.setInt(i++, job.sequence);
                ps.setInt(i++, job.timeoutSeconds);
                ps.setString(i++, job.schemaId);
                ps.setString(i++, job.status != null ? job.status.name() : null);
                setNullableInt(ps, i++, job.maxRetries);
                ps.setString(i++, job.retryStrategy);
                setNullableTimestamp(ps, i++, job.scheduledFor);
                setNullableTimestamp(ps, i++, job.expiresAt);
                setNullableInt(ps, i++, job.attemptCount);
                setNullableTimestamp(ps, i++, job.lastAttemptAt);
                setNullableTimestamp(ps, i++, job.completedAt);
                setNullableLong(ps, i++, job.durationMillis);
                ps.setString(i++, job.lastError);
                ps.setString(i++, job.idempotencyKey);
                ps.setString(i++, toJson(job.metadata));
                ps.setTimestamp(i++, Timestamp.from(job.createdAt));
                ps.setTimestamp(i++, Timestamp.from(job.updatedAt));
                ps.addBatch();
            }
            ps.executeBatch();
        }
    }

    private void insertDispatchJobChangesBatch(Connection conn, List<DispatchJob> jobs) throws SQLException {
        if (jobs.isEmpty()) return;

        String sql = """
            INSERT INTO dispatch_job_changes (dispatch_job_id, operation, changes, created_at)
            VALUES (?, 'INSERT', ?::jsonb, NOW())
            """;

        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            for (DispatchJob job : jobs) {
                ps.setString(1, job.id);
                ps.setString(2, toJson(job));
                ps.addBatch();
            }
            ps.executeBatch();
        }
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private String toJson(Object obj) {
        if (obj == null) return null;
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize to JSON", e);
        }
    }

    private void setNullableInt(PreparedStatement ps, int index, Integer value) throws SQLException {
        if (value != null) {
            ps.setInt(index, value);
        } else {
            ps.setNull(index, Types.INTEGER);
        }
    }

    private void setNullableLong(PreparedStatement ps, int index, Long value) throws SQLException {
        if (value != null) {
            ps.setLong(index, value);
        } else {
            ps.setNull(index, Types.BIGINT);
        }
    }

    private void setNullableTimestamp(PreparedStatement ps, int index, Instant value) throws SQLException {
        if (value != null) {
            ps.setTimestamp(index, Timestamp.from(value));
        } else {
            ps.setNull(index, Types.TIMESTAMP);
        }
    }
}

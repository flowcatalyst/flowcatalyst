package tech.flowcatalyst.streamprocessor.postgres;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import io.agroal.api.AgroalDataSource;
import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;

import java.sql.*;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Projects dispatch job changes to dispatch_jobs_read using pure JDBC.
 *
 * <p>Reads from dispatch_job_changes table linearly (by id) and applies
 * INSERT or UPDATE operations to dispatch_jobs_read in batches.</p>
 *
 * <h2>Algorithm</h2>
 * <ol>
 *   <li>Poll: SELECT from dispatch_job_changes WHERE projected=false ORDER BY id LIMIT batchSize</li>
 *   <li>Batch INSERTs: multi-row INSERT ... ON CONFLICT</li>
 *   <li>Batch UPDATEs: UPDATE ... FROM (VALUES ...) with COALESCE</li>
 *   <li>Mark change records as projected</li>
 * </ol>
 */
@ApplicationScoped
public class DispatchJobProjectionService {

    private static final Logger LOG = Logger.getLogger(DispatchJobProjectionService.class.getName());
    private static final ObjectMapper objectMapper = new ObjectMapper()
        .registerModule(new JavaTimeModule());

    @Inject
    AgroalDataSource dataSource;

    @ConfigProperty(name = "stream-processor.dispatch-jobs.enabled", defaultValue = "true")
    boolean enabled;

    @ConfigProperty(name = "stream-processor.dispatch-jobs.batch-size", defaultValue = "100")
    int batchSize;

    private volatile boolean running = false;
    private volatile Thread pollerThread;

    void onStart(@Observes StartupEvent event) {
        if (!enabled) {
            LOG.info("Dispatch job projection service disabled");
            return;
        }
        start();
    }

    void onShutdown(@Observes ShutdownEvent event) {
        stop();
    }

    public synchronized void start() {
        if (running) {
            LOG.warning("Dispatch job projection service already running");
            return;
        }

        running = true;
        pollerThread = Thread.startVirtualThread(this::pollLoop);
        LOG.info("Dispatch job projection service started (batchSize=" + batchSize + ")");
    }

    public synchronized void stop() {
        if (!running) {
            return;
        }

        LOG.info("Stopping dispatch job projection service...");
        running = false;

        if (pollerThread != null) {
            pollerThread.interrupt();
            try {
                pollerThread.join(5000);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }

        LOG.info("Dispatch job projection service stopped");
    }

    public boolean isRunning() {
        return running;
    }

    private void pollLoop() {
        while (running) {
            try {
                int processed = pollAndProject();

                if (processed == 0) {
                    Thread.sleep(1000);
                } else if (processed < batchSize) {
                    Thread.sleep(100);
                }

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                LOG.log(Level.SEVERE, "Error in dispatch job projection poll loop", e);
                try {
                    Thread.sleep(5000);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }
    }

    private int pollAndProject() throws SQLException {
        List<ChangeRecord> changes = pollChanges();

        if (changes.isEmpty()) {
            return 0;
        }

        // Separate INSERTs and UPDATEs
        List<ChangeRecord> inserts = new ArrayList<>();
        List<ChangeRecord> updates = new ArrayList<>();

        for (ChangeRecord change : changes) {
            if ("INSERT".equals(change.operation)) {
                inserts.add(change);
            } else if ("UPDATE".equals(change.operation)) {
                updates.add(change);
            }
        }

        try (Connection conn = dataSource.getConnection()) {
            conn.setAutoCommit(false);

            try {
                if (!inserts.isEmpty()) {
                    batchInsert(conn, inserts);
                }
                if (!updates.isEmpty()) {
                    batchUpdate(conn, updates);
                }

                markProjected(conn, changes);
                conn.commit();

                LOG.fine("Projected " + changes.size() + " dispatch job changes (" + inserts.size() + " inserts, " + updates.size() + " updates)");
                return changes.size();

            } catch (Exception e) {
                conn.rollback();
                throw e;
            }
        }
    }

    private List<ChangeRecord> pollChanges() throws SQLException {
        String sql = """
            SELECT id, dispatch_job_id, operation, changes
            FROM dispatch_job_changes
            WHERE projected = false
            ORDER BY id
            LIMIT ?
            """;

        List<ChangeRecord> changes = new ArrayList<>(batchSize);

        try (Connection conn = dataSource.getConnection();
             PreparedStatement ps = conn.prepareStatement(sql)) {

            ps.setInt(1, batchSize);

            try (ResultSet rs = ps.executeQuery()) {
                while (rs.next()) {
                    changes.add(new ChangeRecord(
                        rs.getLong("id"),
                        rs.getString("dispatch_job_id"),
                        rs.getString("operation"),
                        rs.getString("changes")
                    ));
                }
            }
        }

        return changes;
    }

    /**
     * Batch INSERT using multi-row INSERT ... ON CONFLICT.
     */
    private void batchInsert(Connection conn, List<ChangeRecord> inserts) throws SQLException {
        StringBuilder sql = new StringBuilder("""
            INSERT INTO dispatch_jobs_read (
                id, external_id, source, kind, code, subject, event_id, correlation_id,
                target_url, protocol, service_account_id, client_id, subscription_id,
                mode, dispatch_pool_id, message_group, status, max_retries,
                attempt_count, last_attempt_at, completed_at, duration_millis, last_error,
                created_at, updated_at, application, subdomain, aggregate
            ) VALUES
            """);

        // Build placeholders
        for (int i = 0; i < inserts.size(); i++) {
            if (i > 0) sql.append(",");
            sql.append("\n(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)");
        }

        sql.append("""

            ON CONFLICT (id) DO UPDATE SET
                status = EXCLUDED.status,
                attempt_count = EXCLUDED.attempt_count,
                last_attempt_at = EXCLUDED.last_attempt_at,
                completed_at = EXCLUDED.completed_at,
                duration_millis = EXCLUDED.duration_millis,
                last_error = EXCLUDED.last_error,
                updated_at = EXCLUDED.updated_at
            """);

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            int paramIndex = 1;

            for (ChangeRecord change : inserts) {
                JsonNode job;
                try {
                    job = objectMapper.readTree(change.changesJson);
                } catch (JsonProcessingException e) {
                    LOG.warning("Failed to parse INSERT changes for job " + change.dispatchJobId);
                    // Set all params to null for this row
                    for (int i = 0; i < 28; i++) {
                        ps.setNull(paramIndex++, Types.VARCHAR);
                    }
                    continue;
                }

                // Parse code into application/subdomain/aggregate
                String code = getTextOrNull(job, "code");
                String[] codeSegments = code != null ? code.split(":", 4) : new String[0];
                String application = codeSegments.length > 0 ? codeSegments[0] : null;
                String subdomain = codeSegments.length > 1 ? codeSegments[1] : null;
                String aggregate = codeSegments.length > 2 ? codeSegments[2] : null;

                ps.setString(paramIndex++, change.dispatchJobId);
                ps.setString(paramIndex++, getTextOrNull(job, "externalId"));
                ps.setString(paramIndex++, getTextOrNull(job, "source"));
                ps.setString(paramIndex++, getTextOrNull(job, "kind"));
                ps.setString(paramIndex++, code);
                ps.setString(paramIndex++, getTextOrNull(job, "subject"));
                ps.setString(paramIndex++, getTextOrNull(job, "eventId"));
                ps.setString(paramIndex++, getTextOrNull(job, "correlationId"));
                ps.setString(paramIndex++, getTextOrNull(job, "targetUrl"));
                ps.setString(paramIndex++, getTextOrNull(job, "protocol"));
                ps.setString(paramIndex++, getTextOrNull(job, "serviceAccountId"));
                ps.setString(paramIndex++, getTextOrNull(job, "clientId"));
                ps.setString(paramIndex++, getTextOrNull(job, "subscriptionId"));
                ps.setString(paramIndex++, getTextOrNull(job, "mode"));
                ps.setString(paramIndex++, getTextOrNull(job, "dispatchPoolId"));
                ps.setString(paramIndex++, getTextOrNull(job, "messageGroup"));
                ps.setString(paramIndex++, getTextOrNull(job, "status"));
                setNullableInt(ps, paramIndex++, getIntOrNull(job, "maxRetries"));
                setNullableInt(ps, paramIndex++, getIntOrNull(job, "attemptCount"));
                setNullableTimestamp(ps, paramIndex++, getInstantOrNull(job, "lastAttemptAt"));
                setNullableTimestamp(ps, paramIndex++, getInstantOrNull(job, "completedAt"));
                setNullableLong(ps, paramIndex++, getLongOrNull(job, "durationMillis"));
                ps.setString(paramIndex++, getTextOrNull(job, "lastError"));
                setNullableTimestamp(ps, paramIndex++, getInstantOrNull(job, "createdAt"));
                setNullableTimestamp(ps, paramIndex++, getInstantOrNull(job, "updatedAt"));
                ps.setString(paramIndex++, application);
                ps.setString(paramIndex++, subdomain);
                ps.setString(paramIndex++, aggregate);
            }

            ps.executeUpdate();
        }
    }

    /**
     * Batch UPDATE using UPDATE ... FROM (VALUES ...) with COALESCE.
     * This allows updating different fields per row in a single statement.
     */
    private void batchUpdate(Connection conn, List<ChangeRecord> updates) throws SQLException {
        // Parse all updates first to build the VALUES
        List<UpdateRow> rows = new ArrayList<>(updates.size());

        for (ChangeRecord change : updates) {
            JsonNode patch;
            try {
                patch = objectMapper.readTree(change.changesJson);
            } catch (JsonProcessingException e) {
                LOG.warning("Failed to parse UPDATE changes for job " + change.dispatchJobId);
                continue;
            }

            rows.add(new UpdateRow(
                change.dispatchJobId,
                getTextOrNull(patch, "status"),
                getIntOrNull(patch, "attemptCount"),
                getInstantOrNull(patch, "lastAttemptAt"),
                getInstantOrNull(patch, "completedAt"),
                getLongOrNull(patch, "durationMillis"),
                getTextOrNull(patch, "lastError"),
                getInstantOrNull(patch, "updatedAt")
            ));
        }

        if (rows.isEmpty()) {
            return;
        }

        // Build UPDATE ... FROM (VALUES ...) statement
        StringBuilder sql = new StringBuilder("""
            UPDATE dispatch_jobs_read AS t
            SET
                status = COALESCE(v.status, t.status),
                attempt_count = COALESCE(v.attempt_count, t.attempt_count),
                last_attempt_at = COALESCE(v.last_attempt_at, t.last_attempt_at),
                completed_at = COALESCE(v.completed_at, t.completed_at),
                duration_millis = COALESCE(v.duration_millis, t.duration_millis),
                last_error = COALESCE(v.last_error, t.last_error),
                updated_at = COALESCE(v.updated_at, t.updated_at)
            FROM (VALUES
            """);

        for (int i = 0; i < rows.size(); i++) {
            if (i > 0) sql.append(",");
            sql.append("\n(?, ?::varchar, ?::int, ?::timestamptz, ?::timestamptz, ?::bigint, ?::text, ?::timestamptz)");
        }

        sql.append("""
            ) AS v(id, status, attempt_count, last_attempt_at, completed_at, duration_millis, last_error, updated_at)
            WHERE t.id = v.id
            """);

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            int paramIndex = 1;

            for (UpdateRow row : rows) {
                ps.setString(paramIndex++, row.id);
                ps.setString(paramIndex++, row.status);
                setNullableInt(ps, paramIndex++, row.attemptCount);
                setNullableTimestamp(ps, paramIndex++, row.lastAttemptAt);
                setNullableTimestamp(ps, paramIndex++, row.completedAt);
                setNullableLong(ps, paramIndex++, row.durationMillis);
                ps.setString(paramIndex++, row.lastError);
                setNullableTimestamp(ps, paramIndex++, row.updatedAt);
            }

            ps.executeUpdate();
        }
    }

    private void markProjected(Connection conn, List<ChangeRecord> changes) throws SQLException {
        StringBuilder sql = new StringBuilder("UPDATE dispatch_job_changes SET projected = true WHERE id IN (");
        for (int i = 0; i < changes.size(); i++) {
            if (i > 0) sql.append(",");
            sql.append("?");
        }
        sql.append(")");

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            for (int i = 0; i < changes.size(); i++) {
                ps.setLong(i + 1, changes.get(i).id);
            }
            ps.executeUpdate();
        }
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private String getTextOrNull(JsonNode node, String field) {
        return node.has(field) && !node.get(field).isNull() ? node.get(field).asText() : null;
    }

    private Integer getIntOrNull(JsonNode node, String field) {
        return node.has(field) && !node.get(field).isNull() ? node.get(field).asInt() : null;
    }

    private Long getLongOrNull(JsonNode node, String field) {
        return node.has(field) && !node.get(field).isNull() ? node.get(field).asLong() : null;
    }

    private Instant getInstantOrNull(JsonNode node, String field) {
        if (node.has(field) && !node.get(field).isNull()) {
            try {
                return Instant.parse(node.get(field).asText());
            } catch (Exception e) {
                return null;
            }
        }
        return null;
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

    private record ChangeRecord(long id, String dispatchJobId, String operation, String changesJson) {}

    private record UpdateRow(
        String id,
        String status,
        Integer attemptCount,
        Instant lastAttemptAt,
        Instant completedAt,
        Long durationMillis,
        String lastError,
        Instant updatedAt
    ) {}
}

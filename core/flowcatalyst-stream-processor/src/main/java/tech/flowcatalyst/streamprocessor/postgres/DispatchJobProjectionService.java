package tech.flowcatalyst.streamprocessor.postgres;

import io.agroal.api.AgroalDataSource;
import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;

import java.sql.Connection;
import java.sql.PreparedStatement;
import java.sql.SQLException;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * Projects dispatch job changes to dispatch_jobs_read using a single writable CTE.
 *
 * <p>Uses one atomic SQL statement per poll cycle that selects unprocessed feed
 * entries, applies INSERTs and UPDATEs to dispatch_jobs_read, and marks feed
 * entries as projected. Zero application-layer data transfer &mdash; JSONB field
 * extraction, type casting, and code hierarchy parsing all happen in-engine.</p>
 *
 * <h2>Algorithm</h2>
 * <ol>
 *   <li>Single CTE: batch SELECT &rarr; INSERT projection &rarr; UPDATE projection &rarr; mark processed</li>
 *   <li>Sleep: 0ms if full batch, 100ms if partial, 1000ms if zero results</li>
 * </ol>
 *
 * <h2>Deduplication</h2>
 * <p>For UPDATE operations with duplicate dispatch_job_ids in the same batch,
 * {@code DISTINCT ON (dispatch_job_id) ... ORDER BY id DESC} takes the latest
 * entry per job. {@code COALESCE} ensures only non-null patch fields are applied.</p>
 */
@ApplicationScoped
public class DispatchJobProjectionService {

    private static final Logger LOG = Logger.getLogger(DispatchJobProjectionService.class.getName());

    /**
     * Single writable CTE that projects a batch of dispatch job changes in one round-trip.
     *
     * <ol>
     *   <li>{@code batch} &mdash; selects unprocessed feed entries (LIMIT batchSize)</li>
     *   <li>{@code projected_inserts} &mdash; UPSERTs new dispatch jobs from INSERT entries,
     *       extracting all fields from JSONB and parsing the code hierarchy</li>
     *   <li>{@code projected_updates} &mdash; patches existing dispatch jobs from UPDATE entries,
     *       using COALESCE for sparse updates and DISTINCT ON for deduplication</li>
     *   <li>Main UPDATE &mdash; marks batch entries as projected</li>
     * </ol>
     */
    private static final String PROJECT_CTE = """
        WITH batch AS (
            SELECT id, dispatch_job_id, operation, changes
            FROM dispatch_job_projection_feed
            WHERE projected = false
            ORDER BY id
            LIMIT ?
        ),
        projected_inserts AS (
            INSERT INTO dispatch_jobs_read (
                id, external_id, source, kind, code, subject, event_id, correlation_id,
                target_url, protocol, service_account_id, client_id, subscription_id,
                mode, dispatch_pool_id, message_group, status, max_retries,
                attempt_count, last_attempt_at, completed_at, duration_millis, last_error,
                created_at, updated_at, application, subdomain, aggregate,
                sequence, timeout_seconds, retry_strategy, scheduled_for, expires_at,
                idempotency_key, is_completed, is_terminal, projected_at
            )
            SELECT
                b.dispatch_job_id,
                b.changes->>'externalId',
                b.changes->>'source',
                b.changes->>'kind',
                b.changes->>'code',
                b.changes->>'subject',
                b.changes->>'eventId',
                b.changes->>'correlationId',
                b.changes->>'targetUrl',
                b.changes->>'protocol',
                b.changes->>'serviceAccountId',
                b.changes->>'clientId',
                b.changes->>'subscriptionId',
                b.changes->>'mode',
                b.changes->>'dispatchPoolId',
                b.changes->>'messageGroup',
                b.changes->>'status',
                COALESCE((b.changes->>'maxRetries')::int, 3),
                COALESCE((b.changes->>'attemptCount')::int, 0),
                (b.changes->>'lastAttemptAt')::timestamptz,
                (b.changes->>'completedAt')::timestamptz,
                (b.changes->>'durationMillis')::bigint,
                b.changes->>'lastError',
                COALESCE((b.changes->>'createdAt')::timestamptz, NOW()),
                COALESCE((b.changes->>'updatedAt')::timestamptz, NOW()),
                split_part(b.changes->>'code', ':', 1),
                NULLIF(split_part(b.changes->>'code', ':', 2), ''),
                NULLIF(split_part(b.changes->>'code', ':', 3), ''),
                (b.changes->>'sequence')::int,
                (b.changes->>'timeoutSeconds')::int,
                b.changes->>'retryStrategy',
                (b.changes->>'scheduledFor')::timestamptz,
                (b.changes->>'expiresAt')::timestamptz,
                b.changes->>'idempotencyKey',
                (b.changes->>'isCompleted')::boolean,
                (b.changes->>'isTerminal')::boolean,
                NOW()
            FROM batch b
            WHERE b.operation = 'INSERT'
            ON CONFLICT (id) DO UPDATE SET
                status = EXCLUDED.status,
                attempt_count = EXCLUDED.attempt_count,
                last_attempt_at = EXCLUDED.last_attempt_at,
                completed_at = EXCLUDED.completed_at,
                duration_millis = EXCLUDED.duration_millis,
                last_error = EXCLUDED.last_error,
                updated_at = EXCLUDED.updated_at,
                scheduled_for = EXCLUDED.scheduled_for,
                expires_at = EXCLUDED.expires_at,
                is_completed = EXCLUDED.is_completed,
                is_terminal = EXCLUDED.is_terminal,
                projected_at = NOW()
        ),
        projected_updates AS (
            UPDATE dispatch_jobs_read AS t
            SET
                status = COALESCE(src.changes->>'status', t.status),
                attempt_count = COALESCE((src.changes->>'attemptCount')::int, t.attempt_count),
                last_attempt_at = COALESCE((src.changes->>'lastAttemptAt')::timestamptz, t.last_attempt_at),
                completed_at = COALESCE((src.changes->>'completedAt')::timestamptz, t.completed_at),
                duration_millis = COALESCE((src.changes->>'durationMillis')::bigint, t.duration_millis),
                last_error = COALESCE(src.changes->>'lastError', t.last_error),
                is_completed = COALESCE((src.changes->>'isCompleted')::boolean, t.is_completed),
                is_terminal = COALESCE((src.changes->>'isTerminal')::boolean, t.is_terminal),
                updated_at = COALESCE((src.changes->>'updatedAt')::timestamptz, t.updated_at),
                projected_at = NOW()
            FROM (
                SELECT DISTINCT ON (dispatch_job_id) dispatch_job_id, changes
                FROM batch
                WHERE operation = 'UPDATE'
                ORDER BY dispatch_job_id, id DESC
            ) src
            WHERE t.id = src.dispatch_job_id
        )
        UPDATE dispatch_job_projection_feed
        SET projected = true
        WHERE id IN (SELECT id FROM batch)
        """;

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

    /**
     * Execute the projection CTE.
     *
     * <p>The writable CTE atomically selects unprocessed feed entries, inserts new
     * dispatch jobs and patches existing ones (extracting all fields from JSONB
     * in-engine), and marks the feed entries as projected &mdash; all in a single
     * database round-trip.</p>
     *
     * @return number of feed entries projected (rows updated by the main UPDATE)
     */
    private int pollAndProject() throws SQLException {
        try (Connection conn = dataSource.getConnection();
             PreparedStatement ps = conn.prepareStatement(PROJECT_CTE)) {

            ps.setInt(1, batchSize);
            int updated = ps.executeUpdate();

            if (updated > 0) {
                LOG.fine("Projected " + updated + " dispatch job changes");
            }
            return updated;
        }
    }
}

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
 * Projects events from the events table to events_read using a single writable CTE.
 *
 * <p>Uses one atomic SQL statement per poll cycle that selects unprojected events,
 * inserts into events_read, and marks the source rows as projected. Zero
 * application-layer data transfer &mdash; type hierarchy parsing (application,
 * subdomain, aggregate) happens in-engine via {@code split_part()}.</p>
 *
 * <h2>Algorithm</h2>
 * <ol>
 *   <li>Single CTE: batch SELECT &rarr; INSERT events_read &rarr; UPDATE events</li>
 *   <li>Sleep: 0ms if batchSize results, 100ms if partial, 1000ms if zero</li>
 * </ol>
 */
@ApplicationScoped
public class EventProjectionService {

    private static final Logger LOG = Logger.getLogger(EventProjectionService.class.getName());

    /**
     * Single writable CTE that projects a batch of events in one round-trip.
     *
     * <ol>
     *   <li>{@code batch} &mdash; selects unprojected events (LIMIT batchSize)</li>
     *   <li>{@code projected} &mdash; UPSERTs into events_read, parsing the type
     *       hierarchy with {@code split_part()}</li>
     *   <li>Main UPDATE &mdash; marks batch rows as projected</li>
     * </ol>
     */
    private static final String PROJECT_CTE = """
        WITH batch AS (
            SELECT id, spec_version, type, source, subject, time, data,
                   correlation_id, causation_id, deduplication_id, message_group, client_id
            FROM events
            WHERE projected = false
            ORDER BY time
            LIMIT ?
        ),
        projected AS (
            INSERT INTO events_read (
                id, spec_version, type, source, subject, time, data,
                correlation_id, causation_id, deduplication_id, message_group,
                client_id, application, subdomain, aggregate, projected_at
            )
            SELECT
                b.id, b.spec_version, b.type, b.source, b.subject, b.time, b.data,
                b.correlation_id, b.causation_id, b.deduplication_id, b.message_group,
                b.client_id,
                split_part(b.type, ':', 1),
                NULLIF(split_part(b.type, ':', 2), ''),
                NULLIF(split_part(b.type, ':', 3), ''),
                NOW()
            FROM batch b
            ON CONFLICT (id) DO NOTHING
        )
        UPDATE events
        SET projected = true
        WHERE id IN (SELECT id FROM batch)
        """;

    @Inject
    AgroalDataSource dataSource;

    @ConfigProperty(name = "stream-processor.events.enabled", defaultValue = "true")
    boolean enabled;

    @ConfigProperty(name = "stream-processor.events.batch-size", defaultValue = "100")
    int batchSize;

    private volatile boolean running = false;
    private volatile Thread pollerThread;

    void onStart(@Observes StartupEvent event) {
        if (!enabled) {
            LOG.info("Event projection service disabled");
            return;
        }
        start();
    }

    void onShutdown(@Observes ShutdownEvent event) {
        stop();
    }

    public synchronized void start() {
        if (running) {
            LOG.warning("Event projection service already running");
            return;
        }

        running = true;
        pollerThread = Thread.startVirtualThread(this::pollLoop);
        LOG.info("Event projection service started (batchSize=" + batchSize + ")");
    }

    public synchronized void stop() {
        if (!running) {
            return;
        }

        LOG.info("Stopping event projection service...");
        running = false;

        if (pollerThread != null) {
            pollerThread.interrupt();
            try {
                pollerThread.join(5000);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }

        LOG.info("Event projection service stopped");
    }

    public boolean isRunning() {
        return running;
    }

    /**
     * Main polling loop.
     */
    private void pollLoop() {
        while (running) {
            try {
                int processed = pollAndProject();

                // Sleep based on results
                if (processed == 0) {
                    Thread.sleep(1000); // No work, sleep 1 second
                } else if (processed < batchSize) {
                    Thread.sleep(100); // Partial batch, sleep 100ms
                }
                // Full batch: no sleep, immediately poll again

            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                break;
            } catch (Exception e) {
                LOG.log(Level.SEVERE, "Error in event projection poll loop", e);
                try {
                    Thread.sleep(5000); // Back off on error
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
     * <p>The writable CTE atomically selects unprojected events, inserts them
     * into events_read (with type hierarchy parsing), and marks them as
     * projected &mdash; all in a single database round-trip.</p>
     *
     * @return number of events projected (rows updated by the main UPDATE)
     */
    private int pollAndProject() throws SQLException {
        try (Connection conn = dataSource.getConnection();
             PreparedStatement ps = conn.prepareStatement(PROJECT_CTE)) {

            ps.setInt(1, batchSize);
            int updated = ps.executeUpdate();

            if (updated > 0) {
                LOG.fine("Projected " + updated + " events");
            }
            return updated;
        }
    }
}

package tech.flowcatalyst.streamprocessor.postgres;

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
 * Projects events from the events table to events_read using pure JDBC.
 *
 * <p>Polls for unprojected events (projected=false) ordered by time,
 * batch inserts to events_read, and marks them as projected.</p>
 *
 * <h2>Algorithm</h2>
 * <ol>
 *   <li>Poll: SELECT from events WHERE projected=false ORDER BY time LIMIT batchSize</li>
 *   <li>If results > 0: batch UPSERT to events_read, UPDATE events SET projected=true</li>
 *   <li>Sleep: 0ms if batchSize results, 100ms if partial, 1000ms if zero</li>
 * </ol>
 */
@ApplicationScoped
public class EventProjectionService {

    private static final Logger LOG = Logger.getLogger(EventProjectionService.class.getName());

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
     * Poll for unprojected events and project them.
     *
     * @return number of events processed
     */
    private int pollAndProject() throws SQLException {
        List<EventRow> events = pollEvents();

        if (events.isEmpty()) {
            return 0;
        }

        try (Connection conn = dataSource.getConnection()) {
            conn.setAutoCommit(false);

            try {
                // Batch UPSERT to events_read
                upsertEventsRead(conn, events);

                // Mark as projected
                markProjected(conn, events);

                conn.commit();

                LOG.fine("Projected " + events.size() + " events");
                return events.size();

            } catch (Exception e) {
                conn.rollback();
                throw e;
            }
        }
    }

    /**
     * Poll for unprojected events.
     */
    private List<EventRow> pollEvents() throws SQLException {
        String sql = """
            SELECT id, spec_version, type, source, subject, time, data,
                   correlation_id, causation_id, deduplication_id, message_group,
                   context_data, client_id
            FROM events
            WHERE projected = false
            ORDER BY time
            LIMIT ?
            """;

        List<EventRow> events = new ArrayList<>(batchSize);

        try (Connection conn = dataSource.getConnection();
             PreparedStatement ps = conn.prepareStatement(sql)) {

            ps.setInt(1, batchSize);

            try (ResultSet rs = ps.executeQuery()) {
                while (rs.next()) {
                    events.add(mapEventRow(rs));
                }
            }
        }

        return events;
    }

    /**
     * Batch UPSERT events to events_read.
     * Note: id IS the event id (1:1 projection, no separate event_id column).
     * Note: context_data is normalized to event_read_context_data table (V12 migration).
     */
    private void upsertEventsRead(Connection conn, List<EventRow> events) throws SQLException {
        // Build multi-row INSERT with ON CONFLICT DO NOTHING
        StringBuilder sql = new StringBuilder("""
            INSERT INTO events_read (
                id, spec_version, type, source, subject, time, data,
                correlation_id, causation_id, deduplication_id, message_group,
                client_id, application, subdomain, aggregate, projected_at
            ) VALUES
            """);

        // Add value placeholders for each row (15 columns + NOW())
        for (int i = 0; i < events.size(); i++) {
            if (i > 0) sql.append(",");
            sql.append("\n(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())");
        }

        sql.append("\nON CONFLICT (id) DO NOTHING");

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            int paramIndex = 1;

            for (EventRow event : events) {
                // Parse type into application/subdomain/aggregate
                String[] typeSegments = event.type != null ? event.type.split(":", 4) : new String[0];
                String application = typeSegments.length > 0 ? typeSegments[0] : null;
                String subdomain = typeSegments.length > 1 ? typeSegments[1] : null;
                String aggregate = typeSegments.length > 2 ? typeSegments[2] : null;

                ps.setString(paramIndex++, event.id);           // id IS the event id
                ps.setString(paramIndex++, event.specVersion);
                ps.setString(paramIndex++, event.type);
                ps.setString(paramIndex++, event.source);
                ps.setString(paramIndex++, event.subject);
                ps.setTimestamp(paramIndex++, event.time != null ? Timestamp.from(event.time) : null);
                ps.setString(paramIndex++, event.data);
                ps.setString(paramIndex++, event.correlationId);
                ps.setString(paramIndex++, event.causationId);
                ps.setString(paramIndex++, event.deduplicationId);
                ps.setString(paramIndex++, event.messageGroup);
                ps.setString(paramIndex++, event.clientId);
                ps.setString(paramIndex++, application);
                ps.setString(paramIndex++, subdomain);
                ps.setString(paramIndex++, aggregate);
            }

            ps.executeUpdate();
        }
    }

    /**
     * Mark events as projected.
     */
    private void markProjected(Connection conn, List<EventRow> events) throws SQLException {
        // Build UPDATE with IN clause
        StringBuilder sql = new StringBuilder("UPDATE events SET projected = true WHERE id IN (");

        for (int i = 0; i < events.size(); i++) {
            if (i > 0) sql.append(",");
            sql.append("?");
        }
        sql.append(")");

        try (PreparedStatement ps = conn.prepareStatement(sql.toString())) {
            int paramIndex = 1;
            for (EventRow event : events) {
                ps.setString(paramIndex++, event.id);
            }
            ps.executeUpdate();
        }
    }

    /**
     * Map a ResultSet row to EventRow.
     */
    private EventRow mapEventRow(ResultSet rs) throws SQLException {
        return new EventRow(
            rs.getString("id"),
            rs.getString("spec_version"),
            rs.getString("type"),
            rs.getString("source"),
            rs.getString("subject"),
            rs.getTimestamp("time") != null ? rs.getTimestamp("time").toInstant() : null,
            rs.getString("data"),
            rs.getString("correlation_id"),
            rs.getString("causation_id"),
            rs.getString("deduplication_id"),
            rs.getString("message_group"),
            rs.getString("context_data"),
            rs.getString("client_id")
        );
    }

    /**
     * Internal record for event data.
     */
    private record EventRow(
        String id,
        String specVersion,
        String type,
        String source,
        String subject,
        Instant time,
        String data,
        String correlationId,
        String causationId,
        String deduplicationId,
        String messageGroup,
        String contextData,
        String clientId
    ) {}
}

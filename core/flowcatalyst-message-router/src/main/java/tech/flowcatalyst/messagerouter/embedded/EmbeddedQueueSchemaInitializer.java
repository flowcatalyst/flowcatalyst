package tech.flowcatalyst.messagerouter.embedded;

import io.agroal.api.AgroalDataSource;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.config.QueueType;
import tech.flowcatalyst.queue.embedded.EmbeddedQueueSchema;

import java.sql.Connection;

/**
 * Initializes the SQLite database schema for the embedded queue on startup.
 * Delegates to the shared schema definition in queue-client.
 *
 * Only initializes when queue type is EMBEDDED.
 */
@ApplicationScoped
public class EmbeddedQueueSchemaInitializer {

    private static final Logger LOG = Logger.getLogger(EmbeddedQueueSchemaInitializer.class);

    @Inject
    @ConfigProperty(name = "message-router.queue-type", defaultValue = "SQS")
    QueueType queueType;

    @Inject
    @io.quarkus.agroal.DataSource("embedded-queue")
    AgroalDataSource dataSource;

    void onStart(@Observes StartupEvent ev) {
        // Only initialize if using embedded queue
        if (queueType != QueueType.EMBEDDED) {
            LOG.infof("Queue type is %s, skipping embedded queue schema initialization", queueType);
            return;
        }

        LOG.info("Initializing embedded queue schema...");

        try (Connection conn = dataSource.getConnection()) {
            // Use the shared schema from queue-client
            EmbeddedQueueSchema.initialize(conn);
            LOG.info("Embedded queue schema initialized successfully");
        } catch (Exception e) {
            LOG.error("Failed to initialize embedded queue schema", e);
            throw new RuntimeException("Failed to initialize embedded queue schema", e);
        }
    }
}

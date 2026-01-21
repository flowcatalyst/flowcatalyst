package tech.flowcatalyst.messagerouter.config;

import io.nats.client.Connection;
import io.nats.client.Nats;
import io.nats.client.Options;
import io.nats.client.api.*;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Produces;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import java.time.Duration;
import java.util.Optional;

/**
 * CDI producer for NATS JetStream connection.
 *
 * <p>Creates a NATS connection and ensures the required JetStream stream
 * and durable consumer exist with the correct configuration.</p>
 *
 * <h2>Stream Configuration</h2>
 * <ul>
 *   <li>WorkQueue retention - messages deleted after ack</li>
 *   <li>File storage for durability (production)</li>
 *   <li>Configurable replicas for HA</li>
 * </ul>
 *
 * <h2>Consumer Configuration</h2>
 * <ul>
 *   <li>Durable pull consumer with explicit ack</li>
 *   <li>120s ack wait (matches SQS visibility timeout)</li>
 *   <li>Configurable max deliver for DLQ behavior</li>
 * </ul>
 */
@ApplicationScoped
public class NatsConnectionProducer {

    private static final Logger LOG = Logger.getLogger(NatsConnectionProducer.class);

    @ConfigProperty(name = "message-router.nats.servers", defaultValue = "nats://localhost:4222")
    String servers;

    @ConfigProperty(name = "message-router.nats.connection-name", defaultValue = "flowcatalyst-router")
    String connectionName;

    @ConfigProperty(name = "message-router.nats.stream-name", defaultValue = "FLOWCATALYST")
    String streamName;

    @ConfigProperty(name = "message-router.nats.consumer-name", defaultValue = "flowcatalyst-router")
    String consumerName;

    @ConfigProperty(name = "message-router.nats.subject", defaultValue = "flowcatalyst.dispatch.>")
    String subject;

    @ConfigProperty(name = "message-router.nats.ack-wait-seconds", defaultValue = "120")
    int ackWaitSeconds;

    @ConfigProperty(name = "message-router.nats.max-deliver", defaultValue = "10")
    int maxDeliver;

    @ConfigProperty(name = "message-router.nats.max-ack-pending", defaultValue = "1000")
    int maxAckPending;

    @ConfigProperty(name = "message-router.nats.storage-type", defaultValue = "File")
    String storageType;

    @ConfigProperty(name = "message-router.nats.replicas", defaultValue = "1")
    int replicas;

    @ConfigProperty(name = "message-router.nats.max-age-days", defaultValue = "7")
    int maxAgeDays;

    @ConfigProperty(name = "message-router.nats.username")
    Optional<String> username;

    @ConfigProperty(name = "message-router.nats.password")
    Optional<String> password;

    @Produces
    @ApplicationScoped
    public Connection createNatsConnection() {
        try {
            // Build connection options
            Options.Builder optionsBuilder = new Options.Builder()
                .servers(servers.split(","))
                .connectionName(connectionName)
                .reconnectWait(Duration.ofSeconds(2))
                .maxReconnects(-1)  // Unlimited reconnects
                .connectionTimeout(Duration.ofSeconds(10));

            // Add authentication if configured
            if (username.isPresent() && password.isPresent()) {
                optionsBuilder.userInfo(username.get(), password.get());
            }

            Options options = optionsBuilder.build();

            // Connect to NATS
            Connection connection = Nats.connect(options);
            LOG.infof("Connected to NATS server(s): %s", servers);

            // Ensure stream and consumer exist
            ensureStreamAndConsumer(connection);

            return connection;

        } catch (Exception e) {
            LOG.errorf(e, "Failed to connect to NATS server(s): %s", servers);
            throw new RuntimeException("Failed to connect to NATS", e);
        }
    }

    /**
     * Ensure the JetStream stream and durable consumer exist with correct configuration.
     */
    private void ensureStreamAndConsumer(Connection connection) throws Exception {
        var jsm = connection.jetStreamManagement();

        // Determine storage type
        StorageType storage = "Memory".equalsIgnoreCase(storageType)
            ? StorageType.Memory
            : StorageType.File;

        // Check if stream exists
        StreamInfo streamInfo = null;
        try {
            streamInfo = jsm.getStreamInfo(streamName);
            LOG.infof("NATS stream [%s] already exists", streamName);
        } catch (Exception e) {
            // Stream doesn't exist, create it
            LOG.infof("Creating NATS stream [%s]", streamName);
        }

        if (streamInfo == null) {
            // Create stream with WorkQueue retention (messages deleted after ack)
            StreamConfiguration streamConfig = StreamConfiguration.builder()
                .name(streamName)
                .subjects(subject)
                .storageType(storage)
                .retentionPolicy(RetentionPolicy.WorkQueue)
                .maxAge(Duration.ofDays(maxAgeDays))
                .replicas(replicas)
                .build();

            jsm.addStream(streamConfig);
            LOG.infof("Created NATS stream [%s] with %s storage, %d replicas",
                streamName, storage, replicas);
        }

        // Check if consumer exists
        ConsumerInfo consumerInfo = null;
        try {
            consumerInfo = jsm.getConsumerInfo(streamName, consumerName);
            LOG.infof("NATS consumer [%s] already exists on stream [%s]", consumerName, streamName);
        } catch (Exception e) {
            // Consumer doesn't exist, create it
            LOG.infof("Creating NATS consumer [%s] on stream [%s]", consumerName, streamName);
        }

        if (consumerInfo == null) {
            // Create durable pull consumer with explicit ack
            ConsumerConfiguration consumerConfig = ConsumerConfiguration.builder()
                .durable(consumerName)
                .ackPolicy(AckPolicy.Explicit)
                .ackWait(Duration.ofSeconds(ackWaitSeconds))
                .maxDeliver(maxDeliver)
                .maxAckPending(maxAckPending)
                .filterSubject(subject)
                .deliverPolicy(DeliverPolicy.All)
                .build();

            jsm.addOrUpdateConsumer(streamName, consumerConfig);
            LOG.infof("Created NATS consumer [%s] - ackWait: %ds, maxDeliver: %d, maxAckPending: %d",
                consumerName, ackWaitSeconds, maxDeliver, maxAckPending);
        }
    }
}

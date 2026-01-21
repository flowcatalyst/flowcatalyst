package tech.flowcatalyst.messagerouter.manager;

import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Tag;
import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import io.vertx.core.DeploymentOptions;
import io.vertx.core.ThreadingModel;
import io.vertx.core.Vertx;
import io.vertx.core.json.JsonArray;
import io.vertx.core.json.JsonObject;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import tech.flowcatalyst.messagerouter.vertx.codec.CodecRegistry;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.client.MessageRouterConfigClient;
import tech.flowcatalyst.messagerouter.config.MessageRouterConfig;
import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.consumer.QueueConsumer;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.InFlightMessage;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.vertx.verticle.QueueConsumerVerticle;
import tech.flowcatalyst.messagerouter.vertx.verticle.QueueManagerVerticle;
import tech.flowcatalyst.messagerouter.warning.WarningService;
import tech.flowcatalyst.standby.StandbyService;

import java.util.*;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Vert.x-based QueueManager implementation.
 * <p>
 * This class acts as an adapter between the existing REST endpoints/health services
 * and the new verticle-based architecture. It:
 * <ul>
 *   <li>Manages verticle lifecycle (deployment/undeployment)</li>
 *   <li>Exposes the same public API as the legacy QueueManager</li>
 *   <li>Routes queries to verticles via event bus</li>
 * </ul>
 * <p>
 * All state lives inside the verticles - this class is stateless except for
 * deployment IDs and Vert.x references.
 */
@ApplicationScoped
public class QueueManager implements MessageCallback {

    private static final Logger LOG = Logger.getLogger(QueueManager.class);

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @Inject
    Vertx vertx;

    @Inject
    @RestClient
    MessageRouterConfigClient configClient;

    @Inject
    QueueMetricsService queueMetrics;

    @Inject
    PoolMetricsService poolMetrics;

    @Inject
    WarningService warningService;

    @Inject
    MeterRegistry meterRegistry;

    @Inject
    SqsClient sqsClient;

    @Inject
    ObjectMapper objectMapper;

    // StandbyService is optional - injected if standby is enabled (from shared module)
    @Inject
    jakarta.enterprise.inject.Instance<StandbyService> standbyServiceInstance;

    private Optional<StandbyService> standbyService() {
        return standbyServiceInstance.isResolvable()
            ? Optional.of(standbyServiceInstance.get())
            : Optional.empty();
    }

    // Deployment IDs for verticles
    private String queueManagerVerticleId;
    private final Map<String, String> consumerVerticleIds = new ConcurrentHashMap<>();
    private final Map<String, QueueConfig> consumerConfigs = new ConcurrentHashMap<>();

    private volatile boolean initialized = false;
    private volatile boolean shutdownInProgress = false;
    private long healthCheckTimerId;

    // Metrics gauges
    private AtomicInteger inPipelineMapSizeGauge;
    private AtomicInteger activePoolCountGauge;

    void onStartup(@Observes StartupEvent event) {
        if (!messageRouterEnabled) {
            LOG.info("Message router is disabled");
            return;
        }

        initializeMetrics();
        deployVerticles();
    }

    void onShutdown(@Observes ShutdownEvent event) {
        LOG.info("QueueManager shutting down...");
        shutdownInProgress = true;

        // Cancel health check timer
        vertx.cancelTimer(healthCheckTimerId);

        // Undeploy all consumer verticles
        for (String deploymentId : consumerVerticleIds.values()) {
            try {
                vertx.undeploy(deploymentId).toCompletionStage().toCompletableFuture().join();
            } catch (Exception e) {
                LOG.warnf("Failed to undeploy consumer verticle: %s", e.getMessage());
            }
        }

        // Undeploy queue manager verticle
        if (queueManagerVerticleId != null) {
            try {
                vertx.undeploy(queueManagerVerticleId).toCompletionStage().toCompletableFuture().join();
            } catch (Exception e) {
                LOG.warnf("Failed to undeploy QueueManagerVerticle: %s", e.getMessage());
            }
        }

        LOG.info("QueueManager shutdown complete");
    }

    private void initializeMetrics() {
        if (meterRegistry == null) {
            inPipelineMapSizeGauge = new AtomicInteger(0);
            activePoolCountGauge = new AtomicInteger(0);
            return;
        }

        inPipelineMapSizeGauge = new AtomicInteger(0);
        meterRegistry.gauge(
                "flowcatalyst.queuemanager.pipeline.size",
                List.of(Tag.of("type", "inPipeline")),
                inPipelineMapSizeGauge
        );

        activePoolCountGauge = new AtomicInteger(0);
        meterRegistry.gauge(
                "flowcatalyst.queuemanager.pools.active",
                List.of(Tag.of("type", "pools")),
                activePoolCountGauge
        );
    }

    private void deployVerticles() {
        LOG.info("Deploying Vert.x verticles...");

        // Register Jackson codecs for typed event bus messages
        CodecRegistry.registerAll(vertx, objectMapper);

        try {
            // Deploy QueueManagerVerticle
            DeploymentOptions qmOptions = new DeploymentOptions()
                    .setThreadingModel(ThreadingModel.VIRTUAL_THREAD);

            QueueManagerVerticle qmVerticle = new QueueManagerVerticle(
                    queueMetrics,
                    poolMetrics,
                    () -> sqsClient,
                    this::fetchConfig,
                    this::standbyService,
                    meterRegistry
            );

            queueManagerVerticleId = vertx.deployVerticle(qmVerticle, qmOptions)
                    .toCompletionStage()
                    .toCompletableFuture()
                    .get(30, TimeUnit.SECONDS);

            LOG.infof("QueueManagerVerticle deployed: %s", queueManagerVerticleId);

            // Deploy consumer verticles based on config
            MessageRouterConfig config = fetchConfig();
            if (config != null && config.queues() != null) {
                for (QueueConfig queueConfig : config.queues()) {
                    deployConsumerVerticle(queueConfig);
                }
            }

            initialized = true;

            // Start consumer health monitoring (every 60 seconds)
            healthCheckTimerId = vertx.setPeriodic(60_000, id -> monitorConsumerHealth());

            LOG.info("Vert.x verticles deployed successfully");

        } catch (Exception e) {
            LOG.errorf(e, "Failed to deploy verticles: %s", e.getClass().getSimpleName());
            warningService.addWarning(
                    "VERTICLE_DEPLOY_FAILED",
                    "ERROR",
                    "Failed to deploy Vert.x verticles: " + e.getClass().getSimpleName() + " - " + e.getMessage(),
                    "QueueManager"
            );
        }
    }

    private void deployConsumerVerticle(QueueConfig queueConfig) {
        try {
            DeploymentOptions options = new DeploymentOptions()
                    .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                    .setConfig(new JsonObject()
                            .put("queueIdentifier", queueConfig.queueUri())
                            .put("queueUrl", queueConfig.queueUri()));

            QueueConsumerVerticle consumerVerticle = new QueueConsumerVerticle(
                    () -> sqsClient,
                    queueMetrics
            );

            String deploymentId = vertx.deployVerticle(consumerVerticle, options)
                    .toCompletionStage()
                    .toCompletableFuture()
                    .get(30, TimeUnit.SECONDS);

            consumerVerticleIds.put(queueConfig.queueUri(), deploymentId);
            consumerConfigs.put(queueConfig.queueUri(), queueConfig);
            LOG.infof("QueueConsumerVerticle deployed for queue [%s]: %s",
                    queueConfig.queueUri(), deploymentId);

        } catch (Exception e) {
            LOG.errorf("Failed to deploy consumer for queue [%s]: %s",
                    queueConfig.queueUri(), e.getMessage());
        }
    }

    /**
     * Periodically monitors consumer health and restarts unhealthy consumers.
     * Runs every 60 seconds to detect and remediate hung consumer threads.
     */
    private void monitorConsumerHealth() {
        if (shutdownInProgress || !initialized) {
            return;
        }

        for (Map.Entry<String, String> entry : consumerVerticleIds.entrySet()) {
            String queueId = entry.getKey();
            String deploymentId = entry.getValue();

            // Query health from verticle via event bus
            vertx.eventBus().<JsonObject>request("consumer." + queueId + ".health", "",
                    new io.vertx.core.eventbus.DeliveryOptions().setSendTimeout(5000))
                .onSuccess(reply -> {
                    boolean healthy = reply.body().getBoolean("healthy", false);
                    boolean running = reply.body().getBoolean("running", false);

                    if (!healthy || !running) {
                        long lastPollTime = reply.body().getLong("lastPollTime", 0L);
                        long timeSinceLastPoll = lastPollTime > 0
                                ? (System.currentTimeMillis() - lastPollTime) / 1000
                                : -1;

                        LOG.warnf("Consumer [%s] unhealthy (healthy=%s, running=%s, lastPoll=%ds ago), restarting...",
                                queueId, healthy, running, timeSinceLastPoll);
                        restartConsumer(queueId, deploymentId);
                    }
                })
                .onFailure(err -> {
                    LOG.warnf("Consumer [%s] not responding to health check: %s. Restarting...",
                            queueId, err.getMessage());
                    restartConsumer(queueId, deploymentId);
                });
        }
    }

    /**
     * Restarts a consumer by undeploying and redeploying it.
     */
    private void restartConsumer(String queueId, String deploymentId) {
        QueueConfig config = consumerConfigs.get(queueId);
        if (config == null) {
            LOG.errorf("Cannot restart consumer [%s]: no config found", queueId);
            return;
        }

        vertx.undeploy(deploymentId)
                .onComplete(ar -> {
                    if (ar.failed()) {
                        LOG.warnf("Failed to undeploy consumer [%s]: %s", queueId, ar.cause().getMessage());
                    }
                    // Redeploy regardless of undeploy result
                    consumerVerticleIds.remove(queueId);
                    deployConsumerVerticle(config);
                    LOG.infof("Consumer [%s] restarted", queueId);
                });
    }

    private MessageRouterConfig fetchConfig() {
        try {
            return configClient.getQueueConfig();
        } catch (Exception e) {
            LOG.warnf("Failed to fetch config: %s", e.getMessage());
            return null;
        }
    }

    // ==================== PUBLIC API (same as legacy QueueManager) ====================

    /**
     * Get in-flight messages sorted by timestamp (oldest first).
     * Queries the QueueManagerVerticle via event bus.
     */
    public List<InFlightMessage> getInFlightMessages(int limit, String messageIdFilter) {
        try {
            JsonObject query = new JsonObject()
                    .put("limit", limit)
                    .put("filter", messageIdFilter != null ? messageIdFilter : "");

            io.vertx.core.eventbus.Message<JsonArray> reply = vertx.eventBus()
                    .<JsonArray>request("router.query.in-flight", query)
                    .toCompletionStage()
                    .toCompletableFuture()
                    .get(10, TimeUnit.SECONDS);

            List<InFlightMessage> result = new ArrayList<>();
            for (int i = 0; i < reply.body().size(); i++) {
                // The verticle returns InFlightMessage objects in the array
                Object item = reply.body().getValue(i);
                if (item instanceof InFlightMessage ifm) {
                    result.add(ifm);
                }
            }
            return result;

        } catch (Exception e) {
            LOG.warnf("Failed to query in-flight messages: %s", e.getMessage());
            return Collections.emptyList();
        }
    }

    /**
     * Gets the health status of all active queue consumers.
     */
    public Map<String, QueueConsumerHealth> getConsumerHealthStatus() {
        Map<String, QueueConsumerHealth> healthStatus = new HashMap<>();

        for (Map.Entry<String, String> entry : consumerVerticleIds.entrySet()) {
            String queueId = entry.getKey();
            // For now, assume healthy if deployment exists
            // TODO: Query actual health from verticle
            healthStatus.put(queueId, new QueueConsumerHealth(
                    queueId,
                    true,
                    System.currentTimeMillis(),
                    0,
                    true
            ));
        }

        return healthStatus;
    }

    /**
     * Check if the manager is initialized.
     */
    public boolean isInitialized() {
        return initialized;
    }

    /**
     * Check if a message is already in the pipeline.
     * Note: In the verticle architecture, this is handled inside QueueManagerVerticle.
     * This method is kept for API compatibility.
     */
    public boolean isMessageInPipeline(String sqsMessageId) {
        // In verticle architecture, deduplication is handled internally
        // This is a no-op for API compatibility
        return false;
    }

    /**
     * Route a batch of messages. This is called by consumers.
     * In the verticle architecture, consumers send directly to event bus.
     * This method is kept for API compatibility with AbstractQueueConsumer.
     */
    public void routeMessageBatch(List<BatchMessage> messages) {
        // Convert to JsonArray and send to event bus
        JsonArray batch = new JsonArray();
        for (BatchMessage bm : messages) {
            batch.add(JsonObject.mapFrom(bm.message())
                    .put("queueIdentifier", bm.queueIdentifier())
                    .put("sqsMessageId", bm.sqsMessageId()));
        }

        vertx.eventBus().send("router.batch", new JsonObject()
                .put("messages", batch)
                .put("queueIdentifier", messages.isEmpty() ? "unknown" : messages.get(0).queueIdentifier()));
    }

    // ==================== MessageCallback Implementation ====================

    @Override
    public void ack(MessagePointer message) {
        String pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id();
        vertx.eventBus().send("router.ack", new JsonObject().put("sqsMessageId", pipelineKey));
    }

    @Override
    public void nack(MessagePointer message) {
        nack(message, 0);
    }

    @Override
    public void nack(MessagePointer message, int delaySeconds) {
        String pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id();
        vertx.eventBus().send("router.nack", new JsonObject()
                .put("sqsMessageId", pipelineKey)
                .put("delaySeconds", delaySeconds));
    }

    // ==================== Inner Classes ====================

    /**
     * Batch message record for API compatibility with AbstractQueueConsumer.
     */
    public record BatchMessage(
            MessagePointer message,
            MessageCallback callback,
            String queueIdentifier,
            String sqsMessageId
    ) {}

    /**
     * Consumer health status record.
     */
    public record QueueConsumerHealth(
            String queueIdentifier,
            boolean isHealthy,
            long lastPollTimeMs,
            long timeSinceLastPollMs,
            boolean isRunning
    ) {}
}

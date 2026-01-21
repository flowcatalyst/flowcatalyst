package tech.flowcatalyst.messagerouter.vertx.verticle;

import io.vertx.core.AbstractVerticle;
import io.vertx.core.DeploymentOptions;
import io.vertx.core.ThreadingModel;
import io.vertx.core.eventbus.Message;
import io.vertx.core.json.JsonObject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.ChangeMessageVisibilityRequest;
import software.amazon.awssdk.services.sqs.model.DeleteMessageRequest;
import tech.flowcatalyst.messagerouter.config.MessageRouterConfig;
import tech.flowcatalyst.messagerouter.config.ProcessingPool;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.metrics.QueueMetricsService;
import tech.flowcatalyst.messagerouter.model.InFlightMessage;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.vertx.channel.PoolChannels;
import tech.flowcatalyst.messagerouter.vertx.channel.RouterChannels;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

import tech.flowcatalyst.standby.StandbyService;
import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Tag;

import java.time.Instant;
import java.util.*;
import java.util.Optional;
import java.util.function.Supplier;
import java.util.stream.Collectors;

/**
 * Central coordinator verticle for the message router.
 * <p>
 * Owns:
 * - In-pipeline message tracking (deduplication)
 * - Message callbacks for ACK/NACK
 * - Pool deployment lifecycle
 * <p>
 * Threading: Virtual Thread (blocking OK)
 */
public class QueueManagerVerticle extends AbstractVerticle {

    private static final Logger LOG = Logger.getLogger(QueueManagerVerticle.class);
    private static final String DEFAULT_POOL_CODE = "DEFAULT-POOL";
    private static final int DEFAULT_POOL_CONCURRENCY = 20;

    // === OWNED STATE (plain HashMap - single threaded verticle) ===
    private final Map<String, String> poolDeploymentIds = new HashMap<>();
    private final Map<String, String> drainingPools = new HashMap<>(); // Pools being gracefully drained
    private final Map<String, MessagePointer> inPipeline = new HashMap<>();
    private final Map<String, MessageCallbackInfo> messageCallbacks = new HashMap<>();
    private final Map<String, String> appMessageIdToPipelineKey = new HashMap<>();
    private final Map<String, Instant> messageSubmitTimes = new HashMap<>();

    // Injected dependencies
    private final QueueMetricsService queueMetrics;
    private final PoolMetricsService poolMetrics;
    private final Supplier<SqsClient> sqsClientSupplier;
    private final Supplier<MessageRouterConfig> configSupplier;
    private final Supplier<Optional<StandbyService>> standbySupplier;
    private final MeterRegistry meterRegistry;

    private SqsClient sqsClient;
    private long configSyncTimerId;
    private long visibilityExtensionTimerId;
    private long leakDetectionTimerId;
    private long drainingCleanupTimerId;
    private boolean standbyMessageLogged = false;
    private Counter defaultPoolUsageCounter;

    public QueueManagerVerticle(
            QueueMetricsService queueMetrics,
            PoolMetricsService poolMetrics,
            Supplier<SqsClient> sqsClientSupplier,
            Supplier<MessageRouterConfig> configSupplier,
            Supplier<Optional<StandbyService>> standbySupplier,
            MeterRegistry meterRegistry) {
        this.queueMetrics = queueMetrics;
        this.poolMetrics = poolMetrics;
        this.sqsClientSupplier = sqsClientSupplier;
        this.configSupplier = configSupplier;
        this.standbySupplier = standbySupplier;
        this.meterRegistry = meterRegistry;
    }

    @Override
    public void start() {
        LOG.info("Starting QueueManagerVerticle");

        // Initialize metrics
        initializeMetrics();

        // Initialize SQS client (blocking - fine on virtual thread)
        this.sqsClient = sqsClientSupplier.get();

        // Register typed consumers using channel addresses
        RouterChannels.batch(vertx).consumer(this::handleBatch);
        RouterChannels.ack(vertx).consumer(this::handleAck);
        RouterChannels.nack(vertx).consumer(this::handleNack);
        RouterChannels.queryInFlight(vertx).consumer(this::handleQueryInFlight);
        RouterChannels.queryPoolStats(vertx).consumer(this::handleQueryPoolStats);

        // Periodic config sync (5 minutes)
        configSyncTimerId = vertx.setPeriodic(300_000, id -> syncConfiguration());

        // Periodic visibility extension (55 seconds - before 60s default timeout)
        visibilityExtensionTimerId = vertx.setPeriodic(55_000, id -> extendMessageVisibility());

        // Periodic leak detection (30 seconds)
        leakDetectionTimerId = vertx.setPeriodic(30_000, id -> checkForMapLeaks());

        // Periodic draining pool cleanup (10 seconds)
        drainingCleanupTimerId = vertx.setPeriodic(10_000, id -> cleanupDrainingPools());

        // Defer initial config sync to allow HTTP server to start first
        vertx.setTimer(1000, id -> syncConfiguration());

        LOG.info("QueueManagerVerticle started");
    }

    @Override
    public void stop() {
        LOG.info("Stopping QueueManagerVerticle");

        // Cancel timers
        vertx.cancelTimer(configSyncTimerId);
        vertx.cancelTimer(visibilityExtensionTimerId);
        vertx.cancelTimer(leakDetectionTimerId);
        vertx.cancelTimer(drainingCleanupTimerId);

        // Undeploy all pools
        for (String deploymentId : poolDeploymentIds.values()) {
            try {
                vertx.undeploy(deploymentId).toCompletionStage().toCompletableFuture().join();
            } catch (Exception e) {
                LOG.warnf("Failed to undeploy pool: %s", e.getMessage());
            }
        }

        LOG.info("QueueManagerVerticle stopped");
    }

    // === METRICS ===

    private void initializeMetrics() {
        if (meterRegistry == null) {
            return;
        }

        // Register gauges for map sizes
        meterRegistry.gauge(
                "flowcatalyst.router.pipeline.size",
                List.of(Tag.of("type", "inPipeline")),
                inPipeline,
                Map::size
        );

        meterRegistry.gauge(
                "flowcatalyst.router.callbacks.size",
                List.of(Tag.of("type", "callbacks")),
                messageCallbacks,
                Map::size
        );

        meterRegistry.gauge(
                "flowcatalyst.router.pools.active",
                List.of(Tag.of("type", "pools")),
                poolDeploymentIds,
                Map::size
        );

        meterRegistry.gauge(
                "flowcatalyst.router.pools.draining",
                List.of(Tag.of("type", "draining")),
                drainingPools,
                Map::size
        );

        // Counter for default pool usage (indicates missing pool configuration)
        defaultPoolUsageCounter = meterRegistry.counter(
                "flowcatalyst.router.defaultpool.usage",
                List.of(Tag.of("pool", DEFAULT_POOL_CODE))
        );

        LOG.info("Metrics initialized");
    }

    // === MESSAGE HANDLING ===

    private void handleBatch(Message<BatchRequest> msg) {
        // Check standby status - only primary processes messages
        Optional<StandbyService> standby = standbySupplier.get();
        if (standby.isPresent() && !standby.get().isPrimary()) {
            if (!standbyMessageLogged) {
                LOG.info("In standby mode, not processing messages. Waiting for primary lock...");
                standbyMessageLogged = true;
            }
            msg.reply(new OkReply());
            return;
        }
        standbyMessageLogged = false; // Reset when we become primary

        BatchRequest batch = msg.body();
        String queueIdentifier = batch.queueIdentifier();
        String batchId = UUID.randomUUID().toString();

        LOG.debugf("Received batch of %d messages from queue [%s]", batch.messages().size(), queueIdentifier);

        for (QueuedMessage queuedMsg : batch.messages()) {
            String sqsMessageId = queuedMsg.sqsMessageId();

            // Deduplication check (safe - single threaded)
            if (inPipeline.containsKey(sqsMessageId)) {
                LOG.debugf("Duplicate message [%s] - SQS redelivery due to visibility timeout", sqsMessageId);

                // CRITICAL: Update stored callback with new receipt handle
                // The old receipt handle is stale after visibility timeout redelivery.
                // If we don't update it, the ACK when processing completes will fail silently.
                MessageCallbackInfo existing = messageCallbacks.get(sqsMessageId);
                if (existing != null) {
                    messageCallbacks.put(sqsMessageId, new MessageCallbackInfo(
                            queuedMsg.queueUrl(),
                            queuedMsg.receiptHandle(),  // NEW valid handle
                            existing.queueIdentifier()
                    ));
                    LOG.debugf("Updated receipt handle for in-pipeline message [%s]", sqsMessageId);
                }

                // NACK the duplicate delivery with 0 delay (it will become visible immediately,
                // but that's fine since we're already processing it)
                nackMessageDirect(queuedMsg.queueUrl(), queuedMsg.receiptHandle(), 0);
                continue;
            }

            String poolCode = queuedMsg.poolCode();

            // Check if pool exists - use DEFAULT_POOL as fallback to prevent message loss
            if (!poolDeploymentIds.containsKey(poolCode)) {
                LOG.warnf("Unknown pool [%s] for message [%s], routing to default pool", poolCode, sqsMessageId);
                if (defaultPoolUsageCounter != null) {
                    defaultPoolUsageCounter.increment();
                }
                ensureDefaultPoolDeployed();
                poolCode = DEFAULT_POOL_CODE;
            }

            // Convert to MessagePointer for internal tracking
            MessagePointer pointer = new MessagePointer(
                    queuedMsg.id(),
                    queuedMsg.poolCode(),
                    queuedMsg.authToken(),
                    queuedMsg.mediationType(),
                    queuedMsg.mediationTarget(),
                    queuedMsg.messageGroupId(),
                    batchId,
                    sqsMessageId
            );

            // Track in pipeline
            inPipeline.put(sqsMessageId, pointer);
            messageCallbacks.put(sqsMessageId, new MessageCallbackInfo(
                    queuedMsg.queueUrl(),
                    queuedMsg.receiptHandle(),
                    queueIdentifier
            ));
            messageSubmitTimes.put(sqsMessageId, Instant.now());

            if (pointer.id() != null) {
                appMessageIdToPipelineKey.put(pointer.id(), sqsMessageId);
            }

            // Route to pool using typed channel (fire-and-forget, pool handles ACK/NACK)
            PoolMessage poolMessage = new PoolMessage(
                    queuedMsg.id(),
                    sqsMessageId,
                    poolCode,
                    queuedMsg.authToken(),
                    queuedMsg.mediationType(),
                    queuedMsg.mediationTarget(),
                    queuedMsg.messageGroupId(),
                    batchId
            );
            PoolChannels.address(poolCode).messagesFireAndForget(vertx).send(poolMessage);

            queueMetrics.recordMessageReceived(queueIdentifier);
        }

        msg.reply(new OkReply());
    }

    private void handleAck(Message<AckRequest> msg) {
        String sqsMessageId = msg.body().sqsMessageId();
        LOG.debugf("ACK received for message [%s]", sqsMessageId);

        MessageCallbackInfo callback = messageCallbacks.remove(sqsMessageId);
        MessagePointer pointer = inPipeline.remove(sqsMessageId);
        messageSubmitTimes.remove(sqsMessageId);

        if (pointer != null && pointer.id() != null) {
            appMessageIdToPipelineKey.remove(pointer.id());
        }

        if (callback != null) {
            try {
                // Blocking SQS delete - fine on virtual thread
                sqsClient.deleteMessage(DeleteMessageRequest.builder()
                        .queueUrl(callback.queueUrl())
                        .receiptHandle(callback.receiptHandle())
                        .build());

                queueMetrics.recordMessageProcessed(callback.queueIdentifier(), true);
            } catch (Exception e) {
                LOG.errorf("Failed to delete message from SQS: %s", e.getMessage());
            }
        }

        msg.reply(new OkReply());
    }

    private void handleNack(Message<NackRequest> msg) {
        NackRequest nack = msg.body();
        String sqsMessageId = nack.sqsMessageId();
        int delaySeconds = nack.delaySeconds();
        LOG.debugf("NACK received for message [%s] with delay %d", sqsMessageId, delaySeconds);

        MessageCallbackInfo callback = messageCallbacks.remove(sqsMessageId);
        MessagePointer pointer = inPipeline.remove(sqsMessageId);
        messageSubmitTimes.remove(sqsMessageId);

        if (pointer != null && pointer.id() != null) {
            appMessageIdToPipelineKey.remove(pointer.id());
        }

        if (callback != null) {
            try {
                // Blocking SQS visibility change - fine on virtual thread
                sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                        .queueUrl(callback.queueUrl())
                        .receiptHandle(callback.receiptHandle())
                        .visibilityTimeout(delaySeconds)
                        .build());

                queueMetrics.recordMessageDeferred(callback.queueIdentifier());
            } catch (Exception e) {
                LOG.errorf("Failed to change message visibility: %s", e.getMessage());
            }
        }

        msg.reply(new OkReply());
    }

    private void nackMessageDirect(String queueUrl, String receiptHandle, int delaySeconds) {
        if (queueUrl != null && receiptHandle != null) {
            try {
                sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                        .queueUrl(queueUrl)
                        .receiptHandle(receiptHandle)
                        .visibilityTimeout(delaySeconds)
                        .build());
            } catch (Exception e) {
                LOG.warnf("Failed to NACK message directly: %s", e.getMessage());
            }
        }
    }

    // === QUERIES ===

    private void handleQueryInFlight(Message<InFlightQuery> msg) {
        InFlightQuery query = msg.body();
        int limit = query.limit();
        String filter = query.filter() != null ? query.filter() : "";

        List<InFlightMessage> result = inPipeline.entrySet().stream()
                .filter(e -> filter.isEmpty() || (e.getValue().id() != null && e.getValue().id().contains(filter)))
                .limit(limit)
                .map(e -> {
                    Instant submitTime = messageSubmitTimes.get(e.getKey());
                    long addedAtMs = submitTime != null ? submitTime.toEpochMilli() : System.currentTimeMillis();
                    MessageCallbackInfo callback = messageCallbacks.get(e.getKey());
                    String queueId = callback != null ? callback.queueIdentifier() : "unknown";
                    return InFlightMessage.from(
                            e.getValue().id(),
                            e.getKey(),  // brokerMessageId = sqsMessageId
                            queueId,
                            addedAtMs,
                            e.getValue().poolCode()
                    );
                })
                .toList();

        // Reply with typed result (not JsonArray)
        msg.reply(new InFlightQueryResult(result));
    }

    private void handleQueryPoolStats(Message<String> msg) {
        // Reply with typed result
        msg.reply(new PoolStatsResult(new HashSet<>(poolDeploymentIds.keySet())));
    }

    // === CONFIGURATION ===

    private void syncConfiguration() {
        LOG.debug("Syncing configuration");

        try {
            MessageRouterConfig config = configSupplier.get();
            if (config == null || config.processingPools() == null) {
                LOG.warn("No configuration available");
                return;
            }

            Set<String> configPoolCodes = config.processingPools().stream()
                    .map(ProcessingPool::code)
                    .collect(Collectors.toSet());

            // Deploy new pools
            for (ProcessingPool poolConfig : config.processingPools()) {
                if (!poolDeploymentIds.containsKey(poolConfig.code())) {
                    deployPool(poolConfig);
                } else {
                    // Send config update using typed channel
                    PoolConfigUpdate update = new PoolConfigUpdate(
                            poolConfig.concurrency(),
                            poolConfig.rateLimitPerMinute()
                    );
                    PoolChannels.address(poolConfig.code()).config(vertx).send(update);
                }
            }

            // Remove pools no longer in config
            for (String existingCode : new ArrayList<>(poolDeploymentIds.keySet())) {
                if (!configPoolCodes.contains(existingCode)) {
                    undeployPool(existingCode);
                }
            }

            LOG.debugf("Config sync complete. Active pools: %d", poolDeploymentIds.size());

        } catch (Exception e) {
            LOG.errorf("Failed to sync configuration: %s", e.getMessage());
        }
    }

    private void deployPool(ProcessingPool config) {
        LOG.infof("Deploying pool [%s] with concurrency=%d, rateLimit=%s",
                config.code(), config.concurrency(), config.rateLimitPerMinute());

        DeploymentOptions options = new DeploymentOptions()
                .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                .setConfig(JsonObject.mapFrom(config));

        try {
            String deploymentId = vertx.deployVerticle(
                            new PoolVerticle(poolMetrics),
                            options)
                    .toCompletionStage()
                    .toCompletableFuture()
                    .join();

            poolDeploymentIds.put(config.code(), deploymentId);

            // Also deploy mediator for this pool
            DeploymentOptions mediatorOptions = new DeploymentOptions()
                    .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                    .setConfig(new JsonObject()
                            .put("poolCode", config.code()));

            vertx.deployVerticle(new MediatorVerticle(), mediatorOptions);

            LOG.infof("Pool [%s] deployed successfully", config.code());

        } catch (Exception e) {
            LOG.errorf("Failed to deploy pool [%s]: %s", config.code(), e.getMessage());
        }
    }

    /**
     * Moves a pool to draining state instead of immediate undeploy.
     * The pool will be fully undeployed once it has no messages in flight.
     */
    private void undeployPool(String poolCode) {
        LOG.infof("Moving pool [%s] to draining state", poolCode);

        String deploymentId = poolDeploymentIds.remove(poolCode);
        if (deploymentId != null) {
            drainingPools.put(poolCode, deploymentId);
        }
    }

    /**
     * Periodically checks draining pools and undeploys them once fully drained.
     */
    private void cleanupDrainingPools() {
        if (drainingPools.isEmpty()) {
            return;
        }

        for (String poolCode : new ArrayList<>(drainingPools.keySet())) {
            // Check if pool has any messages in flight
            boolean hasMessagesInFlight = inPipeline.values().stream()
                    .anyMatch(msg -> poolCode.equals(msg.poolCode()));

            if (!hasMessagesInFlight) {
                String deploymentId = drainingPools.remove(poolCode);
                if (deploymentId != null) {
                    LOG.infof("Pool [%s] fully drained, undeploying", poolCode);
                    try {
                        vertx.undeploy(deploymentId).toCompletionStage().toCompletableFuture().join();
                        LOG.infof("Pool [%s] undeployed successfully", poolCode);
                    } catch (Exception e) {
                        LOG.errorf("Failed to undeploy drained pool [%s]: %s", poolCode, e.getMessage());
                    }
                }
            } else {
                long count = inPipeline.values().stream()
                        .filter(msg -> poolCode.equals(msg.poolCode()))
                        .count();
                LOG.debugf("Pool [%s] still draining, %d messages in flight", poolCode, count);
            }
        }
    }

    /**
     * Lazily creates the default fallback pool for messages with unknown pool codes.
     * This prevents message loss when pool configuration is missing.
     */
    private void ensureDefaultPoolDeployed() {
        if (poolDeploymentIds.containsKey(DEFAULT_POOL_CODE)) {
            return; // Already deployed
        }

        LOG.infof("Creating default fallback pool [%s] with concurrency %d",
                DEFAULT_POOL_CODE, DEFAULT_POOL_CONCURRENCY);

        ProcessingPool defaultConfig = new ProcessingPool(
                DEFAULT_POOL_CODE,
                DEFAULT_POOL_CONCURRENCY,
                null  // No rate limiting for default pool
        );
        deployPool(defaultConfig);
    }

    // === VISIBILITY EXTENSION ===

    private void extendMessageVisibility() {
        if (inPipeline.isEmpty()) {
            return;
        }

        LOG.debugf("Extending visibility for %d in-flight messages", inPipeline.size());

        for (Map.Entry<String, MessagePointer> entry : inPipeline.entrySet()) {
            MessageCallbackInfo callback = messageCallbacks.get(entry.getKey());
            if (callback != null) {
                try {
                    sqsClient.changeMessageVisibility(ChangeMessageVisibilityRequest.builder()
                            .queueUrl(callback.queueUrl())
                            .receiptHandle(callback.receiptHandle())
                            .visibilityTimeout(120) // 2 minutes
                            .build());
                } catch (Exception e) {
                    LOG.warnf("Failed to extend visibility for [%s]: %s", entry.getKey(), e.getMessage());
                }
            }
        }
    }

    // === LEAK DETECTION ===

    /**
     * Periodic check for memory leaks in tracking maps.
     * Detects anomalies like size mismatches between related maps.
     */
    private void checkForMapLeaks() {
        int pipelineSize = inPipeline.size();
        int callbacksSize = messageCallbacks.size();
        int timestampsSize = messageSubmitTimes.size();
        int appIdMapSize = appMessageIdToPipelineKey.size();

        // Check for size mismatches between related maps
        if (pipelineSize != callbacksSize) {
            LOG.warnf("MAP SIZE MISMATCH: inPipeline=%d, messageCallbacks=%d - potential memory leak",
                    pipelineSize, callbacksSize);
        }

        if (pipelineSize != timestampsSize) {
            LOG.warnf("MAP SIZE MISMATCH: inPipeline=%d, messageSubmitTimes=%d - potential memory leak",
                    pipelineSize, timestampsSize);
        }

        // Calculate estimated capacity based on pool concurrency
        int estimatedCapacity = poolDeploymentIds.size() * DEFAULT_POOL_CONCURRENCY * 2;
        if (estimatedCapacity == 0) {
            estimatedCapacity = 100; // Minimum capacity estimate
        }

        // Warn if pipeline size exceeds expected capacity
        if (pipelineSize > estimatedCapacity) {
            LOG.warnf("PIPELINE SIZE EXCEEDS CAPACITY: %d > %d - messages may be stuck or leaking",
                    pipelineSize, estimatedCapacity);
        }

        // Check for stale messages (in pipeline for more than 5 minutes)
        Instant staleThreshold = Instant.now().minusSeconds(300);
        int staleCount = 0;
        for (Map.Entry<String, Instant> entry : messageSubmitTimes.entrySet()) {
            if (entry.getValue().isBefore(staleThreshold)) {
                staleCount++;
                if (staleCount <= 5) { // Log first 5 stale messages
                    LOG.warnf("Stale message detected: [%s] submitted at %s",
                            entry.getKey(), entry.getValue());
                }
            }
        }
        if (staleCount > 5) {
            LOG.warnf("... and %d more stale messages", staleCount - 5);
        }

        LOG.debugf("Leak check: pipeline=%d, callbacks=%d, timestamps=%d, appIdMap=%d, pools=%d",
                pipelineSize, callbacksSize, timestampsSize, appIdMapSize, poolDeploymentIds.size());
    }

    // === HELPER CLASSES ===

    private record MessageCallbackInfo(
            String queueUrl,
            String receiptHandle,
            String queueIdentifier
    ) {}

    // === ACCESSORS FOR MONITORING ===

    public int getInFlightCount() {
        return inPipeline.size();
    }

    public Set<String> getActivePoolCodes() {
        return new HashSet<>(poolDeploymentIds.keySet());
    }
}

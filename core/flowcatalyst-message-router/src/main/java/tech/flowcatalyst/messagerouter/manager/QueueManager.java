package tech.flowcatalyst.messagerouter.manager;

import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Tag;
import io.quarkus.runtime.Quarkus;
import io.quarkus.runtime.ShutdownEvent;
import io.quarkus.runtime.StartupEvent;
import io.quarkus.scheduler.Scheduled;
import io.smallrye.common.annotation.RunOnVirtualThread;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.eclipse.microprofile.rest.client.inject.RestClient;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.client.MessageRouterConfigClient;
import tech.flowcatalyst.messagerouter.config.MessageRouterConfig;
import tech.flowcatalyst.messagerouter.config.ProcessingPool;
import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.consumer.QueueConsumer;
import tech.flowcatalyst.messagerouter.factory.MediatorFactory;
import tech.flowcatalyst.messagerouter.factory.QueueConsumerFactory;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.pool.ProcessPool;
import tech.flowcatalyst.messagerouter.pool.ProcessPoolImpl;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.util.ArrayList;
import java.util.HashMap;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.locks.ReentrantLock;

@ApplicationScoped
public class QueueManager implements MessageCallback {

    private static final Logger LOG = Logger.getLogger(QueueManager.class);
    private static final int MIN_QUEUE_CAPACITY = 50;  // Reduced from 500 to prevent large buffers
    private static final int QUEUE_CAPACITY_MULTIPLIER = 2;  // Buffer size = concurrency * 2 (allows some headroom)
    private static final String DEFAULT_POOL_CODE = "DEFAULT-POOL";
    private static final int DEFAULT_POOL_CONCURRENCY = 20;

    @ConfigProperty(name = "message-router.enabled", defaultValue = "true")
    boolean messageRouterEnabled;

    @ConfigProperty(name = "message-router.max-pools", defaultValue = "2000")
    int maxPools;

    @ConfigProperty(name = "message-router.pool-warning-threshold", defaultValue = "1000")
    int poolWarningThreshold;

    @Inject
    @RestClient
    MessageRouterConfigClient configClient;

    @Inject
    QueueConsumerFactory queueConsumerFactory;

    @Inject
    MediatorFactory mediatorFactory;

    @Inject
    tech.flowcatalyst.messagerouter.health.QueueValidationService queueValidationService;

    @Inject
    PoolMetricsService poolMetrics;

    @Inject
    tech.flowcatalyst.messagerouter.metrics.QueueMetricsService queueMetrics;

    @Inject
    WarningService warningService;

    @Inject
    MeterRegistry meterRegistry;

    // StandbyService is optional - injected if standby is enabled (from shared module)
    @jakarta.inject.Inject
    jakarta.enterprise.inject.Instance<tech.flowcatalyst.standby.StandbyService> standbyServiceInstance;

    private tech.flowcatalyst.standby.StandbyService standbyService() {
        return standbyServiceInstance.isResolvable() ? standbyServiceInstance.get() : null;
    }

    // Consolidated in-flight message tracking - replaces 5 separate maps
    private final InFlightMessageTracker inFlightTracker = new InFlightMessageTracker();

    // Legacy reference to inPipelineMap for ProcessPoolImpl compatibility
    // TODO: Update ProcessPoolImpl to not require this map reference
    private final ConcurrentHashMap<String, MessagePointer> inPipelineMap = new ConcurrentHashMap<>();

    private final ConcurrentHashMap<String, ProcessPool> processPools = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, QueueConsumer> queueConsumers = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, QueueConfig> queueConfigs = new ConcurrentHashMap<>();

    // Draining resources that are being phased out asynchronously
    private final ConcurrentHashMap<String, ProcessPool> drainingPools = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, QueueConsumer> drainingConsumers = new ConcurrentHashMap<>();

    private volatile boolean initialized = false;
    private volatile boolean shutdownInProgress = false;

    // Use ReentrantLock instead of synchronized to avoid pinning virtual threads
    private final ReentrantLock syncLock = new ReentrantLock();

    // Gauges for monitoring map sizes to detect memory leaks
    private AtomicInteger inPipelineMapSizeGauge;
    private AtomicInteger messageCallbacksMapSizeGauge;
    private AtomicInteger activePoolCountGauge;
    private io.micrometer.core.instrument.Counter defaultPoolUsageCounter;

    /**
     * Default constructor for CDI
     */
    public QueueManager() {
        // CDI will inject dependencies
    }

    /**
     * Test-friendly constructor with dependency injection
     * Package-private to allow test access while keeping internal
     */
    QueueManager(
        MessageRouterConfigClient configClient,
        QueueConsumerFactory queueConsumerFactory,
        MediatorFactory mediatorFactory,
        tech.flowcatalyst.messagerouter.health.QueueValidationService queueValidationService,
        PoolMetricsService poolMetrics,
        tech.flowcatalyst.messagerouter.metrics.QueueMetricsService queueMetrics,
        WarningService warningService,
        MeterRegistry meterRegistry,
        boolean messageRouterEnabled,
        int maxPools,
        int poolWarningThreshold
    ) {
        this.configClient = configClient;
        this.queueConsumerFactory = queueConsumerFactory;
        this.mediatorFactory = mediatorFactory;
        this.queueValidationService = queueValidationService;
        this.poolMetrics = poolMetrics;
        this.queueMetrics = queueMetrics;
        this.warningService = warningService;
        this.meterRegistry = meterRegistry;
        this.messageRouterEnabled = messageRouterEnabled;
        this.maxPools = maxPools;
        this.poolWarningThreshold = poolWarningThreshold;
        initializeMetrics();
    }

    void onStartup(@Observes StartupEvent event) {
        logVirtualThreadConfig();
        initializeMetrics();
    }

    /**
     * Log virtual thread scheduler configuration at startup for diagnostics
     */
    private void logVirtualThreadConfig() {
        String parallelism = System.getProperty("jdk.virtualThreadScheduler.parallelism", "not set");
        String maxPoolSize = System.getProperty("jdk.virtualThreadScheduler.maxPoolSize", "not set");
        int availableProcessors = Runtime.getRuntime().availableProcessors();

        LOG.infof("Virtual Thread Config: parallelism=%s, maxPoolSize=%s, availableProcessors=%d",
            parallelism, maxPoolSize, availableProcessors);

        // Test that virtual threads are working by creating one
        Thread.startVirtualThread(() -> {
            LOG.infof("Virtual thread test: running on thread [%s], isVirtual=%s",
                Thread.currentThread().getName(), Thread.currentThread().isVirtual());
        });
    }

    /**
     * Initialize Micrometer gauges for map size monitoring
     */
    private void initializeMetrics() {
        if (meterRegistry == null) {
            // For tests or when metrics are not available
            inPipelineMapSizeGauge = new AtomicInteger(0);
            messageCallbacksMapSizeGauge = new AtomicInteger(0);
            activePoolCountGauge = new AtomicInteger(0);
            return;
        }

        // Create gauge for inPipelineMap size
        inPipelineMapSizeGauge = new AtomicInteger(0);
        meterRegistry.gauge(
            "flowcatalyst.queuemanager.pipeline.size",
            List.of(Tag.of("type", "inPipeline")),
            inPipelineMapSizeGauge
        );

        // Create gauge for messageCallbacks size
        messageCallbacksMapSizeGauge = new AtomicInteger(0);
        meterRegistry.gauge(
            "flowcatalyst.queuemanager.callbacks.size",
            List.of(Tag.of("type", "callbacks")),
            messageCallbacksMapSizeGauge
        );

        // Create gauge for active pool count
        activePoolCountGauge = new AtomicInteger(0);
        meterRegistry.gauge(
            "flowcatalyst.queuemanager.pools.active",
            List.of(Tag.of("type", "pools")),
            activePoolCountGauge
        );

        // Create counter for default pool usage (indicates missing pool configuration)
        defaultPoolUsageCounter = meterRegistry.counter(
            "flowcatalyst.queuemanager.defaultpool.usage",
            List.of(Tag.of("pool", DEFAULT_POOL_CODE))
        );

        LOG.infof("QueueManager metrics initialized (max pools: %d, warning threshold: %d)",
            maxPools, poolWarningThreshold);
    }

    /**
     * Update map size gauges
     */
    private void updateMapSizeGauges() {
        int trackerSize = inFlightTracker.size();
        if (inPipelineMapSizeGauge != null) {
            inPipelineMapSizeGauge.set(trackerSize);
        }
        if (messageCallbacksMapSizeGauge != null) {
            // Callbacks are tracked inside InFlightMessageTracker, same size
            messageCallbacksMapSizeGauge.set(trackerSize);
        }
        if (activePoolCountGauge != null) {
            activePoolCountGauge.set(processPools.size());
        }
    }

    void onShutdown(@Observes ShutdownEvent event) {
        LOG.info("QueueManager shutting down...");

        // FIRST: Stop all background scheduled tasks (metrics, health checks, etc.)
        shutdownInProgress = true;
        LOG.info("Shutdown flag set - all scheduled tasks will now exit");

        // Give scheduled tasks a moment to exit cleanly
        try {
            Thread.sleep(500);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }

        // Now do the actual shutdown
        stopAllConsumers();
        drainAllPools();
        cleanupRemainingMessages();
    }

    /**
     * Clean up any remaining messages in the pipeline during shutdown
     * This ensures messages are properly nacked back to the queue
     */
    private void cleanupRemainingMessages() {
        LOG.info("Cleaning up remaining messages in pipeline...");

        int pipelineSize = inFlightTracker.size();

        if (pipelineSize == 0) {
            LOG.info("No remaining messages to clean up");
            return;
        }

        LOG.infof("Found %d messages in pipeline to clean up", pipelineSize);

        int nackedCount = 0;
        int errorCount = 0;
        long startTime = System.currentTimeMillis();

        // Nack all messages still in pipeline and clear tracking
        var clearedMessages = inFlightTracker.clear().toList();
        for (var tracked : clearedMessages) {
            try {
                tracked.callback().nack(tracked.message());
                nackedCount++;
                LOG.debugf("Nacked message [%s] during shutdown", tracked.messageId());
            } catch (Exception e) {
                errorCount++;
                LOG.errorf(e, "Error nacking message [%s] during shutdown: %s", tracked.messageId(), e.getMessage());
            }
        }

        // Also clear the legacy inPipelineMap used by ProcessPoolImpl
        inPipelineMap.clear();

        // Update gauges one final time
        updateMapSizeGauges();

        long durationMs = System.currentTimeMillis() - startTime;

        LOG.infof("Shutdown cleanup completed in %d ms - nacked: %d, errors: %d, total: %d",
            durationMs, nackedCount, errorCount, pipelineSize);

        // Add warning if there were errors during cleanup
        if (errorCount > 0) {
            warningService.addWarning(
                "SHUTDOWN_CLEANUP_ERRORS",
                "WARN",
                String.format("Encountered %d errors while nacking %d messages during shutdown", errorCount, pipelineSize),
                "QueueManager"
            );
        }
    }

    /**
     * Periodically check for potential memory leaks in the pipeline
     * Runs every 30 seconds to detect anomalies early
     */
    @Scheduled(every = "30s")
    @RunOnVirtualThread
    void checkForMapLeaks() {
        if (!initialized || shutdownInProgress) {
            // Skip check until system is initialized or during shutdown
            return;
        }

        int pipelineSize = inFlightTracker.size();

        // Calculate total pool capacity (sum of all pool queue capacities)
        int totalCapacity = processPools.values().stream()
            .mapToInt(pool -> pool.getConcurrency() * QUEUE_CAPACITY_MULTIPLIER)
            .sum();

        // Add minimum capacity for default pool that might be created
        totalCapacity = Math.max(totalCapacity, MIN_QUEUE_CAPACITY);

        // WARNING: Pipeline size exceeds total pool capacity
        // This indicates messages are not being removed from tracking
        if (pipelineSize > totalCapacity) {
            warningService.addWarning(
                "PIPELINE_MAP_LEAK",
                "WARN",
                String.format("In-flight tracker size (%d) exceeds total pool capacity (%d) - possible memory leak",
                    pipelineSize, totalCapacity),
                "QueueManager"
            );
            LOG.warnf("LEAK DETECTION: in-flight tracker size (%d) > total capacity (%d)", pipelineSize, totalCapacity);
        }

        // Note: Map size mismatch check is no longer needed since InFlightMessageTracker
        // maintains consistency between message and callback tracking internally

        // INFO: Log current tracker size for monitoring
        if (LOG.isDebugEnabled()) {
            LOG.debugf("In-flight tracker size: %d, total capacity: %d", pipelineSize, totalCapacity);
        }
    }

    /**
     * Periodically clean up draining pools and consumers that have finished processing
     * Runs every 10 seconds to check if old resources can be cleaned up
     */
    @Scheduled(every = "10s")
    @RunOnVirtualThread
    void cleanupDrainingResources() {
        if (!initialized || shutdownInProgress) {
            return;
        }

        // Check draining pools
        for (Map.Entry<String, ProcessPool> entry : drainingPools.entrySet()) {
            String poolCode = entry.getKey();
            ProcessPool pool = entry.getValue();

            // Check if pool has finished draining (queue empty and all workers idle)
            if (pool.isFullyDrained()) {
                LOG.infof("Pool [%s] has fully drained, cleaning up resources", poolCode);
                pool.shutdown();
                drainingPools.remove(poolCode);
                poolMetrics.removePoolMetrics(poolCode);
                LOG.infof("Cleaned up draining pool [%s]", poolCode);
            } else {
                if (LOG.isDebugEnabled()) {
                    LOG.debugf("Pool [%s] still draining (queue: %d, active: %d)",
                        poolCode, pool.getQueueSize(), pool.getActiveWorkers());
                }
            }
        }

        // Check draining consumers
        for (Map.Entry<String, QueueConsumer> entry : drainingConsumers.entrySet()) {
            String queueId = entry.getKey();
            QueueConsumer consumer = entry.getValue();

            // Check if consumer has finished (all threads terminated)
            if (consumer.isFullyStopped()) {
                LOG.infof("Consumer [%s] has fully stopped, cleaning up resources", queueId);
                drainingConsumers.remove(queueId);
                LOG.infof("Cleaned up draining consumer [%s]", queueId);
            } else {
                if (LOG.isDebugEnabled()) {
                    LOG.debugf("Consumer [%s] still stopping", queueId);
                }
            }
        }
    }

    /**
     * Periodically monitor consumer health and restart stalled/unhealthy consumers
     * Runs every 60 seconds to detect and remediate hung consumer threads
     */
    @Scheduled(every = "60s")
    @RunOnVirtualThread
    void monitorAndRestartUnhealthyConsumers() {
        if (!initialized || shutdownInProgress) {
            return;
        }

        // Log every health check run to diagnose scheduling issues
        LOG.infof("Health check running - checking %d consumers", queueConsumers.size());

        for (Map.Entry<String, QueueConsumer> entry : queueConsumers.entrySet()) {
            String queueIdentifier = entry.getKey();
            QueueConsumer consumer = entry.getValue();

            // Check if consumer is unhealthy (stalled/hung)
            int instanceId = System.identityHashCode(consumer);
            long lastPollTime = consumer.getLastPollTime();
            long timeSinceLastPoll = lastPollTime > 0 ?
                (System.currentTimeMillis() - lastPollTime) / 1000 : -1;

            LOG.debugf("Health check: queue [%s] instanceId=%d, lastPoll=%ds ago, healthy=%s",
                queueIdentifier, instanceId, timeSinceLastPoll, consumer.isHealthy());

            if (!consumer.isHealthy()) {
                LOG.warnf("Consumer for queue [%s] is unhealthy (instanceId=%d, lastPollTime=%d, lastPoll %ds ago) - initiating restart",
                    queueIdentifier, instanceId, lastPollTime, timeSinceLastPoll);

                // Add warning for visibility
                warningService.addWarning(
                    "CONSUMER_RESTART",
                    "WARN",
                    String.format("Consumer for queue [%s] was unhealthy (last poll %ds ago) and has been restarted",
                        queueIdentifier, timeSinceLastPoll),
                    "QueueManager"
                );

                try {
                    // Get the queue configuration for this consumer
                    QueueConfig queueConfig = queueConfigs.get(queueIdentifier);
                    if (queueConfig == null) {
                        LOG.errorf("Cannot restart consumer [%s] - queue configuration not found", queueIdentifier);
                        continue;
                    }

                    // Stop the unhealthy consumer
                    LOG.infof("Stopping unhealthy consumer for queue [%s]", queueIdentifier);
                    consumer.stop();

                    // Move to draining for cleanup
                    queueConsumers.remove(queueIdentifier);
                    drainingConsumers.put(queueIdentifier, consumer);

                    // Calculate connections (same logic as syncConfiguration)
                    int connections = queueConfig.connections() != null
                        ? queueConfig.connections()
                        : 1; // Default to 1 if not specified

                    // Create and start new consumer
                    LOG.infof("Creating replacement consumer for queue [%s] with %d connections",
                        queueIdentifier, connections);
                    QueueConsumer newConsumer = queueConsumerFactory.createConsumer(queueConfig, connections);
                    newConsumer.start();
                    queueConsumers.put(queueIdentifier, newConsumer);

                    LOG.infof("Successfully restarted consumer for queue [%s]", queueIdentifier);

                } catch (Exception e) {
                    LOG.errorf(e, "Failed to restart consumer for queue [%s]", queueIdentifier);
                    warningService.addWarning(
                        "CONSUMER_RESTART_FAILED",
                        "CRITICAL",
                        String.format("Failed to restart consumer for queue [%s]: %s",
                            queueIdentifier, e.getMessage()),
                        "QueueManager"
                    );
                }
            }
        }
    }

    @Scheduled(every = "${message-router.sync-interval:5m}", delay = 2, delayUnit = java.util.concurrent.TimeUnit.SECONDS)
    @RunOnVirtualThread
    void scheduledSync() {
        if (shutdownInProgress) {
            return;
        }

        // Check standby status - only primary processes messages
        var standby = standbyService();
        if (standby != null && !standby.isPrimary()) {
            if (!initialized) {
                LOG.info("In standby mode, waiting for primary lock...");
                initialized = true;
            }
            return;
        }

        if (!messageRouterEnabled) {
            if (!initialized) {
                LOG.info("Message router is disabled, skipping initialization");
                initialized = true;
            }
            return;
        }

        boolean isInitialSync = !initialized;
        if (isInitialSync) {
            LOG.info("QueueManager initializing on first scheduled sync...");
        } else {
            LOG.info("Running scheduled configuration sync");
        }

        boolean syncSuccess = syncConfiguration(isInitialSync);

        if (isInitialSync) {
            if (syncSuccess) {
                initialized = true;
                LOG.info("QueueManager initialization completed successfully");
            } else {
                LOG.error("Initial configuration sync failed after all retries - shutting down application");
                warningService.addWarning(
                    "CONFIG_SYNC_FAILED",
                    "CRITICAL",
                    "Initial configuration sync failed after 1 minute of retries - application will exit",
                    "QueueManager"
                );
                Quarkus.asyncExit(1);
            }
        } else if (!syncSuccess) {
            LOG.warn("Configuration sync failed - continuing with existing configuration");
            warningService.addWarning(
                "CONFIG_SYNC_FAILED",
                "WARN",
                "Scheduled configuration sync failed - continuing with existing configuration",
                "QueueManager"
            );
        }
    }

    private boolean syncConfiguration(boolean isInitialSync) {
        // Use ReentrantLock instead of synchronized to avoid pinning virtual threads
        syncLock.lock();
        try {
            return doSyncConfiguration(isInitialSync);
        } finally {
            syncLock.unlock();
        }
    }

    private boolean doSyncConfiguration(boolean isInitialSync) {
        // Retry logic: 12 attempts with 5-second delays = 1 minute total
        // For initial sync failures, application will exit
        // For subsequent sync failures, application continues with existing config
        MessageRouterConfig config = null;
        int attempts = 0;
        int maxAttempts = 12;
        int retryDelayMs = 5000;

        while (config == null && attempts < maxAttempts) {
            try {
                attempts++;
                LOG.infof("Fetching queue configuration (attempt %d/%d)...", attempts, maxAttempts);
                config = configClient.getQueueConfig();
            } catch (Exception e) {
                if (attempts >= maxAttempts) {
                    LOG.errorf(e, "Failed to fetch configuration after %d attempts over %d seconds",
                        maxAttempts, (maxAttempts * retryDelayMs) / 1000);
                    return false;
                }
                LOG.warnf("Failed to fetch config (attempt %d/%d), retrying in %d seconds: %s",
                    attempts, maxAttempts, retryDelayMs / 1000, e.getMessage());
                try {
                    Thread.sleep(retryDelayMs);
                } catch (InterruptedException ie) {
                    Thread.currentThread().interrupt();
                    LOG.error("Configuration fetch interrupted");
                    return false;
                }
            }
        }

        if (config == null) {
            LOG.error("Configuration is null after all retries");
            return false;
        }

        try {

            // Step 1: Identify pools that need to be replaced
            // Use parallel approach: stop consuming, start new pool, drain old pool async
            Map<String, ProcessingPool> newPools = new ConcurrentHashMap<>();
            for (ProcessingPool poolConfig : config.processingPools()) {
                newPools.put(poolConfig.code(), poolConfig);
            }

            // Handle pool config changes: update in-place or drain if removed
            for (Map.Entry<String, ProcessPool> entry : processPools.entrySet()) {
                String poolCode = entry.getKey();
                ProcessPool existingPool = entry.getValue();
                ProcessingPool newPoolConfig = newPools.get(poolCode);

                if (newPoolConfig == null) {
                    // Pool removed from config: drain and remove
                    LOG.infof("Pool [%s] removed from config - draining asynchronously", poolCode);
                    processPools.remove(poolCode);
                    drainingPools.put(poolCode, existingPool);
                    updateMapSizeGauges();
                    LOG.infof("Pool [%s] moved to draining state (queue: %d, active: %d)",
                        poolCode, existingPool.getQueueSize(), existingPool.getActiveWorkers());
                } else {
                    // Pool exists in new config: update concurrency and/or rate limit in-place
                    boolean concurrencyChanged = newPoolConfig.effectiveConcurrency() != existingPool.getConcurrency();
                    boolean rateLimitChanged = !java.util.Objects.equals(newPoolConfig.rateLimitPerMinute(), existingPool.getRateLimitPerMinute());

                    if (concurrencyChanged) {
                        int oldConcurrency = existingPool.getConcurrency();
                        int newConcurrency = newPoolConfig.effectiveConcurrency();
                        boolean updateSuccess = existingPool.updateConcurrency(newConcurrency, 60); // 60 second timeout
                        if (updateSuccess) {
                            LOG.infof("Pool [%s] concurrency updated: %d -> %d (in-place)",
                                poolCode, oldConcurrency, newConcurrency);
                        } else {
                            LOG.warnf("Pool [%s] concurrency update timed out waiting for idle slots. " +
                                "Current concurrency: %d, target: %d, active workers: %d",
                                poolCode, oldConcurrency, newConcurrency, existingPool.getActiveWorkers());
                        }
                    }

                    if (rateLimitChanged) {
                        Integer oldLimit = existingPool.getRateLimitPerMinute();
                        Integer newLimit = newPoolConfig.rateLimitPerMinute();
                        existingPool.updateRateLimit(newLimit);
                        LOG.infof("Pool [%s] rate limit updated: %s -> %s (in-place)",
                            poolCode,
                            oldLimit != null ? oldLimit + "/min" : "none",
                            newLimit != null ? newLimit + "/min" : "none");
                    }
                }
            }

            // Step 3: Start new or updated pools
            for (ProcessingPool poolConfig : config.processingPools()) {
                if (!processPools.containsKey(poolConfig.code())) {
                    // Check pool count limits before creating new pool
                    int currentPoolCount = processPools.size();

                    if (currentPoolCount >= maxPools) {
                        LOG.errorf("Cannot create pool [%s]: Maximum pool limit reached (%d/%d). " +
                            "Increase message-router.max-pools or scale up instance size.",
                            poolConfig.code(), currentPoolCount, maxPools);
                        warningService.addWarning(
                            "POOL_LIMIT",
                            "CRITICAL",
                            String.format("Max pool limit reached (%d/%d) - cannot create pool [%s]",
                                currentPoolCount, maxPools, poolConfig.code()),
                            "QueueManager"
                        );
                        continue; // Skip this pool
                    }

                    if (currentPoolCount >= poolWarningThreshold) {
                        LOG.warnf("Pool count approaching limit: %d/%d (warning threshold: %d). " +
                            "Consider increasing max-pools or scaling instance.",
                            currentPoolCount, maxPools, poolWarningThreshold);
                        warningService.addWarning(
                            "POOL_LIMIT",
                            "WARNING",
                            String.format("Pool count %d approaching limit %d (threshold: %d)",
                                currentPoolCount, maxPools, poolWarningThreshold),
                            "QueueManager"
                        );
                    }

                    // Calculate queue capacity: 2x concurrency with minimum of 50
                    int effectiveConcurrency = poolConfig.effectiveConcurrency();
                    int queueCapacity = Math.max(effectiveConcurrency * QUEUE_CAPACITY_MULTIPLIER, MIN_QUEUE_CAPACITY);

                    LOG.infof("Creating new process pool [%s] with concurrency %d and queue capacity %d (pool %d/%d)",
                        poolConfig.code(), effectiveConcurrency, queueCapacity, currentPoolCount + 1, maxPools);

                    // Determine mediator type based on pool code
                    tech.flowcatalyst.messagerouter.model.MediationType mediatorType = determineMediatorType(poolConfig.code());
                    Mediator mediator = mediatorFactory.createMediator(mediatorType);

                    ProcessPool pool = new ProcessPoolImpl(
                        poolConfig.code(),
                        effectiveConcurrency,
                        queueCapacity,
                        poolConfig.rateLimitPerMinute(),
                        mediator,
                        this,
                        inPipelineMap,
                        poolMetrics,
                        warningService
                    );

                    pool.start();
                    processPools.put(poolConfig.code(), pool);
                    updateMapSizeGauges(); // Update pool count metric
                }
            }

            // Step 4: Sync queue consumers using parallel approach
            // Stop old consumer -> Start new consumer -> Old consumer finishes async
            Map<String, QueueConfig> newQueues = new ConcurrentHashMap<>();
            for (QueueConfig queueConfig : config.queues()) {
                String queueIdentifier = queueConfig.queueName() != null
                    ? queueConfig.queueName()
                    : queueConfig.queueUri();
                newQueues.put(queueIdentifier, queueConfig);
            }

            // Phase out consumers for queues that no longer exist
            for (Map.Entry<String, QueueConsumer> entry : queueConsumers.entrySet()) {
                String queueIdentifier = entry.getKey();
                if (!newQueues.containsKey(queueIdentifier)) {
                    LOG.infof("Phasing out consumer for removed queue [%s]", queueIdentifier);
                    QueueConsumer consumer = entry.getValue();

                    // Stop consumer (sets running=false, initiates graceful shutdown)
                    // This stops polling immediately but lets in-flight messages complete
                    consumer.stop();

                    // Move to draining consumers for async cleanup
                    queueConsumers.remove(queueIdentifier);
                    queueConfigs.remove(queueIdentifier);
                    drainingConsumers.put(queueIdentifier, consumer);

                    LOG.infof("Consumer [%s] moved to draining state", queueIdentifier);
                }
            }

            // Validate all queues (raises warnings for missing queues but doesn't stop processing)
            LOG.info("Validating queue accessibility...");
            List<String> queueIssues = queueValidationService.validateQueues(config.queues());
            if (!queueIssues.isEmpty()) {
                LOG.warnf("Found %d queue validation issues - will attempt to create consumers anyway", queueIssues.size());
            }

            // Start consumers for new queues (leave existing ones running)
            for (QueueConfig queueConfig : config.queues()) {
                String queueIdentifier = queueConfig.queueName() != null
                    ? queueConfig.queueName()
                    : queueConfig.queueUri();

                if (!queueConsumers.containsKey(queueIdentifier)) {
                    // Use per-queue connections if specified, otherwise default to 1
                    int connections = queueConfig.connections() != null
                        ? queueConfig.connections()
                        : 1; // Default to 1 connection per queue

                    LOG.infof("Creating new queue consumer for [%s] with %d connections",
                        queueIdentifier, connections);

                    QueueConsumer consumer = queueConsumerFactory.createConsumer(queueConfig, connections);
                    consumer.start();
                    queueConsumers.put(queueIdentifier, consumer);
                    queueConfigs.put(queueIdentifier, queueConfig);
                } else {
                    LOG.debugf("Queue consumer for [%s] already running, leaving unchanged", queueIdentifier);
                }
            }

            LOG.info("Configuration sync completed successfully");
            return true;

        } catch (Exception e) {
            LOG.error("Failed to sync configuration", e);
            return false;
        }
    }

    private void stopAllConsumers() {
        LOG.info("Stopping all queue consumers during shutdown");

        // Initiate shutdown for ALL consumers (non-blocking, fast)
        for (QueueConsumer consumer : queueConsumers.values()) {
            try {
                consumer.stop(); // Sets flag and initiates shutdown, returns immediately
                drainingConsumers.put(consumer.getQueueIdentifier(), consumer);
            } catch (Exception e) {
                LOG.errorf(e, "Error stopping consumer: %s", consumer.getQueueIdentifier());
            }
        }
        queueConsumers.clear();

        // Now wait for all consumers to finish in PARALLEL
        // Max 25s (enough for SQS 20s long poll + small buffer)
        LOG.infof("Waiting for %d consumers to finish stopping (in parallel)...", drainingConsumers.size());
        long startTime = System.currentTimeMillis();
        long maxWaitMs = 25_000; // 25 seconds for all consumers to naturally complete current polls

        while (!drainingConsumers.isEmpty() && (System.currentTimeMillis() - startTime) < maxWaitMs) {
            List<String> stoppedConsumers = new ArrayList<>();

            for (Map.Entry<String, QueueConsumer> entry : drainingConsumers.entrySet()) {
                String queueId = entry.getKey();
                QueueConsumer consumer = entry.getValue();

                if (consumer.isFullyStopped()) {
                    LOG.debugf("Consumer [%s] fully stopped during shutdown", queueId);
                    stoppedConsumers.add(queueId);
                }
            }

            // Remove stopped consumers after iteration
            for (String queueId : stoppedConsumers) {
                drainingConsumers.remove(queueId);
            }

            if (!drainingConsumers.isEmpty()) {
                try {
                    Thread.sleep(500);
                } catch (InterruptedException e) {
                    LOG.warn("Interrupted while waiting for consumers to stop");
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }

        if (!drainingConsumers.isEmpty()) {
            LOG.warnf("%d consumers did not fully stop within 25s, forcing shutdown now", drainingConsumers.size());
            drainingConsumers.clear();
        } else {
            LOG.info("All consumers stopped cleanly");
        }
    }

    private void drainAllPools() {
        LOG.info("Draining all process pools during shutdown");

        // Move all active pools to draining state
        for (ProcessPool pool : processPools.values()) {
            try {
                pool.drain();
                drainingPools.put(pool.getPoolCode(), pool);
            } catch (Exception e) {
                LOG.errorf(e, "Error draining pool: %s", pool.getPoolCode());
            }
        }
        processPools.clear();

        // Wait for all pools to finish draining (blocking during shutdown is ok)
        LOG.infof("Waiting for %d pools to finish draining...", drainingPools.size());
        long startTime = System.currentTimeMillis();
        long maxWaitMs = 60_000; // 60 seconds max wait

        while (!drainingPools.isEmpty() && (System.currentTimeMillis() - startTime) < maxWaitMs) {
            List<String> drainedPools = new ArrayList<>();

            for (Map.Entry<String, ProcessPool> entry : drainingPools.entrySet()) {
                String poolCode = entry.getKey();
                ProcessPool pool = entry.getValue();

                if (pool.isFullyDrained()) {
                    LOG.infof("Pool [%s] fully drained during shutdown", poolCode);
                    pool.shutdown();
                    drainedPools.add(poolCode);
                }
            }

            // Remove drained pools after iteration
            for (String poolCode : drainedPools) {
                drainingPools.remove(poolCode);
            }

            if (!drainingPools.isEmpty()) {
                try {
                    Thread.sleep(500);
                } catch (InterruptedException e) {
                    LOG.warn("Interrupted while waiting for pools to drain");
                    Thread.currentThread().interrupt();
                    break;
                }
            }
        }

        // Force shutdown any remaining pools
        if (!drainingPools.isEmpty()) {
            LOG.warnf("%d pools did not finish draining within %d seconds, forcing shutdown",
                drainingPools.size(), maxWaitMs / 1000);
            for (ProcessPool pool : drainingPools.values()) {
                try {
                    pool.shutdown();
                } catch (Exception e) {
                    LOG.errorf(e, "Error forcing shutdown of pool: %s", pool.getPoolCode());
                }
            }
            drainingPools.clear();
        }

        LOG.info("All pools drained");
    }

    /**
     * Called by queue consumers to route a batch of messages with batch-level policies.
     * Implements the following batch-level rules:
     * 1. Duplicate detection (individual messages)
     * 2. Pool buffer capacity check (nack all messages for full pools)
     * 3. Rate limit check (nack all messages for rate-limited pools)
     * 4. MessageGroup sequential nacking (if one message in group is nacked, nack all subsequent in batch)
     *
     * @param messages list of messages with their callbacks
     */
    public void routeMessageBatch(List<BatchMessage> messages) {
        if (messages.isEmpty()) {
            return;
        }

        // Take snapshot of pools for consistent view during this batch
        // This prevents race conditions if syncConfiguration() modifies pools mid-batch
        // With Issue #11 fix, submitting to a draining pool safely returns false
        final Map<String, ProcessPool> poolSnapshot = Map.copyOf(processPools);

        // Generate unique batch ID for FIFO tracking
        String batchId = UUID.randomUUID().toString();
        LOG.debugf("Processing batch [%s] with %d messages", batchId, messages.size());

        // Phase 1: Filter duplicates and group by pool
        // Check both SQS message ID (same physical message) AND application message ID (requeued message)
        Map<String, List<BatchMessage>> messagesByPool = new HashMap<>();
        List<BatchMessage> duplicates = new ArrayList<>();
        List<BatchMessage> requeuedDuplicates = new ArrayList<>();

        for (BatchMessage batchMsg : messages) {
            String sqsMessageId = batchMsg.sqsMessageId();
            String appMessageId = batchMsg.message().id();
            String pipelineKey = sqsMessageId != null ? sqsMessageId : appMessageId;

            // Check 1: Same broker message ID (physical redelivery from SQS due to visibility timeout)
            // This MUST be checked FIRST because the same broker ID means it's a visibility timeout redelivery,
            // NOT a requeue by an external process
            if (sqsMessageId != null && inFlightTracker.containsKey(sqsMessageId)) {
                LOG.debugf("SQS message [%s] (app ID: %s) already in pipeline - redelivery due to visibility timeout",
                    sqsMessageId, appMessageId);

                // Update the stored callback's receipt handle with the new one from the redelivered message
                // This ensures when processing completes, ACK uses the valid (latest) receipt handle
                updateReceiptHandleIfPossible(sqsMessageId, appMessageId, batchMsg.callback());

                duplicates.add(batchMsg);
            }
            // Check 2: Same application message ID but DIFFERENT broker ID (requeued by external process)
            // This happens when a separate process requeues messages that were stuck in QUEUED status for 20+ min
            // The external process creates a NEW SQS message with the same application message ID
            else if (inFlightTracker.isInFlight(appMessageId)) {
                var existingTracked = inFlightTracker.get(pipelineKey);
                String existingPipelineKey = existingTracked.map(t -> t.pipelineKey()).orElse(null);

                // Only treat as requeued duplicate if the broker message IDs are DIFFERENT
                // If they're the same, it would have been caught by the check above
                if (sqsMessageId != null && !sqsMessageId.equals(existingPipelineKey)) {
                    LOG.infof("Requeued message detected: app ID [%s] already in pipeline (existing SQS ID: %s, new SQS ID: %s) - will ACK to remove duplicate",
                        appMessageId, existingPipelineKey, sqsMessageId);
                    requeuedDuplicates.add(batchMsg);
                } else {
                    // Same broker ID or null - treat as visibility timeout redelivery
                    LOG.debugf("App message [%s] in pipeline with same SQS ID [%s] - visibility timeout redelivery",
                        appMessageId, sqsMessageId);
                    if (existingPipelineKey != null) {
                        updateReceiptHandleIfPossible(existingPipelineKey, appMessageId, batchMsg.callback());
                    }
                    duplicates.add(batchMsg);
                }
            } else {
                String poolCode = batchMsg.message().poolCode();
                messagesByPool.computeIfAbsent(poolCode, k -> new ArrayList<>()).add(batchMsg);
            }
        }

        // Nack all duplicates (SQS redelivery - let SQS retry later)
        // These are deferred, not failures - the original is still processing
        for (BatchMessage dup : duplicates) {
            dup.callback().nack(dup.message());
            queueMetrics.recordMessageDeferred(getQueueIdentifier(dup));
        }

        // ACK all requeued duplicates (external process requeued while original still processing)
        // These need to be ACKed (not NACKed) to permanently remove the duplicate from the queue
        for (BatchMessage requeuedDup : requeuedDuplicates) {
            LOG.infof("ACKing requeued duplicate message [%s] (SQS ID: %s) - original still in pipeline",
                requeuedDup.message().id(), requeuedDup.sqsMessageId());
            requeuedDup.callback().ack(requeuedDup.message());
            queueMetrics.recordMessageProcessed(getQueueIdentifier(requeuedDup), true);
        }

        // Phase 2: Check pool buffer capacity
        Map<String, List<BatchMessage>> messagesToRoute = new HashMap<>();
        List<BatchMessage> toNackPoolFull = new ArrayList<>();

        for (Map.Entry<String, List<BatchMessage>> entry : messagesByPool.entrySet()) {
            String poolCode = entry.getKey();
            List<BatchMessage> poolMessages = entry.getValue();

            // Get pool from snapshot (consistent view for entire batch)
            ProcessPool pool = poolSnapshot.get(poolCode);
            if (pool == null) {
                LOG.warnf("No process pool found for code [%s], routing to default pool", poolCode);
                if (defaultPoolUsageCounter != null) {
                    defaultPoolUsageCounter.increment();
                }
                warningService.addWarning(
                    "ROUTING",
                    "WARN",
                    String.format("No pool found for code [%s], using default pool", poolCode),
                    "QueueManager"
                );
                pool = getOrCreateDefaultPool();
            }

            // Check if pool has capacity for ALL messages for this pool
            int availableCapacity = pool.getQueueCapacity() - pool.getQueueSize();
            if (availableCapacity < poolMessages.size()) {
                LOG.warnf("Pool [%s] buffer full - nacking all %d messages for this pool in batch",
                    poolCode, poolMessages.size());
                toNackPoolFull.addAll(poolMessages);
                warningService.addWarning(
                    "QUEUE_FULL",
                    "WARN",
                    String.format("Pool [%s] queue full, nacking %d messages from batch",
                        poolCode, poolMessages.size()),
                    "QueueManager"
                );
                continue;
            }

            // Pool is available - add to routing list
            // Note: Rate limiting is now handled inside the pool worker (blocking wait)
            messagesToRoute.put(poolCode, poolMessages);
        }

        // Nack all pool-full messages (deferred - will retry when capacity available)
        for (BatchMessage batchMsg : toNackPoolFull) {
            batchMsg.callback().nack(batchMsg.message());
            queueMetrics.recordMessageDeferred(getQueueIdentifier(batchMsg));
        }

        // Phase 3: Route messages with messageGroup sequential nacking
        for (Map.Entry<String, List<BatchMessage>> entry : messagesToRoute.entrySet()) {
            String poolCode = entry.getKey();
            List<BatchMessage> poolMessages = entry.getValue();
            // Use snapshot for consistency - pool reference same as Phase 2
            ProcessPool pool = poolSnapshot.getOrDefault(poolCode, getOrCreateDefaultPool());

            // Group by messageGroupId to enforce sequential nacking within groups
            Map<String, List<BatchMessage>> messagesByGroup = new LinkedHashMap<>();
            for (BatchMessage batchMsg : poolMessages) {
                String groupId = batchMsg.message().messageGroupId();
                if (groupId == null || groupId.isBlank()) {
                    groupId = "__DEFAULT__";
                }
                messagesByGroup.computeIfAbsent(groupId, k -> new ArrayList<>()).add(batchMsg);
            }

            // Process each group sequentially
            for (Map.Entry<String, List<BatchMessage>> groupEntry : messagesByGroup.entrySet()) {
                String groupId = groupEntry.getKey();
                List<BatchMessage> groupMessages = groupEntry.getValue();
                boolean nackRemaining = false;

                for (BatchMessage batchMsg : groupMessages) {
                    MessagePointer message = batchMsg.message();
                    MessageCallback callback = batchMsg.callback();

                    // If we're nacking remaining messages in this group, nack and continue
                    // These are deferred - they'll be retried after the earlier message succeeds
                    if (nackRemaining) {
                        LOG.debugf("Nacking message [%s] - previous message in group [%s] was nacked",
                            message.id(), groupId);
                        callback.nack(message);
                        queueMetrics.recordMessageDeferred(getQueueIdentifier(batchMsg));
                        continue;
                    }

                    // Get SQS message ID for pipeline tracking
                    String sqsMessageId = batchMsg.sqsMessageId();
                    String queueIdentifier = getQueueIdentifier(batchMsg);

                    // Enrich message with batchId and sqsMessageId for tracking
                    MessagePointer enrichedMessage = new MessagePointer(
                        message.id(),
                        message.poolCode(),
                        message.authToken(),
                        message.mediationType(),
                        message.mediationTarget(),
                        message.messageGroupId(),
                        message.highPriority(),  // Preserve high priority flag
                        batchId,  // Add batch ID
                        sqsMessageId  // Add SQS message ID
                    );

                    // Track message in-flight
                    var trackResult = inFlightTracker.track(enrichedMessage, callback, queueIdentifier);
                    if (trackResult instanceof InFlightMessageTracker.TrackResult.Duplicate dup) {
                        // This shouldn't happen since we checked for duplicates in Phase 1,
                        // but handle it gracefully
                        LOG.warnf("Unexpected duplicate during tracking: message [%s], existing key [%s]",
                            enrichedMessage.id(), dup.existingPipelineKey());
                        callback.nack(enrichedMessage);
                        queueMetrics.recordMessageDeferred(queueIdentifier);
                        continue;
                    }

                    String pipelineKey = ((InFlightMessageTracker.TrackResult.Tracked) trackResult).pipelineKey();

                    // Also add to legacy inPipelineMap for ProcessPoolImpl compatibility
                    inPipelineMap.put(pipelineKey, enrichedMessage);

                    // Try to submit to pool
                    boolean submitted = pool.submit(enrichedMessage);
                    if (!submitted) {
                        LOG.warnf("Failed to submit message [%s] to pool [%s] - nacking this and all subsequent in group [%s]",
                            enrichedMessage.id(), poolCode, groupId);

                        // Remove from tracking since we're nacking
                        inFlightTracker.remove(pipelineKey);
                        inPipelineMap.remove(pipelineKey);

                        // Nack this message (deferred - pool submission failed, will retry)
                        callback.nack(enrichedMessage);
                        queueMetrics.recordMessageDeferred(queueIdentifier);

                        // Set flag to nack all remaining messages in this group
                        nackRemaining = true;
                    } else {
                        LOG.debugf("Routed message [%s] to pool [%s]", enrichedMessage.id(), poolCode);
                        queueMetrics.recordMessageProcessed(queueIdentifier, true);
                    }
                }
            }
        }

        updateMapSizeGauges();
    }

    /**
     * Called by queue consumers to route a single message to the appropriate process pool.
     * @deprecated Use routeMessageBatch for batch-level policies and better performance.
     *             This method is kept for backward compatibility with tests.
     *
     * @param message the message to route
     * @param callback the callback to invoke for ack/nack
     * @param queueIdentifier the queue this message came from
     * @return true if message was routed successfully, false otherwise
     */
    @Deprecated
    public boolean routeMessage(MessagePointer message, MessageCallback callback, String queueIdentifier) {
        String pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id();
        String appMessageId = message.id();

        // Check if message would be rejected BEFORE routing
        // This is necessary because routeMessageBatch doesn't return success/failure per message
        boolean wasInPipeline = inFlightTracker.containsKey(pipelineKey);
        boolean wasAppIdInPipeline = inFlightTracker.isInFlight(appMessageId);

        // Delegate to batch routing with a single-element list
        java.util.List<BatchMessage> batch = java.util.List.of(new BatchMessage(
            message,
            callback,
            queueIdentifier,
            message.sqsMessageId()
        ));
        routeMessageBatch(batch);

        // Return false if message was already in pipeline (duplicate)
        // Return true only if it was newly added
        if (wasInPipeline || wasAppIdInPipeline) {
            return false;
        }
        return inFlightTracker.containsKey(pipelineKey);
    }

    /**
     * Helper method to get queue identifier from batch message
     */
    private String getQueueIdentifier(BatchMessage batchMsg) {
        // This will be provided by the consumer when creating BatchMessage
        return batchMsg.queueIdentifier();
    }

    /**
     * Attempts to update the receipt handle on the stored callback when a message is redelivered.
     * This is a best-effort operation - if it fails, the pendingDeleteSqsMessageIds fallback will handle it.
     *
     * @param pipelineKey the pipeline key (SQS message ID) for the stored callback
     * @param appMessageId the application message ID (for logging)
     * @param newCallback the callback from the redelivered message containing the new receipt handle
     */
    private void updateReceiptHandleIfPossible(String pipelineKey, String appMessageId, MessageCallback newCallback) {
        try {
            var storedCallbackOpt = inFlightTracker.getCallback(pipelineKey);
            if (storedCallbackOpt.isEmpty()) {
                LOG.warnf("Cannot update receipt handle for message [%s] (key: %s) - no stored callback found",
                    appMessageId, pipelineKey);
                return;
            }
            MessageCallback storedCallback = storedCallbackOpt.get();

            if (!(storedCallback instanceof tech.flowcatalyst.messagerouter.callback.ReceiptHandleUpdatable updatable)) {
                LOG.debugf("Stored callback for message [%s] does not support receipt handle updates", appMessageId);
                return;
            }

            if (!(newCallback instanceof tech.flowcatalyst.messagerouter.callback.ReceiptHandleUpdatable newUpdatable)) {
                LOG.warnf("New callback for message [%s] does not support receipt handle retrieval", appMessageId);
                return;
            }

            String newReceiptHandle = newUpdatable.getReceiptHandle();
            if (newReceiptHandle == null || newReceiptHandle.isBlank()) {
                LOG.warnf("New receipt handle for message [%s] is null or blank - cannot update", appMessageId);
                return;
            }

            String oldReceiptHandle = updatable.getReceiptHandle();
            updatable.updateReceiptHandle(newReceiptHandle);
            LOG.infof("Updated receipt handle for in-pipeline message [%s] (key: %s). Old handle: %s..., New handle: %s...",
                appMessageId, pipelineKey,
                oldReceiptHandle != null ? oldReceiptHandle.substring(0, Math.min(20, oldReceiptHandle.length())) : "null",
                newReceiptHandle.substring(0, Math.min(20, newReceiptHandle.length())));

        } catch (Exception e) {
            // Log but don't fail - the pendingDeleteSqsMessageIds fallback will handle this case
            LOG.warnf(e, "Failed to update receipt handle for message [%s] (key: %s) - will rely on pendingDelete fallback",
                appMessageId, pipelineKey);
        }
    }

    /**
     * Record for batch message processing
     */
    public record BatchMessage(
        MessagePointer message,
        MessageCallback callback,
        String queueIdentifier,
        String sqsMessageId  // AWS SQS internal message ID for deduplication tracking
    ) {}

    @Override
    public void ack(MessagePointer message) {
        // Use sqsMessageId as key if available, otherwise fall back to app message ID
        String pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id();

        // Remove from tracker (handles all internal map cleanup)
        var removed = inFlightTracker.remove(pipelineKey);

        // Also remove from legacy inPipelineMap for ProcessPoolImpl compatibility
        inPipelineMap.remove(pipelineKey);

        // Call the stored callback
        removed.ifPresent(tracked -> tracked.callback().ack(message));

        updateMapSizeGauges();
    }

    @Override
    public void nack(MessagePointer message) {
        // Use sqsMessageId as key if available, otherwise fall back to app message ID
        String pipelineKey = message.sqsMessageId() != null ? message.sqsMessageId() : message.id();

        // Remove from tracker (handles all internal map cleanup)
        var removed = inFlightTracker.remove(pipelineKey);

        // Also remove from legacy inPipelineMap for ProcessPoolImpl compatibility
        inPipelineMap.remove(pipelineKey);

        // Call the stored callback
        removed.ifPresent(tracked -> tracked.callback().nack(message));

        updateMapSizeGauges();
    }

    /**
     * Gets the health status of all active queue consumers.
     *
     * @return map of queue identifier to consumer health status
     */
    public Map<String, QueueConsumerHealth> getConsumerHealthStatus() {
        Map<String, QueueConsumerHealth> healthStatus = new HashMap<>();

        // Check active consumers
        for (Map.Entry<String, QueueConsumer> entry : queueConsumers.entrySet()) {
            String queueId = entry.getKey();
            QueueConsumer consumer = entry.getValue();

            boolean isHealthy = consumer.isHealthy();
            long lastPollTime = consumer.getLastPollTime();
            long timeSinceLastPoll = lastPollTime > 0 ?
                System.currentTimeMillis() - lastPollTime : -1;
            int instanceId = System.identityHashCode(consumer);

            healthStatus.put(queueId, new QueueConsumerHealth(
                queueId,
                isHealthy,
                lastPollTime,
                timeSinceLastPoll,
                !consumer.isFullyStopped(),
                instanceId,
                consumer.getQueueIdentifier()  // The actual queue URL/identifier from the consumer
            ));
        }

        return healthStatus;
    }

    /**
     * Simple record for consumer health status
     */
    public record QueueConsumerHealth(
        String queueIdentifier,
        boolean isHealthy,
        long lastPollTimeMs,
        long timeSinceLastPollMs,
        boolean isRunning,
        int instanceId,
        String consumerQueueIdentifier  // The consumer's own queue identifier (may differ from map key)
    ) {}

    private tech.flowcatalyst.messagerouter.model.MediationType determineMediatorType(String poolCode) {
        // Map pool codes to mediator types
        // Currently only HTTP is supported
        return tech.flowcatalyst.messagerouter.model.MediationType.HTTP;
    }

    /**
     * Gets or lazily creates the default pool for messages with unknown pool codes.
     * This pool acts as a fallback to prevent message loss when pool configuration is missing.
     *
     * @return the default process pool
     */
    private ProcessPool getOrCreateDefaultPool() {
        return processPools.computeIfAbsent(DEFAULT_POOL_CODE, code -> {
            int queueCapacity = Math.max(DEFAULT_POOL_CONCURRENCY * QUEUE_CAPACITY_MULTIPLIER, MIN_QUEUE_CAPACITY);

            LOG.infof("Creating default fallback pool [%s] with concurrency %d and queue capacity %d",
                DEFAULT_POOL_CODE, DEFAULT_POOL_CONCURRENCY, queueCapacity);

            tech.flowcatalyst.messagerouter.model.MediationType mediatorType = determineMediatorType(DEFAULT_POOL_CODE);
            Mediator mediator = mediatorFactory.createMediator(mediatorType);

            ProcessPool pool = new ProcessPoolImpl(
                DEFAULT_POOL_CODE,
                DEFAULT_POOL_CONCURRENCY,
                queueCapacity,
                null, // No rate limiting for default pool
                mediator,
                this,
                inPipelineMap,
                poolMetrics,
                warningService
            );

            pool.start();
            return pool;
        });
    }

    /**
     * Get in-flight messages sorted by timestamp (oldest first)
     *
     * @param limit maximum number of messages to return
     * @param messageIdFilter optional message ID to filter by (matches application message ID)
     * @return list of in-flight messages
     */
    public java.util.List<tech.flowcatalyst.messagerouter.model.InFlightMessage> getInFlightMessages(
            int limit, String messageIdFilter) {
        return inFlightTracker.stream()
            .map(tracked -> {
                String brokerMessageId = tracked.pipelineKey();
                String appMessageId = tracked.messageId();
                long timestamp = tracked.trackedAt().toEpochMilli();
                String queueId = tracked.queueId() != null ? tracked.queueId() : "unknown";
                String poolCode = tracked.message() != null ? tracked.message().poolCode() : "unknown";
                return tech.flowcatalyst.messagerouter.model.InFlightMessage.from(
                    appMessageId,
                    brokerMessageId,
                    queueId,
                    timestamp,
                    poolCode
                );
            })
            .filter(msg -> messageIdFilter == null || messageIdFilter.isEmpty() ||
                    msg.messageId().toLowerCase().contains(messageIdFilter.toLowerCase()))
            .sorted(java.util.Comparator.comparingLong(
                tech.flowcatalyst.messagerouter.model.InFlightMessage::elapsedTimeMs).reversed())
            .limit(limit)
            .collect(java.util.stream.Collectors.toList());
    }

    /**
     * Check if a message is already in the pipeline (redelivery detection)
     * @param sqsMessageId The SQS message ID to check (preferred) or application message ID
     * @return true if message is already in pipeline (redelivery), false if new message
     */
    public boolean isMessageInPipeline(String sqsMessageId) {
        return inFlightTracker.containsKey(sqsMessageId);
    }

    /**
     * Get the number of messages currently in-flight
     * @return count of in-flight messages
     */
    public int getInFlightCount() {
        return inFlightTracker.size();
    }
}

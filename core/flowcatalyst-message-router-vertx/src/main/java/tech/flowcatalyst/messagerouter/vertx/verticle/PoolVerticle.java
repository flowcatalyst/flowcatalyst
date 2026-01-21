package tech.flowcatalyst.messagerouter.vertx.verticle;

import io.github.resilience4j.ratelimiter.RateLimiter;
import io.github.resilience4j.ratelimiter.RateLimiterConfig;
import io.vertx.core.AbstractVerticle;
import io.vertx.core.DeploymentOptions;
import io.vertx.core.ThreadingModel;
import io.vertx.core.eventbus.Message;
import io.vertx.core.json.JsonObject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.vertx.channel.PoolChannels;
import tech.flowcatalyst.messagerouter.vertx.message.RouterMessages.*;

import java.time.Duration;
import java.util.*;

/**
 * Pool verticle that manages message groups via dynamic worker verticle deployment.
 * <p>
 * Pure actor model - no Semaphores, no synchronized blocks, no BlockingQueues.
 * Concurrency is controlled by the number of deployed GroupWorkerVerticles.
 * <p>
 * Owns:
 * <ul>
 *   <li>Worker deployment tracking (groupId → deploymentId)</li>
 *   <li>Pending queue for messages when at concurrency limit</li>
 *   <li>Failed batch tracking for FIFO ordering</li>
 *   <li>Rate limiter (optional)</li>
 * </ul>
 * <p>
 * Threading: Event Loop (non-blocking). Workers run on Virtual Threads.
 */
public class PoolVerticle extends AbstractVerticle {

    private static final Logger LOG = Logger.getLogger(PoolVerticle.class);

    // === OWNED STATE (plain collections - single threaded verticle) ===
    private String poolCode;
    private int maxConcurrency;

    // groupId → deploymentId of worker verticle
    private final Map<String, String> workerDeployments = new HashMap<>();

    // Messages waiting for a worker slot (when at max concurrency)
    private final Queue<PoolMessage> pendingMessages = new LinkedList<>();

    // Groups waiting for a worker slot
    private final Set<String> pendingGroups = new HashSet<>();

    // Failed batch+group tracking for FIFO ordering
    private final Set<String> failedBatchGroups = new HashSet<>();

    // Rate limiter (optional)
    private RateLimiter rateLimiter;

    private final PoolMetricsService poolMetrics;

    public PoolVerticle(PoolMetricsService poolMetrics) {
        this.poolMetrics = poolMetrics;
    }

    @Override
    public void start() {
        JsonObject config = config();
        this.poolCode = config.getString("code");
        this.maxConcurrency = config.getInteger("concurrency", 10);

        Integer rateLimitPerMinute = config.getInteger("rateLimitPerMinute");
        if (rateLimitPerMinute != null && rateLimitPerMinute > 0) {
            this.rateLimiter = createRateLimiter(rateLimitPerMinute);
        }

        LOG.infof("PoolVerticle [%s] starting with concurrency=%d, rateLimit=%s",
                poolCode, maxConcurrency, rateLimitPerMinute);

        // Listen for messages and config updates
        PoolChannels.Address poolAddress = PoolChannels.address(poolCode);
        poolAddress.messages(vertx).consumer(this::handleMessage);
        poolAddress.config(vertx).consumer(this::handleConfigUpdate);

        // Listen for worker lifecycle events
        vertx.eventBus().<GroupWorkerVerticle.WorkerIdle>consumer(
                "pool." + poolCode + ".workerIdle", this::handleWorkerIdle);
        vertx.eventBus().<GroupWorkerVerticle.BatchGroupFailed>consumer(
                "pool." + poolCode + ".batchGroupFailed", this::handleBatchGroupFailed);

        // Periodic cleanup of failed batch groups (every 5 minutes)
        vertx.setPeriodic(300_000, id -> failedBatchGroups.clear());

        // Periodic metrics update
        vertx.setPeriodic(1_000, id -> updateMetrics());

        LOG.infof("PoolVerticle [%s] started", poolCode);
    }

    @Override
    public void stop() {
        LOG.infof("PoolVerticle [%s] stopping, undeploying %d workers", poolCode, workerDeployments.size());

        // Undeploy all workers
        for (String deploymentId : workerDeployments.values()) {
            vertx.undeploy(deploymentId);
        }
        workerDeployments.clear();
    }

    // === MESSAGE HANDLING ===

    private void handleMessage(Message<PoolMessage> msg) {
        PoolMessage message = msg.body();
        String groupId = message.messageGroupId() != null ? message.messageGroupId() : "__DEFAULT__";
        String batchId = message.batchId();

        // Check batch+group FIFO failure
        String batchGroupKey = batchId + "|" + groupId;
        if (failedBatchGroups.contains(batchGroupKey)) {
            LOG.debugf("Batch+group [%s] previously failed, NACKing message [%s]", batchGroupKey, message.sqsMessageId());
            msg.fail(1, "Batch+group failed");
            return;
        }

        // Check rate limit
        if (rateLimiter != null && !rateLimiter.acquirePermission()) {
            LOG.debugf("Rate limit exceeded for pool [%s], queueing message [%s]", poolCode, message.sqsMessageId());
            pendingMessages.offer(message);
            poolMetrics.recordRateLimitExceeded(poolCode);
            msg.reply(new OkReply()); // Acknowledge receipt, will process later
            return;
        }

        // Check if worker exists for this group
        if (workerDeployments.containsKey(groupId)) {
            // Worker exists - route message to it
            routeToWorker(groupId, message, msg);
            return;
        }

        // Need to deploy new worker - check concurrency limit
        if (workerDeployments.size() >= maxConcurrency) {
            // At capacity - queue message and group
            LOG.debugf("Pool [%s] at max concurrency (%d), queueing message for group [%s]",
                    poolCode, maxConcurrency, groupId);
            pendingMessages.offer(message);
            pendingGroups.add(groupId);
            msg.reply(new OkReply()); // Acknowledge receipt, will process when slot available
            return;
        }

        // Deploy new worker for this group
        deployWorker(groupId, message, msg);
    }

    private void deployWorker(String groupId, PoolMessage firstMessage, Message<PoolMessage> originalMsg) {
        LOG.debugf("Deploying worker for pool [%s] group [%s]", poolCode, groupId);

        JsonObject workerConfig = new JsonObject()
                .put("poolCode", poolCode)
                .put("groupId", groupId);

        DeploymentOptions options = new DeploymentOptions()
                .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                .setConfig(workerConfig);

        vertx.deployVerticle(new GroupWorkerVerticle(), options)
                .onSuccess(deploymentId -> {
                    workerDeployments.put(groupId, deploymentId);
                    LOG.infof("Worker deployed for pool [%s] group [%s], deployment: %s",
                            poolCode, groupId, deploymentId);

                    // Route the first message
                    routeToWorker(groupId, firstMessage, originalMsg);

                    // Worker deployed - concurrency count updated via updateMetrics()
                })
                .onFailure(err -> {
                    LOG.errorf("Failed to deploy worker for pool [%s] group [%s]: %s",
                            poolCode, groupId, err.getMessage());
                    originalMsg.fail(2, "Worker deployment failed");
                });
    }

    private void routeToWorker(String groupId, PoolMessage message, Message<PoolMessage> originalMsg) {
        String address = "pool." + poolCode + ".group." + groupId;

        vertx.eventBus().<OkReply>request(address, message)
                .onSuccess(reply -> {
                    poolMetrics.recordMessageSubmitted(poolCode);
                    originalMsg.reply(new OkReply());
                })
                .onFailure(err -> {
                    LOG.warnf("Failed to route message to worker [%s]: %s", groupId, err.getMessage());
                    originalMsg.fail(3, "Routing failed");
                });
    }

    // === WORKER LIFECYCLE ===

    private void handleWorkerIdle(Message<GroupWorkerVerticle.WorkerIdle> msg) {
        GroupWorkerVerticle.WorkerIdle idle = msg.body();
        String groupId = idle.groupId();
        String deploymentId = idle.deploymentId();

        LOG.debugf("Worker idle notification for pool [%s] group [%s]", poolCode, groupId);

        // Verify this is still the active deployment for this group
        String currentDeploymentId = workerDeployments.get(groupId);
        if (currentDeploymentId == null || !currentDeploymentId.equals(deploymentId)) {
            LOG.debugf("Ignoring stale idle notification for group [%s]", groupId);
            return;
        }

        // Undeploy the worker
        vertx.undeploy(deploymentId)
                .onSuccess(v -> {
                    workerDeployments.remove(groupId);
                    LOG.infof("Worker undeployed for pool [%s] group [%s]", poolCode, groupId);
                    // Worker undeployed - concurrency count updated via updateMetrics()

                    // Check if there are pending messages/groups waiting for a slot
                    processPendingQueue();
                })
                .onFailure(err -> {
                    LOG.warnf("Failed to undeploy worker for group [%s]: %s", groupId, err.getMessage());
                    // Remove from tracking anyway
                    workerDeployments.remove(groupId);
                    processPendingQueue();
                });
    }

    private void handleBatchGroupFailed(Message<GroupWorkerVerticle.BatchGroupFailed> msg) {
        GroupWorkerVerticle.BatchGroupFailed failed = msg.body();
        String batchGroupKey = failed.batchId() + "|" + failed.groupId();
        failedBatchGroups.add(batchGroupKey);
        LOG.debugf("Marked batch+group [%s] as failed for FIFO ordering", batchGroupKey);
    }

    private void processPendingQueue() {
        // Process pending groups first (they need new workers)
        while (!pendingGroups.isEmpty() && workerDeployments.size() < maxConcurrency) {
            String groupId = pendingGroups.iterator().next();
            pendingGroups.remove(groupId);

            // Find first message for this group
            PoolMessage message = findAndRemoveMessageForGroup(groupId);
            if (message != null) {
                // Deploy worker and route message
                deployWorkerForPending(groupId, message);
            }
        }

        // Process any remaining pending messages (for existing groups)
        while (!pendingMessages.isEmpty()) {
            PoolMessage message = pendingMessages.peek();
            String groupId = message.messageGroupId() != null ? message.messageGroupId() : "__DEFAULT__";

            if (workerDeployments.containsKey(groupId)) {
                // Worker exists - route directly
                pendingMessages.poll();
                routeToWorkerDirect(groupId, message);
            } else if (workerDeployments.size() < maxConcurrency) {
                // Can deploy new worker
                pendingMessages.poll();
                deployWorkerForPending(groupId, message);
            } else {
                // Still at capacity, stop processing
                break;
            }
        }
    }

    private PoolMessage findAndRemoveMessageForGroup(String groupId) {
        Iterator<PoolMessage> it = pendingMessages.iterator();
        while (it.hasNext()) {
            PoolMessage msg = it.next();
            String msgGroupId = msg.messageGroupId() != null ? msg.messageGroupId() : "__DEFAULT__";
            if (msgGroupId.equals(groupId)) {
                it.remove();
                return msg;
            }
        }
        return null;
    }

    private void deployWorkerForPending(String groupId, PoolMessage message) {
        JsonObject workerConfig = new JsonObject()
                .put("poolCode", poolCode)
                .put("groupId", groupId);

        DeploymentOptions options = new DeploymentOptions()
                .setThreadingModel(ThreadingModel.VIRTUAL_THREAD)
                .setConfig(workerConfig);

        vertx.deployVerticle(new GroupWorkerVerticle(), options)
                .onSuccess(deploymentId -> {
                    workerDeployments.put(groupId, deploymentId);
                    routeToWorkerDirect(groupId, message);
                    // Worker deployed - concurrency count updated via updateMetrics()
                })
                .onFailure(err -> {
                    LOG.errorf("Failed to deploy worker for pending group [%s]: %s", groupId, err.getMessage());
                    // Message is lost - could re-queue or NACK
                });
    }

    private void routeToWorkerDirect(String groupId, PoolMessage message) {
        String address = "pool." + poolCode + ".group." + groupId;
        vertx.eventBus().request(address, message)
                .onSuccess(reply -> poolMetrics.recordMessageSubmitted(poolCode))
                .onFailure(err -> LOG.warnf("Failed to route pending message to worker [%s]: %s",
                        groupId, err.getMessage()));
    }

    // === CONFIG UPDATE ===

    private void handleConfigUpdate(Message<PoolConfigUpdate> msg) {
        PoolConfigUpdate config = msg.body();
        LOG.infof("Received config update for pool [%s]", poolCode);

        // Update concurrency limit
        this.maxConcurrency = config.concurrency();

        // Update rate limiter
        Integer newRateLimit = config.rateLimitPerMinute();
        if (newRateLimit != null && newRateLimit > 0) {
            this.rateLimiter = createRateLimiter(newRateLimit);
        } else {
            this.rateLimiter = null;
        }

        LOG.infof("Pool [%s] config updated: concurrency=%d, rateLimit=%s",
                poolCode, maxConcurrency, newRateLimit);

        // Process pending queue in case concurrency increased
        processPendingQueue();
    }

    // === METRICS ===

    private void updateMetrics() {
        int activeWorkers = workerDeployments.size();
        int availableSlots = maxConcurrency - activeWorkers;
        int pendingCount = pendingMessages.size();
        int pendingGroupCount = pendingGroups.size();

        poolMetrics.updatePoolGauges(poolCode, activeWorkers, availableSlots, pendingCount, pendingGroupCount);
    }

    // === HELPERS ===

    private RateLimiter createRateLimiter(int limitPerMinute) {
        RateLimiterConfig config = RateLimiterConfig.custom()
                .limitRefreshPeriod(Duration.ofMinutes(1))
                .limitForPeriod(limitPerMinute)
                .timeoutDuration(Duration.ZERO)
                .build();

        return RateLimiter.of("pool-" + poolCode, config);
    }

    // === ACCESSORS FOR MONITORING ===

    public String getPoolCode() {
        return poolCode;
    }

    public int getActiveWorkers() {
        return workerDeployments.size();
    }

    public int getPendingCount() {
        return pendingMessages.size();
    }

    public int getAvailableSlots() {
        return maxConcurrency - workerDeployments.size();
    }
}

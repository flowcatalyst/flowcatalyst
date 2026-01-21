package tech.flowcatalyst.messagerouter.pool;

import io.github.resilience4j.ratelimiter.RateLimiter;
import io.github.resilience4j.ratelimiter.RateLimiterConfig;
import org.jboss.logging.Logger;
import org.jboss.logging.MDC;
import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.mediator.Mediator;
import tech.flowcatalyst.messagerouter.metrics.PoolMetricsService;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResult;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.time.Duration;
import java.util.concurrent.*;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.locks.ReentrantLock;

/**
 * Process pool implementation with per-message-group FIFO ordering using dedicated virtual threads.
 *
 * <h2>Architecture: Per-Group Virtual Threads</h2>
 * Each message group gets its own dedicated Java 21 virtual thread that processes messages sequentially.
 * This architecture provides:
 * <ul>
 *   <li><b>FIFO within group:</b> Messages with same messageGroupId process sequentially</li>
 *   <li><b>Concurrent across groups:</b> Different messageGroupIds process in parallel</li>
 *   <li><b>Zero blocking:</b> No scanning overhead, no worker contention</li>
 *   <li><b>Auto-cleanup:</b> Idle groups cleaned up after 5 minutes</li>
 *   <li><b>Scales to 100K+ groups:</b> O(1) routing, minimal memory per group (~2KB)</li>
 * </ul>
 *
 * <p><b>Example:</b> Messages for "order-12345" and "order-67890":
 * <pre>
 * Message arrives for "order-12345":
 *   → Get or create queue + virtual thread for "order-12345"
 *   → Virtual thread blocks on queue.poll() (no CPU waste)
 *   → Processes msg1, msg2, msg3 sequentially (FIFO)
 *   → After 5 min idle, thread exits and group cleaned up
 *
 * Meanwhile, "order-67890" processes concurrently in its own virtual thread
 * </pre>
 *
 * <h2>Performance Characteristics</h2>
 * <ul>
 *   <li><b>Routing:</b> O(1) - direct queue access via ConcurrentHashMap</li>
 *   <li><b>Idle CPU:</b> 0% - virtual threads block on queue, no scanning</li>
 *   <li><b>Memory per group:</b> ~2KB (virtual thread + queue overhead)</li>
 *   <li><b>Scales to:</b> 100K+ concurrent message groups</li>
 *   <li><b>No memory leak:</b> Idle groups auto-cleaned after 5 minutes</li>
 * </ul>
 *
 * <h2>Buffer Sizing</h2>
 * Each message group gets its own queue with capacity={@code queueCapacity}:
 * <ul>
 *   <li>Total capacity: dynamic (active groups × queueCapacity)</li>
 *   <li>Per-group isolation: one group cannot starve others</li>
 *   <li>Backward compatible: messages without messageGroupId use DEFAULT_GROUP</li>
 * </ul>
 *
 * <p>Buffer capacity is calculated as max(concurrency × 2, 50):
 * <ul>
 *   <li>5 workers → 50 buffer per group</li>
 *   <li>100 workers → 200 buffer per group</li>
 *   <li>200 workers → 400 buffer per group</li>
 * </ul>
 *
 * <h2>Concurrency Control</h2>
 * Pool-level concurrency is enforced by a semaphore with {@code concurrency} permits.
 * Each message group's virtual thread must acquire a semaphore permit before processing.
 * This ensures total concurrent processing never exceeds the configured limit.
 *
 * <h2>Backpressure</h2>
 * When a group's buffer is full, messages are rejected and rely on queue visibility timeout
 * for redelivery. This allows SQS/ActiveMQ to act as overflow buffer when the system is overwhelmed.
 *
 * <p>See <a href="../../../../../MESSAGE_GROUP_FIFO.md">MESSAGE_GROUP_FIFO.md</a> for detailed documentation.
 *
 * @see MessagePointer#messageGroupId()
 * @see <a href="../../../../../MESSAGE_GROUP_FIFO.md">Message Group FIFO Ordering Documentation</a>
 */
public class ProcessPoolImpl implements ProcessPool {

    private static final Logger LOG = Logger.getLogger(ProcessPoolImpl.class);

    private final String poolCode;
    private volatile int concurrency;
    private final int queueCapacity;
    private final Semaphore semaphore;
    private final ExecutorService executorService;
    private final ScheduledExecutorService gaugeUpdater;
    private ScheduledFuture<?> gaugeUpdateTask;
    private final AtomicBoolean running = new AtomicBoolean(false);
    private volatile RateLimiter rateLimiter;  // Pool-level rate limiter (null if not configured, volatile for atomic replacement)
    private volatile Integer rateLimitPerMinute;  // Track rate limit value separately for updates
    private final Mediator mediator;
    private final MessageCallback messageCallback;
    private final ConcurrentMap<String, MessagePointer> inPipelineMap;
    private final PoolMetricsService poolMetrics;
    private final WarningService warningService;

    // Use ReentrantLock instead of synchronized to avoid pinning virtual threads
    private final ReentrantLock configLock = new ReentrantLock();

    // Per-message-group queues for FIFO ordering within groups, concurrent across groups
    // Two tiers: high priority checked first, then regular
    // Key: messageGroupId (e.g., "order-12345"), Value: Queue for that group's messages
    // Each group has its own dedicated virtual thread that blocks on queue.poll()
    private final ConcurrentHashMap<String, BlockingQueue<MessagePointer>> highPriorityGroupQueues = new ConcurrentHashMap<>();
    private final ConcurrentHashMap<String, BlockingQueue<MessagePointer>> regularGroupQueues = new ConcurrentHashMap<>();

    // Track active virtual threads per message group for death detection
    // Key: messageGroupId, Value: true if thread is actively running
    // When a thread exits (normally or abnormally), it removes itself from this map
    private final ConcurrentHashMap<String, Boolean> activeGroupThreads = new ConcurrentHashMap<>();

    // Track total messages across all group queues for metrics
    private final AtomicInteger totalQueuedMessages = new AtomicInteger(0);

    // Default group for messages without a messageGroupId (backward compatibility)
    private static final String DEFAULT_GROUP = "__DEFAULT__";

    // Idle timeout before cleaning up inactive message groups (5 minutes)
    private static final long IDLE_TIMEOUT_MINUTES = 5;

    // Batch+Group FIFO tracking: When a message in a batch+group fails, all subsequent
    // messages in that batch+group must be nacked to preserve FIFO ordering
    // Key: "batchId|messageGroupId", Value: true if this batch+group has a failed message
    private final ConcurrentHashMap<String, Boolean> failedBatchGroups = new ConcurrentHashMap<>();

    // Track remaining messages per batch+group for cleanup
    // Key: "batchId|messageGroupId", Value: count of messages still in flight
    private final ConcurrentHashMap<String, AtomicInteger> batchGroupMessageCount = new ConcurrentHashMap<>();

    /**
     * Creates a new process pool.
     *
     * @param poolCode unique identifier for this pool
     * @param concurrency number of concurrent workers
     * @param queueCapacity blocking queue capacity (should be max(concurrency × 2, 50))
     * @param rateLimitPerMinute optional pool-level rate limit (null if not configured)
     * @param mediator mediator for processing messages
     * @param messageCallback callback for ack/nack operations
     * @param inPipelineMap shared map for message deduplication
     * @param poolMetrics metrics service for recording pool statistics
     * @param warningService service for recording warnings
     */
    public ProcessPoolImpl(
            String poolCode,
            int concurrency,
            int queueCapacity,
            Integer rateLimitPerMinute,
            Mediator mediator,
            MessageCallback messageCallback,
            ConcurrentMap<String, MessagePointer> inPipelineMap,
            PoolMetricsService poolMetrics,
            WarningService warningService) {
        this.poolCode = poolCode;
        this.concurrency = concurrency;
        this.queueCapacity = queueCapacity;
        this.semaphore = new Semaphore(concurrency);
        this.executorService = Executors.newVirtualThreadPerTaskExecutor();
        this.gaugeUpdater = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread t = new Thread(r, "gauge-updater-" + poolCode);
            t.setDaemon(true);
            return t;
        });
        this.mediator = mediator;
        this.messageCallback = messageCallback;
        this.inPipelineMap = inPipelineMap;
        this.poolMetrics = poolMetrics;
        this.warningService = warningService;

        // Initialize pool capacity metrics
        poolMetrics.initializePoolCapacity(poolCode, concurrency, queueCapacity);

        // Create pool-level rate limiter if configured
        if (rateLimitPerMinute != null && rateLimitPerMinute > 0) {
            LOG.infof("Creating pool-level rate limiter for [%s] with limit %d/min", poolCode, rateLimitPerMinute);
            this.rateLimitPerMinute = rateLimitPerMinute;
            this.rateLimiter = RateLimiter.of(
                "pool-" + poolCode,
                RateLimiterConfig.custom()
                    .limitRefreshPeriod(Duration.ofMinutes(1))
                    .limitForPeriod(rateLimitPerMinute)
                    .timeoutDuration(Duration.ZERO)
                    .build()
            );
        } else {
            this.rateLimitPerMinute = null;
            this.rateLimiter = null;
            LOG.infof("No rate limiting configured for pool [%s]", poolCode);
        }
    }

    @Override
    public void start() {
        if (running.compareAndSet(false, true)) {
            LOG.infof("Starting process pool [%s] with concurrency %d (per-group virtual threads)", poolCode, concurrency);
            // No upfront worker threads needed - virtual threads created on-demand per message group
            // Start periodic gauge updates (every 500ms for responsive metrics)
            gaugeUpdateTask = gaugeUpdater.scheduleAtFixedRate(
                this::updateGauges,
                0,
                500,
                TimeUnit.MILLISECONDS
            );
        }
    }

    @Override
    public void drain() {
        LOG.infof("Draining process pool [%s] - will finish processing buffered messages asynchronously", poolCode);
        running.set(false);

        // Non-blocking drain: Just stop accepting new work
        // The pool will continue processing messages already in its queue
        // The cleanup scheduled task will call shutdown() when fully drained

        LOG.infof("Process pool [%s] set to draining mode (queued: %d, active: %d, groups: %d)",
            poolCode, totalQueuedMessages.get(), concurrency - semaphore.availablePermits(), getActiveGroupCount());
    }

    @Override
    public boolean submit(MessagePointer message) {
        // Reject messages if pool is draining
        if (!running.get()) {
            LOG.debugf("Pool [%s] is draining, rejecting message [%s]", poolCode, message.id());
            return false;
        }

        // Route message to appropriate group queue
        String groupId = message.messageGroupId();
        if (groupId == null || groupId.isBlank()) {
            groupId = DEFAULT_GROUP;
        }

        final String finalGroupId = groupId;

        // Track this message for batch+group FIFO ordering
        String batchId = message.batchId();
        if (batchId != null && !batchId.isBlank()) {
            String batchGroupKey = batchId + "|" + finalGroupId;
            batchGroupMessageCount.computeIfAbsent(batchGroupKey, k -> new AtomicInteger(0))
                .incrementAndGet();
            LOG.debugf("Tracking message [%s] in batch+group [%s], count incremented",
                message.id(), batchGroupKey);
        }

        // Route message to appropriate priority tier queue
        // High priority messages go to highPriorityGroupQueues, regular to regularGroupQueues
        ConcurrentHashMap<String, BlockingQueue<MessagePointer>> targetQueues =
            message.highPriority() ? highPriorityGroupQueues : regularGroupQueues;

        // Get or create queue for this message group in the appropriate tier
        // Per-group queues are unbounded - pool-level capacity enforced by totalQueuedMessages check above
        // Note: No side effects in computeIfAbsent to comply with ConcurrentHashMap contract
        BlockingQueue<MessagePointer> groupQueue = targetQueues.computeIfAbsent(
            groupId,
            k -> new LinkedBlockingQueue<>()
        );

        // Ensure thread is running for this group (atomic check-and-start)
        // putIfAbsent is atomic - only one thread will get null and start the worker
        // This handles both new groups and dead thread restarts
        // Note: One thread handles BOTH high and regular priority queues for a group
        boolean startedThread = activeGroupThreads.putIfAbsent(finalGroupId, Boolean.TRUE) == null;
        if (startedThread) {
            // Check if either queue already has messages - indicates this is a restart after thread death
            BlockingQueue<MessagePointer> highQueue = highPriorityGroupQueues.get(finalGroupId);
            BlockingQueue<MessagePointer> regularQueue = regularGroupQueues.get(finalGroupId);
            boolean hasOrphanedMessages = (highQueue != null && !highQueue.isEmpty()) ||
                                          (regularQueue != null && !regularQueue.isEmpty());
            if (hasOrphanedMessages) {
                LOG.warnf("Virtual thread for message group [%s] appears to have died - restarting", finalGroupId);
                warningService.addWarning(
                    "GROUP_THREAD_RESTART",
                    "WARN",
                    String.format("Virtual thread for group [%s] in pool [%s] died and was restarted", finalGroupId, poolCode),
                    "ProcessPool:" + poolCode
                );
            } else {
                LOG.debugf("Created new message group [%s] with dedicated virtual thread", finalGroupId);
            }
            executorService.submit(() -> processMessageGroup(finalGroupId));
        }

        // Check total capacity before submitting
        // Use compareAndSet loop to atomically check and increment
        while (true) {
            int current = totalQueuedMessages.get();
            if (current >= queueCapacity) {
                // At capacity - reject message
                LOG.debugf("Pool [%s] at capacity (%d/%d), rejecting message [%s]",
                    poolCode, current, queueCapacity, message.id());
                // Clean up batch+group tracking for rejected message
                if (batchId != null && !batchId.isBlank()) {
                    String batchGroupKey = batchId + "|" + finalGroupId;
                    decrementAndCleanupBatchGroup(batchGroupKey);
                }
                return false;
            }
            // Try to reserve a slot atomically
            if (totalQueuedMessages.compareAndSet(current, current + 1)) {
                break;
            }
            // Another thread modified counter, retry
            // CRITICAL: On single-core environments, this spin loop can starve ALL other
            // virtual threads (including consumers and scheduled tasks) because virtual
            // threads use cooperative scheduling and only yield on blocking I/O.
            Thread.onSpinWait(); // CPU optimization hint
            Thread.yield();     // Allow virtual thread scheduler to run other threads
        }

        // Offer message to group queue (should always succeed since queues are unbounded)
        boolean submitted = groupQueue.offer(message);
        if (submitted) {
            poolMetrics.recordMessageSubmitted(poolCode);
            updateGauges();
        } else {
            // Unexpected failure - release the reserved slot
            totalQueuedMessages.decrementAndGet();
            // Clean up batch+group tracking for rejected message
            if (batchId != null && !batchId.isBlank()) {
                String batchGroupKey = batchId + "|" + finalGroupId;
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        }
        return submitted;
    }

    @Override
    public String getPoolCode() {
        return poolCode;
    }

    @Override
    public int getConcurrency() {
        return concurrency;
    }

    @Override
    public Integer getRateLimitPerMinute() {
        return rateLimitPerMinute;
    }

    @Override
    public boolean isFullyDrained() {
        return totalQueuedMessages.get() == 0 && semaphore.availablePermits() == concurrency;
    }

    @Override
    public void shutdown() {
        // Stop gauge updates first
        if (gaugeUpdateTask != null) {
            gaugeUpdateTask.cancel(false);
        }
        gaugeUpdater.shutdown();

        // Then shutdown worker executor
        executorService.shutdown();
        try {
            if (!executorService.awaitTermination(10, TimeUnit.SECONDS)) {
                LOG.warnf("Executor service for pool [%s] did not terminate within 10 seconds", poolCode);
                executorService.shutdownNow();
            }
        } catch (InterruptedException e) {
            executorService.shutdownNow();
            Thread.currentThread().interrupt();
        }
    }

    @Override
    public int getQueueSize() {
        return totalQueuedMessages.get();
    }

    @Override
    public int getActiveWorkers() {
        return concurrency - semaphore.availablePermits();
    }

    @Override
    public int getQueueCapacity() {
        return queueCapacity;
    }

    @Override
    public boolean isRateLimited() {
        // Store in local variable to avoid race condition with updateRateLimit()
        RateLimiter limiter = this.rateLimiter;
        if (limiter == null) {
            return false;
        }
        // Check available permissions without consuming a permit
        // If no permissions available, the pool is currently rate limited
        return limiter.getMetrics().getAvailablePermissions() <= 0;
    }

    /**
     * Updates the concurrency limit for this pool in-place without draining.
     *
     * <h2>Behavior</h2>
     * <ul>
     *   <li><b>Increase:</b> Immediately releases permits to semaphore</li>
     *   <li><b>Decrease:</b> Tries to acquire excess permits with timeout. If timeout expires,
     *       keeps current limit (doesn't block indefinitely)</li>
     * </ul>
     *
     * @param newLimit the new concurrency limit
     * @param timeoutSeconds timeout in seconds for acquiring permits when decreasing (typically 60)
     * @return true if update succeeded, false if decrease timed out (current limit retained)
     */
    @Override
    public boolean updateConcurrency(int newLimit, int timeoutSeconds) {
        if (newLimit <= 0) {
            LOG.warnf("Pool [%s] rejecting invalid concurrency limit: %d", poolCode, newLimit);
            return false;
        }

        configLock.lock();
        try {
            int currentLimit = concurrency;

            if (newLimit == currentLimit) {
                LOG.debugf("Pool [%s] concurrency update to %d matches current limit, no change needed", poolCode, newLimit);
                return true;
            }

            if (newLimit > currentLimit) {
                // Increasing concurrency: Release additional permits
                int permitDifference = newLimit - currentLimit;
                semaphore.release(permitDifference);
                concurrency = newLimit;
                LOG.infof("Pool [%s] concurrency increased from %d to %d (released %d permits)",
                    poolCode, currentLimit, newLimit, permitDifference);
                return true;
            } else {
                // Decreasing concurrency: Try to acquire excess permits with timeout
                int permitDifference = currentLimit - newLimit;
                try {
                    boolean acquired = semaphore.tryAcquire(permitDifference, timeoutSeconds, TimeUnit.SECONDS);
                    if (acquired) {
                        concurrency = newLimit;
                        LOG.infof("Pool [%s] concurrency decreased from %d to %d (acquired %d idle permits)",
                            poolCode, currentLimit, newLimit, permitDifference);
                        return true;
                    } else {
                        LOG.warnf("Pool [%s] concurrency decrease from %d to %d timed out after %d seconds " +
                            "(waiting for %d idle slots). Retaining current limit %d. Active workers: %d",
                            poolCode, currentLimit, newLimit, timeoutSeconds, permitDifference, currentLimit,
                            currentLimit - semaphore.availablePermits());
                        return false;
                    }
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                    LOG.warnf("Pool [%s] concurrency decrease interrupted, retaining current limit %d",
                        poolCode, currentLimit);
                    return false;
                }
            }
        } finally {
            configLock.unlock();
        }
    }

    /**
     * Updates the rate limit for this pool in-place.
     *
     * <h2>Behavior</h2>
     * <ul>
     *   <li>Atomically replaces the rate limiter instance</li>
     *   <li>If setting to null, disables rate limiting</li>
     *   <li>If increasing or decreasing, creates new rate limiter with updated limit</li>
     * </ul>
     *
     * @param newRateLimitPerMinute the new rate limit (null to disable rate limiting)
     */
    @Override
    public void updateRateLimit(Integer newRateLimitPerMinute) {
        configLock.lock();
        try {
            Integer currentLimit = this.rateLimitPerMinute;

            if (currentLimit == null && newRateLimitPerMinute == null) {
                LOG.debugf("Pool [%s] rate limit update to null matches current state, no change needed", poolCode);
                return;
            }

            if (currentLimit != null && newRateLimitPerMinute != null && currentLimit.equals(newRateLimitPerMinute)) {
                LOG.debugf("Pool [%s] rate limit update to %d/min matches current limit, no change needed",
                    poolCode, newRateLimitPerMinute);
                return;
            }

            if (newRateLimitPerMinute == null || newRateLimitPerMinute <= 0) {
                // Disable rate limiting
                this.rateLimitPerMinute = null;
                this.rateLimiter = null;
                LOG.infof("Pool [%s] rate limiting disabled (was: %s)",
                    poolCode, currentLimit != null ? currentLimit + "/min" : "none");
            } else {
                // Create new rate limiter with updated limit
                LOG.infof("Pool [%s] rate limit updated from %s to %d/min",
                    poolCode, currentLimit != null ? currentLimit + "/min" : "none", newRateLimitPerMinute);
                this.rateLimitPerMinute = newRateLimitPerMinute;
                this.rateLimiter = RateLimiter.of(
                    "pool-" + poolCode,
                    RateLimiterConfig.custom()
                        .limitRefreshPeriod(Duration.ofMinutes(1))
                        .limitForPeriod(newRateLimitPerMinute)
                        .timeoutDuration(Duration.ZERO)
                        .build()
                );
            }
        } finally {
            configLock.unlock();
        }
    }

    /**
     * Process messages for a single message group.
     * This method runs in its own dedicated virtual thread per group.
     * Polls high priority queue first (non-blocking), then regular queue (with timeout).
     * Auto-cleans up after idle period when both queues are empty.
     *
     * @param groupId the message group ID
     */
    private void processMessageGroup(String groupId) {
        LOG.debugf("Starting message group processor for [%s]", groupId);

        try {
            while (running.get()) {
                MessagePointer message = null;
                String messageId = null;
                boolean semaphoreAcquired = false;

                try {
                    // Get both priority tier queues for this group
                    BlockingQueue<MessagePointer> highQueue = highPriorityGroupQueues.get(groupId);
                    BlockingQueue<MessagePointer> regularQueue = regularGroupQueues.get(groupId);

                    // 1. Check high priority queue first (non-blocking)
                    if (highQueue != null) {
                        message = highQueue.poll();
                    }

                    // 2. If no high priority message, check regular queue with timeout
                    if (message == null) {
                        if (regularQueue != null) {
                            // Block on regular queue with idle timeout
                            message = regularQueue.poll(IDLE_TIMEOUT_MINUTES, TimeUnit.MINUTES);
                        } else if (highQueue != null) {
                            // Only high priority queue exists - block on it with timeout
                            message = highQueue.poll(IDLE_TIMEOUT_MINUTES, TimeUnit.MINUTES);
                        } else {
                            // No queues exist yet - wait briefly and retry
                            Thread.sleep(100);
                            continue;
                        }
                    }

                    if (message == null) {
                        // Idle timeout - check if both queues are empty and cleanup
                        boolean highEmpty = (highQueue == null || highQueue.isEmpty());
                        boolean regularEmpty = (regularQueue == null || regularQueue.isEmpty());

                        if (highEmpty && regularEmpty) {
                            LOG.debugf("Message group [%s] idle for %d minutes, cleaning up",
                                groupId, IDLE_TIMEOUT_MINUTES);
                            highPriorityGroupQueues.remove(groupId);
                            regularGroupQueues.remove(groupId);
                            activeGroupThreads.remove(groupId);  // Clean exit
                            return; // Exit this virtual thread
                        }
                        continue; // A queue became non-empty, keep processing
                    }

                    // We have a message!
                    totalQueuedMessages.decrementAndGet();
                    messageId = message.id();

                    // 2. Set MDC context for structured logging
                    setMDCContext(message);

                    // 3. Check if batch+group has already failed (FIFO enforcement)
                    // If a previous message in this batch+group failed, nack all subsequent messages
                    String batchId = message.batchId();
                    String messageGroupId = message.messageGroupId() != null ? message.messageGroupId() : DEFAULT_GROUP;
                    String batchGroupKey = batchId != null ? batchId + "|" + messageGroupId : null;

                    if (batchGroupKey != null && failedBatchGroups.containsKey(batchGroupKey)) {
                        LOG.warnf("Message [%s] from failed batch+group [%s], nacking to preserve FIFO ordering",
                            message.id(), batchGroupKey);
                        // Set 10s visibility for faster retry after batch failure
                        if (messageCallback instanceof tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl visibilityControl) {
                            visibilityControl.setFastFailVisibility(message);
                        }
                        nackSafely(message);
                        // Record this as a processing failure in metrics
                        poolMetrics.recordProcessingFailure(poolCode, 0, "BATCH_GROUP_FAILED");
                        decrementAndCleanupBatchGroup(batchGroupKey);
                        updateGauges(); // Update gauges since we polled from queue
                        continue; // Skip to next message
                    }

                    // 4. Wait for rate limit permit (blocking with config-change awareness)
                    // Virtual threads make this blocking wait cheap - no wasted resources
                    // Messages stay in memory instead of being NACKed back to SQS
                    waitForRateLimitPermit();

                    // 5. Acquire semaphore permit for pool-level concurrency control
                    // This ensures we don't exceed the configured concurrency limit
                    semaphore.acquire();
                    semaphoreAcquired = true;

                    // 6. Update gauges to reflect semaphore acquisition
                    updateGauges();

                    // 7. Process message through mediator
                    LOG.infof("Processing message [%s] in pool [%s] via mediator to [%s]",
                        message.id(), poolCode, message.mediationTarget());
                    long startTime = System.currentTimeMillis();
                    MediationOutcome outcome = mediator.process(message);
                    long durationMs = System.currentTimeMillis() - startTime;
                    LOG.infof("Message [%s] processing completed with result [%s] in %dms",
                        message.id(), outcome.result(), durationMs);

                    // 8. Handle mediation outcome (result + optional delay)
                    handleMediationOutcome(message, outcome, durationMs);

                } catch (InterruptedException e) {
                    LOG.warnf("Message group processor [%s] interrupted, exiting gracefully", groupId);
                    Thread.currentThread().interrupt();
                    // Nack message if we have one
                    if (message != null) {
                        nackSafely(message);
                        // Record this as a processing failure in metrics
                        poolMetrics.recordProcessingFailure(poolCode, 0, "INTERRUPTED");

                        // Decrement batch+group count on interruption
                        String intBatchId = message.batchId();
                        if (intBatchId != null && !intBatchId.isBlank()) {
                            String intMessageGroupId = message.messageGroupId() != null ? message.messageGroupId() : DEFAULT_GROUP;
                            String intBatchGroupKey = intBatchId + "|" + intMessageGroupId;
                            decrementAndCleanupBatchGroup(intBatchGroupKey);
                        }
                    }
                    break; // Exit thread

                } catch (Exception e) {
                    LOG.errorf(e, "Unexpected error processing message in group [%s]", groupId);
                    logExceptionContext(message, e);

                    // Nack message if we have one
                    if (message != null) {
                        nackSafely(message);
                        recordProcessingError(message, e);

                        // Decrement batch+group count on exception
                        String exBatchId = message.batchId();
                        if (exBatchId != null && !exBatchId.isBlank()) {
                            String exMessageGroupId = message.messageGroupId() != null ? message.messageGroupId() : DEFAULT_GROUP;
                            String exBatchGroupKey = exBatchId + "|" + exMessageGroupId;
                            decrementAndCleanupBatchGroup(exBatchGroupKey);
                        }
                    }

                } finally {
                    // CRITICAL: Cleanup always happens here, regardless of exception path
                    performCleanup(messageId, semaphoreAcquired);
                }
            }
        } finally {
            // Final cleanup when thread exits
            // Remove from active threads tracking - this allows detection of unexpected death
            activeGroupThreads.remove(groupId);

            // Check if there are orphaned messages in either queue
            // This can happen if we exited due to an error rather than idle timeout
            BlockingQueue<MessagePointer> finalHighQueue = highPriorityGroupQueues.get(groupId);
            BlockingQueue<MessagePointer> finalRegularQueue = regularGroupQueues.get(groupId);
            int remainingHigh = (finalHighQueue != null) ? finalHighQueue.size() : 0;
            int remainingRegular = (finalRegularQueue != null) ? finalRegularQueue.size() : 0;
            int remainingMessages = remainingHigh + remainingRegular;

            if (remainingMessages > 0 && running.get()) {
                LOG.warnf("Message group processor [%s] exiting with %d messages still in queue (high: %d, regular: %d) - " +
                    "these will be processed when a new message arrives and restarts the thread",
                    groupId, remainingMessages, remainingHigh, remainingRegular);
                warningService.addWarning(
                    "GROUP_THREAD_EXIT_WITH_MESSAGES",
                    "WARN",
                    String.format("Group [%s] thread exited with %d orphaned messages (high: %d, regular: %d)",
                        groupId, remainingMessages, remainingHigh, remainingRegular),
                    "ProcessPool:" + poolCode
                );
            } else {
                LOG.debugf("Message group processor [%s] exiting cleanly", groupId);
            }
        }
    }

    /**
     * Set MDC context for structured logging
     */
    private void setMDCContext(MessagePointer message) {
        MDC.put("messageId", message.id());
        MDC.put("poolCode", poolCode);
        MDC.put("mediationType", message.mediationType().toString());
        MDC.put("targetUri", message.mediationTarget());
    }

    /**
     * Waits for a rate limit permit, handling config changes gracefully.
     * Uses a timed poll loop to detect when rate limiter is replaced or removed.
     * Virtual threads make this blocking wait cheap.
     *
     * <p>This method handles the following scenarios:
     * <ul>
     *   <li>Rate limit removed (100→null): Returns immediately on next check</li>
     *   <li>Rate limit changed (100→200): Uses new limiter on next poll</li>
     *   <li>Permits available: acquirePermission() succeeds immediately</li>
     *   <li>Shutdown: running flag false, exits loop</li>
     * </ul>
     */
    private void waitForRateLimitPermit() {
        boolean recordedRateLimit = false;

        while (running.get()) {
            // Store in local variable to detect config changes
            RateLimiter limiter = this.rateLimiter;
            if (limiter == null) {
                return; // No rate limiting configured, proceed immediately
            }

            if (limiter.acquirePermission()) {
                return; // Got permit, proceed with processing
            }

            // Record rate limit event once per wait (not every poll)
            if (!recordedRateLimit) {
                poolMetrics.recordRateLimitExceeded(poolCode);
                recordedRateLimit = true;
                LOG.debugf("Pool [%s] rate limited - waiting for permit", poolCode);
            }

            // No permit available - wait briefly then re-check
            // This handles: rate limit removed, rate limit changed, permits available
            try {
                Thread.sleep(100); // 100ms poll interval - cheap with virtual threads
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
        }
    }

    /**
     * Handle the outcome of message mediation (result + optional delay).
     */
    private void handleMediationOutcome(MessagePointer message, MediationOutcome outcome, long durationMs) {
        // Defensive: mediator should never return null, but guard against it
        if (outcome == null || outcome.result() == null) {
            LOG.errorf("CRITICAL: Mediator returned null outcome/result for message [%s], treating as transient error", message.id());
            outcome = MediationOutcome.errorProcess((Integer) null);
            warningService.addWarning(
                "MEDIATOR_NULL_RESULT",
                "CRITICAL",
                "Mediator returned null outcome for message " + message.id(),
                "ProcessPool:" + poolCode
            );
        }

        MediationResult result = outcome.result();
        MDC.put("result", result.name());
        MDC.put("durationMs", String.valueOf(durationMs));

        // Get batch+group key for FIFO tracking
        String batchId = message.batchId();
        String messageGroupId = message.messageGroupId() != null ? message.messageGroupId() : DEFAULT_GROUP;
        String batchGroupKey = batchId != null ? batchId + "|" + messageGroupId : null;

        if (result == MediationResult.SUCCESS) {
            LOG.infof("Message [%s] processed successfully - ACKing and removing from queue", message.id());
            poolMetrics.recordProcessingSuccess(poolCode, durationMs);
            messageCallback.ack(message);

            // Decrement batch+group count on success
            if (batchGroupKey != null) {
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        } else if (result == MediationResult.ERROR_CONFIG) {
            // Configuration error (4xx) - ACK to prevent infinite retries
            // Note: HttpMediator generates detailed warnings with actual HTTP status codes,
            // so we don't generate a duplicate generic warning here
            LOG.warnf("Message [%s] configuration error - ACKing to prevent retry: %s", message.id(), result);
            poolMetrics.recordProcessingFailure(poolCode, durationMs, result.name());
            messageCallback.ack(message);

            // Decrement batch+group count (config error is treated as ack)
            if (batchGroupKey != null) {
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        } else if (result == MediationResult.ERROR_PROCESS) {
            // Transient error: Message will be retried via queue visibility timeout
            // Do NOT count as failure - these may eventually succeed
            // Examples: 200 with ack=false (not ready yet), 5xx errors (infrastructure issues)
            poolMetrics.recordProcessingTransient(poolCode, durationMs);

            // Set visibility timeout based on delay from outcome
            if (messageCallback instanceof tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl visibilityControl) {
                if (outcome.hasCustomDelay()) {
                    int delaySeconds = outcome.getEffectiveDelaySeconds();
                    LOG.warnf("Message [%s] encountered transient error with custom delay=%ds - NACKing for delayed retry: %s",
                        message.id(), delaySeconds, result);
                    visibilityControl.setVisibilityDelay(message, delaySeconds);
                } else {
                    LOG.warnf("Message [%s] encountered transient error - NACKing for retry: %s", message.id(), result);
                    visibilityControl.resetVisibilityToDefault(message);
                }
            } else {
                LOG.warnf("Message [%s] encountered transient error - NACKing for retry: %s", message.id(), result);
            }

            messageCallback.nack(message);

            // Mark batch+group as failed to trigger cascading nacks
            if (batchGroupKey != null) {
                boolean wasAlreadyFailed = failedBatchGroups.putIfAbsent(batchGroupKey, Boolean.TRUE) != null;
                if (!wasAlreadyFailed) {
                    LOG.warnf("Batch+group [%s] marked as failed - all remaining messages in this batch+group will be nacked",
                        batchGroupKey);
                }
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        } else if (result == MediationResult.ERROR_CONNECTION) {
            // Connection errors - transient, NACK for visibility timeout retry
            LOG.warnf("Message [%s] connection error - NACKing for retry: %s", message.id(), result);
            poolMetrics.recordProcessingFailure(poolCode, durationMs, result.name());

            if (messageCallback instanceof tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl visibilityControl) {
                visibilityControl.resetVisibilityToDefault(message);
            }

            messageCallback.nack(message);

            // Mark batch+group as failed to trigger cascading nacks
            if (batchGroupKey != null) {
                boolean wasAlreadyFailed = failedBatchGroups.putIfAbsent(batchGroupKey, Boolean.TRUE) != null;
                if (!wasAlreadyFailed) {
                    LOG.warnf("Batch+group [%s] marked as failed - all remaining messages in this batch+group will be nacked",
                        batchGroupKey);
                }
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        } else {
            // Unknown result - log warning and treat as transient error
            LOG.warnf("Message [%s] unexpected result: %s - NACKing for retry", message.id(), result);
            poolMetrics.recordProcessingFailure(poolCode, durationMs, result.name());

            if (messageCallback instanceof tech.flowcatalyst.messagerouter.callback.MessageVisibilityControl visibilityControl) {
                visibilityControl.resetVisibilityToDefault(message);
            }

            messageCallback.nack(message);

            // Mark batch+group as failed to trigger cascading nacks
            if (batchGroupKey != null) {
                boolean wasAlreadyFailed = failedBatchGroups.putIfAbsent(batchGroupKey, Boolean.TRUE) != null;
                if (!wasAlreadyFailed) {
                    LOG.warnf("Batch+group [%s] marked as failed - all remaining messages in this batch+group will be nacked",
                        batchGroupKey);
                }
                decrementAndCleanupBatchGroup(batchGroupKey);
            }
        }
    }

    /**
     * Safely nack a message, catching any exceptions
     */
    private void nackSafely(MessagePointer message) {
        try {
            messageCallback.nack(message);
        } catch (Exception e) {
            LOG.errorf(e, "Error nacking message during exception handling: %s", message.id());
        }
    }

    /**
     * Log exception context for diagnostics
     */
    private void logExceptionContext(MessagePointer message, Exception e) {
        warningService.addWarning(
            "PROCESSING",
            "WARN",
            String.format("Unexpected error processing message %s: %s",
                message != null ? message.id() : "unknown",
                e.getMessage()),
            "ProcessPool:" + poolCode
        );
    }

    /**
     * Record processing error in metrics
     */
    private void recordProcessingError(MessagePointer message, Exception e) {
        poolMetrics.recordProcessingFailure(
            poolCode,
            0,  // No duration for exceptions
            "EXCEPTION_" + e.getClass().getSimpleName()
        );
    }

    /**
     * Perform cleanup of all resources
     * This method is called in the finally block and must NEVER throw exceptions
     * NOTE: inPipelineMap removal is handled by QueueManager.ack()/nack()
     *
     * CRITICAL: Semaphore release MUST happen first and is wrapped in its own try-catch
     * to guarantee it executes even if other cleanup operations fail.
     */
    private void performCleanup(String messageId, boolean semaphoreAcquired) {
        // CRITICAL: Release semaphore permit FIRST in its own try-catch
        // This MUST happen even if everything else fails to prevent permit leaks
        if (semaphoreAcquired) {
            try {
                semaphore.release();
            } catch (Exception e) {
                // This should never happen, but log if it does
                LOG.errorf(e, "CRITICAL: Failed to release semaphore for message [%s] in pool [%s]. " +
                    "This will cause a permit leak!", messageId, poolCode);
                warningService.addWarning(
                    "SEMAPHORE_RELEASE_FAILED",
                    "CRITICAL",
                    String.format("Failed to release semaphore for message [%s] in pool [%s]", messageId, poolCode),
                    "ProcessPool:" + poolCode
                );
            }
        }

        // Now do remaining cleanup - these are less critical
        try {
            // Update gauges
            if (semaphoreAcquired) {
                updateGauges();
            }
            MDC.clear();
        } catch (Exception e) {
            // Non-critical cleanup failure - log but don't propagate
            LOG.warnf(e, "Error during non-critical cleanup for message: %s", messageId);
        }
    }

    private void updateGauges() {
        int activeWorkers = concurrency - semaphore.availablePermits();
        int availablePermits = semaphore.availablePermits();
        int queueSize = totalQueuedMessages.get();
        int messageGroupCount = getActiveGroupCount();

        // DEBUG: Log per-group queue sizes when we have queued messages
        if (queueSize > 0 && messageGroupCount > 1) {
            StringBuilder groupStats = new StringBuilder();
            groupStats.append(String.format("Pool [%s] distribution: ", poolCode));
            // Log high priority queue sizes
            highPriorityGroupQueues.forEach((groupId, queue) -> {
                int size = queue.size();
                if (size > 0) {
                    groupStats.append(String.format("[%s(HI): %d] ", groupId, size));
                }
            });
            // Log regular queue sizes
            regularGroupQueues.forEach((groupId, queue) -> {
                int size = queue.size();
                if (size > 0) {
                    groupStats.append(String.format("[%s: %d] ", groupId, size));
                }
            });
            LOG.warnf("*** POOL DISTRIBUTION *** %s | Total: %d, Groups: %d, Active: %d/%d",
                groupStats.toString(), queueSize, messageGroupCount, activeWorkers, concurrency);
        }

        poolMetrics.updatePoolGauges(poolCode, activeWorkers, availablePermits, queueSize, messageGroupCount);
    }

    /**
     * Returns the count of unique active message groups across both priority tiers.
     */
    private int getActiveGroupCount() {
        java.util.Set<String> allGroups = new java.util.HashSet<>();
        allGroups.addAll(highPriorityGroupQueues.keySet());
        allGroups.addAll(regularGroupQueues.keySet());
        return allGroups.size();
    }

    /**
     * Decrement the message count for a batch+group and clean up tracking maps when count reaches zero.
     * This method is called when:
     * - A message is successfully processed
     * - A message fails and is nacked
     * - A message submission fails
     *
     * @param batchGroupKey the batch+group key in format "batchId|messageGroupId"
     */
    private void decrementAndCleanupBatchGroup(String batchGroupKey) {
        AtomicInteger counter = batchGroupMessageCount.get(batchGroupKey);
        if (counter != null) {
            int remaining = counter.decrementAndGet();
            LOG.debugf("Batch+group [%s] count decremented, remaining: %d", batchGroupKey, remaining);

            if (remaining <= 0) {
                // All messages in this batch+group have been processed
                // Clean up both tracking maps
                batchGroupMessageCount.remove(batchGroupKey);
                failedBatchGroups.remove(batchGroupKey);
                LOG.debugf("Batch+group [%s] fully processed, cleaned up tracking maps", batchGroupKey);
            }
        }
    }
}

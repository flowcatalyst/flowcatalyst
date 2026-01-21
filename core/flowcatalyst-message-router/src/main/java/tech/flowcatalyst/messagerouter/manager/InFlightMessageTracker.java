package tech.flowcatalyst.messagerouter.manager;

import tech.flowcatalyst.messagerouter.callback.MessageCallback;
import tech.flowcatalyst.messagerouter.model.MessagePointer;

import java.time.Instant;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.locks.ReentrantReadWriteLock;
import java.util.function.Function;
import java.util.stream.Stream;

/**
 * Tracks messages currently in-flight through the processing pipeline.
 *
 * <p>This class consolidates 5 previously separate maps into a single cohesive unit,
 * ensuring consistency when tracking/removing messages and preventing map synchronization bugs.
 *
 * <h2>Tracked State Per Message</h2>
 * <ul>
 *   <li>Pipeline key (SQS message ID or app message ID)</li>
 *   <li>Application message ID (for requeued message detection)</li>
 *   <li>Broker message ID (SQS/AMQP message ID)</li>
 *   <li>Queue ID (source queue identifier)</li>
 *   <li>Callback (for ack/nack operations)</li>
 *   <li>Timestamp (when message was tracked)</li>
 * </ul>
 *
 * <h2>Thread Safety</h2>
 * <p>All operations are thread-safe. The class uses a read-write lock for compound
 * operations that need consistency across multiple internal maps.
 *
 * @see TrackedMessage
 * @see TrackResult
 */
public class InFlightMessageTracker {

    // Primary storage: pipelineKey -> TrackedMessage
    private final ConcurrentHashMap<String, TrackedMessage> trackedMessages = new ConcurrentHashMap<>();

    // Secondary index: appMessageId -> pipelineKey (for requeue detection)
    private final ConcurrentHashMap<String, String> appMessageIdIndex = new ConcurrentHashMap<>();

    // Lock for compound operations that need consistency across both maps
    private final ReentrantReadWriteLock lock = new ReentrantReadWriteLock();

    /**
     * Record containing all tracked state for a message.
     *
     * @param pipelineKey the key used to track this message (SQS message ID or app ID)
     * @param messageId the application message ID
     * @param brokerMessageId the broker's message ID (e.g., SQS message ID)
     * @param queueId the source queue identifier
     * @param message the message pointer
     * @param callback the callback for ack/nack
     * @param trackedAt when the message was added to tracking
     */
    public record TrackedMessage(
        String pipelineKey,
        String messageId,
        String brokerMessageId,
        String queueId,
        MessagePointer message,
        MessageCallback callback,
        Instant trackedAt
    ) {}

    /**
     * Result of attempting to track a message.
     */
    public sealed interface TrackResult permits TrackResult.Tracked, TrackResult.Duplicate {
        /**
         * Message was successfully tracked.
         *
         * @param pipelineKey the key assigned to track this message
         */
        record Tracked(String pipelineKey) implements TrackResult {}

        /**
         * Message is a duplicate - already being tracked.
         *
         * @param existingPipelineKey the key of the existing tracked message
         * @param isRequeue true if this is a requeue (same app ID, different broker ID)
         */
        record Duplicate(String existingPipelineKey, boolean isRequeue) implements TrackResult {}
    }

    /**
     * Track a new message in the pipeline.
     *
     * <p>This method performs duplicate detection using both:
     * <ul>
     *   <li>Broker message ID (same physical message redelivered)</li>
     *   <li>Application message ID (requeued message with new broker ID)</li>
     * </ul>
     *
     * @param message the message to track
     * @param callback the callback for ack/nack operations
     * @param queueId the source queue identifier
     * @return tracking result indicating success or duplicate
     */
    public TrackResult track(MessagePointer message, MessageCallback callback, String queueId) {
        String pipelineKey = getPipelineKey(message);
        String appMessageId = message.id();
        String brokerMessageId = message.sqsMessageId();

        lock.writeLock().lock();
        try {
            // Check 1: Same broker message ID (physical redelivery due to visibility timeout)
            if (trackedMessages.containsKey(pipelineKey)) {
                return new TrackResult.Duplicate(pipelineKey, false);
            }

            // Check 2: Same app message ID but different broker ID (requeued by external process)
            String existingPipelineKey = appMessageIdIndex.get(appMessageId);
            if (existingPipelineKey != null) {
                // Verify it's still tracked (not a stale index entry)
                if (trackedMessages.containsKey(existingPipelineKey)) {
                    return new TrackResult.Duplicate(existingPipelineKey, true);
                }
                // Stale index entry - clean it up
                appMessageIdIndex.remove(appMessageId);
            }

            // Track the message
            TrackedMessage tracked = new TrackedMessage(
                pipelineKey,
                appMessageId,
                brokerMessageId,
                queueId,
                message,
                callback,
                Instant.now()
            );

            trackedMessages.put(pipelineKey, tracked);
            appMessageIdIndex.put(appMessageId, pipelineKey);

            return new TrackResult.Tracked(pipelineKey);

        } finally {
            lock.writeLock().unlock();
        }
    }

    /**
     * Remove a message from tracking and return its tracked state.
     *
     * @param pipelineKey the pipeline key of the message to remove
     * @return the tracked message if found, empty otherwise
     */
    public Optional<TrackedMessage> remove(String pipelineKey) {
        lock.writeLock().lock();
        try {
            TrackedMessage removed = trackedMessages.remove(pipelineKey);
            if (removed != null) {
                appMessageIdIndex.remove(removed.messageId());
            }
            return Optional.ofNullable(removed);
        } finally {
            lock.writeLock().unlock();
        }
    }

    /**
     * Remove a message using the message pointer (derives pipeline key automatically).
     *
     * @param message the message to remove
     * @return the tracked message if found, empty otherwise
     */
    public Optional<TrackedMessage> remove(MessagePointer message) {
        return remove(getPipelineKey(message));
    }

    /**
     * Get the callback for a tracked message.
     *
     * @param pipelineKey the pipeline key
     * @return the callback if message is tracked, empty otherwise
     */
    public Optional<MessageCallback> getCallback(String pipelineKey) {
        lock.readLock().lock();
        try {
            TrackedMessage tracked = trackedMessages.get(pipelineKey);
            return tracked != null ? Optional.of(tracked.callback()) : Optional.empty();
        } finally {
            lock.readLock().unlock();
        }
    }

    /**
     * Get a tracked message by its pipeline key.
     *
     * @param pipelineKey the pipeline key
     * @return the tracked message if found, empty otherwise
     */
    public Optional<TrackedMessage> get(String pipelineKey) {
        lock.readLock().lock();
        try {
            return Optional.ofNullable(trackedMessages.get(pipelineKey));
        } finally {
            lock.readLock().unlock();
        }
    }

    /**
     * Get the message pointer for a tracked message.
     *
     * @param pipelineKey the pipeline key
     * @return the message pointer if tracked, empty otherwise
     */
    public Optional<MessagePointer> getMessage(String pipelineKey) {
        return get(pipelineKey).map(TrackedMessage::message);
    }

    /**
     * Check if a message ID is currently in-flight.
     *
     * @param appMessageId the application message ID
     * @return true if a message with this app ID is being tracked
     */
    public boolean isInFlight(String appMessageId) {
        lock.readLock().lock();
        try {
            String pipelineKey = appMessageIdIndex.get(appMessageId);
            return pipelineKey != null && trackedMessages.containsKey(pipelineKey);
        } finally {
            lock.readLock().unlock();
        }
    }

    /**
     * Check if a pipeline key is currently tracked.
     *
     * @param pipelineKey the pipeline key
     * @return true if this pipeline key is being tracked
     */
    public boolean containsKey(String pipelineKey) {
        return trackedMessages.containsKey(pipelineKey);
    }

    /**
     * Get the number of messages currently in-flight.
     *
     * @return count of tracked messages
     */
    public int size() {
        return trackedMessages.size();
    }

    /**
     * Clear all tracked messages.
     * Use with caution - typically only during shutdown.
     *
     * @return stream of all tracked messages that were cleared
     */
    public Stream<TrackedMessage> clear() {
        lock.writeLock().lock();
        try {
            var messages = trackedMessages.values().stream().toList();
            trackedMessages.clear();
            appMessageIdIndex.clear();
            return messages.stream();
        } finally {
            lock.writeLock().unlock();
        }
    }

    /**
     * Stream all tracked messages (for iteration/cleanup).
     *
     * <p>Note: The returned stream provides a snapshot view. Modifications during
     * iteration may not be reflected.
     *
     * @return stream of all tracked messages
     */
    public Stream<TrackedMessage> stream() {
        lock.readLock().lock();
        try {
            // Return a copy to avoid ConcurrentModificationException
            return trackedMessages.values().stream().toList().stream();
        } finally {
            lock.readLock().unlock();
        }
    }

    /**
     * Update the callback for a tracked message (e.g., when receipt handle changes).
     *
     * @param pipelineKey the pipeline key
     * @param callbackUpdater function to update the callback
     * @return true if the message was found and updated
     */
    public boolean updateCallback(String pipelineKey, Function<MessageCallback, MessageCallback> callbackUpdater) {
        lock.writeLock().lock();
        try {
            TrackedMessage existing = trackedMessages.get(pipelineKey);
            if (existing == null) {
                return false;
            }

            MessageCallback newCallback = callbackUpdater.apply(existing.callback());
            TrackedMessage updated = new TrackedMessage(
                existing.pipelineKey(),
                existing.messageId(),
                existing.brokerMessageId(),
                existing.queueId(),
                existing.message(),
                newCallback,
                existing.trackedAt()
            );

            trackedMessages.put(pipelineKey, updated);
            return true;

        } finally {
            lock.writeLock().unlock();
        }
    }

    /**
     * Get the pipeline key for a message.
     * Uses SQS message ID if available, otherwise falls back to app message ID.
     *
     * @param message the message
     * @return the pipeline key
     */
    public static String getPipelineKey(MessagePointer message) {
        return message.sqsMessageId() != null ? message.sqsMessageId() : message.id();
    }
}

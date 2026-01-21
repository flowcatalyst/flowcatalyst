package tech.flowcatalyst.messagerouter.metrics;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;

import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;

@ApplicationScoped
public class MicrometerQueueMetricsService implements QueueMetricsService {

    @Inject
    MeterRegistry meterRegistry;

    private final Map<String, QueueMetricsHolder> queueMetrics = new ConcurrentHashMap<>();

    @Override
    public void recordMessageReceived(String queueIdentifier) {
        getOrCreateMetrics(queueIdentifier).messagesReceived.increment();
    }

    @Override
    public void recordMessageProcessed(String queueIdentifier, boolean success) {
        QueueMetricsHolder metrics = getOrCreateMetrics(queueIdentifier);
        if (success) {
            metrics.messagesConsumed.increment();
        } else {
            metrics.messagesFailed.increment();
        }
        metrics.lastProcessedTime = System.currentTimeMillis();

        // Track timestamped outcome for 30-minute rolling window
        long now = System.currentTimeMillis();
        metrics.recordedOutcomes.add(new TimestampedQueueOutcome(now, success));
    }

    @Override
    public void recordMessageDeferred(String queueIdentifier) {
        getOrCreateMetrics(queueIdentifier).messagesDeferred.increment();
    }

    @Override
    public void recordQueueDepth(String queueIdentifier, long depth) {
        getOrCreateMetrics(queueIdentifier).currentDepth.set(depth);
    }

    @Override
    public void recordQueueMetrics(String queueIdentifier, long pendingMessages, long messagesNotVisible) {
        QueueMetricsHolder metrics = getOrCreateMetrics(queueIdentifier);
        metrics.pendingMessages.set(pendingMessages);
        metrics.messagesNotVisible.set(messagesNotVisible);
    }

    @Override
    public QueueStats getQueueStats(String queueIdentifier) {
        QueueMetricsHolder metrics = queueMetrics.get(queueIdentifier);
        if (metrics == null) {
            return QueueStats.empty(queueIdentifier);
        }

        return buildStats(queueIdentifier, metrics);
    }

    @Override
    public Map<String, QueueStats> getAllQueueStats() {
        Map<String, QueueStats> allStats = new ConcurrentHashMap<>();
        queueMetrics.forEach((queueId, metrics) ->
            allStats.put(queueId, buildStats(queueId, metrics))
        );
        return allStats;
    }

    private QueueMetricsHolder getOrCreateMetrics(String queueIdentifier) {
        return queueMetrics.computeIfAbsent(queueIdentifier, queueId -> {
            Counter received = Counter.builder("flowcatalyst.queue.messages.received")
                .tag("queue", queueId)
                .description("Total messages received from queue")
                .register(meterRegistry);

            Counter consumed = Counter.builder("flowcatalyst.queue.messages.consumed")
                .tag("queue", queueId)
                .description("Total messages successfully consumed")
                .register(meterRegistry);

            Counter failed = Counter.builder("flowcatalyst.queue.messages.failed")
                .tag("queue", queueId)
                .description("Total messages failed")
                .register(meterRegistry);

            Counter deferred = Counter.builder("flowcatalyst.queue.messages.deferred")
                .tag("queue", queueId)
                .description("Total messages deferred (rate limiting, capacity - not failures)")
                .register(meterRegistry);

            // Create AtomicLongs first, then register - gauge() can return null after extended runtime
            // if weak references are cleaned up or there are gauge conflicts
            AtomicLong depth = new AtomicLong(0);
            meterRegistry.gauge(
                "flowcatalyst.queue.depth",
                io.micrometer.core.instrument.Tags.of("queue", queueId),
                depth
            );

            AtomicLong pending = new AtomicLong(0);
            meterRegistry.gauge(
                "flowcatalyst.queue.pending",
                io.micrometer.core.instrument.Tags.of("queue", queueId),
                pending
            );

            AtomicLong notVisible = new AtomicLong(0);
            meterRegistry.gauge(
                "flowcatalyst.queue.not_visible",
                io.micrometer.core.instrument.Tags.of("queue", queueId),
                notVisible
            );

            return new QueueMetricsHolder(received, consumed, failed, deferred, depth, pending, notVisible);
        });
    }

    private QueueStats buildStats(String queueIdentifier, QueueMetricsHolder metrics) {
        long totalMessages = (long) metrics.messagesReceived.count();
        long totalConsumed = (long) metrics.messagesConsumed.count();
        long totalFailed = (long) metrics.messagesFailed.count();
        long totalDeferred = (long) metrics.messagesDeferred.count();
        long currentSize = metrics.currentDepth.get();

        double successRate = totalMessages > 0
            ? (double) totalConsumed / totalMessages
            : 1.0;  // Empty queues show 100% (no failures yet)

        // Calculate throughput: messages per second over last minute
        long now = System.currentTimeMillis();
        long elapsedSeconds = (now - metrics.startTime) / 1000;
        double throughput = elapsedSeconds > 0
            ? (double) totalConsumed / elapsedSeconds
            : 0.0;

        // Calculate rolling window metrics
        long fiveMinutesAgoMs = now - (5 * 60 * 1000);
        long thirtyMinutesAgoMs = now - (30 * 60 * 1000);

        long consumed5min = 0;
        long failed5min = 0;
        long consumed30min = 0;
        long failed30min = 0;

        // Clean up old outcomes and count recent ones
        synchronized(metrics.recordedOutcomes) {
            metrics.recordedOutcomes.removeIf(outcome -> outcome.timestamp < thirtyMinutesAgoMs);

            for (TimestampedQueueOutcome outcome : metrics.recordedOutcomes) {
                if (outcome.success) {
                    if (outcome.timestamp >= fiveMinutesAgoMs) {
                        consumed5min++;
                    }
                    consumed30min++;
                } else {
                    if (outcome.timestamp >= fiveMinutesAgoMs) {
                        failed5min++;
                    }
                    failed30min++;
                }
            }
        }

        long totalMessages5min = consumed5min + failed5min;
        double successRate5min = totalMessages5min > 0
            ? (double) consumed5min / totalMessages5min
            : 1.0;  // Empty window shows 100%

        long totalMessages30min = consumed30min + failed30min;
        double successRate30min = totalMessages30min > 0
            ? (double) consumed30min / totalMessages30min
            : 1.0;  // Empty window shows 100%

        return new QueueStats(
            queueIdentifier,
            totalMessages,
            totalConsumed,
            totalFailed,
            successRate,
            currentSize,
            throughput,
            metrics.pendingMessages.get(),
            metrics.messagesNotVisible.get(),
            totalMessages5min,
            consumed5min,
            failed5min,
            successRate5min,
            totalMessages30min,
            consumed30min,
            failed30min,
            successRate30min,
            totalDeferred
        );
    }

    private static class QueueMetricsHolder {
        final Counter messagesReceived;
        final Counter messagesConsumed;
        final Counter messagesFailed;
        final Counter messagesDeferred;
        final AtomicLong currentDepth;
        final AtomicLong pendingMessages;
        final AtomicLong messagesNotVisible;
        final long startTime;
        volatile long lastProcessedTime;
        final List<TimestampedQueueOutcome> recordedOutcomes;

        QueueMetricsHolder(Counter received, Counter consumed, Counter failed, Counter deferred,
                          AtomicLong depth, AtomicLong pending, AtomicLong notVisible) {
            this.messagesReceived = received;
            this.messagesConsumed = consumed;
            this.messagesFailed = failed;
            this.messagesDeferred = deferred;
            this.currentDepth = depth;
            this.pendingMessages = pending;
            this.messagesNotVisible = notVisible;
            this.startTime = System.currentTimeMillis();
            this.lastProcessedTime = System.currentTimeMillis();
            this.recordedOutcomes = new ArrayList<>();
        }
    }

    /**
     * Timestamped outcome for 30-minute rolling window calculation
     */
    private static class TimestampedQueueOutcome {
        final long timestamp;
        final boolean success;

        TimestampedQueueOutcome(long timestamp, boolean success) {
            this.timestamp = timestamp;
            this.success = success;
        }
    }
}

package io.flowcatalyst.ingestor.functions.trace;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.metrics.Counter;
import io.flowcatalyst.framework.metrics.Metrics;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import java.time.Duration;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.locks.Condition;
import java.util.concurrent.locks.ReentrantLock;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.core.JacksonException;

/// Ships function trace records to the platform (`POST /control/functions/trace-log`) from a thread of its own, as the
/// ingestion trace shipper does for ingestions. The queue is bounded by count and by bytes and is one FIFO per
/// function: when it is full the oldest record of the function with the most queued is dropped and counted, so a chatty
/// function cannot push every other function's records out, and a slow or absent platform costs records, never a call.
/// A batch takes the functions in turn and goes when [FunctionTraceOptions#flushRecords] are waiting or the oldest has
/// waited [FunctionTraceOptions#flushEvery]; it holds at most 500 records and is under 1 MiB of encoded JSON.
///
/// What the platform answers decides what becomes of a batch: `413` splits it in two (a lone record still too large is
/// dropped); `401` and `403` are tried once more at once (the token was cleared by the call, so the next one fetches a
/// new token); `408`, `429`, `5xx` and no answer are tried again after a growing pause, up to [#ATTEMPTS] times; any
/// other refusal drops the batch, since sending it again would be refused again. Records carry ids, so a batch the
/// platform took but whose answer was lost is stored once when sent again. What is dropped is logged as a count, never
/// as content, and counted in `fc_fn_trace_dropped_total`; `fc_fn_trace_records_total{kind}` counts records queued and
/// `fc_fn_trace_flush_failures_total` batches the platform did not take.
public final class FunctionTraceShipper implements FunctionTraceSink, AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(FunctionTraceShipper.class);

    /// Tries a batch gets when the platform does not take it for a reason that may pass.
    public static final int ATTEMPTS = 6;

    /// What the platform's batch limit leaves for the records: under 1 MiB, less the envelope.
    private static final int BATCH_BYTES = Control.MAX_TRACE_BATCH_BYTES - 16 * 1024;

    private static final long LOG_EVERY_NANOS = Duration.ofMinutes(1).toNanos();

    private record Queued(Control.TraceRecord record, int bytes, long queuedNanos) {
    }

    private final ControlPlane plane;
    private final String runnerId;
    private final String pool;
    private final int maxRecords;
    private final long maxBytes;
    private final int flushRecords;
    private final Duration flushEvery;
    private final Duration firstBackoff;
    private final Duration maxBackoff;
    private final Counter dropMetric;
    private final Counter failureMetric;
    private final Counter recordsMetric;

    private final ReentrantLock lock = new ReentrantLock();
    private final Condition changed = lock.newCondition();
    /// One FIFO per function address, in the order the functions first queued.
    private final Map<String, ArrayDeque<Queued>> queues = new LinkedHashMap<>();
    private int queuedCount;
    private long queuedBytes;
    private boolean closing;
    private long closeDeadlineNanos;

    private final AtomicLong droppedTotal = new AtomicLong();
    private final AtomicLong shippedTotal = new AtomicLong();
    private final AtomicLong failuresTotal = new AtomicLong();
    private long droppedLogged;
    private long lastLogNanos = System.nanoTime() - LOG_EVERY_NANOS;
    private @Nullable Thread thread;

    /// @param runnerId names this host in every batch
    /// @param pool     the function pool this host serves
    public FunctionTraceShipper(ControlPlane plane, String runnerId, String pool, FunctionTraceOptions options, Metrics metrics) {
        this.plane = Objects.requireNonNull(plane, "plane");
        this.runnerId = Objects.requireNonNull(runnerId, "runnerId");
        this.pool = Objects.requireNonNull(pool, "pool");
        this.maxRecords = Math.max(1, options.queueRecords());
        this.maxBytes = options.queueBytes();
        this.flushRecords = Math.max(1, Math.min(options.flushRecords(), Control.MAX_TRACE_BATCH_RECORDS));
        this.flushEvery = options.flushEvery();
        this.firstBackoff = options.firstBackoff();
        this.maxBackoff = options.maxBackoff();
        this.recordsMetric = metrics.counter("fc_fn_trace_records_total", "Trace records queued, by kind.", "kind");
        this.dropMetric = metrics.counter("fc_fn_trace_dropped_total", "Trace records dropped: queue overflow, a batch refused or given up on.");
        this.failureMetric = metrics.counter("fc_fn_trace_flush_failures_total", "Trace batches the platform did not take.");
    }

    /// Starts the thread that ships.
    public void start() {
        lock.lock();
        try {
            if (thread == null) {
                thread = Thread.ofPlatform().daemon().name("fn-trace-shipper").start(this::loop);
            }
        } finally {
            lock.unlock();
        }
    }

    @Override
    public void offer(Control.TraceRecord record) {
        int bytes;
        try {
            bytes = Json.mapper().writeValueAsBytes(record).length;
        } catch (JacksonException e) {
            dropped(1, null);
            return;
        }
        int dropped = 0;
        boolean kept = false;
        String why = "the queue is full";
        lock.lock();
        try {
            if (closing) {
                dropped = 1;
                why = "the host is stopping";
            } else {
                kept = true;
                while (queuedCount > 0 && (queuedCount >= maxRecords || queuedBytes + bytes > maxBytes)) {
                    dropOldestOfLargest();
                    dropped++;
                }
                queues.computeIfAbsent(record.address(), a -> new ArrayDeque<>()).addLast(new Queued(record, bytes, System.nanoTime()));
                queuedCount++;
                queuedBytes += bytes;
                if (queuedCount >= flushRecords || queuedCount == 1) {
                    changed.signalAll();
                }
            }
        } finally {
            lock.unlock();
        }
        if (kept) {
            recordsMetric.labels(record.kind()).inc();
        }
        if (dropped > 0) {
            dropped(dropped, why);
        }
    }

    /// Removes the oldest record of the function with the most queued (the lower address on a tie). The lock is held.
    private void dropOldestOfLargest() {
        String victim = null;
        int most = 0;
        for (Map.Entry<String, ArrayDeque<Queued>> e : queues.entrySet()) {
            int size = e.getValue().size();
            if (size > most || (size == most && victim != null && e.getKey().compareTo(victim) < 0)) {
                victim = e.getKey();
                most = size;
            }
        }
        if (victim == null) {
            return;
        }
        ArrayDeque<Queued> queue = Objects.requireNonNull(queues.get(victim));
        Queued oldest = queue.removeFirst();
        queuedBytes -= oldest.bytes();
        queuedCount--;
        if (queue.isEmpty()) {
            queues.remove(victim);
        }
    }

    /// Records dropped so far: overflow, a batch the platform refused, a batch given up on.
    public long dropped() {
        return droppedTotal.get();
    }

    /// Records the platform took so far.
    public long shipped() {
        return shippedTotal.get();
    }

    /// Batches the platform did not take so far (every failed attempt counts).
    public long flushFailures() {
        return failuresTotal.get();
    }

    /// Records waiting.
    public int queued() {
        lock.lock();
        try {
            return queuedCount;
        } finally {
            lock.unlock();
        }
    }

    /// Flushes what is queued, for at most `bound`, and stops the thread.
    public void close(Duration bound) {
        Thread running;
        lock.lock();
        try {
            closing = true;
            closeDeadlineNanos = System.nanoTime() + bound.toNanos();
            changed.signalAll();
            running = thread;
        } finally {
            lock.unlock();
        }
        if (running == null) {
            return;
        }
        try {
            running.join(Math.max(1, bound.toMillis()));
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
        if (running.isAlive()) {
            running.interrupt();
        }
    }

    @Override
    public void close() {
        close(Duration.ofSeconds(3));
    }

    private void loop() {
        ArrayDeque<List<Queued>> pending = new ArrayDeque<>();
        try {
            while (true) {
                List<Queued> batch = pending.isEmpty() ? next() : pending.removeFirst();
                if (batch.isEmpty()) {
                    return;
                }
                List<List<Queued>> halves = send(batch);
                // A batch the platform found too large is split in two, ahead of everything else.
                for (int i = halves.size() - 1; i >= 0; i--) {
                    pending.addFirst(halves.get(i));
                }
            }
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        } catch (Throwable t) {
            LOG.error("the function trace shipper stopped", t);
        } finally {
            int left = 0;
            for (List<Queued> batch : pending) {
                left += batch.size();
            }
            lock.lock();
            try {
                left += queuedCount;
                queues.clear();
                queuedCount = 0;
                queuedBytes = 0;
            } finally {
                lock.unlock();
            }
            if (left > 0) {
                dropped(left, "the host is stopping");
            }
        }
    }

    /// Waits for a batch to be due and takes it; empty when closed and nothing is left.
    private List<Queued> next() throws InterruptedException {
        lock.lock();
        try {
            while (true) {
                if (queuedCount == 0) {
                    if (closing) {
                        return List.of();
                    }
                    changed.await();
                    continue;
                }
                long oldest = Long.MAX_VALUE;
                for (ArrayDeque<Queued> queue : queues.values()) {
                    oldest = Math.min(oldest, Objects.requireNonNull(queue.peekFirst()).queuedNanos());
                }
                long due = oldest + flushEvery.toNanos() - System.nanoTime();
                if (closing || queuedCount >= flushRecords || due <= 0) {
                    break;
                }
                changed.awaitNanos(due);
            }
            return take();
        } finally {
            lock.unlock();
        }
    }

    /// At most one batch of records, the functions in turn (each function's records stay in order). The lock is held.
    private List<Queued> take() {
        List<Queued> batch = new ArrayList<>();
        long bytes = 0;
        boolean full = false;
        while (!full && queuedCount > 0 && batch.size() < Control.MAX_TRACE_BATCH_RECORDS) {
            for (Iterator<ArrayDeque<Queued>> it = queues.values().iterator(); it.hasNext();) {
                ArrayDeque<Queued> queue = it.next();
                Queued head = Objects.requireNonNull(queue.peekFirst());
                if (batch.size() >= Control.MAX_TRACE_BATCH_RECORDS || (!batch.isEmpty() && bytes + head.bytes() + 1 > BATCH_BYTES)) {
                    full = true;
                    break;
                }
                queue.removeFirst();
                queuedBytes -= head.bytes();
                queuedCount--;
                bytes += head.bytes() + 1;
                batch.add(head);
                if (queue.isEmpty()) {
                    it.remove();
                }
            }
        }
        return batch;
    }

    /// Sends one batch, trying again as the platform's answer allows. Returns the two halves to send instead when the
    /// platform found it too large, else empty.
    private List<List<Queued>> send(List<Queued> batch) throws InterruptedException {
        List<Control.TraceRecord> records = batch.stream().map(Queued::record).toList();
        Duration backoff = firstBackoff;
        boolean authRetried = false;
        String why;
        for (int attempt = 1; ; attempt++) {
            try {
                Control.TraceAck ack = plane.traceLog(new Control.TraceBatch(runnerId, pool, records));
                shippedTotal.addAndGet(ack.accepted());
                if (ack.dropped() > 0) {
                    LOG.debug("the platform dropped {} of {} function trace record(s)", ack.dropped(), records.size());
                }
                return List.of();
            } catch (ControlPlaneException e) {
                if ("INTERRUPTED".equals(e.code())) {
                    throw new InterruptedException();
                }
                failuresTotal.incrementAndGet();
                failureMetric.inc();
                int status = e.status();
                if (status == 413) {
                    if (batch.size() > 1) {
                        int half = batch.size() / 2;
                        return List.of(List.copyOf(batch.subList(0, half)), List.copyOf(batch.subList(half, batch.size())));
                    }
                    dropped(1, "a record is too large for the platform");
                    return List.of();
                }
                if (status == 401 || status == 403) {
                    if (!authRetried) {
                        authRetried = true;
                        // The token was cleared by the call; this one fetches a new one.
                        attempt--;
                        continue;
                    }
                    dropped(records.size(), "the platform refused the host (" + status + ")");
                    return List.of();
                }
                if (status != 0 && status != 408 && status != 429 && status < 500) {
                    dropped(records.size(), "the platform answered " + status);
                    return List.of();
                }
                why = status == 0 ? "the platform could not be reached" : "the platform answered " + status;
            }
            if (attempt >= ATTEMPTS || expired()) {
                dropped(records.size(), why);
                return List.of();
            }
            pause(backoff);
            backoff = backoff.multipliedBy(2).compareTo(maxBackoff) > 0 ? maxBackoff : backoff.multipliedBy(2);
        }
    }

    /// Whether the shipper is closing and its time is up.
    private boolean expired() {
        lock.lock();
        try {
            return closing && System.nanoTime() - closeDeadlineNanos >= 0;
        } finally {
            lock.unlock();
        }
    }

    /// Waits the whole pause, but only a moment once the shipper is closing.
    private void pause(Duration wait) throws InterruptedException {
        boolean stopping;
        lock.lock();
        try {
            long left = wait.toNanos();
            // Records queued meanwhile wake the wait; they must not shorten the pause.
            while (left > 0 && !closing) {
                left = changed.awaitNanos(left);
            }
            stopping = closing;
        } finally {
            lock.unlock();
        }
        if (stopping) {
            TimeUnit.MILLISECONDS.sleep(20);
        }
    }

    private void dropped(int count, @Nullable String why) {
        long total = droppedTotal.addAndGet(count);
        dropMetric.inc(count);
        if (why == null) {
            return;
        }
        long now = System.nanoTime();
        boolean log;
        long since;
        synchronized (this) {
            log = now - lastLogNanos >= LOG_EVERY_NANOS;
            since = total - droppedLogged;
            if (log) {
                lastLogNanos = now;
                droppedLogged = total;
            }
        }
        if (log) {
            LOG.warn("{} function trace record(s) dropped ({} in all): {}", since, total, why);
        }
    }
}

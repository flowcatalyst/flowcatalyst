package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.ingestor.contract.TraceBatch;
import io.flowcatalyst.ingestor.contract.TraceRecord;
import java.io.IOException;
import java.time.Duration;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.locks.Condition;
import java.util.concurrent.locks.ReentrantLock;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import tools.jackson.core.JacksonException;

/**
 * Ships trace records to the platform from a thread of its own. The queue is bounded by count and by
 * bytes; when it is full the oldest record is dropped and counted, so a slow or absent platform costs
 * records, never a poll or an exchange. A batch goes when {@link #FLUSH_RECORDS} are waiting or the
 * oldest has waited {@link #FLUSH_EVERY}. A batch the platform did not take for a reason that may pass
 * (408, 429, 5xx, no answer) is tried again with a growing pause, up to {@link #ATTEMPTS} times; a 401
 * or 403 is tried once more (the platform token was cleared); any other refusal drops the batch,
 * since sending it again would be refused again. Records carry ids, so a batch sent twice is stored
 * once. What is dropped is logged as a count, never as content.
 */
final class TraceShipper implements TraceSink, AutoCloseable {

  private static final Logger LOG = LoggerFactory.getLogger(TraceShipper.class);

  /** The default bound on the queue, in records. */
  static final int DEFAULT_QUEUE = 2000;

  /** The bound on the queue, in bytes of JSON. */
  static final long MAX_QUEUE_BYTES = 16L * 1024 * 1024;

  /** Records waiting that make a batch go at once. */
  static final int FLUSH_RECORDS = 200;

  /** The longest the oldest record waits for a batch to go. */
  static final Duration FLUSH_EVERY = Duration.ofSeconds(2);

  /** Tries a batch gets when the platform does not take it for a reason that may pass. */
  static final int ATTEMPTS = 6;

  /** The longest wait for the platform's answer. */
  private static final Duration CALL_TIMEOUT = Duration.ofSeconds(15);

  /** Under the platform's 1 MiB for a batch: what the envelope takes. */
  private static final int BATCH_BYTES = TraceBatch.MAX_BYTES - 64 * 1024;

  /** A record over this is not sent whole. */
  private static final int MAX_RECORD_BYTES = 512 * 1024;

  private static final long LOG_EVERY_NANOS = Duration.ofMinutes(1).toNanos();

  private record Queued(TraceRecord record, int bytes, long queuedNanos) {}

  private final TraceUplink uplink;
  private final int maxRecords;
  private final long maxBytes;
  private final int flushRecords;
  private final Duration flushEvery;
  private final Duration firstBackoff;
  private final Duration maxBackoff;

  private final ReentrantLock lock = new ReentrantLock();
  private final Condition changed = lock.newCondition();
  private final ArrayDeque<Queued> queue = new ArrayDeque<>();
  private long queuedBytes;
  private boolean closing;
  private long closeDeadlineNanos;

  private final AtomicLong droppedTotal = new AtomicLong();
  private final AtomicLong shippedTotal = new AtomicLong();
  private long droppedLogged;
  private long lastLogNanos = System.nanoTime() - LOG_EVERY_NANOS;
  private @Nullable Thread thread;

  /**
   * Creates the shipper; call {@link #start()}.
   *
   * @param uplink carries a batch to the platform
   * @param maxRecords the most records queued
   */
  TraceShipper(TraceUplink uplink, int maxRecords) {
    this(uplink, maxRecords, MAX_QUEUE_BYTES, FLUSH_RECORDS, FLUSH_EVERY, Duration.ofMillis(500), Duration.ofSeconds(15));
  }

  TraceShipper(
      TraceUplink uplink, int maxRecords, long maxBytes, int flushRecords, Duration flushEvery, Duration firstBackoff, Duration maxBackoff) {
    this.uplink = uplink;
    this.maxRecords = Math.max(1, maxRecords);
    this.maxBytes = maxBytes;
    this.flushRecords = Math.max(1, Math.min(flushRecords, TraceBatch.MAX_RECORDS));
    this.flushEvery = flushEvery;
    this.firstBackoff = firstBackoff;
    this.maxBackoff = maxBackoff;
  }

  /** Starts the thread that ships. */
  void start() {
    lock.lock();
    try {
      if (thread == null) {
        thread = Thread.ofPlatform().daemon().name("ingest-trace-shipper").start(this::loop);
      }
    } finally {
      lock.unlock();
    }
  }

  @Override
  public void offer(TraceRecord record) {
    int bytes;
    TraceRecord kept = record;
    try {
      bytes = Json.mapper().writeValueAsBytes(record).length;
      if (bytes > MAX_RECORD_BYTES) {
        kept =
            new TraceRecord(
                record.id(), record.ingestionId(), record.version(), record.windowId(), record.captureId(), record.at(), record.kind(), record.level(), record.message(),
                Map.of("truncated", true, "bytes", bytes));
        bytes = Json.mapper().writeValueAsBytes(kept).length;
      }
    } catch (JacksonException e) {
      droppedTotal.incrementAndGet();
      return;
    }
    int dropped = 0;
    lock.lock();
    try {
      if (closing) {
        dropped = 1;
      } else {
        queue.addLast(new Queued(kept, bytes, System.nanoTime()));
        queuedBytes += bytes;
        while (queue.size() > maxRecords || queuedBytes > maxBytes) {
          Queued oldest = queue.removeFirst();
          queuedBytes -= oldest.bytes();
          dropped++;
        }
        if (queue.size() >= flushRecords) {
          changed.signalAll();
        } else if (queue.size() == 1) {
          // The thread waits for a first record, then for the flush time.
          changed.signalAll();
        }
      }
    } finally {
      lock.unlock();
    }
    if (dropped > 0) {
      dropped(dropped, "the queue is full");
    }
  }

  /** Records dropped so far: overflow, a batch the platform refused, a batch given up on. */
  long dropped() {
    return droppedTotal.get();
  }

  /** Records the platform took so far. */
  long shipped() {
    return shippedTotal.get();
  }

  /** Records waiting. */
  int queued() {
    lock.lock();
    try {
      return queue.size();
    } finally {
      lock.unlock();
    }
  }

  /** Flushes what is queued, for at most {@code bound}, and stops the thread. */
  void close(Duration bound) {
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
    close(Duration.ofSeconds(2));
  }

  private void loop() {
    try {
      while (true) {
        List<Queued> batch = next();
        if (batch.isEmpty()) {
          return;
        }
        send(batch);
      }
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
    } catch (Throwable t) {
      LOG.error("the trace shipper stopped", t);
    } finally {
      lock.lock();
      try {
        int left = queue.size();
        queue.clear();
        queuedBytes = 0;
        if (left > 0) {
          dropped(left, "the runtime is stopping");
        }
      } finally {
        lock.unlock();
      }
    }
  }

  /** Waits for a batch to be due and takes it; empty when closed and nothing is left. */
  private List<Queued> next() throws InterruptedException {
    lock.lock();
    try {
      while (true) {
        if (queue.isEmpty()) {
          if (closing) {
            return List.of();
          }
          changed.await();
          continue;
        }
        long due = queue.peekFirst().queuedNanos() + flushEvery.toNanos() - System.nanoTime();
        if (closing || queue.size() >= flushRecords || due <= 0) {
          break;
        }
        changed.awaitNanos(due);
      }
      List<Queued> batch = new ArrayList<>();
      long bytes = 0;
      while (!queue.isEmpty() && batch.size() < flushRecords) {
        Queued head = queue.peekFirst();
        if (!batch.isEmpty() && bytes + head.bytes() + 1 > BATCH_BYTES) {
          break;
        }
        queue.removeFirst();
        queuedBytes -= head.bytes();
        bytes += head.bytes() + 1;
        batch.add(head);
      }
      return batch;
    } finally {
      lock.unlock();
    }
  }

  private void send(List<Queued> batch) throws InterruptedException {
    List<TraceRecord> records = batch.stream().map(Queued::record).toList();
    Duration backoff = firstBackoff;
    boolean authRetried = false;
    String why;
    for (int attempt = 1; ; attempt++) {
      try {
        TraceUplink.Result result = uplink.sendTraces(records, CALL_TIMEOUT);
        int status = result.status();
        if (result.taken()) {
          shippedTotal.addAndGet(result.accepted());
          if (result.dropped() > 0) {
            LOG.debug("the platform dropped {} of {} trace record(s)", result.dropped(), records.size());
          }
          return;
        }
        if (status == 401 || status == 403) {
          if (!authRetried) {
            authRetried = true;
            // The token was cleared by the call; this one fetches a new one.
            attempt--;
            continue;
          }
          dropped(records.size(), "the platform refused the runtime (" + status + ")");
          return;
        }
        if (status != 408 && status != 429 && status < 500) {
          dropped(records.size(), "the platform answered " + status);
          return;
        }
        why = "the platform answered " + status;
      } catch (IOException e) {
        why = "the platform could not be reached";
      }
      if (attempt >= ATTEMPTS || expired()) {
        dropped(records.size(), why);
        return;
      }
      pause(backoff);
      backoff = backoff.multipliedBy(2).compareTo(maxBackoff) > 0 ? maxBackoff : backoff.multipliedBy(2);
    }
  }

  /** Whether the shipper is closing and its time is up. */
  private boolean expired() {
    lock.lock();
    try {
      return closing && System.nanoTime() - closeDeadlineNanos >= 0;
    } finally {
      lock.unlock();
    }
  }

  /** Waits the whole pause, but only a moment once the shipper is closing. */
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

  private void dropped(int count, String why) {
    long total = droppedTotal.addAndGet(count);
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
      LOG.warn("{} trace record(s) dropped ({} in all): {}", since, total, why);
    }
  }
}

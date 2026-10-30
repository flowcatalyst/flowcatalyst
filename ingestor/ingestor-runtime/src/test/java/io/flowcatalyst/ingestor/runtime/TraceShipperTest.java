package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.TraceRecord;
import java.io.IOException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** The queue, the batches, and what is tried again, against a platform that answers as a test says. */
@Timeout(60)
class TraceShipperTest {

  /** A platform that answers each call from a list of statuses (the last repeats), or fails to answer. */
  static final class FakeUplink implements TraceUplink {
    final List<List<String>> batches = Collections.synchronizedList(new ArrayList<>());
    final AtomicInteger calls = new AtomicInteger();
    volatile List<Integer> statuses = List.of(202);
    /** How many of the first calls get no answer. */
    volatile int silentCalls;
    volatile CountDownLatch gate = new CountDownLatch(0);

    @Override
    public Result sendTraces(List<TraceRecord> records, Duration timeout) throws IOException, InterruptedException {
      gate.await();
      int call = calls.getAndIncrement();
      if (call < silentCalls) {
        throw new IOException("connection refused");
      }
      int status = statuses.get(Math.min(call, statuses.size() - 1));
      if (status >= 200 && status < 300) {
        batches.add(records.stream().map(TraceRecord::id).toList());
        return new Result(status, records.size(), 0);
      }
      return new Result(status, 0, 0);
    }

    List<String> all() {
      synchronized (batches) {
        return batches.stream().flatMap(List::stream).toList();
      }
    }
  }

  final FakeUplink platform = new FakeUplink();
  TraceShipper shipper;

  private static TraceRecord record(int n) {
    return new TraceRecord("r" + n, "ing_1", 1, "itw_1", "c-1", n, "http", "info", "message " + n, Map.of("n", n));
  }

  private TraceShipper shipper(int maxRecords, int flushRecords, Duration flushEvery) {
    shipper = new TraceShipper(platform, maxRecords, TraceShipper.MAX_QUEUE_BYTES, flushRecords, flushEvery, Duration.ofMillis(1), Duration.ofMillis(5));
    shipper.start();
    return shipper;
  }

  @AfterEach
  void stop() {
    if (shipper != null) {
      shipper.close(Duration.ofSeconds(1));
    }
  }

  @Test
  void aFullBatchGoesAtOnceAndTheRestWaitForTheFlushTime() throws Exception {
    shipper(100, 3, Duration.ofMillis(300));

    for (int i = 1; i <= 4; i++) {
      shipper.offer(record(i));
    }

    await(() -> platform.all().size() == 4);
    assertThat(platform.batches.getFirst()).containsExactly("r1", "r2", "r3");
    assertThat(platform.batches.get(1)).containsExactly("r4");
  }

  @Test
  void aLoneRecordGoesWhenTheFlushTimeHasPassed() throws Exception {
    shipper(100, 200, Duration.ofMillis(100));

    shipper.offer(record(1));

    await(() -> platform.all().size() == 1);
    assertThat(shipper.shipped()).isEqualTo(1);
  }

  @Test
  void aFullQueueDropsTheOldestAndNeverBlocksTheCaller() throws Exception {
    platform.gate = new CountDownLatch(1);
    shipper(3, 1, Duration.ofHours(1));
    shipper.offer(record(1));
    // r1 is taken by the thread and waits on the platform.
    await(() -> shipper.queued() == 0);

    long started = System.nanoTime();
    for (int i = 2; i <= 7; i++) {
      shipper.offer(record(i));
    }
    long tookMs = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started);

    assertThat(tookMs).isLessThan(1000);
    assertThat(shipper.queued()).isEqualTo(3);
    assertThat(shipper.dropped()).isEqualTo(3);
    platform.gate.countDown();
    await(() -> platform.all().size() == 4);
    assertThat(platform.all()).containsExactly("r1", "r5", "r6", "r7");
  }

  @Test
  void theQueueIsAlsoBoundedInBytes() throws Exception {
    platform.gate = new CountDownLatch(1);
    shipper = new TraceShipper(platform, 1000, 2_000, 1, Duration.ofHours(1), Duration.ofMillis(1), Duration.ofMillis(5));
    shipper.start();
    shipper.offer(record(0));
    await(() -> shipper.queued() == 0);

    for (int i = 1; i <= 30; i++) {
      shipper.offer(record(i));
    }

    assertThat(shipper.queued()).isLessThan(30).isGreaterThan(0);
    assertThat(shipper.dropped()).isGreaterThan(0);
    platform.gate.countDown();
  }

  @Test
  void aBatchThePlatformCouldNotTakeIsSentAgainUntilItIsTaken() throws Exception {
    platform.statuses = List.of(503, 429, 408, 202);
    shipper(100, 1, Duration.ofHours(1));

    shipper.offer(record(1));

    await(() -> platform.all().size() == 1);
    assertThat(platform.calls.get()).isEqualTo(4);
    assertThat(shipper.dropped()).isZero();
  }

  @Test
  void noAnswerIsTriedAgainToo() throws Exception {
    platform.silentCalls = 3;
    shipper(100, 1, Duration.ofHours(1));

    shipper.offer(record(1));

    await(() -> platform.all().size() == 1);
    assertThat(platform.calls.get()).isEqualTo(4);
  }

  @Test
  void aBatchIsGivenUpOnAfterTheAttemptsAreUsed() throws Exception {
    platform.statuses = List.of(500);
    shipper(100, 1, Duration.ofHours(1));

    shipper.offer(record(1));

    await(() -> shipper.dropped() == 1);
    assertThat(platform.calls.get()).isEqualTo(TraceShipper.ATTEMPTS);
  }

  @Test
  void aRefusedTokenIsTriedOnceMoreAndThenTheBatchIsDropped() throws Exception {
    platform.statuses = List.of(401, 202);
    shipper(100, 1, Duration.ofHours(1));
    shipper.offer(record(1));
    await(() -> platform.all().size() == 1);
    assertThat(platform.calls.get()).isEqualTo(2);

    platform.calls.set(0);
    platform.statuses = List.of(403);
    shipper.offer(record(2));
    await(() -> shipper.dropped() == 1);
    assertThat(platform.calls.get()).isEqualTo(2);
  }

  @Test
  void anyOtherClientErrorDropsTheBatchAtOnce() throws Exception {
    platform.statuses = List.of(400, 202);
    shipper(100, 1, Duration.ofHours(1));

    shipper.offer(record(1));
    await(() -> shipper.dropped() == 1);
    shipper.offer(record(2));

    await(() -> platform.all().size() == 1);
    assertThat(platform.all()).containsExactly("r2");
    assertThat(platform.calls.get()).isEqualTo(2);
  }

  @Test
  void closingFlushesWhatIsQueuedWithinTheBound() {
    shipper(100, 200, Duration.ofHours(1));
    shipper.offer(record(1));
    shipper.offer(record(2));

    shipper.close(Duration.ofSeconds(5));

    assertThat(platform.all()).containsExactly("r1", "r2");
  }

  @Test
  void aRecordOfferedAfterCloseIsDroppedNotQueued() {
    shipper(100, 200, Duration.ofHours(1));
    shipper.close(Duration.ofSeconds(1));

    shipper.offer(record(1));

    assertThat(shipper.queued()).isZero();
    assertThat(shipper.dropped()).isEqualTo(1);
  }

  @Test
  void aRecordTooBigToSendWholeKeepsItsLineAndSaysItsDataWasCut() throws Exception {
    shipper(100, 1, Duration.ofHours(1));
    var big = new TraceRecord("big", "ing_1", 1, "itw_1", "c-1", 1, "http", "info", "m", Map.of("body", "x".repeat(600 * 1024)));
    List<TraceRecord> seen = Collections.synchronizedList(new ArrayList<>());
    shipper.close(Duration.ofSeconds(1));
    shipper =
        new TraceShipper(
            (records, timeout) -> {
              seen.addAll(records);
              return new TraceUplink.Result(202, records.size(), 0);
            },
            100,
            TraceShipper.MAX_QUEUE_BYTES,
            1,
            Duration.ofHours(1),
            Duration.ofMillis(1),
            Duration.ofMillis(5));
    shipper.start();

    shipper.offer(big);

    await(() -> !seen.isEmpty());
    assertThat(seen.getFirst().data()).containsEntry("truncated", true);
  }
}

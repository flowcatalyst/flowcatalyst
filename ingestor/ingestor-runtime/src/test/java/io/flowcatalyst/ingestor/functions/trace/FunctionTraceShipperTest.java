package io.flowcatalyst.ingestor.functions.trace;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.metrics.Metrics;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.function.BooleanSupplier;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// The queue, the batches and what is tried again, against a platform that answers as a test says.
@Timeout(60)
class FunctionTraceShipperTest {

    final FakeControlPlane platform = new FakeControlPlane();
    /// Every call's record ids, in order, whether the platform took it or not.
    final List<List<String>> calls = Collections.synchronizedList(new ArrayList<>());
    FunctionTraceShipper shipper;

    @AfterEach
    void stop() {
        if (shipper != null) {
            shipper.close(Duration.ofSeconds(1));
        }
    }

    private FunctionTraceShipper shipper(int maxRecords, long maxBytes, int flushRecords, Duration flushEvery) {
        FunctionTraceOptions options = new FunctionTraceOptions(maxRecords, maxBytes, 65536, flushRecords, flushEvery, Duration.ofMillis(1), Duration.ofMillis(5));
        shipper = new FunctionTraceShipper(platform, "runner-1", "default", options, Metrics.noop());
        shipper.start();
        return shipper;
    }

    /// Answers each call from `statuses` (the last repeats): 2xx takes the batch, 0 is no answer, anything else refuses it.
    private void answers(int... statuses) {
        AtomicInteger n = new AtomicInteger();
        platform.traceAnswer = batch -> {
            calls.add(batch.records().stream().map(Control.TraceRecord::id).toList());
            int status = statuses[Math.min(n.getAndIncrement(), statuses.length - 1)];
            if (status >= 200 && status < 300) {
                return new Control.TraceAck(batch.records().size(), 0);
            }
            throw new ControlPlaneException(status, "", "refused", null);
        };
    }

    private static Control.TraceRecord record(String address, int n) {
        return record(address, n, null);
    }

    private static Control.TraceRecord record(String address, int n, Object data) {
        return new Control.TraceRecord(address + "-" + n, n, address, 1, "inv", "", "", "http", "log", "info", "message " + n, data);
    }

    private static void await(BooleanSupplier condition) {
        long end = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (!condition.getAsBoolean()) {
            if (System.nanoTime() > end) {
                throw new AssertionError("timed out waiting");
            }
            try {
                Thread.sleep(2);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new AssertionError(e);
            }
        }
    }

    private List<String> shippedIds() {
        return platform.traceRecords().stream().map(Control.TraceRecord::id).toList();
    }

    @Test
    void aFullBatchGoesAtOnceAndTheRestWaitForTheFlushTime() throws Exception {
        shipper(100, 1L << 30, 3, Duration.ofSeconds(2));

        shipper.offer(record("a.b", 1));
        shipper.offer(record("a.b", 2));
        Thread.sleep(150);
        assertThat(platform.traceCalls.get()).as("two of three, and the flush time is far off").isZero();
        shipper.offer(record("a.b", 3));

        await(() -> shippedIds().size() == 3);
        assertThat(platform.traceBatches.getFirst().records()).extracting(Control.TraceRecord::id).containsExactly("a.b-1", "a.b-2", "a.b-3");
        assertThat(platform.traceBatches).allSatisfy(b -> {
            assertThat(b.runnerId()).isEqualTo("runner-1");
            assertThat(b.pool()).isEqualTo("default");
        });
        shipper.offer(record("a.b", 4));
        await(() -> shippedIds().size() == 4);
        assertThat(platform.traceBatches.get(1).records()).extracting(Control.TraceRecord::id).containsExactly("a.b-4");
    }

    @Test
    void aLoneRecordGoesWhenTheFlushTimeHasPassed() {
        shipper(100, 1L << 30, 200, Duration.ofMillis(100));

        shipper.offer(record("a.b", 1));

        await(() -> shippedIds().size() == 1);
        assertThat(shipper.shipped()).isEqualTo(1);
        assertThat(shipper.dropped()).isZero();
    }

    @Test
    void aBatchTakesTheFunctionsInTurn() {
        shipper(100, 1L << 30, 6, Duration.ofHours(1));

        for (int i = 1; i <= 3; i++) {
            shipper.offer(record("a.one", i));
        }
        for (int i = 1; i <= 3; i++) {
            shipper.offer(record("b.two", i));
        }

        await(() -> shippedIds().size() == 6);
        assertThat(shippedIds()).containsExactly("a.one-1", "b.two-1", "a.one-2", "b.two-2", "a.one-3", "b.two-3");
    }

    @Test
    void aBatchHoldsAtMost500Records() {
        shipper(5000, 1L << 30, 1000, Duration.ofMillis(20));

        for (int i = 1; i <= 1200; i++) {
            shipper.offer(record("a.b", i));
        }

        await(() -> shippedIds().size() == 1200);
        assertThat(platform.traceBatches).allSatisfy(b -> assertThat(b.records().size()).isLessThanOrEqualTo(Control.MAX_TRACE_BATCH_RECORDS));
        assertThat(shippedIds()).hasSize(1200);
    }

    @Test
    void aBatchIsUnderOneMebibyteOfJson() {
        shipper(5000, 1L << 30, 500, Duration.ofMillis(20));

        for (int i = 1; i <= 15; i++) {
            shipper.offer(record("a.b", i, Map.of("blob", "x".repeat(100_000))));
        }

        await(() -> shippedIds().size() == 15);
        assertThat(platform.traceBatches.size()).isGreaterThan(1);
        for (Control.TraceBatch b : platform.traceBatches) {
            assertThat(Json.mapper().writeValueAsBytes(b).length).isLessThan(Control.MAX_TRACE_BATCH_BYTES);
        }
    }

    @Test
    void a413SplitsTheBatchAndALoneRecordStillTooLargeIsDropped() {
        // refuses a batch of more than two records, and any batch holding the record "a.b-3"
        platform.traceAnswer = batch -> {
            calls.add(batch.records().stream().map(Control.TraceRecord::id).toList());
            if (batch.records().size() > 2 || batch.records().stream().anyMatch(r -> r.id().equals("a.b-3"))) {
                throw new ControlPlaneException(413, "", "too large", null);
            }
            return new Control.TraceAck(batch.records().size(), 0);
        };
        shipper(100, 1L << 30, 5, Duration.ofHours(1));

        for (int i = 1; i <= 5; i++) {
            shipper.offer(record("a.b", i));
        }

        await(() -> shipper.shipped() == 4);
        assertThat(shipper.dropped()).isEqualTo(1);
        assertThat(shippedIds()).containsExactly("a.b-1", "a.b-2", "a.b-4", "a.b-5");
        assertThat(calls.getFirst()).hasSize(5);
        assertThat(calls.get(1)).as("the first half").containsExactly("a.b-1", "a.b-2");
    }

    @Test
    void a401IsTriedOnceMoreAtOnceAndA403TwiceDropsTheBatch() {
        answers(401, 202);
        shipper(100, 1L << 30, 1, Duration.ofHours(1));
        shipper.offer(record("a.b", 1));
        await(() -> shipper.shipped() == 1);
        assertThat(calls).hasSize(2);

        answers(403, 403, 202);
        shipper.offer(record("a.b", 2));
        await(() -> shipper.dropped() == 1);
        assertThat(calls).as("two refusals, then given up on").hasSize(4);
        shipper.offer(record("a.b", 3));
        await(() -> shipper.shipped() == 2);
    }

    @Test
    void aServerErrorIsTriedAgainWithTheSameIdsUntilTaken() {
        answers(503, 500, 429, 408, 0, 202);
        shipper(100, 1L << 30, 2, Duration.ofHours(1));

        shipper.offer(record("a.b", 1));
        shipper.offer(record("a.b", 2));

        await(() -> shipper.shipped() == 2);
        assertThat(calls).hasSize(6);
        assertThat(calls).allSatisfy(ids -> assertThat(ids).containsExactly("a.b-1", "a.b-2"));
        assertThat(shipper.dropped()).isZero();
        assertThat(shipper.flushFailures()).isEqualTo(5);
    }

    @Test
    void aBatchThatIsNeverTakenIsDroppedAfterTheAttempts() {
        answers(503);
        shipper(100, 1L << 30, 2, Duration.ofHours(1));

        shipper.offer(record("a.b", 1));
        shipper.offer(record("a.b", 2));

        await(() -> shipper.dropped() == 2);
        assertThat(calls).hasSize(FunctionTraceShipper.ATTEMPTS);
        assertThat(shipper.shipped()).isZero();
    }

    @Test
    void anyOther4xxDropsTheBatchAtOnce() {
        answers(400);
        shipper(100, 1L << 30, 2, Duration.ofHours(1));

        shipper.offer(record("a.b", 1));
        shipper.offer(record("a.b", 2));

        await(() -> shipper.dropped() == 2);
        assertThat(calls).hasSize(1);

        answers(404);
        shipper.offer(record("a.b", 3));
        shipper.offer(record("a.b", 4));
        await(() -> shipper.dropped() == 4);
        assertThat(calls).hasSize(2);
    }

    @Test
    void aFullQueueDropsTheOldestOfTheBusiestFunctionAndNeverBlocksTheCaller() throws Exception {
        CountDownLatch gate = new CountDownLatch(1);
        platform.traceAnswer = batch -> {
            try {
                gate.await();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new ControlPlaneException(0, "INTERRUPTED", "interrupted", e);
            }
            return new Control.TraceAck(batch.records().size(), 0);
        };
        shipper(4, 1L << 30, 1, Duration.ofHours(1));
        shipper.offer(record("c.three", 0));
        // c.three-0 is taken by the thread and waits on the platform
        await(() -> shipper.queued() == 0);

        long started = System.nanoTime();
        shipper.offer(record("a.one", 1));
        shipper.offer(record("a.one", 2));
        shipper.offer(record("a.one", 3));
        shipper.offer(record("b.two", 1));
        shipper.offer(record("a.one", 4)); // full: the oldest of a.one (3 queued against 1) goes
        shipper.offer(record("b.two", 2)); // full: a.one has 3 against 2, so a.one-2 goes
        long tookMs = TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started);

        assertThat(tookMs).isLessThan(1000);
        assertThat(shipper.queued()).isEqualTo(4);
        assertThat(shipper.dropped()).isEqualTo(2);
        gate.countDown();
        await(() -> shipper.shipped() == 5);
        assertThat(shippedIds()).containsExactlyInAnyOrder("c.three-0", "a.one-3", "a.one-4", "b.two-1", "b.two-2");
    }

    @Test
    void theQueueIsAlsoBoundedInBytes() throws Exception {
        CountDownLatch gate = new CountDownLatch(1);
        platform.traceAnswer = batch -> {
            try {
                gate.await();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new ControlPlaneException(0, "INTERRUPTED", "interrupted", e);
            }
            return new Control.TraceAck(batch.records().size(), 0);
        };
        shipper(1000, 3000, 1, Duration.ofHours(1));
        shipper.offer(record("a.b", 0));
        await(() -> shipper.queued() == 0);

        for (int i = 1; i <= 10; i++) {
            shipper.offer(record("a.b", i, Map.of("blob", "x".repeat(900))));
        }

        assertThat(shipper.queued()).isLessThan(10).isGreaterThan(0);
        assertThat(shipper.dropped()).isEqualTo(10 - shipper.queued());
        gate.countDown();
    }

    @Test
    void closingFlushesWhatIsQueuedWithinTheBound() {
        shipper(100, 1L << 30, 200, Duration.ofHours(1));
        for (int i = 1; i <= 5; i++) {
            shipper.offer(record("a.b", i));
        }

        shipper.close(Duration.ofSeconds(2));

        assertThat(shippedIds()).containsExactly("a.b-1", "a.b-2", "a.b-3", "a.b-4", "a.b-5");
        shipper.offer(record("a.b", 6));
        assertThat(shipper.queued()).as("nothing is taken once closed").isZero();
        assertThat(shipper.dropped()).isEqualTo(1);
    }

    @Test
    void aPlatformThatIsDownIsGivenUpOnWhenClosingWithinTheBound() {
        answers(503);
        shipper(100, 1L << 30, 200, Duration.ofHours(1));
        shipper.offer(record("a.b", 1));

        long started = System.nanoTime();
        shipper.close(Duration.ofMillis(300));

        assertThat(TimeUnit.NANOSECONDS.toMillis(System.nanoTime() - started)).isLessThan(3000);
        assertThat(shipper.shipped()).isZero();
    }
}

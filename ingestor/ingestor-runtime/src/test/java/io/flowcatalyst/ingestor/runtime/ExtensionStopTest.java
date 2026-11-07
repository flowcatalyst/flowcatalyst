package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.extension.PolledSource;
import io.flowcatalyst.ingestor.extension.Settings;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.Consumer;
import org.apache.camel.impl.DefaultCamelContext;
import org.apache.camel.support.service.ServiceHelper;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/**
 * Stopping an extension's polled source: the runtime sees its poll in progress, suspends it without
 * interrupting it, and the extension learns of the stop at its next dispatch ({@code
 * docs/ingestor-gap-fixes.md}, the extension API follow-up).
 */
@Timeout(60)
class ExtensionStopTest {

  /** What the extension saw, in order. */
  final List<String> seen = new CopyOnWriteArrayList<>();
  /** The payloads whose jobs were made. */
  final List<String> taken = new CopyOnWriteArrayList<>();
  /** What the extension's failure log wrote. */
  final List<String> lines = new CopyOnWriteArrayList<>();
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  final SourceHealth health = new SourceHealth();
  CamelContext camel;
  Reconciler reconciler;
  final Ingestion ingestion = SourceHarness.ingestion("ing_x", Source.Custom.of("ticker", Map.of()), new Split.None());

  /**
   * A source whose first poll waits, before its first dispatch, until let go (recording an interrupt,
   * and going on waiting); every poll then records what it sees, dispatches one payload and, when the
   * payload was not taken, reports the part that failed, as an extension does, and, told it is
   * stopping, throws, as some extensions do.
   */
  final class Ticker extends PolledSource {
    final CountDownLatch entered = new CountDownLatch(1);
    final CountDownLatch go = new CountDownLatch(1);
    final AtomicInteger polls = new AtomicInteger();
    /** How soon each poll asks to be polled again. */
    volatile Duration every = Duration.ofHours(1);
    /** How long each poll after the first takes before it dispatches. */
    volatile Duration takes = Duration.ZERO;
    /** Whether a poll told it is stopping throws, as some extensions do, or returns. */
    volatile boolean throwWhenStopping = true;
    /** Whether the first poll has, by the time it is let go, read a page its source will not serve again. */
    volatile boolean destructive;

    Ticker(ExtensionContext context) {
      super(context, "ticker");
    }

    @Override
    protected Duration poll(Payloads payloads) throws Exception {
      int n = polls.incrementAndGet();
      if (n == 1) {
        entered.countDown();
        while (true) {
          try {
            go.await();
            break;
          } catch (InterruptedException e) {
            seen.add("interrupted");
          }
        }
        if (destructive) {
          // The page was read (and is gone from the source) before the stop came: it is dispatched
          // all the same; anything more is not read, and a plain dispatch is refused.
          seen.add("taken=" + payloads.dispatchTaken("page 1", "queue:page-1") + " stopping=" + payloads.stopping());
          seen.add("dispatch=" + payloads.dispatch("more"));
          return every;
        }
      }
      if (n > 1 && !takes.isZero()) {
        Thread.sleep(takes.toMillis());
      }
      seen.add("poll " + n + " stopping=" + payloads.stopping());
      boolean took = payloads.dispatch("p" + n);
      seen.add("poll " + n + " dispatch=" + took);
      if (!took) {
        partFailed("the platform did not take p" + n);
        if (throwWhenStopping && payloads.stopping()) {
          // Some extensions end a stopped poll by throwing.
          throw new IllegalStateException("stopped before p" + n);
        }
      }
      return every;
    }

    @Override
    protected Duration retryDelay() {
      return Duration.ofMillis(10);
    }
  }

  Ticker source;

  @BeforeEach
  void start() {
    camel = new DefaultCamelContext();
    camel.start();
    source = new Ticker(context(camel, ingestion));
    source.failureLog(new FailureLog((m, c) -> lines.add(m), Duration.ofMinutes(1), InstantSource.system()));
    reconciler =
        new Reconciler(
            camel,
            i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> jobs.stream().map(JobRecord::payload).forEach(taken::add), counters, health),
            health);
    reconciler.reconcile(List.of(ingestion));
  }

  @AfterEach
  void stop() {
    source.go.countDown();
    camel.stop();
  }

  static ExtensionContext context(CamelContext camel, Ingestion ingestion) {
    return new ExtensionContext() {
      @Override
      public CamelContext camel() {
        return camel;
      }

      @Override
      public Ingestion ingestion() {
        return ingestion;
      }

      @Override
      public Settings settings() {
        throw new UnsupportedOperationException();
      }

      @Override
      public ServiceClients http() {
        throw new UnsupportedOperationException();
      }

      @Override
      public String clientName(String prefix) {
        return prefix;
      }

      @Override
      public State state() {
        throw new UnsupportedOperationException();
      }
    };
  }

  private Consumer consumer() {
    return camel.getRoute(ingestion.id()).getConsumer();
  }

  /** Nothing was reported: no failure counted, the ingestion is not FAILING, nothing logged. */
  private void nothingReported() {
    assertThat(counters.drain(InstantSource.system().instant()).failures()).isEmpty();
    assertThat(health.problem(ingestion.id(), ingestion.version())).isNull();
    assertThat(lines).isEmpty();
  }

  @Test
  void aSuspendedSourceEndsItsPollAtItsNextDispatchWithoutAFailureAndPollsAgainWhenResumed() throws Exception {
    assertThat(source.entered.await(10, TimeUnit.SECONDS)).isTrue();
    assertThat(source.polling()).isTrue();

    // As the watchdog does: without the Camel context's lock.
    ServiceHelper.suspendService(consumer());
    assertThat(ServiceHelper.isSuspended(consumer())).isTrue();
    source.go.countDown();
    await(() -> !source.polling());

    // Told it is stopping; its payload did not reach the route; the poll ended there, and nothing
    // of it is a failure: not reported, not logged, no backoff.
    assertThat(seen).containsExactly("poll 1 stopping=true", "poll 1 dispatch=false");
    assertThat(taken).isEmpty();
    nothingReported();
    // No further poll while suspended.
    Thread.sleep(1_500);
    assertThat(source.polls.get()).isEqualTo(1);

    ServiceHelper.resumeService(consumer());
    await(() -> taken.contains("p2"));
    assertThat(seen).containsExactly("poll 1 stopping=true", "poll 1 dispatch=false", "poll 2 stopping=false", "poll 2 dispatch=true");
    nothingReported();
  }

  @Test
  void aSourceWhosePollIsInProgressIsNotStoppedCleanlyUntilThePollEnds() throws Exception {
    assertThat(source.entered.await(10, TimeUnit.SECONDS)).isTrue();

    CompletableFuture<Reconciler.Stopped> stopping = new CompletableFuture<>();
    Thread.ofPlatform().start(() -> stopping.complete(reconciler.stop(i -> true, Duration.ofSeconds(12))));
    Thread.sleep(1_000);
    // The poll is still running: the stop waits for it rather than cutting it off.
    assertThat(stopping).isNotDone();
    assertThat(seen).isEmpty();

    source.go.countDown();
    Reconciler.Stopped stopped = stopping.get(10, TimeUnit.SECONDS);

    assertThat(stopped.clean()).extracting(Ingestion::id).containsExactly("ing_x");
    assertThat(seen).containsExactly("poll 1 stopping=true", "poll 1 dispatch=false");
    assertThat(taken).isEmpty();
    nothingReported();
  }

  @Test
  void aPollStuckPastTheStopsBoundIsInterruptedAndNotCountedClean() throws Exception {
    assertThat(source.entered.await(10, TimeUnit.SECONDS)).isTrue();

    long began = System.nanoTime();
    Reconciler.Stopped stopped = reconciler.stop(i -> true, Duration.ofSeconds(4));

    assertThat(Duration.ofNanos(System.nanoTime() - began)).isLessThan(Duration.ofMillis(4_500));
    assertThat(stopped.count()).isEqualTo(1);
    // It was waited for, then interrupted; it took no notice and is still running.
    assertThat(stopped.clean()).isEmpty();
    await(() -> !seen.isEmpty());
    assertThat(seen).containsExactly("interrupted");
    assertThat(source.polling()).isTrue();
  }

  @Test
  void aStoppedSourceGivesThePollInProgressAMomentToEndBeforeInterruptingIt() throws Exception {
    assertThat(source.entered.await(10, TimeUnit.SECONDS)).isTrue();

    // Stopped directly, as Camel's own close does, while the poll is about to reach its dispatch.
    Thread stopping = Thread.ofPlatform().start(() -> ServiceHelper.stopService(consumer()));
    Thread.sleep(100);
    source.go.countDown();
    stopping.join(5_000);

    assertThat(stopping.isAlive()).isFalse();
    assertThat(ServiceHelper.isStopped(consumer())).isTrue();
    assertThat(seen).containsExactly("poll 1 stopping=true", "poll 1 dispatch=false");
    assertThat(source.polling()).isFalse();
    assertThat(taken).isEmpty();
    nothingReported();
  }
  @Test
  void aSourceSuspendedAndResumedPollsOnOneScheduleWhetherItWasSuspendedBetweenPollsOrDuringOne() throws Exception {
    source.every = Duration.ofMillis(600);
    source.takes = Duration.ofMillis(300);
    source.throwWhenStopping = false;
    source.go.countDown();
    await(() -> source.polls.get() >= 2);

    // Between two polls: the poll scheduled is not left to run besides the one the resume schedules.
    await(() -> !source.polling());
    ServiceHelper.suspendService(consumer());
    ServiceHelper.resumeService(consumer());
    // During a poll: it ends without scheduling the next, which the resume does.
    await(() -> source.polling());
    ServiceHelper.suspendService(consumer());
    await(() -> !source.polling());
    ServiceHelper.resumeService(consumer());

    int before = source.polls.get();
    Thread.sleep(2_700);
    // One schedule: a poll every 300 ms + 600 ms, so three in 2.7 s; two schedules would make six.
    assertThat(source.polls.get() - before).isBetween(2, 4);
  }
  @Test
  void aPageReadBeforeTheStopCameIsDispatchedWithDispatchTakenAndTheSourceIsThenStoppedCleanly() throws Exception {
    source.destructive = true;
    assertThat(source.entered.await(10, TimeUnit.SECONDS)).isTrue();

    CompletableFuture<Reconciler.Stopped> stopping = new CompletableFuture<>();
    Thread.ofPlatform().start(() -> stopping.complete(reconciler.stop(i -> true, Duration.ofSeconds(12))));
    // The read returns while the source is already suspended.
    await(() -> !ServiceHelper.isStarted(consumer()) || ServiceHelper.isSuspended(consumer()));
    source.go.countDown();
    Reconciler.Stopped stopped = stopping.get(10, TimeUnit.SECONDS);

    // Its jobs were made though the source was stopping; a plain dispatch was still refused.
    assertThat(seen).containsExactly("taken=true stopping=true", "dispatch=false");
    assertThat(taken).containsExactly("page 1");
    // The stop waited for it, and counts the source as stopped cleanly.
    assertThat(stopped.clean()).extracting(Ingestion::id).containsExactly("ing_x");
    nothingReported();
  }
}

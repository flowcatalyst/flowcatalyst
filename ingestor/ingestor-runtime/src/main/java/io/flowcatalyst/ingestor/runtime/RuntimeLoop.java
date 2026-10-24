package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.RuntimeReport.ExchangeWindow;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.TestRequest;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.function.Function;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * One round per interval, run as a framework background loop: ask the platform what to run,
 * reconcile the routes, run the connection tests it hands over, and report route states, exchange
 * counts, failures and test answers. When the platform cannot be reached the shared routes keep
 * running and what was counted is kept for the next report.
 *
 * <p>A single-owner ingestion runs under a lease. The platform gives the ingestion to another
 * runtime once the lease has run out, so this runtime stops it first: one interval before the
 * lease, counted on its own monotonic clock from when it sent the poll, would end.
 */
final class RuntimeLoop {

  private static final Logger LOG = LoggerFactory.getLogger(RuntimeLoop.class);

  /** Unsent windows kept across failed reports; beyond this the oldest are dropped. */
  private static final int MAX_UNSENT_WINDOWS = 10_000;

  /** Unsent failures kept across failed reports; beyond this the oldest are dropped. */
  private static final int MAX_UNSENT_FAILURES = 1_000;

  private final String runtimeId;
  private final Duration interval;
  private final RuntimeApi platform;
  private final Reconciler reconciler;
  private final ExchangeCounters counters;
  private final Function<TestRequest, ConnectionTest> tester;
  private final InstantSource clock;
  private final MonotonicClock monotonic;

  /** The monotonic reading after which the single-owner ingestions must not be running. */
  private long leaseEndsNanos;

  private final List<ExchangeWindow> unsent = new ArrayList<>();
  private final List<Failure> unsentFailures = new ArrayList<>();
  private final List<ConnectionTest> unsentTests = new ArrayList<>();
  private volatile @Nullable Instant lastPoll;

  RuntimeLoop(
      String runtimeId,
      Duration interval,
      RuntimeApi platform,
      Reconciler reconciler,
      ExchangeCounters counters,
      Function<TestRequest, ConnectionTest> tester,
      InstantSource clock,
      MonotonicClock monotonic) {
    this.runtimeId = runtimeId;
    this.interval = interval;
    this.platform = platform;
    this.reconciler = reconciler;
    this.counters = counters;
    this.tester = tester;
    this.clock = clock;
    this.monotonic = monotonic;
    this.leaseEndsNanos = monotonic.nanos();
  }

  /**
   * One round: poll, reconcile, test, report.
   *
   * @throws InterruptedException when interrupted while calling the platform
   */
  void tick() throws InterruptedException {
    long asked = monotonic.nanos();
    try {
      PollAnswer answer = platform.poll();
      leaseEndsNanos = asked + answer.leaseFor().toNanos();
      List<Ingestion> desired = answer.ingestions();
      if (leaseEnding()) {
        // The answer took most of the lease to arrive, or the lease is shorter than two intervals.
        LOG.warn("the lease of {} cannot cover the next interval; not running single-owner ingestions", answer.leaseFor());
        desired = desired.stream().filter(i -> !i.source().singleOwner()).toList();
      }
      reconciler.reconcile(desired);
      lastPoll = clock.instant();
      test(answer.testList());
    } catch (IOException e) {
      LOG.warn("could not ask the platform what to run; keeping the shared routes: {}", e.getMessage());
      if (leaseEnding()) {
        int stopped = reconciler.stop(i -> i.source().singleOwner());
        if (stopped > 0) {
          LOG.warn("stopped {} single-owner ingestion(s): their lease could not be renewed", stopped);
        }
      }
    }
    report();
  }

  /** Runs the tests the platform handed over, none that has run out of time; the answers go in the report. */
  private void test(List<TestRequest> requests) {
    for (TestRequest request : requests) {
      if (request.expiresAt().isBefore(clock.instant())) {
        LOG.info("test {} expired before it was run", request.id());
        continue;
      }
      LOG.info("testing the source of ingestion {} of client {} for the platform", request.ingestion().code(), request.ingestion().clientIdentifier());
      unsentTests.add(tester.apply(request));
    }
  }

  private void report() throws InterruptedException {
    Instant now = clock.instant();
    ExchangeCounters.Drained drained = counters.drain(now);
    unsent.addAll(drained.windows());
    unsentFailures.addAll(drained.failures());
    // What the platform takes in one report; the rest goes in the next rounds'.
    List<ExchangeWindow> windows = List.copyOf(unsent.subList(0, Math.min(unsent.size(), RuntimeReport.MAX_WINDOWS)));
    List<Failure> failures = List.copyOf(unsentFailures.subList(0, Math.min(unsentFailures.size(), RuntimeReport.MAX_FAILURES)));
    List<ConnectionTest> tests = List.copyOf(unsentTests.subList(0, Math.min(unsentTests.size(), RuntimeReport.MAX_TESTS)));
    List<RouteState> routes = reconciler.states();
    try {
      platform.report(
          new RuntimeReport(runtimeId, routes.subList(0, Math.min(routes.size(), RuntimeReport.MAX_ROUTES)), windows, failures, tests));
      unsent.subList(0, windows.size()).clear();
      unsentFailures.subList(0, failures.size()).clear();
      unsentTests.subList(0, tests.size()).clear();
    } catch (RejectedException e) {
      // Sent again it would be refused again, and nothing after it would ever be reported.
      LOG.error("the platform will not take this report, so it is dropped ({} window(s), {} failure(s), {} test answer(s)): {}", windows.size(), failures.size(), tests.size(), e.getMessage());
      unsent.subList(0, windows.size()).clear();
      unsentFailures.subList(0, failures.size()).clear();
      unsentTests.subList(0, tests.size()).clear();
    } catch (IOException e) {
      LOG.warn("could not send the report; {} window(s) kept for the next one: {}", unsent.size(), e.getMessage());
      if (unsent.size() > MAX_UNSENT_WINDOWS) {
        unsent.subList(0, unsent.size() - MAX_UNSENT_WINDOWS).clear();
      }
      if (unsentFailures.size() > MAX_UNSENT_FAILURES) {
        unsentFailures.subList(0, unsentFailures.size() - MAX_UNSENT_FAILURES).clear();
      }
    }
  }

  /** Whether the lease ends before the round after this one could renew it. */
  private boolean leaseEnding() {
    return monotonic.nanos() + interval.toNanos() >= leaseEndsNanos;
  }

  /** Fails when the platform has not answered a poll within three intervals. */
  void requireRecentPoll() {
    Instant last = lastPoll;
    if (last == null) {
      throw new IllegalStateException("the platform has not answered yet");
    }
    if (last.plus(interval.multipliedBy(3)).isBefore(clock.instant())) {
      throw new IllegalStateException("the platform last answered at " + last);
    }
  }
}

package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.LeaseRelease;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.RuntimeReport.ExchangeWindow;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.TestRequest;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Queue;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.ConcurrentLinkedQueue;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.FutureTask;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.function.Function;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * One round per interval, run as a framework background loop: ask the platform what to run,
 * reconcile the routes, start the connection tests it hands over, re-read the running routes'
 * secrets when it is time, and report route states, exchange counts, failures and test answers.
 * When the platform cannot be reached the shared routes keep running and what was counted is kept
 * for the next report. A round that throws anything is logged and the next one runs; running out of
 * memory ends the process.
 *
 * <p>A single-owner ingestion runs under a lease. The platform gives the ingestion to another
 * runtime once the lease has run out, so this runtime stops it first: the round stops it when the
 * lease would end before the next round could renew it, and a watchdog ({@link #watch()}), run every
 * second apart from the rounds, stops it once the lease is within {@link #margin()} of its end, so a
 * round stuck on a slow platform, or on a route that does not start, cannot keep it running: the
 * watchdog never waits for the reconciler ({@link Reconciler#revoke}). The lease is counted on this
 * runtime's own monotonic clock from when it sent the poll; the poll and the report wait at most a
 * sixth of the lease for an answer.
 *
 * <p>A lease is given back ({@link RuntimeApi#release}) only for a route that has actually stopped:
 * after a round stopped single-owner routes the platform no longer lists, and at shutdown ({@link
 * #goodbye}). A route the watchdog or a failed poll stopped is left to its lease. When the platform
 * refuses the poll because another live process holds this runtime's id, the leases are not renewed,
 * exactly as when it cannot be reached, no report is sent (what is counted is kept, within the
 * usual caps, for when this process takes over), readiness fails, and the polls go on.
 */
final class RuntimeLoop {

  private static final Logger LOG = LoggerFactory.getLogger(RuntimeLoop.class);

  /** Unsent windows kept across failed reports; beyond this the oldest are dropped. */
  private static final int MAX_UNSENT_WINDOWS = 10_000;

  /** Unsent failures kept across failed reports; beyond this the oldest are dropped. */
  private static final int MAX_UNSENT_FAILURES = 1_000;

  /** Unsent test answers kept across failed reports; beyond this the oldest are dropped. */
  static final int MAX_UNSENT_TESTS = 100;

  /** Reports that failed in a row, with the platform answering polls, after which the failures and test answers of the next are dropped. */
  static final int WEDGED_REPORTS = 5;

  /** Below the platform's limit: what the report's own fields and the encoding's rounding take. */
  private static final int REPORT_HEADROOM = 64 * 1024;

  /** The least margin the watchdog leaves before the lease ends. */
  static final Duration LEASE_MARGIN = Duration.ofSeconds(30);

  /** The longest a test may run. */
  static final Duration MAX_TEST = Duration.ofMinutes(2);

  /** The longest a round waits for the tests it started before it reports; later ones go in a later report. */
  static final Duration TEST_WAIT = Duration.ofSeconds(10);

  /** The longest the single-owner routes are given to stop when the lease runs out. */
  static final Duration STOP_WITHIN = Duration.ofSeconds(20);

  /** The longest the lease release at shutdown may wait for the platform. */
  static final Duration GOODBYE_WITHIN = Duration.ofSeconds(5);

  /** How much longer than its bound the goodbye's call is waited for, to give up. */
  static final Duration GOODBYE_GRACE = Duration.ofSeconds(1);

  private record Running(Future<?> future, long deadlineNanos, AtomicBoolean answered) {}

  private final String runtimeId;
  private final Duration interval;
  private final RuntimeApi platform;
  private final Reconciler reconciler;
  private final ExchangeCounters counters;
  private final Function<TestRequest, ConnectionTest> tester;
  private final InstantSource clock;
  private final MonotonicClock monotonic;
  private final Duration platformTimeout;
  private final @Nullable Secrets secrets;
  private final Duration secretsEvery;
  private final TraceWindows traces;
  private final ExecutorService testers = Executors.newThreadPerTaskExecutor(Thread.ofVirtual().name("ingest-test-", 0).factory());

  /** The monotonic reading after which the single-owner ingestions must not be running. */
  private volatile long leaseEndsNanos;

  /** The lease of the last answer, or null before the first. */
  private volatile @Nullable Duration lease;

  /** What the platform said when it last refused a poll because another live process holds this runtime's id; null while it takes them. */
  private volatile @Nullable String idInUse;

  private long nextSecretsCheck;
  private int failedReports;

  private final List<ExchangeWindow> unsent = new ArrayList<>();
  private final List<Failure> unsentFailures = new ArrayList<>();
  private final List<ConnectionTest> unsentTests = new ArrayList<>();
  private final Queue<ConnectionTest> answered = new ConcurrentLinkedQueue<>();
  private final Map<String, Running> testing = new ConcurrentHashMap<>();
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
    this(runtimeId, interval, platform, reconciler, counters, tester, clock, monotonic, Duration.ofSeconds(30), null, Duration.ZERO);
  }

  /**
   * Creates the loop.
   *
   * @param runtimeId names this runtime
   * @param interval how often a round runs
   * @param platform the runtime API
   * @param reconciler the routes
   * @param counters the exchange counts and failures
   * @param tester runs a connection test
   * @param clock the time
   * @param monotonic the clock leases are counted on
   * @param platformTimeout the longest a call to the platform may wait; the poll and report wait less when the lease is short
   * @param secrets the secrets the routes were built with, re-read every {@code secretsEvery}; null not to
   * @param secretsEvery how often they are re-read (no more often than a round)
   */
  RuntimeLoop(
      String runtimeId,
      Duration interval,
      RuntimeApi platform,
      Reconciler reconciler,
      ExchangeCounters counters,
      Function<TestRequest, ConnectionTest> tester,
      InstantSource clock,
      MonotonicClock monotonic,
      Duration platformTimeout,
      @Nullable Secrets secrets,
      Duration secretsEvery) {
    this(runtimeId, interval, platform, reconciler, counters, tester, clock, monotonic, platformTimeout, secrets, secretsEvery, TraceWindows.disabled());
  }

  /**
   * Creates the loop.
   *
   * @param runtimeId names this runtime
   * @param interval how often a round runs
   * @param platform the runtime API
   * @param reconciler the routes
   * @param counters the exchange counts and failures
   * @param tester runs a connection test
   * @param clock the time
   * @param monotonic the clock leases are counted on
   * @param platformTimeout the longest a call to the platform may wait; the poll and report wait less when the lease is short
   * @param secrets the secrets the routes were built with, re-read every {@code secretsEvery}; null not to
   * @param secretsEvery how often they are re-read (no more often than a round)
   * @param traces the trace windows each poll answer updates
   */
  RuntimeLoop(
      String runtimeId,
      Duration interval,
      RuntimeApi platform,
      Reconciler reconciler,
      ExchangeCounters counters,
      Function<TestRequest, ConnectionTest> tester,
      InstantSource clock,
      MonotonicClock monotonic,
      Duration platformTimeout,
      @Nullable Secrets secrets,
      Duration secretsEvery,
      TraceWindows traces) {
    this.traces = traces;
    this.runtimeId = runtimeId;
    this.interval = interval;
    this.platform = platform;
    this.reconciler = reconciler;
    this.counters = counters;
    this.tester = tester;
    this.clock = clock;
    this.monotonic = monotonic;
    this.platformTimeout = platformTimeout;
    this.secrets = secrets;
    this.secretsEvery = secretsEvery;
    this.leaseEndsNanos = monotonic.nanos();
    this.nextSecretsCheck = monotonic.nanos() + secretsEvery.toNanos();
  }

  /**
   * One round: poll, reconcile, test, re-read secrets, report. Anything a round throws but an
   * interruption is logged and the next round runs as usual.
   *
   * @throws InterruptedException when interrupted while calling the platform
   */
  void tick() throws InterruptedException {
    try {
      round();
    } catch (InterruptedException e) {
      throw e;
    } catch (OutOfMemoryError e) {
      Fatal.outOfMemory("the runtime loop", e);
    } catch (Throwable t) {
      // The framework ends a loop on an Error; this one must keep renewing leases and reporting.
      LOG.error("a round of the runtime loop failed; the next one runs as usual", t);
    }
  }

  private void round() throws InterruptedException {
    long asked = monotonic.nanos();
    boolean answeredPoll = false;
    try {
      RuntimeApi.Polled polled = platform.poll(callTimeout());
      PollAnswer answer = polled.answer();
      lease = answer.leaseFor();
      leaseEndsNanos = asked + answer.leaseFor().toNanos();
      idInUse = null;
      List<Ingestion> desired = answer.ingestions();
      List<RuntimeApi.Unreadable> notRun = polled.unreadable();
      if (leaseEnding()) {
        // The answer took most of the lease to arrive, or the lease is shorter than two intervals.
        LOG.warn("the lease of {} cannot cover the next interval; not running single-owner ingestions", answer.leaseFor());
        String reason = "the lease (" + answer.leaseFor() + ") is too short for the poll interval (" + interval + ")";
        List<RuntimeApi.Unreadable> refused = new ArrayList<>(notRun);
        for (Ingestion ingestion : desired) {
          if (ingestion.source().singleOwner()) {
            refused.add(new RuntimeApi.Unreadable(ingestion.id(), ingestion.version(), reason));
          }
        }
        notRun = refused;
        desired = desired.stream().filter(i -> !i.source().singleOwner()).toList();
      }
      // Tracing is never worth failing a round for.
      try {
        traces.update(answer.traceList());
      } catch (RuntimeException e) {
        LOG.warn("the trace windows of the platform's answer could not be taken: {}", e.getMessage());
      }
      List<Ingestion> stopped = reconciler.reconcile(desired, notRun);
      lastPoll = clock.instant();
      answeredPoll = true;
      giveBack(unlisted(stopped, polled), false, callTimeout());
      answered.addAll(polled.unreadableTests());
      test(answer.testList());
    } catch (RuntimeIdInUseException e) {
      idInUse = e.getMessage();
      LOG.error(
          "the platform refuses this process's polls: runtime id {} is in use by another live process; its leases are not renewed, no report is sent, the shared routes keep running, and the polls go on ({})",
          runtimeId,
          e.getMessage());
      stopIfLeaseEnding();
    } catch (IOException e) {
      LOG.warn("could not ask the platform what to run; keeping the shared routes: {}", e.getMessage());
      stopIfLeaseEnding();
    }
    secrets();
    if (idInUse != null) {
      // Another live process reports under this runtime id; this one's route states would overwrite
      // the owner's. What was counted is kept for when this process takes over.
      keepUnsent();
      return;
    }
    report(answeredPoll);
  }

  /** Takes what was counted and answered into the unsent lists, within their caps, without sending a report. */
  private void keepUnsent() {
    ExchangeCounters.Drained drained = counters.drain(clock.instant());
    unsent.addAll(drained.windows());
    unsentFailures.addAll(drained.failures());
    overdueTests();
    for (ConnectionTest test = answered.poll(); test != null; test = answered.poll()) {
      unsentTests.add(test);
    }
    capUnsent();
  }

  private void capUnsent() {
    if (unsentTests.size() > MAX_UNSENT_TESTS) {
      unsentTests.subList(0, unsentTests.size() - MAX_UNSENT_TESTS).clear();
    }
    if (unsent.size() > MAX_UNSENT_WINDOWS) {
      unsent.subList(0, unsent.size() - MAX_UNSENT_WINDOWS).clear();
    }
    if (unsentFailures.size() > MAX_UNSENT_FAILURES) {
      unsentFailures.subList(0, unsentFailures.size() - MAX_UNSENT_FAILURES).clear();
    }
  }

  /** A poll that was not answered: the single-owner routes are stopped when the lease would end before the next round. Their leases are not given back. */
  private void stopIfLeaseEnding() {
    if (leaseEnding()) {
      int stopped = reconciler.stop(i -> i.source().singleOwner(), STOP_WITHIN).count();
      if (stopped > 0) {
        LOG.warn("stopped {} single-owner ingestion(s): their lease could not be renewed", stopped);
      }
    }
  }

  /** The single-owner ingestions among those stopped that the platform's answer no longer lists, in any form. */
  private static List<String> unlisted(List<Ingestion> stopped, RuntimeApi.Polled polled) {
    if (stopped.isEmpty()) {
      return List.of();
    }
    Set<String> listed = new HashSet<>();
    polled.answer().ingestions().forEach(i -> listed.add(i.id()));
    polled.unreadable().forEach(u -> listed.add(u.ingestionId()));
    return stopped.stream().filter(i -> i.source().singleOwner() && !listed.contains(i.id())).map(Ingestion::id).toList();
  }

  /**
   * Gives back the leases of single-owner ingestions whose routes have stopped, in calls of at most
   * {@link LeaseRelease#MAX_IDS}, all within {@code within}; with {@code goodbye}, one call is made
   * even when there are none. A release that fails is logged and not tried again: the leases run out.
   */
  private void giveBack(List<String> ids, boolean goodbye, Duration within) throws InterruptedException {
    if (ids.isEmpty() && !goodbye) {
      return;
    }
    long deadline = monotonic.nanos() + within.toNanos();
    int from = 0;
    do {
      int to = Math.min(ids.size(), from + LeaseRelease.MAX_IDS);
      boolean last = to == ids.size();
      long left = deadline - monotonic.nanos();
      if (left <= 0) {
        LOG.warn("no time left to give back {} lease(s); they run out", ids.size() - from);
        return;
      }
      try {
        int released = platform.release(ids.subList(from, to), goodbye && last, Duration.ofNanos(left));
        if (released > 0) {
          LOG.info("gave back {} lease(s) of stopped single-owner ingestion(s)", released);
        }
      } catch (IOException e) {
        LOG.warn("could not give back {} lease(s); they run out instead: {}", to - from, e.getMessage());
      }
      from = to;
    } while (from < ids.size());
  }

  /**
   * At shutdown, after the routes were stopped: gives back the leases of the single-owner ones that
   * stopped cleanly and tells the platform this runtime is leaving, once, even with none to give back,
   * waiting at most {@link #GOODBYE_WITHIN} (and a second more for the call to give up).
   *
   * @param stoppedCleanly the ingestions whose routes stopped cleanly
   */
  void goodbye(List<Ingestion> stoppedCleanly) {
    goodbye(stoppedCleanly, GOODBYE_WITHIN);
  }

  /**
   * {@link #goodbye(List)}, waiting at most {@code within} for the platform (and {@link
   * #GOODBYE_GRACE} more for the call to give up).
   *
   * @param stoppedCleanly the ingestions whose routes stopped cleanly
   * @param within the longest the platform is given to answer
   */
  void goodbye(List<Ingestion> stoppedCleanly, Duration within) {
    List<String> ids = stoppedCleanly.stream().filter(i -> i.source().singleOwner()).map(Ingestion::id).toList();
    // On a thread of its own, so that nothing the call does (such as fetching a token) can hold the
    // shutdown past the bound.
    Thread call =
        Thread.ofVirtual()
            .name("ingest-goodbye")
            .start(
                () -> {
                  try {
                    giveBack(ids, true, within);
                  } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                  }
                });
    try {
      if (!call.join(within.plus(GOODBYE_GRACE))) {
        LOG.warn("the platform did not answer the goodbye within {}; the leases run out instead", within);
        call.interrupt();
      }
    } catch (InterruptedException e) {
      call.interrupt();
      Thread.currentThread().interrupt();
    }
  }

  /**
   * The lease watchdog: stops the single-owner routes once the lease is within {@link #margin()} of
   * its end without a renewal, however long the round in progress takes, and without waiting for it
   * ({@link Reconciler#revoke}). Run every second. It never gives a lease back.
   */
  void watch() {
    try {
      if (monotonic.nanos() - (leaseEndsNanos - margin().toNanos()) < 0) {
        return;
      }
      if (reconciler.running().stream().noneMatch(i -> i.source().singleOwner())) {
        return;
      }
      int stopped = reconciler.revoke(i -> i.source().singleOwner(), STOP_WITHIN);
      if (stopped > 0) {
        LOG.warn("stopped {} single-owner ingestion(s): their lease ends within {} and has not been renewed", stopped, margin());
      }
    } catch (OutOfMemoryError e) {
      Fatal.outOfMemory("the lease watchdog", e);
    } catch (Throwable t) {
      LOG.error("the lease watchdog failed; it runs again in a second", t);
    }
  }

  /**
   * How long before the lease ends the watchdog stops the single-owner routes: {@link
   * #LEASE_MARGIN} or twice a call's timeout, whichever is longer, and never more than half the lease.
   */
  Duration margin() {
    Duration margin = callTimeout().multipliedBy(2);
    if (margin.compareTo(LEASE_MARGIN) < 0) {
      margin = LEASE_MARGIN;
    }
    Duration current = lease;
    if (current != null && margin.compareTo(current.dividedBy(2)) > 0) {
      margin = current.dividedBy(2);
    }
    return margin;
  }

  /** How long the poll and the report wait: the platform timeout, and at most a sixth of the lease (at least a second). */
  Duration callTimeout() {
    Duration current = lease;
    Duration bound = current == null ? Duration.ofSeconds(30) : current.dividedBy(6);
    if (bound.compareTo(Duration.ofSeconds(1)) < 0) {
      bound = Duration.ofSeconds(1);
    }
    return platformTimeout.compareTo(bound) < 0 ? platformTimeout : bound;
  }

  /**
   * Starts the tests the platform handed over, none that has run out of time or is already running,
   * each on its own thread with a deadline, and waits a little for them: an answer goes in the first
   * report after it.
   */
  private void test(List<TestRequest> requests) throws InterruptedException {
    long now = monotonic.nanos();
    List<Running> started = new ArrayList<>();
    for (TestRequest request : requests) {
      if (request.expiresAt().isBefore(clock.instant())) {
        LOG.info("test {} expired before it was run", request.id());
        continue;
      }
      if (testing.containsKey(request.id())) {
        continue;
      }
      LOG.info("testing the source of ingestion {} of client {} for the platform", request.ingestion().code(), request.ingestion().clientIdentifier());
      Duration left = Duration.between(clock.instant(), request.expiresAt());
      Duration allowed = left.compareTo(MAX_TEST) < 0 ? left : MAX_TEST;
      AtomicBoolean once = new AtomicBoolean();
      FutureTask<Void> task =
          new FutureTask<>(
              () -> {
                try {
                  ConnectionTest result = tester.apply(request);
                  if (once.compareAndSet(false, true)) {
                    answered.add(result);
                  }
                } catch (OutOfMemoryError e) {
                  Fatal.outOfMemory("a connection test", e);
                } catch (Throwable t) {
                  LOG.warn("test {} failed", request.id(), t);
                  if (once.compareAndSet(false, true)) {
                    answered.add(new ConnectionTest(request.id(), ConnectionTest.Outcome.FAILED, List.of(ExchangeCounters.clean(t.getMessage()) + "."), null, null));
                  }
                } finally {
                  testing.remove(request.id());
                }
              },
              null);
      Running running = new Running(task, now + allowed.toNanos(), once);
      // Listed before it can end, so its end always takes it off the list.
      testing.put(request.id(), running);
      testers.execute(task);
      started.add(running);
    }
    long waitUntil = now + Math.min(TEST_WAIT.toNanos(), interval.toNanos() / 2);
    for (Running running : started) {
      long left = Math.min(waitUntil, running.deadlineNanos()) - monotonic.nanos();
      if (left <= 0) {
        break;
      }
      try {
        running.future().get(left, TimeUnit.NANOSECONDS);
      } catch (TimeoutException | ExecutionException | java.util.concurrent.CancellationException e) {
        // Answered later, or cut off at its deadline.
      }
    }
  }

  /** Cuts off the tests past their deadline, answering them as failed. */
  private void overdueTests() {
    long now = monotonic.nanos();
    testing.forEach(
        (id, running) -> {
          if (now - running.deadlineNanos() < 0) {
            return;
          }
          running.future().cancel(true);
          testing.remove(id);
          if (running.answered().compareAndSet(false, true)) {
            LOG.warn("test {} did not finish in time; it is answered as failed", id);
            answered.add(new ConnectionTest(id, ConnectionTest.Outcome.FAILED, List.of("The test did not finish in time."), null, null));
          }
        });
  }

  /**
   * Every {@code secretsEvery}: reads the secrets each running route was built with again, and builds
   * a route whose secret changed again so it uses the new value; and, every round, lets go of what
   * was kept for routes that are gone and of kept values past their time. An HTTP ingestion checks
   * its secret on each request and is not rebuilt.
   */
  private void secrets() {
    Secrets kept = secrets;
    if (kept == null) {
      return;
    }
    List<Ingestion> running = reconciler.running();
    long now = monotonic.nanos();
    if (now - nextSecretsCheck >= 0) {
      nextSecretsCheck = now + secretsEvery.toNanos();
      for (Ingestion ingestion : running) {
        if (ingestion.source() instanceof Source.Http) {
          continue;
        }
        if (kept.rotated(ingestion) && reconciler.rebuild(ingestion.id())) {
          LOG.info("ingestion {} was built again: a secret it uses has changed", ingestion.id());
        }
      }
      running = reconciler.running();
    }
    kept.retain(running);
  }

  private void report(boolean platformAnswered) throws InterruptedException {
    Instant now = clock.instant();
    ExchangeCounters.Drained drained = counters.drain(now);
    unsent.addAll(drained.windows());
    unsentFailures.addAll(drained.failures());
    overdueTests();
    for (ConnectionTest test = answered.poll(); test != null; test = answered.poll()) {
      unsentTests.add(test);
    }
    if (unsentTests.size() > MAX_UNSENT_TESTS) {
      unsentTests.subList(0, unsentTests.size() - MAX_UNSENT_TESTS).clear();
    }
    // What the platform takes in one report, by count and by size, oldest first; the rest goes in the next rounds'.
    Budget budget = new Budget(RuntimeReport.MAX_BYTES - REPORT_HEADROOM);
    List<RouteState> routes = budget.fit(reconciler.states(), RuntimeReport.MAX_ROUTES);
    List<ConnectionTest> tests = budget.fit(unsentTests, RuntimeReport.MAX_TESTS);
    List<ExchangeWindow> windows = budget.fit(unsent, RuntimeReport.MAX_WINDOWS);
    List<Failure> failures = budget.fit(unsentFailures, RuntimeReport.MAX_FAILURES);
    try {
      platform.report(new RuntimeReport(runtimeId, routes, windows, failures, tests), callTimeout());
      failedReports = 0;
      unsent.subList(0, windows.size()).clear();
      unsentFailures.subList(0, failures.size()).clear();
      unsentTests.subList(0, tests.size()).clear();
    } catch (RejectedException e) {
      // Sent again it would be refused again, and nothing after it would ever be reported.
      LOG.error("the platform will not take this report, so it is dropped ({} window(s), {} failure(s), {} test answer(s)): {}", windows.size(), failures.size(), tests.size(), e.getMessage());
      failedReports = 0;
      unsent.subList(0, windows.size()).clear();
      unsentFailures.subList(0, failures.size()).clear();
      unsentTests.subList(0, tests.size()).clear();
    } catch (IOException e) {
      failedReports++;
      LOG.warn("could not send the report; {} window(s) kept for the next one: {}", unsent.size(), e.getMessage());
      if (platformAnswered && failedReports >= WEDGED_REPORTS) {
        // The platform answers polls but not this report: something in it fails every time. The
        // counts are kept (a window sent again replaces itself); what went wrong is let go.
        LOG.error("the report has failed {} times in a row while the platform answers; dropping its {} failure(s) and {} test answer(s)", failedReports, failures.size(), tests.size());
        unsentFailures.subList(0, failures.size()).clear();
        unsentTests.subList(0, tests.size()).clear();
        failedReports = 0;
      }
      if (unsent.size() > MAX_UNSENT_WINDOWS) {
        unsent.subList(0, unsent.size() - MAX_UNSENT_WINDOWS).clear();
      }
      if (unsentFailures.size() > MAX_UNSENT_FAILURES) {
        unsentFailures.subList(0, unsentFailures.size() - MAX_UNSENT_FAILURES).clear();
      }
    }
  }

  /** The bytes left in one report; each list takes what fits, in order. */
  private static final class Budget {
    private long left;

    Budget(long bytes) {
      this.left = bytes;
    }

    <T> List<T> fit(List<T> items, int most) {
      List<T> taken = new ArrayList<>();
      for (T item : items) {
        if (taken.size() >= most) {
          break;
        }
        long size = Json.mapper().writeValueAsBytes(item).length + 1L;
        if (size > left) {
          break;
        }
        left -= size;
        taken.add(item);
      }
      return List.copyOf(taken);
    }
  }

  /** Whether the lease ends before the round after this one could renew it. */
  private boolean leaseEnding() {
    return monotonic.nanos() + interval.toNanos() >= leaseEndsNanos;
  }

  /** Stops the tests still running; called when the runtime stops. */
  void close() {
    testers.shutdownNow();
  }

  /**
   * Fails when the platform refuses the polls because another live process holds this runtime's id,
   * or has not answered a poll within three intervals.
   */
  void requireRecentPoll() {
    if (idInUse != null) {
      throw new IllegalStateException("runtime id " + runtimeId + " is in use by another live process; the platform refuses this process's polls");
    }
    Instant last = lastPoll;
    if (last == null) {
      throw new IllegalStateException("the platform has not answered yet");
    }
    if (last.plus(interval.multipliedBy(3)).isBefore(clock.instant())) {
      throw new IllegalStateException("the platform last answered at " + last);
    }
  }
}

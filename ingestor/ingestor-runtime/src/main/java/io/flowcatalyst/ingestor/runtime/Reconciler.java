package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.extension.PolledSource;
import java.time.Duration;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.concurrent.locks.ReentrantLock;
import java.util.function.Consumer;
import java.util.function.Function;
import java.util.function.Predicate;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;
import org.apache.camel.Route;
import org.apache.camel.RoutesBuilder;
import org.apache.camel.support.ScheduledBatchPollingConsumer;
import org.apache.camel.support.ScheduledPollConsumer;
import org.apache.camel.support.service.ServiceHelper;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Makes the routes in the Camel context match the ingestions the platform says this runtime should run:
 * one route per ingestion, rebuilt when anything in it changes, removed when it is no longer listed.
 * The version is not enough: going back to an older version lowers it, and a change of a setting
 * outside the version (keeping failed payloads, the code) leaves it as it was.
 *
 * <p>Changes are made one at a time, under {@link #lock}; {@link #find} and {@link #running} read a
 * copy taken after each change, so an HTTP ingestion is found while another route is being stopped or
 * built. The lease watchdog's {@link #revoke} never waits for the lock (a change may be stuck starting
 * a route whose source does not answer): it marks the started ingestions it takes as revoked, which
 * every reader and every later change treats as no longer started, and suspends the consumers that
 * read their sources directly, which takes neither this lock nor the Camel context's. The runtime's
 * own stop ({@link #shutdown}) waits for the lock a bounded while and then does the same.
 *
 * <p>Routes are stopped together ({@link #removeAll}): every one stops taking at once, each finishes
 * what it took at the same time as the others, and only then are they stopped one by one under the
 * Camel context's lock, which is quick once nothing is in flight.
 */
final class Reconciler {

  private static final Logger LOG = LoggerFactory.getLogger(Reconciler.class);

  /** How long one route is given to finish the exchanges in flight when it is stopped. */
  static final Duration ROUTE_STOP = Duration.ofSeconds(10);

  /**
   * What a {@link #stop} did.
   *
   * @param count how many started ingestions it stopped
   * @param clean those whose routes stopped within the bound with no exchange left running, and that
   *     the watchdog had not revoked: only their leases may be given back
   */
  record Stopped(int count, List<Ingestion> clean) {

    /** Copies the list. */
    Stopped {
      clean = List.copyOf(clean);
    }
  }

  /**
   * A started ingestion and its routes, the one that reads the source first. Made new on every start,
   * so a revoke marks this start and suspends these routes, never a later start of the same ingestion.
   */
  private static final class Live {
    final Ingestion ingestion;
    final List<Route> routes;
    final AtomicBoolean revoked = new AtomicBoolean();
    /** Set once the consumer that reads the source was suspended (or, one that cannot be, stopped) without failing. */
    private volatile boolean intakeStopped;

    Live(Ingestion ingestion, List<Route> routes) {
      this.ingestion = ingestion;
      this.routes = List.copyOf(routes);
    }

    boolean revoked() {
      return revoked.get();
    }

    /**
     * Stops the route that reads the source taking anything more, leaving the routes behind it (a
     * camel-route ingestion's) running, so the exchange in flight can finish through them.
     */
    void suspendIntake() {
      if (routes.isEmpty()) {
        intakeStopped = true;
        return;
      }
      org.apache.camel.Consumer consumer = routes.getFirst().getConsumer();
      // A consumer whose suspension failed is FAILED, which Camel also counts as stopped.
      intakeStopped = quiesce(routes.getFirst()) && (ServiceHelper.isSuspended(consumer) || ServiceHelper.isStopped(consumer));
    }

    /**
     * Whether the route that reads the source has stopped taking: suspended, or stopped (a consumer
     * that cannot be suspended is stopped instead), without failing. True when there is none.
     */
    boolean intakeStopped() {
      return intakeStopped;
    }

    /**
     * Whether the route that reads the source is in the middle of a poll (listing, reading,
     * committing): a Camel polled consumer's, or an extension's ({@link PolledSource#polling()}).
     */
    boolean polling() {
      if (routes.isEmpty()) {
        return false;
      }
      Route intake = routes.getFirst();
      return (intake.getConsumer() instanceof ScheduledPollConsumer poller && poller.isPolling())
          || (intake.getEndpoint() instanceof PolledSource source && source.polling());
    }

    /**
     * Suspends a route's consumer without the Camel context's lock. A batch consumer (a file, S3 or
     * SQS poll) goes on through the rest of its batch while merely suspended, so it is first told,
     * as Camel's own graceful stop tells it, to end the batch after the exchange in flight; what the
     * batch did not reach stays with the source.
     *
     * @return false when it failed
     */
    private static boolean quiesce(Route route) {
      try {
        org.apache.camel.Consumer consumer = route.getConsumer();
        if (consumer instanceof ScheduledBatchPollingConsumer batch) {
          batch.deferShutdown(route.getShutdownRunningTask());
        }
        ServiceHelper.suspendService(consumer);
        return true;
      } catch (RuntimeException e) {
        LOG.warn("route {} could not be suspended", route.getId(), e);
        return false;
      }
    }
  }

  private final CamelContext camel;
  private final Function<Ingestion, RoutesBuilder> routes;
  private final Map<String, RouteState> states = new LinkedHashMap<>();
  private final Map<String, Live> started = new LinkedHashMap<>();
  private final SourceHealth health;
  private final Consumer<String> forgotten;
  private final FailureLog failures;

  /** Held by every change and by {@link #states}; the watchdog only ever tries it. Package-private for tests. */
  final ReentrantLock lock = new ReentrantLock();

  /** What the readers read: a copy of {@code started}, replaced after every change. */
  private volatile List<Live> published = List.of();

  /**
   * Set when the runtime begins to shut down ({@link #shutdown}): from then on no route is started,
   * so a round still running (one stuck starting a route, or one the application left behind) cannot
   * start again an ingestion whose lease the goodbye gave back.
   */
  private volatile boolean closed;

  /** How often a stop looks at whether the routes it stopped taking have finished what they took. */
  private static final Duration DRAIN_POLL = Duration.ofMillis(50);

  /** Of a stop's bound, what is kept for stopping and removing the routes once drained. */
  private static final Duration STOP_RESERVE = Duration.ofSeconds(1);

  /** The graceful bound Camel is given to stop a route that did not drain in time: it is then cut off. */
  private static final Duration CUT_OFF = Duration.ofSeconds(1);

  Reconciler(CamelContext camel, Function<Ingestion, RoutesBuilder> routes) {
    this(camel, routes, new SourceHealth());
  }

  Reconciler(CamelContext camel, Function<Ingestion, RoutesBuilder> routes, SourceHealth health) {
    this(camel, routes, health, id -> {});
  }

  /**
   * Creates the reconciler.
   *
   * @param camel the context the routes are in
   * @param routes the routes of an ingestion
   * @param health which sources keep failing
   * @param forgotten told the id of each ingestion whose route is gone, so what was kept for it (its secrets) is let go
   */
  Reconciler(CamelContext camel, Function<Ingestion, RoutesBuilder> routes, SourceHealth health, Consumer<String> forgotten) {
    this(camel, routes, health, forgotten, Pacing.system().failures());
  }

  /**
   * Creates the reconciler.
   *
   * @param camel the context the routes are in
   * @param routes the routes of an ingestion
   * @param health which sources keep failing
   * @param forgotten told the id of each ingestion whose route is gone
   * @param failures where a route that keeps failing to start is logged (a route is tried again on every reconcile)
   */
  Reconciler(
      CamelContext camel, Function<Ingestion, RoutesBuilder> routes, SourceHealth health, Consumer<String> forgotten, FailureLog failures) {
    this.failures = failures;
    this.camel = camel;
    this.routes = routes;
    this.health = health;
    this.forgotten = forgotten;
  }

  /**
   * Applies the desired set. A route that fails to build or start is recorded as failed and tried
   * again on the next call; the others are unaffected.
   *
   * @param desired every ingestion that should be running
   * @return the ingestions whose routes this call stopped cleanly and did not start again
   */
  List<Ingestion> reconcile(List<Ingestion> desired) {
    return reconcile(desired, List.of());
  }

  /**
   * Applies the desired set, and records the ingestions listed that this runtime will not run (a
   * definition it cannot read, a lease too short) as failed with their reason, stopping a route an
   * earlier version of one of them had. A started ingestion the watchdog revoked is stopped and, when
   * desired, started again.
   *
   * @param desired every ingestion that should be running
   * @param notRun the ingestions the platform listed that this runtime does not run, and why
   * @return the ingestions whose routes this call stopped cleanly (within the bound, with no exchange
   *     left running) and did not start again; never one the watchdog revoked
   */
  List<Ingestion> reconcile(List<Ingestion> desired, List<RuntimeApi.Unreadable> notRun) {
    lock.lock();
    try {
      Map<String, Ingestion> wanted = new LinkedHashMap<>();
      for (Ingestion ingestion : desired) {
        wanted.put(ingestion.id(), ingestion);
      }
      Map<String, RuntimeApi.Unreadable> unread = new LinkedHashMap<>();
      for (RuntimeApi.Unreadable each : notRun) {
        if (!wanted.containsKey(each.ingestionId())) {
          unread.put(each.ingestionId(), each);
        }
      }
      List<String> leaving = new ArrayList<>();
      for (RouteState state : new ArrayList<>(states.values())) {
        Ingestion ingestion = wanted.get(state.ingestionId());
        Live live = started.get(state.ingestionId());
        boolean keep =
            ingestion != null
                && state.status() == RouteState.Status.STARTED
                && live != null
                && !live.revoked()
                && ingestion.equals(live.ingestion);
        RuntimeApi.Unreadable still = unread.get(state.ingestionId());
        boolean sameFailure = still != null && live == null && state.equals(failed(still));
        if (!keep && !sameFailure) {
          leaving.add(state.ingestionId());
        }
      }
      List<Ingestion> stopped = removeAll(leaving, ROUTE_STOP.multipliedBy(2));
      for (Ingestion ingestion : wanted.values()) {
        if (closed) {
          // The runtime is shutting down (it may have begun while this round was starting a
          // route): nothing more is started.
          break;
        }
        if (!states.containsKey(ingestion.id())) {
          states.put(ingestion.id(), add(ingestion));
        }
      }
      for (RuntimeApi.Unreadable each : unread.values()) {
        if (!states.containsKey(each.ingestionId())) {
          LOG.warn("ingestion {} version {} cannot be run: {}", each.ingestionId(), each.version(), each.reason());
          states.put(each.ingestionId(), failed(each));
        }
      }
      publish();
      return stopped.stream().filter(i -> !wanted.containsKey(i.id())).toList();
    } finally {
      lock.unlock();
    }
  }

  private static RouteState failed(RuntimeApi.Unreadable each) {
    return new RouteState(each.ingestionId(), each.version(), RouteState.Status.FAILED, each.reason());
  }

  /**
   * Stops and forgets every started ingestion that matches, until a later reconcile lists it again.
   * The routes are stopped at the same time, each given {@link #ROUTE_STOP} to finish what it has
   * in flight, and all within {@code within}.
   *
   * @param which the ingestions to stop
   * @param within the longest the stops may take together
   * @return how many were stopped, and which of them stopped cleanly
   */
  Stopped stop(Predicate<Ingestion> which, Duration within) {
    lock.lock();
    try {
      List<String> ids = started.values().stream().filter(live -> which.test(live.ingestion)).map(live -> live.ingestion.id()).toList();
      List<Ingestion> clean = removeAll(ids, within);
      publish();
      return new Stopped(ids.size(), clean);
    } finally {
      lock.unlock();
    }
  }

  /**
   * The runtime's stop: from now on no route is started; every started ingestion is stopped as by
   * {@link #stop(Predicate, Duration)}, if {@link #lock} can be had within {@code lockWait}. A change
   * may hold it, stuck starting a route whose source does not answer (and holding the Camel context's
   * lock too), so after {@code lockWait} it goes on without it, the watchdog's way: every started
   * ingestion is revoked and the consumer that reads its source suspended directly (a batch consumer
   * told to end its batch after the exchange in flight, one that cannot be suspended stopped), and each is
   * waited for, within what is left of {@code within}, to finish what it took: no exchange in flight
   * in any of its routes and no poll in progress. One that got there counts as stopped cleanly, though
   * its routes are left for the Camel context's close: it takes nothing more from its source and
   * nothing of it is running, so it can make no job and move no watermark. A route the stuck change
   * starts after this is not among them: it was not started when they were taken.
   *
   * @param within the longest the whole stop may take
   * @param lockWait the longest it waits for the lock before going on without it
   * @return how many were stopped, and which of them stopped cleanly
   */
  Stopped shutdown(Duration within, Duration lockWait) {
    closed = true;
    long deadline = System.nanoTime() + within.toNanos();
    boolean locked = false;
    try {
      locked = lock.tryLock(Math.min(lockWait.toNanos(), within.toNanos()), TimeUnit.NANOSECONDS);
    } catch (InterruptedException e) {
      Thread.currentThread().interrupt();
    }
    if (locked) {
      try {
        List<String> ids = List.copyOf(started.keySet());
        List<Ingestion> clean = removeAll(ids, left(deadline));
        publish();
        return new Stopped(ids.size(), clean);
      } finally {
        lock.unlock();
      }
    }
    LOG.warn("a change of the routes has not let go of them within {}; stopping what they take without waiting for it", lockWait);
    List<Live> taken = take(live -> true);
    // Only what reads the source: a camel-route ingestion's exchange in flight finishes through the routes behind it.
    Map<Live, Thread> suspending = suspendAll(taken, Live::suspendIntake);
    Set<Live> drained = drain(suspending, deadline);
    List<Ingestion> clean = new ArrayList<>();
    for (Live live : taken) {
      if (drained.contains(live) && live.intakeStopped()) {
        clean.add(live.ingestion);
      }
    }
    LOG.info("stopped taking for {} ingestion(s); {} of them have nothing left running", taken.size(), clean.size());
    return new Stopped(taken.size(), clean);
  }

  private static Duration left(long deadline) {
    return Duration.ofNanos(Math.max(0, deadline - System.nanoTime()));
  }

  /** Marks revoked, and returns, the published started ingestions that match and were not revoked already. */
  private List<Live> take(Predicate<Ingestion> which) {
    List<Live> taken = new ArrayList<>();
    for (Live live : published) {
      if (which.test(live.ingestion) && live.revoked.compareAndSet(false, true)) {
        taken.add(live);
      }
    }
    return taken;
  }

  /** Suspends each, at the same time, each on its own thread; taking neither lock. */
  private static Map<Live, Thread> suspendAll(List<Live> lives, Consumer<Live> how) {
    Map<Live, Thread> suspending = new LinkedHashMap<>();
    for (Live live : lives) {
      suspending.put(live, Thread.ofVirtual().name("suspend-" + live.ingestion.id()).start(() -> how.accept(live)));
    }
    return suspending;
  }

  /**
   * Waits, until {@code deadline}, for each to be suspended and then quiet ({@link #quiet}) twice in a
   * row, {@link #DRAIN_POLL} apart (a poll that passed its last check before the suspension may still
   * be about to begin).
   *
   * @return those that got there
   */
  private Set<Live> drain(Map<Live, Thread> suspending, long deadline) {
    Map<Live, Boolean> once = new LinkedHashMap<>();
    Set<Live> drained = new HashSet<>();
    while (true) {
      for (Map.Entry<Live, Thread> each : suspending.entrySet()) {
        Live live = each.getKey();
        if (drained.contains(live)) {
          continue;
        }
        boolean quiet = !each.getValue().isAlive() && quiet(live);
        if (quiet && once.getOrDefault(live, false)) {
          drained.add(live);
        }
        once.put(live, quiet);
      }
      long left = deadline - System.nanoTime();
      if (drained.size() == suspending.size() || left <= 0) {
        return drained;
      }
      try {
        Thread.sleep(Duration.ofNanos(Math.min(left, DRAIN_POLL.toNanos())));
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return drained;
      }
    }
  }

  /**
   * Whether nothing of the ingestion is running: no exchange in flight in any of its routes and,
   * for a single-owner one, no poll in progress (a poll lists or reads before it makes an exchange,
   * and a database poll stores its watermark after it). Read without any lock.
   */
  private boolean quiet(Live live) {
    if (live.ingestion.source().singleOwner() && live.polling()) {
      return false;
    }
    for (Route route : live.routes) {
      if (camel.getInflightRepository().size(route.getId()) > 0) {
        return false;
      }
    }
    return true;
  }

  /**
   * Stops and forgets every started ingestion that matches, as {@link #stop(Predicate, Duration)}.
   *
   * @param which the ingestions to stop
   * @return how many were stopped
   */
  int stop(Predicate<Ingestion> which) {
    return stop(which, ROUTE_STOP.multipliedBy(2)).count();
  }

  /**
   * The lease watchdog's stop: never waits for {@link #lock}, which a reconcile stuck starting a route
   * may hold, nor for the Camel context's, which that start may hold too. Each matching started
   * ingestion is marked revoked (from then on it is not {@linkplain #running running}, not reported,
   * not rebuilt, and the next change stops its routes and, if it is listed again, starts it anew) and
   * the consumer of the route that reads its source is suspended, so it takes nothing more from the
   * source; the routes behind it (a camel-route ingestion's) keep running, so the exchange in flight
   * finishes through them. When nothing holds
   * the lock the revoked routes are then stopped and removed as usual; otherwise the next change does
   * it.
   *
   * @param which the ingestions to stop
   * @param within the longest the suspensions may take together
   * @return how many were revoked
   */
  int revoke(Predicate<Ingestion> which, Duration within) {
    List<Live> taken = take(which);
    if (taken.isEmpty()) {
      return 0;
    }
    Map<Live, Thread> suspending = suspendAll(taken, Live::suspendIntake);
    long deadline = System.nanoTime() + within.toNanos();
    for (Map.Entry<Live, Thread> each : suspending.entrySet()) {
      try {
        long left = deadline - System.nanoTime();
        if (left <= 0 || !each.getValue().join(Duration.ofNanos(left))) {
          LOG.warn("ingestion {} was not suspended within {}; it goes on being suspended", each.getKey().ingestion.id(), within);
        }
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return taken.size();
      }
    }
    if (lock.tryLock()) {
      try {
        removeAll(started.values().stream().filter(Live::revoked).map(live -> live.ingestion.id()).toList(), within);
        publish();
      } finally {
        lock.unlock();
      }
    }
    return taken.size();
  }

  /**
   * Builds an ingestion's route again from the same definition: it is stopped, letting the exchange
   * in flight finish, and started, with what its endpoint reads at start (its secrets) read anew.
   *
   * @param ingestionId the ingestion
   * @return whether it was running (and not revoked) and was rebuilt
   */
  boolean rebuild(String ingestionId) {
    lock.lock();
    try {
      Live live = started.get(ingestionId);
      if (live == null || live.revoked()) {
        return false;
      }
      removeAll(List.of(ingestionId), ROUTE_STOP.multipliedBy(2));
      states.put(ingestionId, add(live.ingestion));
      publish();
      return true;
    } finally {
      lock.unlock();
    }
  }

  /**
   * The ingestions whose route is started and not revoked. Never waits for a route being stopped or built.
   *
   * @return them, in no particular order
   */
  List<Ingestion> running() {
    List<Ingestion> now = new ArrayList<>();
    for (Live live : published) {
      if (!live.revoked()) {
        now.add(live.ingestion);
      }
    }
    return List.copyOf(now);
  }

  /**
   * The running ingestions with a client and a code. A code is unique within an application and
   * client, so two applications may each have one. Never waits for a route being stopped or built.
   *
   * @param application the application's code, or null for any
   * @param client the client's identifier or platform id
   * @param code the ingestion's code
   * @return the ingestions whose route is started: none, one, or several when the application was not named
   */
  List<Ingestion> find(@Nullable String application, String client, String code) {
    return running().stream()
        .filter(i -> application == null || i.applicationCode().equals(application))
        .filter(i -> i.clientIdentifier().equals(client) || i.clientId().equals(client))
        .filter(i -> i.code().equals(code))
        .toList();
  }

  /**
   * The state of every ingestion from the last reconcile; a started one whose source keeps failing is
   * {@code FAILING}, and one the watchdog revoked is left out, as a stopped one is.
   */
  List<RouteState> states() {
    lock.lock();
    try {
      List<RouteState> now = new ArrayList<>();
      for (RouteState state : states.values()) {
        Live live = started.get(state.ingestionId());
        if (live != null && live.revoked()) {
          continue;
        }
        String problem = state.status() == RouteState.Status.STARTED ? health.problem(state.ingestionId(), state.version()) : null;
        now.add(problem == null ? state : new RouteState(state.ingestionId(), state.version(), RouteState.Status.FAILING, problem));
      }
      return List.copyOf(now);
    } finally {
      lock.unlock();
    }
  }

  private void publish() {
    published = List.copyOf(started.values());
  }

  private RouteState add(Ingestion ingestion) {
    try {
      camel.addRoutes(routes.apply(ingestion));
      List<Route> its = new ArrayList<>();
      for (String id : List.of(ingestion.id() + IngestionRoute.SOURCE_ROUTE, ingestion.id(), ingestion.id() + IngestionRoute.FAILURES_ROUTE)) {
        Route route = camel.getRoute(id);
        if (route != null) {
          its.add(route);
        }
      }
      started.put(ingestion.id(), new Live(ingestion, its));
      LOG.info("started ingestion {} version {}", ingestion.id(), ingestion.version());
      failures.recovered(ingestion.id() + ":start");
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.STARTED, null);
    } catch (Exception e) {
      String reason = ExchangeCounters.clean(e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage());
      // It is tried again on every reconcile, so the same failure comes back: logged in full once,
      // then a line a minute (R3). A definition that cannot be run as it is has no stack trace to add.
      String line = "ingestion " + ingestion.id() + " version " + ingestion.version() + " failed to start" + (e instanceof UnsupportedIngestionException ? ": " + reason : "");
      failures.failed(ingestion.id() + ":start", line, e instanceof UnsupportedIngestionException ? null : e);
      removeRoute(ingestion.id());
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.FAILED, reason);
    }
  }

  /**
   * Forgets the ingestions and stops their routes, all at the same time, in three steps:
   *
   * <ol>
   *   <li>the consumer that reads each one's source is suspended at once, each on its own thread and
   *       without the Camel context's lock, so none takes anything more while the others finish;
   *   <li>each is waited for, at the same time, until nothing of it is running ({@link #quiet}),
   *       each at most {@link #ROUTE_STOP} and all within {@code within} (less {@link #STOP_RESERVE},
   *       kept for the next step);
   *   <li>the routes are stopped and removed, the drained ones first: Camel stops one route at a time
   *       under its context's lock, which is quick once nothing is in flight. One that did not drain
   *       is given {@link #CUT_OFF} to finish and is then cut off; its payload stays with the source.
   * </ol>
   *
   * A route that has not stopped within {@code within} is left to finish stopping on its own thread;
   * it is no longer started.
   *
   * @return the started ingestions among them whose routes stopped within {@code within}, each with
   *     no exchange left running in it (and, for a single-owner one, no poll), and that the watchdog
   *     had not revoked
   */
  private List<Ingestion> removeAll(List<String> ids, Duration within) {
    if (ids.isEmpty()) {
      return List.of();
    }
    long start = System.nanoTime();
    long deadline = start + within.toNanos();
    Map<String, Live> lives = new LinkedHashMap<>();
    for (String id : ids) {
      health.forget(id);
      states.remove(id);
      Live live = started.remove(id);
      if (live != null) {
        lives.put(id, live);
      }
    }
    publish();
    // 1. Nothing more is taken from any of them.
    Map<Live, Thread> suspending = suspendAll(List.copyOf(lives.values()), Live::suspendIntake);
    // 2. What each took is finished, at the same time.
    long drainedBy = Math.min(start + ROUTE_STOP.toNanos(), deadline - Math.min(STOP_RESERVE.toNanos(), within.toNanos() / 4));
    Set<Live> drained = drain(suspending, drainedBy);
    // 3. Stopped and removed: the drained ones, quick, then the others, cut off.
    List<String> first = new ArrayList<>();
    List<String> then = new ArrayList<>();
    for (String id : ids) {
      Live live = lives.get(id);
      (live == null || drained.contains(live) ? first : then).add(id);
    }
    Set<String> clean = java.util.concurrent.ConcurrentHashMap.newKeySet();
    Set<String> inTime = new HashSet<>();
    boolean interrupted = stopAll(first, lives, clean, inTime, deadline, within);
    if (!interrupted) {
      stopAll(then, lives, clean, inTime, deadline, within);
    }
    ids.forEach(forgotten);
    List<Ingestion> stopped = new ArrayList<>();
    for (Live live : lives.values()) {
      String id = live.ingestion.id();
      if (inTime.contains(id) && clean.contains(id) && !live.revoked()) {
        stopped.add(live.ingestion);
      }
    }
    return stopped;
  }

  /**
   * Stops and removes the ingestions' routes, each on its own thread, and waits for them until
   * {@code deadline}; adds to {@code clean} those that stopped cleanly, to {@code inTime} those that
   * stopped by then.
   *
   * @return whether the wait was interrupted
   */
  private boolean stopAll(List<String> ids, Map<String, Live> lives, Set<String> clean, Set<String> inTime, long deadline, Duration within) {
    List<Thread> stopping = new ArrayList<>();
    for (String id : ids) {
      Live live = lives.get(id);
      // A drained route stops at once; one that is not is cut off after a short while.
      Duration grace = live == null || quiet(live) ? ROUTE_STOP : CUT_OFF;
      stopping.add(
          Thread.ofVirtual()
              .name("stop-" + id)
              .start(
                  () -> {
                    if (removeRoute(id, grace) && (live == null || !live.ingestion.source().singleOwner() || !live.polling())) {
                      clean.add(id);
                    }
                  }));
    }
    for (int i = 0; i < ids.size(); i++) {
      try {
        long left = deadline - System.nanoTime();
        if (left > 0 && stopping.get(i).join(Duration.ofNanos(left))) {
          inTime.add(ids.get(i));
        } else {
          LOG.warn("route {} did not stop within {}; it goes on stopping, and its lease is left to run out", ids.get(i), within);
        }
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return true;
      }
    }
    return false;
  }

  /** Stops and removes an ingestion's routes, each given {@link #ROUTE_STOP} to finish its exchange in flight. */
  private boolean removeRoute(String ingestionId) {
    return removeRoute(ingestionId, ROUTE_STOP);
  }

  /**
   * Stops and removes an ingestion's routes, each given {@code grace} to finish its exchange in flight.
   *
   * @return whether each stopped with no exchange left running in it (none cut off at the timeout
   *     and still going) and was removed
   */
  private boolean removeRoute(String ingestionId, Duration grace) {
    boolean clean = true;
    // A camel-route ingestion's own route first, so nothing is sent to the routes behind it.
    for (String extra : List.of(ingestionId + IngestionRoute.SOURCE_ROUTE, ingestionId + IngestionRoute.FAILURES_ROUTE)) {
      if (camel.getRoute(extra) != null) {
        try {
          camel.getRouteController().stopRoute(extra, grace.toMillis(), TimeUnit.MILLISECONDS);
          clean &= atRest(extra);
          clean &= camel.removeRoute(extra);
        } catch (Exception e) {
          clean = false;
          LOG.warn("route {} could not be stopped", extra, e);
        }
      }
    }
    Route route = camel.getRoute(ingestionId);
    if (route == null) {
      return clean;
    }
    try {
      Endpoint source = route.getEndpoint();
      // Gracefully: the exchange in flight finishes (its jobs are sent, the file moved) or, after
      // the timeout, is cut off and its payload stays with the source.
      camel.getRouteController().stopRoute(ingestionId, grace.toMillis(), TimeUnit.MILLISECONDS);
      clean &= atRest(ingestionId);
      clean &= camel.removeRoute(ingestionId);
      // The endpoint was built for this route alone: let go of its connections and clients.
      camel.removeEndpoint(source);
      ServiceHelper.stopAndShutdownService(source);
      LOG.info("stopped ingestion {}", ingestionId);
      return clean;
    } catch (Exception e) {
      LOG.warn("ingestion {} could not be stopped", ingestionId, e);
      return false;
    }
  }

  /**
   * Whether a route just stopped is at rest: stopped, and with no exchange still running in it. Camel
   * cuts an exchange off at the stop's timeout but cannot end its thread, so it may still be running.
   */
  private boolean atRest(String routeId) {
    org.apache.camel.ServiceStatus status = camel.getRouteController().getRouteStatus(routeId);
    return status != null && status.isStopped() && camel.getInflightRepository().size(routeId) == 0;
  }
}

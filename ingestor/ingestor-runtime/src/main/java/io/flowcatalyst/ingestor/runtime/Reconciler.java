package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.extension.FailureLog;
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
 * every reader and every later change treats as no longer started, and suspends their consumers
 * directly, which takes neither this lock nor the Camel context's.
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

    Live(Ingestion ingestion, List<Route> routes) {
      this.ingestion = ingestion;
      this.routes = List.copyOf(routes);
    }

    boolean revoked() {
      return revoked.get();
    }

    /** Stops the routes reading: no new poll or message is taken; the exchange in flight goes on. */
    void suspend() {
      for (Route route : routes) {
        try {
          ServiceHelper.suspendService(route.getConsumer());
        } catch (RuntimeException e) {
          LOG.warn("route {} could not be suspended", route.getId(), e);
        }
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
   * its routes' consumers are suspended, so they take nothing more from the source. When nothing holds
   * the lock the revoked routes are then stopped and removed as usual; otherwise the next change does
   * it.
   *
   * @param which the ingestions to stop
   * @param within the longest the suspensions may take together
   * @return how many were revoked
   */
  int revoke(Predicate<Ingestion> which, Duration within) {
    List<Live> taken = new ArrayList<>();
    for (Live live : published) {
      if (which.test(live.ingestion) && live.revoked.compareAndSet(false, true)) {
        taken.add(live);
      }
    }
    if (taken.isEmpty()) {
      return 0;
    }
    List<Thread> suspending = new ArrayList<>();
    for (Live live : taken) {
      suspending.add(Thread.ofVirtual().name("revoke-" + live.ingestion.id()).start(live::suspend));
    }
    long deadline = System.nanoTime() + within.toNanos();
    for (int i = 0; i < suspending.size(); i++) {
      try {
        long left = deadline - System.nanoTime();
        if (left <= 0 || !suspending.get(i).join(Duration.ofNanos(left))) {
          LOG.warn("ingestion {} was not suspended within {}; it goes on being suspended", taken.get(i).ingestion.id(), within);
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
   * Forgets the ingestions and stops their routes at the same time. A route that has not stopped
   * within {@code within} is left to finish stopping on its own thread; it is no longer started.
   *
   * @return the started ingestions among them whose routes stopped within {@code within}, each with
   *     no exchange left running in it, and that the watchdog had not revoked
   */
  private List<Ingestion> removeAll(List<String> ids, Duration within) {
    if (ids.isEmpty()) {
      return List.of();
    }
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
    Set<String> clean = java.util.concurrent.ConcurrentHashMap.newKeySet();
    List<Thread> stopping = new ArrayList<>();
    for (String id : ids) {
      stopping.add(
          Thread.ofVirtual()
              .name("stop-" + id)
              .start(
                  () -> {
                    if (removeRoute(id)) {
                      clean.add(id);
                    }
                  }));
    }
    Set<String> inTime = new HashSet<>();
    long deadline = System.nanoTime() + within.toNanos();
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
        break;
      }
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
   * Stops and removes an ingestion's routes.
   *
   * @return whether each stopped with no exchange left running in it (none cut off at the timeout
   *     and still going) and was removed
   */
  private boolean removeRoute(String ingestionId) {
    boolean clean = true;
    // A camel-route ingestion's own route first, so nothing is sent to the routes behind it.
    for (String extra : List.of(ingestionId + IngestionRoute.SOURCE_ROUTE, ingestionId + IngestionRoute.FAILURES_ROUTE)) {
      if (camel.getRoute(extra) != null) {
        try {
          camel.getRouteController().stopRoute(extra, ROUTE_STOP.toSeconds(), TimeUnit.SECONDS);
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
      camel.getRouteController().stopRoute(ingestionId, ROUTE_STOP.toSeconds(), TimeUnit.SECONDS);
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

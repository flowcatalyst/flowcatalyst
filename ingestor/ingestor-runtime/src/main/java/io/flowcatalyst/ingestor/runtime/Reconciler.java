package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import java.time.Duration;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.TimeUnit;
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
 * <p>Changes are made one at a time; {@link #find} reads a copy taken after each change, so an HTTP
 * ingestion is found while another route is being stopped or built.
 */
final class Reconciler {

  private static final Logger LOG = LoggerFactory.getLogger(Reconciler.class);

  /** How long one route is given to finish the exchanges in flight when it is stopped. */
  static final Duration ROUTE_STOP = Duration.ofSeconds(10);

  private final CamelContext camel;
  private final Function<Ingestion, RoutesBuilder> routes;
  private final Map<String, RouteState> states = new LinkedHashMap<>();
  private final Map<String, Ingestion> started = new LinkedHashMap<>();
  private final SourceHealth health;
  private final Consumer<String> forgotten;
  /** What {@link #find} reads: a copy of {@code started}, replaced after every change. */
  private volatile List<Ingestion> running = List.of();

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
   */
  void reconcile(List<Ingestion> desired) {
    reconcile(desired, List.of());
  }

  /**
   * Applies the desired set, and records the ingestions whose definition could not be read as
   * failed, stopping a route an earlier version of one of them had.
   *
   * @param desired every ingestion that should be running
   * @param unreadable the ingestions the platform listed whose definition this runtime cannot read
   */
  synchronized void reconcile(List<Ingestion> desired, List<RuntimeApi.Unreadable> unreadable) {
    Map<String, Ingestion> wanted = new LinkedHashMap<>();
    for (Ingestion ingestion : desired) {
      wanted.put(ingestion.id(), ingestion);
    }
    Map<String, RuntimeApi.Unreadable> unread = new LinkedHashMap<>();
    for (RuntimeApi.Unreadable each : unreadable) {
      if (!wanted.containsKey(each.ingestionId())) {
        unread.put(each.ingestionId(), each);
      }
    }
    List<String> leaving = new ArrayList<>();
    for (RouteState state : new ArrayList<>(states.values())) {
      Ingestion ingestion = wanted.get(state.ingestionId());
      boolean keep =
          ingestion != null
              && state.status() == RouteState.Status.STARTED
              && ingestion.equals(started.get(state.ingestionId()));
      RuntimeApi.Unreadable still = unread.get(state.ingestionId());
      boolean sameUnreadable =
          still != null && state.status() == RouteState.Status.FAILED && state.version() == still.version() && !started.containsKey(state.ingestionId());
      if (!keep && !sameUnreadable) {
        leaving.add(state.ingestionId());
      }
    }
    removeAll(leaving, ROUTE_STOP.multipliedBy(2));
    for (Ingestion ingestion : wanted.values()) {
      if (!states.containsKey(ingestion.id())) {
        states.put(ingestion.id(), add(ingestion));
      }
    }
    for (RuntimeApi.Unreadable each : unread.values()) {
      if (!states.containsKey(each.ingestionId())) {
        LOG.warn("ingestion {} version {} cannot be run: {}", each.ingestionId(), each.version(), each.reason());
        states.put(each.ingestionId(), new RouteState(each.ingestionId(), each.version(), RouteState.Status.FAILED, each.reason()));
      }
    }
    publish();
  }

  /**
   * Stops and forgets every started ingestion that matches, until a later reconcile lists it again.
   * The routes are stopped at the same time, each given {@link #ROUTE_STOP} to finish what it has
   * in flight, and all within {@code within}.
   *
   * @param which the ingestions to stop
   * @param within the longest the stops may take together
   * @return how many were stopped
   */
  synchronized int stop(Predicate<Ingestion> which, Duration within) {
    List<String> ids = started.values().stream().filter(which).map(Ingestion::id).toList();
    removeAll(ids, within);
    publish();
    return ids.size();
  }

  /**
   * Stops and forgets every started ingestion that matches, as {@link #stop(Predicate, Duration)}.
   *
   * @param which the ingestions to stop
   * @return how many were stopped
   */
  int stop(Predicate<Ingestion> which) {
    return stop(which, ROUTE_STOP.multipliedBy(2));
  }

  /**
   * Builds an ingestion's route again from the same definition: it is stopped, letting the exchange
   * in flight finish, and started, with what its endpoint reads at start (its secrets) read anew.
   *
   * @param ingestionId the ingestion
   * @return whether it was running and was rebuilt
   */
  synchronized boolean rebuild(String ingestionId) {
    Ingestion ingestion = started.get(ingestionId);
    if (ingestion == null) {
      return false;
    }
    removeAll(List.of(ingestionId), ROUTE_STOP.multipliedBy(2));
    states.put(ingestionId, add(ingestion));
    publish();
    return true;
  }

  /**
   * The ingestions whose route is started.
   *
   * @return them, in no particular order
   */
  List<Ingestion> running() {
    return running;
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
    return running.stream()
        .filter(i -> application == null || i.applicationCode().equals(application))
        .filter(i -> i.clientIdentifier().equals(client) || i.clientId().equals(client))
        .filter(i -> i.code().equals(code))
        .toList();
  }

  /** The state of every ingestion from the last reconcile; a started one whose source keeps failing is {@code FAILING}. */
  synchronized List<RouteState> states() {
    List<RouteState> now = new ArrayList<>();
    for (RouteState state : states.values()) {
      String problem = state.status() == RouteState.Status.STARTED ? health.problem(state.ingestionId(), state.version()) : null;
      now.add(problem == null ? state : new RouteState(state.ingestionId(), state.version(), RouteState.Status.FAILING, problem));
    }
    return List.copyOf(now);
  }

  private void publish() {
    running = List.copyOf(started.values());
  }

  private RouteState add(Ingestion ingestion) {
    try {
      camel.addRoutes(routes.apply(ingestion));
      started.put(ingestion.id(), ingestion);
      LOG.info("started ingestion {} version {}", ingestion.id(), ingestion.version());
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.STARTED, null);
    } catch (Exception e) {
      String reason = ExchangeCounters.clean(e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage());
      if (e instanceof UnsupportedIngestionException) {
        // The definition cannot be run as it is: nothing a stack trace adds.
        LOG.warn("ingestion {} version {} failed to start: {}", ingestion.id(), ingestion.version(), reason);
      } else {
        LOG.warn("ingestion {} version {} failed to start", ingestion.id(), ingestion.version(), e);
      }
      removeRoute(ingestion.id());
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.FAILED, reason);
    }
  }

  /**
   * Forgets the ingestions and stops their routes at the same time. A route that has not stopped
   * within {@code within} is left to finish stopping on its own thread; it is no longer started.
   */
  private void removeAll(List<String> ids, Duration within) {
    if (ids.isEmpty()) {
      return;
    }
    for (String id : ids) {
      health.forget(id);
      states.remove(id);
      started.remove(id);
    }
    publish();
    if (ids.size() == 1) {
      removeRoute(ids.getFirst());
      forgotten.accept(ids.getFirst());
      return;
    }
    List<Thread> stopping = new ArrayList<>();
    for (String id : ids) {
      stopping.add(Thread.ofVirtual().name("stop-" + id).start(() -> removeRoute(id)));
    }
    long deadline = System.nanoTime() + within.toNanos();
    for (Thread thread : stopping) {
      try {
        long left = deadline - System.nanoTime();
        if (left <= 0 || !thread.join(Duration.ofNanos(left))) {
          LOG.warn("route {} did not stop within {}; it goes on stopping", thread.getName().substring("stop-".length()), within);
        }
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        break;
      }
    }
    ids.forEach(forgotten);
  }

  private void removeRoute(String ingestionId) {
    // A camel-route ingestion's own route first, so nothing is sent to the routes behind it.
    for (String extra : List.of(ingestionId + IngestionRoute.SOURCE_ROUTE, ingestionId + IngestionRoute.FAILURES_ROUTE)) {
      if (camel.getRoute(extra) != null) {
        try {
          camel.getRouteController().stopRoute(extra, ROUTE_STOP.toSeconds(), TimeUnit.SECONDS);
          camel.removeRoute(extra);
        } catch (Exception e) {
          LOG.warn("route {} could not be stopped", extra, e);
        }
      }
    }
    Route route = camel.getRoute(ingestionId);
    if (route == null) {
      return;
    }
    try {
      Endpoint source = route.getEndpoint();
      // Gracefully: the exchange in flight finishes (its jobs are sent, the file moved) or, after
      // the timeout, is cut off and its payload stays with the source.
      camel.getRouteController().stopRoute(ingestionId, ROUTE_STOP.toSeconds(), TimeUnit.SECONDS);
      camel.removeRoute(ingestionId);
      // The endpoint was built for this route alone: let go of its connections and clients.
      camel.removeEndpoint(source);
      ServiceHelper.stopAndShutdownService(source);
      LOG.info("stopped ingestion {}", ingestionId);
    } catch (Exception e) {
      LOG.warn("ingestion {} could not be stopped", ingestionId, e);
    }
  }
}

package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
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
 */
final class Reconciler {

  private static final Logger LOG = LoggerFactory.getLogger(Reconciler.class);

  private final CamelContext camel;
  private final Function<Ingestion, RoutesBuilder> routes;
  private final Map<String, RouteState> states = new LinkedHashMap<>();
  private final Map<String, Ingestion> started = new LinkedHashMap<>();
  private final SourceHealth health;

  Reconciler(CamelContext camel, Function<Ingestion, RoutesBuilder> routes) {
    this(camel, routes, new SourceHealth());
  }

  Reconciler(CamelContext camel, Function<Ingestion, RoutesBuilder> routes, SourceHealth health) {
    this.camel = camel;
    this.routes = routes;
    this.health = health;
  }

  /**
   * Applies the desired set. A route that fails to build or start is recorded as failed and tried
   * again on the next call; the others are unaffected.
   *
   * @param desired every ingestion that should be running
   */
  synchronized void reconcile(List<Ingestion> desired) {
    Map<String, Ingestion> wanted = new LinkedHashMap<>();
    for (Ingestion ingestion : desired) {
      wanted.put(ingestion.id(), ingestion);
    }
    for (RouteState state : new ArrayList<>(states.values())) {
      Ingestion ingestion = wanted.get(state.ingestionId());
      boolean keep =
          ingestion != null
              && state.status() == RouteState.Status.STARTED
              && ingestion.equals(started.get(state.ingestionId()));
      if (!keep) {
        remove(state.ingestionId());
      }
    }
    for (Ingestion ingestion : wanted.values()) {
      if (!states.containsKey(ingestion.id())) {
        states.put(ingestion.id(), add(ingestion));
      }
    }
  }

  /**
   * Stops and forgets every started ingestion that matches, until a later reconcile lists it again.
   *
   * @param which the ingestions to stop
   * @return how many were stopped
   */
  synchronized int stop(Predicate<Ingestion> which) {
    List<String> ids = started.values().stream().filter(which).map(Ingestion::id).toList();
    ids.forEach(this::remove);
    return ids.size();
  }

  /**
   * The running ingestions with a client and a code. A code is unique within an application and
   * client, so two applications may each have one.
   *
   * @param application the application's code, or null for any
   * @param client the client's identifier or platform id
   * @param code the ingestion's code
   * @return the ingestions whose route is started: none, one, or several when the application was not named
   */
  synchronized List<Ingestion> find(@Nullable String application, String client, String code) {
    return started.values().stream()
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

  private RouteState add(Ingestion ingestion) {
    try {
      camel.addRoutes(routes.apply(ingestion));
      started.put(ingestion.id(), ingestion);
      LOG.info("started ingestion {} version {}", ingestion.id(), ingestion.version());
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.STARTED, null);
    } catch (Exception e) {
      LOG.warn("ingestion {} version {} failed to start", ingestion.id(), ingestion.version(), e);
      removeRoute(ingestion.id());
      String reason = ExchangeCounters.clean(e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage());
      return new RouteState(ingestion.id(), ingestion.version(), RouteState.Status.FAILED, reason);
    }
  }

  private void remove(String ingestionId) {
    health.forget(ingestionId);
    states.remove(ingestionId);
    started.remove(ingestionId);
    removeRoute(ingestionId);
  }

  private void removeRoute(String ingestionId) {
    // A camel-route ingestion's own route first, so nothing is sent to the routes behind it.
    for (String extra : List.of(ingestionId + IngestionRoute.SOURCE_ROUTE, ingestionId + IngestionRoute.FAILURES_ROUTE)) {
      if (camel.getRoute(extra) != null) {
        try {
          camel.getRouteController().stopRoute(extra);
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
      camel.getRouteController().stopRoute(ingestionId);
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

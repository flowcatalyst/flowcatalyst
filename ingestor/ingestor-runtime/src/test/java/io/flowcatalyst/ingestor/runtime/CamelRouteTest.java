package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import java.io.IOException;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import org.apache.camel.CamelContext;
import org.apache.camel.Route;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** A camel-route source: its own route from the YAML, in front of the ingestion's split and jobs. */
@Timeout(60)
class CamelRouteTest {

  static final String TICKS =
      """
      - from:
          uri: timer:orders
          parameters:
            period: 200
          steps:
            - setBody:
                constant: '[{"order":1},{"order":2}]'
      """;

  final List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  final SourceHealth health = new SourceHealth();
  final List<Failure> failures = new CopyOnWriteArrayList<>();
  volatile boolean platformDown;
  CamelContext camel;
  Reconciler reconciler;

  @BeforeEach
  void start() {
    camel = new DefaultCamelContext();
    camel.start();
    SourceEndpoints endpoints = new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null, null, Extensions.of(List.of(), true));
    JobSink sink =
        (ingestion, jobs) -> {
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, sink, counters, health), health);
  }

  @AfterEach
  void stop() {
    camel.stop();
  }

  private static Ingestion route(String id, long version, String yaml) {
    Ingestion i = SourceHarness.ingestion(id, Source.Custom.of("camel-route", Map.of("yaml", yaml)), new Split.Json("$"));
    return SourceHarness.ingestion(i.id(), version, i.source(), i.split(), i.target(), false);
  }

  private List<Failure> drained() {
    failures.addAll(counters.drain(InstantSource.system().instant()).failures());
    return new ArrayList<>(failures);
  }

  private List<String> routeIds() {
    return camel.getRoutes().stream().map(Route::getRouteId).sorted().toList();
  }

  @Test
  void aRoutesBodyIsSplitIntoJobsAndANewVersionReplacesItsRoutes() throws Exception {
    reconciler.reconcile(List.of(route("ing_r", 1, TICKS)));

    await(() -> batches.stream().mapToLong(List::size).sum() >= 4);
    assertThat(batches.getFirst()).extracting(JobRecord::payload).containsExactly("{\"order\":1}", "{\"order\":2}");
    assertThat(reconciler.states()).singleElement().extracting(RouteState::status).isEqualTo(RouteState.Status.STARTED);
    assertThat(routeIds()).containsExactly("ing_r", "ing_r-failures", "ing_r-source");

    reconciler.reconcile(List.of(route("ing_r", 2, TICKS.replace("\"order\":1", "\"order\":10"))));
    await(() -> batches.getLast().getFirst().payload().equals("{\"order\":10}"));
    assertThat(routeIds()).containsExactly("ing_r", "ing_r-failures", "ing_r-source");

    reconciler.reconcile(List.of());
    assertThat(routeIds()).isEmpty();
  }

  @Test
  void aStepThatThrowsIsTheSourcesFailureAndTheIngestionIsFailingUntilAPayloadGetsThrough() throws Exception {
    String failing =
        """
        - route:
            from:
              uri: timer:orders?period=200
              steps:
                - throwException:
                    exceptionType: java.lang.IllegalStateException
                    message: the orders service answered 500
        """;
    reconciler.reconcile(List.of(route("ing_f", 1, failing)));

    await(() -> reconciler.states().getFirst().status() == RouteState.Status.FAILING);
    assertThat(reconciler.states().getFirst().error()).isEqualTo("the orders service answered 500");
    await(() -> !drained().isEmpty());
    assertThat(drained()).first().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
      assertThat(f.message()).isEqualTo("the orders service answered 500");
    });
    assertThat(batches).isEmpty();

    // Saved again with a step that works: it gets through and is no longer failing.
    reconciler.reconcile(List.of(route("ing_f", 2, TICKS)));
    await(() -> !batches.isEmpty());
    await(() -> reconciler.states().getFirst().status() == RouteState.Status.STARTED);
  }

  @Test
  void jobsThePlatformDidNotTakeAreReportedOnceAndNotAsTheSourcesFailure() throws Exception {
    platformDown = true;
    reconciler.reconcile(List.of(route("ing_p", 1, TICKS)));

    await(() -> !drained().isEmpty());
    Thread.sleep(500);
    assertThat(drained()).extracting(Failure::kind).containsOnly(Failure.Kind.NOT_DISPATCHED);
    assertThat(reconciler.states().getFirst().status()).isEqualTo(RouteState.Status.STARTED);
  }

  @Test
  void aRouteThatIsNotOneRouteIsRefusedWithWhy() {
    String beans =
        """
        - beans:
            - name: anything
              type: java.lang.Object
        - from:
            uri: timer:x
        """;
    reconciler.reconcile(List.of(route("ing_b", 1, beans), route("ing_u", 1, "- from:\n    steps: []\n")));

    assertThat(reconciler.states()).extracting(RouteState::status).containsOnly(RouteState.Status.FAILED);
    assertThat(reconciler.states()).extracting(RouteState::error)
        .containsExactly("the route must be one route, not 2 elements", "the route from needs its uri, such as timer:tick?period=60000");
    assertThat(routeIds()).isEmpty();
  }

  @Test
  void aStepCanUseTheSimpleLanguageWithWhatTheTimerSets() throws Exception {
    String counted =
        """
        - from:
            uri: timer:counted?period=200&includeMetadata=true
            steps:
              - setBody:
                  simple: '[{"tick": ${header.CamelTimerCounter}}]'
        """;
    reconciler.reconcile(List.of(route("ing_c", 1, counted)));

    await(() -> !batches.isEmpty());
    assertThat(batches.getFirst()).extracting(JobRecord::payload).containsExactly("{\"tick\":1}");
  }

  @Test
  void theGuidesHeartbeatRouteRunsAsItIsWritten() throws Exception {
    // The heartbeat route of docs/camel-routes.md, with a beat every 200 ms.
    String yaml =
        """
        from:
          uri: timer:acme-heartbeat
          parameters:
            period: 60000
            includeMetadata: true
          steps:
            - setBody:
                simple: '[{"beat": ${header.CamelTimerCounter}, "at": "${date:now:yyyy-MM-dd''T''HH:mm:ssXXX}"}]'
        """;
    assertThat(yaml).contains("period: 60000");
    reconciler.reconcile(List.of(route("ing_h", 1, yaml.replace("period: 60000", "period: 200"))));

    await(() -> !batches.isEmpty());
    assertThat(batches.getFirst()).singleElement().extracting(JobRecord::payload).asString()
        .matches("\\{\"beat\":1,\"at\":\"\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(Z|[+-]\\d{2}:\\d{2})\"}");
  }

  @Test
  void anExchangeStoppedInAFilterIsNotDispatchedNorAFailure() throws Exception {
    // The guide's filter (docs/ingest/camel-routes.md), with a made-up answer in place of the API's.
    String filtered =
        """
        from:
          uri: timer:status
          parameters:
            period: 100
          steps:
            - setBody:
                constant: '[{"incident": "%s"}]'
            - filter:
                simple: "${body} not contains 'OPEN'"
                steps:
                  - stop: {}
        """;
    reconciler.reconcile(List.of(route("ing_f", 1, filtered.formatted("CLOSED"))));
    await(() -> camel.getRoutes().size() == 3);
    Thread.sleep(600);
    assertThat(batches).isEmpty();
    assertThat(drained()).isEmpty();

    reconciler.reconcile(List.of(route("ing_f", 2, filtered.formatted("OPEN"))));
    await(() -> !batches.isEmpty());
    assertThat(batches.getFirst()).extracting(JobRecord::payload).containsExactly("{\"incident\":\"OPEN\"}");
  }

  @Test
  void aRuntimeWithoutRoutesEnabledFailsTheIngestionAndDoesNotReportTheKind() {
    SourceEndpoints off = new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null, null, Extensions.none());
    Reconciler plain = new Reconciler(camel, i -> new IngestionRoute(i, off, (ingestion, jobs) -> {}, counters, health), health);

    plain.reconcile(List.of(route("ing_off", 1, "from:\n  uri: timer:x")));

    assertThat(plain.states()).singleElement().satisfies(state -> {
      assertThat(state.status()).isEqualTo(RouteState.Status.FAILED);
      assertThat(state.error()).contains("INGEST_CAMEL_ROUTES_ENABLED");
    });
    assertThat(Extensions.none().kinds()).isEmpty();
    assertThat(Extensions.of(List.of(), true).kinds())
        .singleElement()
        .satisfies(kind -> {
          assertThat(kind.kind()).isEqualTo("camel-route");
          assertThat(kind.requiredPermission()).isEqualTo("platform:ingestion:ingestion:route");
        });
  }
}

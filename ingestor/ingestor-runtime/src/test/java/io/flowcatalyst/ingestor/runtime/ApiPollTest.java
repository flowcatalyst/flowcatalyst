package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.ApiAuth;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.TestRequest;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.ApiPoll.ResultCheck;
import io.flowcatalyst.ingestor.contract.Split;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.core.MonotonicClock;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.framework.metrics.Metrics;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** An API poll against a small HTTP server: its logins, its cursor, and what it does when refused. */
@Timeout(60)
class ApiPollTest {

  HttpServer server;
  String base;
  HttpClients http;
  ServiceClients clients;
  /** Each client made by a test has a name of its own, as the runtime's do. */
  final AtomicInteger clientCount = new AtomicInteger();
  final AtomicInteger tokensIssued = new AtomicInteger();
  final AtomicInteger apiCalls = new AtomicInteger();
  final Set<String> validTokens = ConcurrentHashMap.newKeySet();
  final List<String> logins = new CopyOnWriteArrayList<>();
  final List<String> cursorsSent = new CopyOnWriteArrayList<>();
  volatile boolean refuseEverything;

  final Secrets secrets = SourceHarness.secrets(Map.of("api-secret", "s3cret", "api-password", "pw"));

  @BeforeEach
  void start() throws IOException {
    server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
    server.createContext("/oauth/token", this::token);
    server.createContext("/login", this::login);
    server.createContext("/v1/orders", this::orders);
    server.createContext("/v1/flagged", this::flagged);
    server.start();
    base = "http://127.0.0.1:" + server.getAddress().getPort();
    http = HttpClients.create();
    // The framework's own registry, as the runtime has it: a name registered twice is refused.
    clients = ServiceClients.create(http, Metrics.memory(1000, MonotonicClock.system()));
  }

  @AfterEach
  void stop() {
    server.stop(0);
    http.close();
  }

  private void token(HttpExchange exchange) throws IOException {
    String expected = "Basic " + Base64.getEncoder().encodeToString("ingest:s3cret".getBytes(StandardCharsets.UTF_8));
    String form = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
    if (!expected.equals(exchange.getRequestHeaders().getFirst("Authorization")) || !form.contains("grant_type=client_credentials")) {
      answer(exchange, 401, "{\"error\":\"invalid_client\"}", Map.of());
      return;
    }
    String token = "tok-" + tokensIssued.incrementAndGet();
    validTokens.add(token);
    answer(exchange, 200, "{\"access_token\":\"" + token + "\",\"token_type\":\"Bearer\",\"expires_in\":3600}", Map.of());
  }

  private void login(HttpExchange exchange) throws IOException {
    String body = new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
    logins.add(body);
    validTokens.add("L1");
    answer(exchange, 200, "{\"data\":{\"token\":\"L1\"},\"expires\":60}", Map.of());
  }

  private void orders(HttpExchange exchange) throws IOException {
    apiCalls.incrementAndGet();
    String authorization = String.valueOf(exchange.getRequestHeaders().getFirst("Authorization"));
    String presented = authorization.replaceFirst("^(Bearer|Token) ", "");
    if (refuseEverything || !validTokens.contains(presented)) {
      answer(exchange, 401, "{\"error\":\"unauthorized\"}", Map.of());
      return;
    }
    String query = String.valueOf(exchange.getRequestURI().getRawQuery());
    String after = query.contains("after=") ? query.replaceFirst(".*after=([^&]*).*", "$1") : "";
    cursorsSent.add(after);
    String body =
        switch (after) {
          case "" -> "{\"items\":[{\"id\":1},{\"id\":2}],\"next\":\"c2\"}";
          case "c2" -> "{\"items\":[{\"id\":3}],\"next\":\"c3\"}";
          default -> "{\"items\":[],\"next\":null}";
        };
    answer(exchange, 200, body, Map.of());
  }

  private void flagged(HttpExchange exchange) throws IOException {
    answer(exchange, 200, "[{\"id\":9}]", query(exchange).contains("more") ? Map.of("X-Has-More", "true") : Map.of());
  }

  private static String query(HttpExchange exchange) {
    return String.valueOf(exchange.getRequestURI().getRawQuery());
  }

  private static void answer(HttpExchange exchange, int status, String body, Map<String, String> headers) throws IOException {
    byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
    headers.forEach((k, v) -> exchange.getResponseHeaders().add(k, v));
    exchange.getResponseHeaders().add("Content-Type", "application/json");
    exchange.sendResponseHeaders(status, bytes.length);
    exchange.getResponseBody().write(bytes);
    exchange.close();
  }

  private ApiAuth.ClientCredentials clientCredentials() {
    return new ApiAuth.ClientCredentials(
        base + "/oauth/token", "ingest", new SecretRef("api-secret"), "orders:read", null, ApiAuth.ClientCredentials.CredentialsIn.BASIC_HEADER);
  }

  private Source.ApiPoll orders(ApiAuth auth, ResultCheck results) {
    return new Source.ApiPoll(
        base + "/v1/orders?status=open",
        Source.ApiPoll.Method.GET,
        List.of(new Source.ApiPoll.Header("Accept", "application/json")),
        null,
        auth,
        results,
        new Source.ApiPoll.Cursor("$.next", "after", null),
        Duration.ofSeconds(1),
        Duration.ofSeconds(1));
  }

  private static Ingestion ingestion(String id, Source source) {
    return SourceHarness.ingestion(id, 1, source, new Split.Json("$.items"), SourceHarness.target(Target.Mode.BLOCK_ON_ERROR, 10), false);
  }

  private ApiPollClient client(Ingestion ingestion) {
    return new ApiPollClient(ingestion, (Source.ApiPoll) ingestion.source(), secrets, "api-poll-test-" + clientCount.incrementAndGet(), clients, System::nanoTime);
  }

  @Test
  void anApiIsReadPageByPageAlongItsCursorWithOneTokenAndRestsWhenNothingIsNew() throws Exception {
    List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
    MemoryState state = new MemoryState();
    ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
    SourceHealth health = new SourceHealth();
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints = new CamelSourceEndpoints(secrets, state, null, clients);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> batches.add(jobs), counters, health), health);
      reconciler.reconcile(List.of(ingestion("ing_api", orders(clientCredentials(), new ResultCheck.JsonPath("$.items")))));

      await(() -> batches.stream().mapToLong(List::size).sum() == 3 && cursorsSent.contains("c3"));

      assertThat(batches.stream().flatMap(List::stream).map(JobRecord::payload)).containsExactly("{\"id\":1}", "{\"id\":2}", "{\"id\":3}");
      assertThat(state.values).containsEntry("ing_api/" + ApiPollEndpoint.CURSOR, "c3");
      assertThat(cursorsSent).startsWith("", "c2", "c3");
      assertThat(tokensIssued).hasValue(1);
      assertThat(reconciler.states()).containsExactly(new RouteState("ing_api", 1, RouteState.Status.STARTED, null));
    } finally {
      camel.stop();
    }
  }

  @Test
  void aRoutesClientsAreGivenBackWhenItIsReplacedOrStoppedAndATestHoldsItsOwn() throws Exception {
    List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
    ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
    SourceHealth health = new SourceHealth();
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints = new CamelSourceEndpoints(secrets, new MemoryState(), null, clients);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> batches.add(jobs), counters, health), health);
      Ingestion first = ingestion("ing_api", orders(clientCredentials(), new ResultCheck.JsonPath("$.items")));
      reconciler.reconcile(List.of(first));
      await(() -> !batches.isEmpty());
      assertThat(clients.names()).containsExactlyInAnyOrder("api-poll-ing-api-v1", "api-poll-ing-api-v1-token");

      // A test of the same ingestion while it runs: its own clients, given back when it is done.
            assertThat(test(first).outcome()).isEqualTo(io.flowcatalyst.ingestor.contract.ConnectionTest.Outcome.RESULTS);
      assertThat(clients.names()).containsExactlyInAnyOrder("api-poll-ing-api-v1", "api-poll-ing-api-v1-token");

      Ingestion saved =
          SourceHarness.ingestion("ing_api", 2, first.source(), first.split(), first.target(), false);
      reconciler.reconcile(List.of(saved));
      await(() -> clients.names().contains("api-poll-ing-api-v2"));
      assertThat(clients.names()).noneMatch(name -> name.contains("-v1"));

      reconciler.reconcile(List.of());
      await(() -> clients.names().isEmpty());
    } finally {
      camel.stop();
    }
  }

  @Test
  void aTokenTheApiRefusesIsReplacedOnceAndTheRequestSentAgain() {
    ApiPollClient client = client(ingestion("ing_api", orders(clientCredentials(), new ResultCheck.Records())));
    assertThat(client.poll(null)).isInstanceOf(ApiPollClient.Answer.class);

    validTokens.clear();
    ApiPollClient.Outcome again = client.poll(null);

    assertThat(again).isInstanceOf(ApiPollClient.Answer.class);
    assertThat(tokensIssued).hasValue(2);
    assertThat(again.steps()).anyMatch(s -> s.contains("fetching a new token"));
    // Nothing a person reads holds the secret or a token.
    assertThat(String.join("\n", again.steps())).doesNotContain("s3cret").doesNotContain("tok-");
  }

  @Test
  void aLoginTheApiKeepsRefusingIsReportedAndTriedOnlyEveryFiveMinutes() throws Exception {
    refuseEverything = true;
    ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
    SourceHealth health = new SourceHealth();
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      SourceEndpoints endpoints = new CamelSourceEndpoints(secrets, new MemoryState(), null, clients);
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, endpoints, (ingestion, jobs) -> {}, counters, health), health);
      reconciler.reconcile(List.of(ingestion("ing_api", orders(clientCredentials(), new ResultCheck.Records()))));

      await(() -> reconciler.states().getFirst().status() == RouteState.Status.FAILING);
      int calls = apiCalls.get();
      // With a 1 s idle delay it would have asked twice more by now.
      Thread.sleep(2500);

      assertThat(apiCalls.get()).isEqualTo(calls);
      // Asked with the token, then with a fresh one: both refused.
      assertThat(calls).isEqualTo(2);
      assertThat(reconciler.states().getFirst().error()).contains("the API refused the login: 401");
      List<Failure> failures = counters.drain(InstantSource.system().instant()).failures();
      assertThat(failures).singleElement().satisfies(f -> {
        assertThat(f.kind()).isEqualTo(Failure.Kind.AUTH);
        assertThat(f.message()).contains("tried again every 5 minutes, or at once when the ingestion is saved");
      });

      // Saved again: a new version is a new route, which asks at once.
      refuseEverything = false;
      Ingestion saved =
          SourceHarness.ingestion("ing_api", 2, orders(clientCredentials(), new ResultCheck.Records()), new Split.Json("$.items"),
              ingestion("x", orders(clientCredentials(), new ResultCheck.Records())).target(), false);
      reconciler.reconcile(List.of(saved));
      await(() -> apiCalls.get() > calls);
      await(() -> reconciler.states().getFirst().status() == RouteState.Status.STARTED);
    } finally {
      camel.stop();
    }
  }

  @Test
  void anApisOwnLoginIsPostedAsJsonAndItsTokenSentWithThePrefixGiven() {
    ApiAuth.Login login =
        new ApiAuth.Login(
            base + "/login", ApiAuth.Login.Format.JSON, "user", "pass", "ingest", new SecretRef("api-password"),
            Map.of("grant", "password"), "$.data.token", "$.expires", Duration.ofMinutes(10), "Authorization", "Token ");
    ApiPollClient client = client(ingestion("ing_api", orders(login, new ResultCheck.JsonPath("$.items"))));

    ApiPollClient.Outcome outcome = client.poll(null);

    assertThat(outcome).isInstanceOfSatisfying(ApiPollClient.Answer.class, a -> {
      assertThat(a.hasResults()).isTrue();
      assertThat(a.cursor()).isEqualTo("c2");
    });
    assertThat(logins).singleElement().satisfies(body -> assertThat(body).contains("\"user\":\"ingest\"", "\"pass\":\"pw\"", "\"grant\":\"password\""));
    assertThat(outcome.steps()).anyMatch(s -> s.contains("kept for 60 s"));
    assertThat(client.poll(null).steps()).anyMatch(s -> s.contains("Using the token already held"));
  }

  @Test
  void aHeaderCanSayWhetherAnAnswerHasResults() {
    Source.ApiPoll more =
        new Source.ApiPoll(base + "/v1/flagged?more=1", Source.ApiPoll.Method.GET, null, null, new ApiAuth.None(),
            new ResultCheck.Header("X-Has-More", "TRUE"), null, Duration.ofSeconds(1), Duration.ofSeconds(1));
    Source.ApiPoll none =
        new Source.ApiPoll(base + "/v1/flagged", Source.ApiPoll.Method.GET, null, null, new ApiAuth.None(),
            new ResultCheck.Header("X-Has-More", null), null, Duration.ofSeconds(1), Duration.ofSeconds(1));

    assertThat(client(ingestion("a", more)).poll(null)).isInstanceOfSatisfying(ApiPollClient.Answer.class, a -> assertThat(a.hasResults()).isTrue());
    assertThat(client(ingestion("b", none)).poll(null)).isInstanceOfSatisfying(ApiPollClient.Answer.class, a -> assertThat(a.hasResults()).isFalse());
  }

  @Test
  void aWrongClientSecretIsARefusalAndAnUnsetSecretStopsTheRouteBeingBuilt() {
    ApiAuth.ClientCredentials wrong =
        new ApiAuth.ClientCredentials(base + "/oauth/token", "ingest", new SecretRef("api-password"), null, null, ApiAuth.ClientCredentials.CredentialsIn.BASIC_HEADER);
    assertThat(client(ingestion("a", orders(wrong, new ResultCheck.Records()))).poll(null))
        .isInstanceOfSatisfying(ApiPollClient.Refused.class, r -> assertThat(r.message()).contains("refused the credentials: 401"));

    ApiAuth.Bearer unset = new ApiAuth.Bearer(new SecretRef("missing"));
    assertThatThrownBy(() -> client(ingestion("b", orders(unset, new ResultCheck.Records()))))
        .isInstanceOf(UnsupportedIngestionException.class)
        .hasMessageContaining("missing");
    assertThat(new ArrayList<>(cursorsSent)).isEmpty();
  }

  private TestRunner probe() {
    return new TestRunner(secrets, clients, Extensions.none(), new DefaultCamelContext(), System::nanoTime);
  }

  private io.flowcatalyst.ingestor.contract.ConnectionTest test(Ingestion ingestion) {
    return probe().apply(new TestRequest("tst_1", Instant.now().plusSeconds(60), ingestion));
  }

  private static Ingestion keeping(Ingestion i) {
    return SourceHarness.ingestion(i.id(), i.version(), i.source(), i.split(), i.target(), true);
  }

  @Test
  void testingAConnectionLogsInAndAsksOnceAndSaysWhatTheSplitWouldMakeOfIt() {
    Ingestion ingestion = ingestion("unsaved", orders(clientCredentials(), new ResultCheck.JsonPath("$.items")));

    io.flowcatalyst.ingestor.contract.ConnectionTest test = test(ingestion);

    assertThat(test.outcome()).isEqualTo(io.flowcatalyst.ingestor.contract.ConnectionTest.Outcome.RESULTS);
    assertThat(test.records()).isEqualTo(2);
    assertThat(test.id()).isEqualTo("tst_1");
    assertThat(test.steps()).anyMatch(s -> s.startsWith("Fetched a token")).anyMatch(s -> s.contains("$.items holds 2 entries"));
    assertThat(String.join("\n", test.steps())).doesNotContain("s3cret").doesNotContain("tok-");
    // Nothing of the answer comes back unless the ingestion keeps payloads.
    assertThat(test.excerpt()).isNull();
    assertThat(test(keeping(ingestion)).excerpt()).startsWith("{\"items\":[{\"id\":1}");
    assertThat(cursorsSent).containsOnly("");
  }

  @Test
  void aTestSaysWhenTheLoginIsRefusedOrASecretIsNotSetOrTheSourceCannotBeTriedYet() {
    refuseEverything = true;
    assertThat(test(ingestion("a", orders(clientCredentials(), new ResultCheck.Records()))).outcome())
        .isEqualTo(io.flowcatalyst.ingestor.contract.ConnectionTest.Outcome.REFUSED);
    assertThat(test(ingestion("b", orders(new ApiAuth.Bearer(new SecretRef("missing")), new ResultCheck.Records()))))
        .satisfies(t -> {
          assertThat(t.outcome()).isEqualTo(io.flowcatalyst.ingestor.contract.ConnectionTest.Outcome.FAILED);
          assertThat(t.steps()).singleElement().satisfies(step -> assertThat(step).contains("missing"));
        });
    assertThat(test(SourceHarness.ingestion("c", new Source.Sqs("eu-west-1", "q", null), new Split.None())).outcome())
        .isEqualTo(io.flowcatalyst.ingestor.contract.ConnectionTest.Outcome.NOT_SUPPORTED);
  }
}

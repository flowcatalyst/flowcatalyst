package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.TraceRecord;
import io.flowcatalyst.ingestor.contract.TraceWindow;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.PolledSource;
import io.flowcatalyst.ingestor.extension.Settings;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import tools.jackson.databind.JsonNode;

/**
 * A polled source's poll, run with tracing on, against a small HTTP server that logs in the way MiX
 * does: the poll's calls, the payload it dispatched and the poll's end are written as records that
 * share a capture, and no credential reaches them.
 */
@Timeout(60)
class TraceCaptureTest {

  static final String PASSWORD = "hunter2";
  static final String CLIENT_SECRET = "shh-client-secret";
  static final String TOKEN = "tok-123456-abcdef";

  final TestClock clock = new TestClock(Instant.parse("2026-10-30T10:00:00Z"));
  final List<TraceRecord> shipped = new CopyOnWriteArrayList<>();
  final List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
  final ExchangeCounters counters = new ExchangeCounters(clock);
  final TraceWindows windows = new TraceWindows(clock, shipped::add, id -> List.of(PASSWORD));
  HttpServer server;
  HttpClients http;
  CamelContext camel;

  @BeforeEach
  void start() throws Exception {
    server = HttpServer.create(new InetSocketAddress(InetAddress.getLoopbackAddress(), 0), 0);
    server.createContext(
        "/token",
        exchange -> {
          exchange.getRequestBody().readAllBytes();
          byte[] out = ("{\"access_token\":\"" + TOKEN + "\",\"token_type\":\"Bearer\",\"expires_in\":3600}").getBytes(StandardCharsets.UTF_8);
          exchange.getResponseHeaders().add("Content-Type", "application/json");
          exchange.sendResponseHeaders(200, out.length);
          exchange.getResponseBody().write(out);
          exchange.close();
        });
    server.createContext(
        "/data",
        exchange -> {
          byte[] out = "{\"items\":[1,2]}".getBytes(StandardCharsets.UTF_8);
          exchange.getResponseHeaders().add("Content-Type", "application/json");
          exchange.sendResponseHeaders(String.valueOf(exchange.getRequestURI().getQuery()).contains("fail") ? 500 : 200, out.length);
          exchange.getResponseBody().write(out);
          exchange.close();
        });
    server.start();
    http = HttpClients.create();
    camel = new DefaultCamelContext();
    camel.start();
  }

  @AfterEach
  void stop() {
    camel.stop();
    http.close();
    server.stop(0);
  }

  private Ingestion ingestion(Split split) {
    return SourceHarness.ingestion("ing_1", Source.Custom.of("ticker", Map.of()), split);
  }

  private void switchOn(int sampleEverySecs) {
    windows.update(List.of(new TraceWindow("ing_1", "itw_1", 900, sampleEverySecs, clock.instant().plus(Duration.ofHours(1)))));
  }

  private ExtensionContext context(Ingestion ingestion) {
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

  private ServiceClient mix() {
    return http.service(ServiceOptions.builder("mix", URI.create("http://127.0.0.1:" + server.getAddress().getPort())).requestTimeout(Duration.ofSeconds(5)).build());
  }

  private List<TraceRecord> ofKind(String kind) {
    return shipped.stream().filter(r -> r.kind().equals(kind)).toList();
  }

  private static JsonNode json(TraceRecord record) {
    return Json.mapper().valueToTree(record.data());
  }

  @Test
  void aPollWithTracingOnRecordsItsCallsItsPayloadAndItsEndAndCountsEveryPollEmptyOnesToo() throws Exception {
    switchOn(1);
    Ingestion ingestion = ingestion(new Split.Csv(false));
    AtomicInteger polls = new AtomicInteger();
    PolledSource source =
        new PolledSource(context(ingestion), "ticker") {
          @Override
          protected Duration poll(Payloads payloads) throws Exception {
            // Each poll is a sample interval after the last.
            clock.advance(Duration.ofSeconds(2));
            int n = polls.incrementAndGet();
            ServiceClient client = mix();
            client.post("/token").form(Map.of("grant_type", "password", "username", "fleet", "password", PASSWORD, "client_secret", CLIENT_SECRET)).send();
            client.get("/data").header("Authorization", "Bearer " + TOKEN).query("page", "1").send();
            if (n == 1) {
              payloads.dispatch("A\nB\n", "ticker:page-1");
            }
            return n < 3 ? Duration.ofMillis(50) : Duration.ofHours(1);
          }

          @Override
          protected Duration retryDelay() {
            return Duration.ofMillis(10);
          }
        };
    Reconciler reconciler =
        new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> batches.add(jobs), counters, new SourceHealth(), windows));

    reconciler.reconcile(List.of(ingestion));

    await(() -> ofKind("poll").size() == 3);
    List<TraceRecord> first = shipped.stream().filter(r -> r.captureId().equals(ofKind("poll").getFirst().captureId())).toList();
    assertThat(first).extracting(TraceRecord::kind).containsExactly("http", "http", "exchange", "poll");
    assertThat(first).extracting(TraceRecord::windowId).containsOnly("itw_1");
    assertThat(first).extracting(TraceRecord::ingestionId).containsOnly("ing_1");

    JsonNode login = json(first.get(0));
    assertThat(login.get("client").asString()).isEqualTo("mix");
    assertThat(login.get("request").get("method").asString()).isEqualTo("POST");
    assertThat(login.get("request").get("url").asString()).endsWith("/token");
    assertThat(login.get("request").get("body").asString()).contains("grant_type=password", "username=fleet").doesNotContain(PASSWORD, CLIENT_SECRET);
    assertThat(login.get("response").get("status").asInt()).isEqualTo(200);
    assertThat(Json.mapper().readTree(login.get("response").get("body").asString()).get("access_token").asString()).isEqualTo("[redacted]");
    assertThat(login.get("attempt").asInt()).isEqualTo(1);

    JsonNode data = json(first.get(1));
    assertThat(data.get("request").get("url").asString()).endsWith("/data?page=1");
    assertThat(data.get("request").get("headers").get("Authorization").get(0).asString()).isEqualTo("[redacted]");
    assertThat(data.get("response").get("body").asString()).contains("items");

    JsonNode exchange = json(first.get(2));
    assertThat(exchange.get("outcome").asString()).isEqualTo("completed");
    assertThat(exchange.get("identity").asString()).isEqualTo("ticker:page-1");
    assertThat(exchange.get("records").asInt()).isEqualTo(2);
    assertThat(exchange.get("jobs").asInt()).isEqualTo(2);
    assertThat(exchange.get("payload").get("body").asString()).isEqualTo("A\nB\n");
    assertThat(exchange.get("payloadBytes").asInt()).isEqualTo(4);

    JsonNode poll = json(first.get(3));
    assertThat(poll.get("httpCalls").asInt()).isEqualTo(2);
    assertThat(poll.get("payloads").asInt()).isEqualTo(1);
    assertThat(poll.get("records").asInt()).isEqualTo(2);
    assertThat(poll.get("jobs").asInt()).isEqualTo(2);
    assertThat(poll.get("outcome").asString()).isEqualTo("ok");
    assertThat(poll.get("nextDelayMs").asInt()).isEqualTo(50);

    // The empty polls are recorded too: their calls show why nothing came.
    for (TraceRecord empty : ofKind("poll").subList(1, 3)) {
      assertThat(json(empty).get("payloads").asInt()).isZero();
      assertThat(json(empty).get("httpCalls").asInt()).isEqualTo(2);
    }
    assertThat(ofKind("exchange")).hasSize(1);

    // Nothing sensitive anywhere in what would be shipped, the secret value fetched for the ingestion included.
    String everything = Json.mapper().writeValueAsString(shipped);
    assertThat(everything).doesNotContain(PASSWORD).doesNotContain(CLIENT_SECRET).doesNotContain(TOKEN);

    ExchangeCounters.Drained drained = counters.drain(clock.instant());
    assertThat(drained.windows()).singleElement().satisfies(w -> {
      assertThat(w.polls()).isEqualTo(3);
      assertThat(w.lastPollAt()).isNotNull();
      assertThat(w.completed()).isEqualTo(1);
    });
  }

  @Test
  void aPollThatThrowsIsRecordedAsFailedWithItsCallAndStillCounted() throws Exception {
    switchOn(1);
    Ingestion ingestion = ingestion(new Split.None());
    PolledSource source =
        new PolledSource(context(ingestion), "ticker") {
          @Override
          protected Duration poll(Payloads payloads) {
            mix().get("/data").query("fail", "1").send();
            throw new IllegalStateException("source down");
          }

          @Override
          protected Duration retryDelay() {
            return Duration.ofHours(1);
          }
        };
    Reconciler reconciler =
        new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> batches.add(jobs), counters, new SourceHealth(), windows));

    reconciler.reconcile(List.of(ingestion));

    await(() -> ofKind("poll").size() == 1);
    TraceRecord call = ofKind("http").getFirst();
    assertThat(call.level()).isEqualTo("warn");
    assertThat(json(call).get("response").get("status").asInt()).isEqualTo(500);
    JsonNode poll = json(ofKind("poll").getFirst());
    assertThat(poll.get("outcome").asString()).isEqualTo("failed");
    assertThat(poll.get("error").asString()).isEqualTo("source down");
    assertThat(ofKind("poll").getFirst().level()).isEqualTo("error");
    assertThat(counters.drain(clock.instant()).windows()).singleElement().satisfies(w -> assertThat(w.polls()).isEqualTo(1));
  }

  @Test
  void withTracingOffAPollRecordsNothingButIsCounted() throws Exception {
    Ingestion ingestion = ingestion(new Split.None());
    AtomicInteger polls = new AtomicInteger();
    PolledSource source =
        new PolledSource(context(ingestion), "ticker") {
          @Override
          protected Duration poll(Payloads payloads) throws Exception {
            mix().get("/data").send();
            payloads.dispatch("x", "id:" + polls.incrementAndGet());
            return Duration.ofHours(1);
          }

          @Override
          protected Duration retryDelay() {
            return Duration.ofHours(1);
          }
        };
    Reconciler reconciler =
        new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> batches.add(jobs), counters, new SourceHealth(), windows));

    reconciler.reconcile(List.of(ingestion));

    await(() -> !batches.isEmpty());
    await(() -> counters.drain(clock.instant()).windows().stream().anyMatch(w -> w.polls() == 1));
    assertThat(shipped).isEmpty();
  }

  @Test
  void anExchangeOutsideAPollIsSampledOnItsOwnAndRecordsItsOutcome() throws Exception {
    switchOn(5);
    Ingestion ingestion = ingestion(new Split.Json("$"));
    Reconciler reconciler =
        new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> c.getEndpoint("direct:" + x.id()), (x, jobs) -> batches.add(jobs), counters, new SourceHealth(), windows));
    reconciler.reconcile(List.of(ingestion));
    try (var producer = camel.createProducerTemplate()) {
      producer.sendBodyAndHeader("direct:ing_1", "[{\"a\":1},{\"a\":2}]", "CamelFileName", "orders.json");
      // Within the sample interval: not captured.
      producer.sendBody("direct:ing_1", "[{\"a\":3}]");
      clock.advance(Duration.ofSeconds(5));
      // Not JSON: rejected, and captured.
      producer.sendBody("direct:ing_1", "{not json");
    }

    await(() -> ofKind("exchange").size() == 2);
    TraceRecord done = ofKind("exchange").get(0);
    TraceRecord rejected = ofKind("exchange").get(1);
    assertThat(done.captureId()).isNotEqualTo(rejected.captureId());
    assertThat(json(done).get("outcome").asString()).isEqualTo("completed");
    assertThat(json(done).get("records").asInt()).isEqualTo(2);
    assertThat(json(done).get("source").get("fileName").asString()).isEqualTo("orders.json");
    assertThat(json(rejected).get("outcome").asString()).isEqualTo("rejected");
    assertThat(json(rejected).get("error").asString()).isNotBlank();
    assertThat(rejected.level()).isEqualTo("warn");
    assertThat(ofKind("poll")).isEmpty();
    assertThat(batches).hasSize(2);
  }
}

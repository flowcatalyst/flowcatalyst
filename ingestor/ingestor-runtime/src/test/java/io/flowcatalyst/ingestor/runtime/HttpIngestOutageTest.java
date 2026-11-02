package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.TestOidcProvider;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.http.TestClient;
import io.flowcatalyst.ingestor.contract.HttpAuth;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.atomic.AtomicInteger;
import java.util.concurrent.atomic.AtomicLong;
import org.apache.camel.CamelContext;
import org.apache.camel.ProducerTemplate;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;

/** The HTTP listener while the platform's secrets are unreachable (docs/ingestor-failure-pacing.md, R7). */
class HttpIngestOutageTest {

  private record Line(String message, @Nullable Throwable cause) {}

  @Test
  void manyRequestsDuringASecretsOutageMakeOneRevealPerTenSecondsAndAtMostTwoLogLinesAMinute() throws Exception {
    TestClock clock = new TestClock(Instant.parse("2026-11-02T10:00:00Z"));
    AtomicLong nanos = new AtomicLong();
    AtomicInteger reveals = new AtomicInteger();
    List<Line> lines = new ArrayList<>();
    FailureLog failures = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), clock);
    SecretSource down =
        (ingestion, name) -> {
          reveals.incrementAndGet();
          throw new IOException("connection refused");
        };
    Secrets secrets = new Secrets(down, name -> null, Duration.ofMinutes(1), nanos::get);
    TestOidcProvider platform = TestOidcProvider.start(InstantSource.system());
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try (ProducerTemplate producer = camel.createProducerTemplate()) {
      Reconciler reconciler =
          new Reconciler(
              camel,
              i -> new IngestionRoute(i, new CamelSourceEndpoints(SourceHarness.secrets(java.util.Map.of()), new MemoryState(), null), (x, jobs) -> {}, new ExchangeCounters(clock)));
      Ingestion ingestion =
          SourceHarness.ingestion(
              "ing_bearer",
              "logistics",
              "orders",
              1,
              new Source.Http(new HttpAuth.Bearer(new SecretRef("token")), null, null),
              new Split.Json("$"),
              SourceHarness.target(Target.Mode.NEXT_ON_ERROR, 100),
              false);
      reconciler.reconcile(List.of(ingestion));
      HttpIngest ingest =
          new HttpIngest(
              reconciler,
              secrets,
              HttpIngest.limiter(nanos::get),
              producer,
              PlatformTokens.createAllowingPlainHttp(platform.issuer(), Duration.ofSeconds(5), InstantSource.system()),
              nanos::get,
              failures);
      Routes routes = new Routes();
      routes.post(HttpIngest.PATTERN, ingest::handle);
      TestClient http = TestClient.of(routes);

      for (int i = 0; i < 200; i++) {
        assertThat(http.post("/ingest/acme/orders").header("Authorization", "Bearer x").body("[]").send().status()).isEqualTo(503);
        nanos.addAndGet(Duration.ofMillis(40).toNanos()); // 8 seconds in all
        clock.advance(Duration.ofMillis(40));
      }
      assertThat(reveals.get()).isEqualTo(1);
      assertThat(lines).hasSize(1);
      assertThat(lines.getFirst().message()).contains("ing_bearer cannot check credentials");

      // A minute on: the next request asks the platform again, and the log says how many it did not write.
      nanos.addAndGet(Duration.ofSeconds(60).toNanos());
      clock.advance(Duration.ofSeconds(60));
      assertThat(http.post("/ingest/acme/orders").header("Authorization", "Bearer x").body("[]").send().status()).isEqualTo(503);
      assertThat(reveals.get()).isEqualTo(2);
      assertThat(lines).hasSize(2);
      assertThat(lines.getLast().message()).contains("more like it not logged");

    } finally {
      camel.stop();
      platform.close();
    }
  }
}

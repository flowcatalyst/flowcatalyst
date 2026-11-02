package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.extension.PolledSource;
import io.flowcatalyst.ingestor.extension.Settings;
import java.io.IOException;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/** An extension's polled source: its polls keep coming whatever one throws, and it can say where a payload came from. */
class PolledSourceTest {

  @Test
  void aPollThatThrowsAnErrorIsReportedAndTheNextPollStillComesAndAPayloadCanCarryItsIdentity() throws Exception {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      Ingestion ingestion = SourceHarness.ingestion("ing_ext", Source.Custom.of("ticker", Map.of()), new Split.Csv(false));
      AtomicInteger polls = new AtomicInteger();
      ExtensionContext context =
          new ExtensionContext() {
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
      PolledSource source =
          new PolledSource(context, "ticker") {
            @Override
            protected Duration poll(Payloads payloads) throws Exception {
              if (polls.incrementAndGet() == 1) {
                throw new NoClassDefFoundError("io/example/Missing");
              }
              payloads.dispatch("A\nB\n", "ticker:page-" + polls.get());
              return Duration.ofHours(1);
            }

            @Override
            protected Duration retryDelay() {
              return Duration.ofMillis(10);
            }
          };
      List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
      ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> batches.add(jobs), counters));

      reconciler.reconcile(List.of(ingestion));

      await(() -> !batches.isEmpty());
      assertThat(polls.get()).isEqualTo(2);
      assertThat(batches.getFirst()).extracting(JobRecord::externalId)
          .containsExactly(Identities.externalId("ticker:page-2", 0), Identities.externalId("ticker:page-2", 1));
      assertThat(counters.drain(InstantSource.system().instant()).failures())
          .singleElement()
          .satisfies(f -> {
            assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
            assertThat(f.message()).contains("io/example/Missing");
          });
    } finally {
      camel.stop();
    }
  }

  private static ExtensionContext context(CamelContext camel, Ingestion ingestion) {
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

  private record Line(String message, @Nullable Throwable cause) {}

  /** A sink the platform is not taking jobs through: the route's submit fails at once, as when the runtime-wide hold is on. */
  private static final JobSink HELD =
      new JobSink() {
        @Override
        public void create(Ingestion ingestion, List<JobRecord> records) {}

        @Override
        public void admit() throws IOException {
          throw new PlatformHeldException("the platform is not taking jobs; held until later");
        }
      };

  // R5 of docs/ingestor-failure-pacing.md.

  @Test
  @Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
  void anExtensionThatReturnsZeroAfterAPayloadThePlatformDidNotTakeIsPacedAndItsFailureHasNoStackTrace() throws Exception {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      Ingestion ingestion = SourceHarness.ingestion("ing_zero", Source.Custom.of("ticker", Map.of()), new Split.None());
      List<Long> polledAt = new CopyOnWriteArrayList<>();
      List<Line> lines = new CopyOnWriteArrayList<>();
      PolledSource source =
          new PolledSource(context(camel, ingestion), "ticker") {
            @Override
            protected Duration poll(Payloads payloads) throws Exception {
              polledAt.add(System.nanoTime());
              assertThat(payloads.dispatch("{\"a\":1}")).isFalse();
              return Duration.ZERO;
            }

            @Override
            protected Duration retryDelay() {
              return Duration.ofMillis(10);
            }
          };
      source.pacing(new Backoff(Duration.ofMillis(10), Duration.ofMillis(40)), Backoff.REFUSED_LOGIN);
      source.failureLog(new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), InstantSource.system()));
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, HELD, new ExchangeCounters(InstantSource.system())));

      reconciler.reconcile(List.of(ingestion));

      await(() -> polledAt.size() >= 3);
      // Returned zero, it would be polled again in 250 ms if it had succeeded; a failed poll waits a second at least.
      assertThat(Duration.ofNanos(polledAt.get(2) - polledAt.get(1))).isGreaterThanOrEqualTo(Duration.ofMillis(950));
      assertThat(Duration.ofNanos(polledAt.get(1) - polledAt.get(0))).isGreaterThanOrEqualTo(Duration.ofMillis(950));
      // Nothing was thrown: the extension logs nothing for a payload the route has already reported.
      assertThat(lines).isEmpty();
    } finally {
      camel.stop();
    }
  }

  @Test
  @Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
  void aRefusedLoginIsReportedAsAuthSaysWhenItTriesAgainAndIsNotTriedAgainSoon() throws Exception {
    CamelContext camel = new DefaultCamelContext();
    camel.start();
    try {
      Ingestion ingestion = SourceHarness.ingestion("ing_login", Source.Custom.of("ticker", Map.of()), new Split.None());
      AtomicInteger polls = new AtomicInteger();
      List<Line> lines = new CopyOnWriteArrayList<>();
      PolledSource source =
          new PolledSource(context(camel, ingestion), "ticker") {
            @Override
            protected Duration poll(Payloads payloads) {
              polls.incrementAndGet();
              throw new HttpClientException(HttpClientException.Kind.AUTH, "the token endpoint answered 401", null);
            }

            @Override
            protected Duration retryDelay() {
              return Duration.ofMillis(10);
            }
          };
      source.failureLog(new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), InstantSource.system()));
      ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
      Reconciler reconciler = new Reconciler(camel, i -> new IngestionRoute(i, (c, x) -> source, (x, jobs) -> {}, counters));

      reconciler.reconcile(List.of(ingestion));

      await(() -> polls.get() == 1 && !lines.isEmpty());
      Thread.sleep(1500);
      assertThat(polls.get()).isEqualTo(1);
      assertThat(counters.drain(InstantSource.system().instant()).failures()).singleElement().satisfies(f -> {
        assertThat(f.kind()).isEqualTo(Failure.Kind.AUTH);
        assertThat(f.message()).contains("tries again at");
      });
      assertThat(lines).singleElement().satisfies(l -> {
        assertThat(l.message()).contains("login was refused");
        assertThat(l.cause()).isNull();
      });
    } finally {
      camel.stop();
    }
  }
}

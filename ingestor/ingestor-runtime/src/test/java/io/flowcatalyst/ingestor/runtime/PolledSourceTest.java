package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.httpclient.module.ServiceClients;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.ExtensionContext;
import io.flowcatalyst.ingestor.extension.PolledSource;
import io.flowcatalyst.ingestor.extension.Settings;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.Test;

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
}

package io.flowcatalyst.ingestor.runtime;

import java.util.Map;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.Target;
import java.io.IOException;
import java.net.URI;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.BooleanSupplier;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;

/** What every source's test needs: a Camel context, a stand-in platform, and the real endpoints. */
final class SourceHarness implements AutoCloseable {

  final List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
  final MemoryState state = new MemoryState();
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  final CamelContext camel = new DefaultCamelContext();
  final Reconciler reconciler;
  volatile boolean platformDown;

  SourceHarness(Secrets secrets, URI awsEndpoint) {
    camel.start();
    JobSink sink =
        (ingestion, jobs) -> {
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    SourceEndpoints endpoints = new CamelSourceEndpoints(secrets, state, awsEndpoint);
    reconciler = new Reconciler(camel, ingestion -> new IngestionRoute(ingestion, endpoints, sink, counters));
  }

  /** The secrets of client {@code acme}, by name; no other client has any. */
  static Secrets secrets(Map<String, String> values) {
    return Secrets.of(values);
  }

  static Target target(Target.Mode mode, int batchSize) {
    return new Target("logistics:orders:record:map", "https://f.example/map", "pool_1", null, null, mode, batchSize);
  }

  static Ingestion ingestion(String id, Source source, Split split) {
    return ingestion(id, 1, source, split, target(Target.Mode.BLOCK_ON_ERROR, 2), false);
  }

  /** An ingestion of client {@code acme} (platform id {@code clt_acme}) of the {@code logistics} application, its code its id. */
  static Ingestion ingestion(String id, long version, Source source, Split split, Target target, boolean keepFailedPayloads) {
    return ingestion(id, "logistics", id, version, source, split, target, keepFailedPayloads);
  }

  static Ingestion ingestion(
      String id, String application, String code, long version, Source source, Split split, Target target, boolean keepFailedPayloads) {
    return new Ingestion(id, version, "clt_acme", "acme", application, code, code, source, split, target, keepFailedPayloads);
  }

  List<String> payloads() {
    return batches.stream().flatMap(List::stream).map(JobRecord::payload).toList();
  }

  /** How many exchanges failed since this was last asked. */
  long failedSinceLastAsked() {
    return counters.drain(InstantSource.system().instant()).windows().stream().mapToLong(w -> w.failed()).sum();
  }

  static void await(BooleanSupplier condition) throws InterruptedException {
    long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
    while (!condition.getAsBoolean()) {
      assertThat(System.nanoTime()).as("waited 30 s").isLessThan(deadline);
      Thread.sleep(200);
    }
  }

  @Override
  public void close() {
    camel.stop();
  }
}

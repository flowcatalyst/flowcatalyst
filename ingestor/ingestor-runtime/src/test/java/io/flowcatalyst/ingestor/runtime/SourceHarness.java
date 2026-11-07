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
import java.util.function.UnaryOperator;
import org.apache.camel.CamelContext;
import org.apache.camel.Endpoint;
import org.apache.camel.impl.DefaultCamelContext;

/** What every source's test needs: a Camel context, a stand-in platform, and the real endpoints. */
final class SourceHarness implements AutoCloseable {

  final List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
  /** For each batch sent, whether the thread that sent it (the route's, which consumed the payload) was virtual. */
  final List<Boolean> virtual = new CopyOnWriteArrayList<>();
  final MemoryState state = new MemoryState();
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  final SourceHealth health = new SourceHealth();
  final CamelContext camel = new DefaultCamelContext();
  final Reconciler reconciler;
  volatile boolean platformDown;
  /** The platform is not taking jobs and the runtime knows it (R2): a batch fails at once, without retries. */
  volatile boolean platformHeld;

  SourceHarness(Secrets secrets, URI awsEndpoint) {
    this(secrets, awsEndpoint, Pacing.system(), UnaryOperator.identity());
  }

  /**
   * A harness whose routes pace and log by {@code pacing}, and whose endpoints {@code adjust} may change
   * before the route starts (give one a client of the test's own, say).
   */
  SourceHarness(Secrets secrets, URI awsEndpoint, Pacing pacing, UnaryOperator<Endpoint> adjust) {
    camel.start();
    JobSink sink =
        (ingestion, jobs) -> {
          virtual.add(Thread.currentThread().isVirtual());
          if (platformHeld) {
            throw new PlatformHeldException("the platform is not taking jobs");
          }
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    CamelSourceEndpoints real = new CamelSourceEndpoints(secrets, state, awsEndpoint);
    SourceEndpoints endpoints =
        new SourceEndpoints() {
          @Override
          public Endpoint endpoint(CamelContext context, Ingestion ingestion) {
            return adjust.apply(real.endpoint(context, ingestion));
          }

          @Override
          public Runnable guard(Ingestion ingestion) {
            return real.guard(ingestion);
          }
        };
    reconciler =
        new Reconciler(
            camel, ingestion -> new IngestionRoute(ingestion, endpoints, sink, counters, health, TraceWindows.disabled(), pacing), health);
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

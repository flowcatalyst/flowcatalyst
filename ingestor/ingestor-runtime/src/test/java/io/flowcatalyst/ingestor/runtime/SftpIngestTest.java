package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import java.io.IOException;
import java.time.Duration;
import java.time.InstantSource;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.function.BooleanSupplier;
import org.apache.camel.CamelContext;
import org.apache.camel.impl.DefaultCamelContext;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIf;
import org.testcontainers.containers.GenericContainer;
import org.testcontainers.images.builder.Transferable;

/** The sftp source against a real SFTP server in a container. */
@EnabledIf("io.flowcatalyst.ingestor.runtime.Docker#available")
class SftpIngestTest {

  static final String HOME = "/home/ingest/upload";

  @SuppressWarnings("resource")
  static final GenericContainer<?> SFTP =
      new GenericContainer<>("atmoz/sftp:alpine").withCommand("ingest:s3cret:::upload").withExposedPorts(22);

  @BeforeAll
  static void startServer() {
    SFTP.start();
  }

  @AfterAll
  static void stopServer() {
    SFTP.stop();
  }

  final List<List<JobRecord>> batches = new CopyOnWriteArrayList<>();
  volatile boolean platformDown;
  final ExchangeCounters counters = new ExchangeCounters(InstantSource.system());
  CamelContext camel;
  Reconciler reconciler;

  @BeforeEach
  void start() throws Exception {
    SFTP.execInContainer("sh", "-c", "rm -rf " + HOME + "/* " + HOME + "/.done " + HOME + "/.rejected");
    camel = new DefaultCamelContext();
    camel.start();
    JobSink sink =
        (ingestion, jobs) -> {
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    Secrets secrets = SourceHarness.secrets(Map.of("sftp", "s3cret"));
    SourceEndpoints endpoints = new CamelSourceEndpoints(secrets, new MemoryState(), null);
    reconciler = new Reconciler(camel, ingestion -> new IngestionRoute(ingestion, endpoints, sink, counters));
  }

  @AfterEach
  void stop() {
    camel.stop();
  }

  private static Ingestion files(String id, String secret, String include, Split split) {
    return SourceHarness.ingestion(
        id, 1,
        new Source.Sftp(
            SFTP.getHost(), SFTP.getMappedPort(22), "upload", "ingest", new SecretRef(secret), null, include, Duration.ofSeconds(1)),
        split,
        SourceHarness.target(Target.Mode.BLOCK_ON_ERROR, 2),
        false);
  }

  private static void put(String name, String content) {
    SFTP.copyFileToContainer(Transferable.of(content), HOME + "/" + name);
  }

  private static String list(String folder) throws Exception {
    return SFTP.execInContainer("sh", "-c", "ls -A " + HOME + "/" + folder + " 2>/dev/null").getStdout().strip();
  }

  private static void await(BooleanSupplier condition) throws InterruptedException {
    long deadline = System.nanoTime() + Duration.ofSeconds(30).toNanos();
    while (!condition.getAsBoolean()) {
      assertThat(System.nanoTime()).as("waited 30 s").isLessThan(deadline);
      Thread.sleep(200);
    }
  }

  private static boolean listed(String folder, String name) {
    try {
      return list(folder).contains(name);
    } catch (Exception e) {
      throw new IllegalStateException(e);
    }
  }

  @Test
  void aFileBecomesJobsAndIsMovedAsideAndOnlyMatchingFilesAreTaken() throws Exception {
    put("orders.csv", "id,sku\n1,A\n2,B\n3,C\n");
    put("notes.txt", "not for us");
    reconciler.reconcile(List.of(files("files", "sftp", "*.csv", new Split.Csv(true))));
    assertThat(reconciler.states()).extracting(RouteState::status).containsExactly(RouteState.Status.STARTED);

    await(() -> listed(".done", "orders.csv"));

    assertThat(batches).hasSize(2);
    assertThat(batches.stream().flatMap(List::stream).map(JobRecord::payload)).containsExactly("1,A", "2,B", "3,C");
    assertThat(list("")).contains("notes.txt").doesNotContain("orders.csv");
  }

  @Test
  void aFileThePlatformWouldNotTakeStaysAndIsReadAgain() throws Exception {
    platformDown = true;
    put("orders.csv", "1,A\n2,B\n");
    reconciler.reconcile(List.of(files("files", "sftp", null, new Split.Csv(false))));

    // Read at least twice while the platform is down, and still where it was.
    await(() -> counters.drain(InstantSource.system().instant()).windows().stream().mapToLong(w -> w.failed()).sum() >= 1);
    await(() -> counters.drain(InstantSource.system().instant()).windows().stream().mapToLong(w -> w.failed()).sum() >= 1);
    assertThat(list("")).contains("orders.csv");
    assertThat(batches).isEmpty();

    platformDown = false;
    await(() -> listed(".done", "orders.csv"));
    assertThat(batches).singleElement().satisfies(b -> assertThat(b).hasSize(2));
  }

  @Test
  void aFileThatCannotBeReadIsMovedToRejectedOnce() throws Exception {
    put("orders.json", "{\"orders\": [{\"id\":1},{\"id\":");
    reconciler.reconcile(List.of(files("files", "sftp", null, new Split.Json("$.orders"))));

    await(() -> listed(".rejected", "orders.json"));

    assertThat(list("")).doesNotContain("orders.json");
    assertThat(list(".done")).isEmpty();
    assertThat(batches).isEmpty();
  }

  @Test
  void anIngestionWhoseSecretIsNotSetIsReportedAsFailed() {
    reconciler.reconcile(List.of(files("files", "missing", null, new Split.None())));

    assertThat(reconciler.states())
        .containsExactly(new RouteState("files", 1, RouteState.Status.FAILED, "secret 'missing' is not set for client acme"));
  }
}

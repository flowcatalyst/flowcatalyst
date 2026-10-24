package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.Target;
import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.contract.TestRequest;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import org.apache.camel.CamelContext;
import org.apache.camel.CamelExecutionException;
import org.apache.camel.Route;
import org.apache.camel.impl.DefaultCamelContext;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/** The loop, the reconciler and the route against a real Camel context, fed through direct endpoints. */
class RuntimeLoopTest {

  /** The platform's runtime API held in memory. */
  static final class FakePlatform implements RuntimeApi {
    List<Ingestion> active = List.of();
    List<TestRequest> tests = List.of();
    final List<RuntimeReport> reports = new ArrayList<>();
    boolean down;
    boolean reportDown;
    @Nullable String rejects;
    Duration lease = Duration.ofMinutes(3);

    @Override
    public PollAnswer poll() throws IOException {
      if (down) {
        throw new IOException("connection refused");
      }
      return new PollAnswer(Instant.EPOCH, lease, active, tests);
    }

    @Override
    public void report(RuntimeReport report) throws IOException {
      if (down || reportDown) {
        throw new IOException("connection refused");
      }
      if (rejects != null) {
        throw new RejectedException(rejects, 422);
      }
      reports.add(report);
    }
  }

  final FakePlatform controlPlane = new FakePlatform();
  final List<TestRequest> tested = new ArrayList<>();
  final List<List<JobRecord>> batches = new ArrayList<>();
  boolean platformDown;
  final InstantSource clock = InstantSource.system();
  final ExchangeCounters counters = new ExchangeCounters(clock);
  final SourceHealth health = new SourceHealth();
  CamelContext camel;
  SourceEndpoints endpoints = (context, ingestion) -> context.getEndpoint("direct:" + ingestion.id());
  RuntimeLoop loop;
  long monotonicNanos = 1_000_000_000L;

  @BeforeEach
  void start() {
    camel = new DefaultCamelContext();
    camel.start();
    JobSink sink =
        (ingestion, jobs) -> {
          if (platformDown) {
            throw new IOException("platform unavailable");
          }
          batches.add(jobs);
        };
    Reconciler reconciler =
        new Reconciler(camel, ingestion -> new IngestionRoute(ingestion, (c, i) -> endpoints.endpoint(c, i), sink, counters, health), health);
    loop =
        new RuntimeLoop(
            "runtime-1",
            Duration.ofMinutes(1),
            controlPlane,
            reconciler,
            counters,
            request -> {
              tested.add(request);
              return new ConnectionTest(request.id(), ConnectionTest.Outcome.RESULTS, List.of("answered"), 2L, null);
            },
            clock,
            () -> monotonicNanos);
  }

  @AfterEach
  void stop() {
    camel.stop();
  }

  private static Ingestion ingestion(String id, long version, Split split, int batchSize) {
    return SourceHarness.ingestion(
        id, version, new Source.Sqs("eu-west-1", id, null), split, SourceHarness.target(Target.Mode.BLOCK_ON_ERROR, batchSize), false);
  }

  private void send(String ingestionId, String body) {
    try (var producer = camel.createProducerTemplate()) {
      producer.sendBody("direct:" + ingestionId, body);
    } catch (IOException e) {
      throw new IllegalStateException(e);
    }
  }

  private RuntimeReport lastReport() {
    return controlPlane.reports.getLast();
  }

  @Test
  void aCsvPayloadBecomesOrderedJobsInBatchesAndIsCountedForItsClient() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(true), 2));
    loop.tick();

    send("ing_1", "sku,qty\r\nA,1\r\nB,2\r\nC,3\r\n");
    loop.tick();

    assertThat(batches).hasSize(2);
    assertThat(batches.get(0)).extracting(JobRecord::payload).containsExactly("A,1", "B,2");
    assertThat(batches.get(1)).extracting(JobRecord::payload).containsExactly("C,3");
    List<JobRecord> jobs = batches.stream().flatMap(List::stream).toList();
    assertThat(jobs).extracting(JobRecord::messageGroup).containsOnly(jobs.getFirst().messageGroup());
    assertThat(jobs).extracting(JobRecord::correlationId).containsOnly(jobs.getFirst().messageGroup());
    // Where the job goes, who signs it and for whom are the ingestion's target, not the record's.
    assertThat(jobs).extracting(JobRecord::externalId).containsOnlyNulls();

    assertThat(lastReport().routes())
        .containsExactly(new RouteState("ing_1", 1, RouteState.Status.STARTED, null));
    assertThat(lastReport().metrics()).singleElement().satisfies(w -> {
      assertThat(w.ingestionId()).isEqualTo("ing_1");
      assertThat(w.completed()).isEqualTo(1);
      assertThat(w.failed()).isZero();
      assertThat(w.records()).isEqualTo(3);
      assertThat(w.dispatchJobs()).isEqualTo(3);
    });
  }

  @Test
  void anUnsplitPayloadIsOneJobAndXmlIsOneJobPerElement() throws Exception {
    controlPlane.active =
        List.of(ingestion("whole", 1, new Split.None(), 10), ingestion("xml", 1, new Split.Xml("order"), 10));
    loop.tick();

    send("whole", "{\"a\":1}");
    send("xml", "<orders><order id=\"1\"/><order id=\"2\"/></orders>");

    assertThat(batches.get(0)).extracting(JobRecord::payload).containsExactly("{\"a\":1}");
    assertThat(batches.get(1)).extracting(JobRecord::payload).containsExactly("<order id=\"1\"/>", "<order id=\"2\"/>");
  }

  @Test
  void aJsonPayloadIsOneJobPerElementOfTheArrayThePathNames() throws Exception {
    controlPlane.active =
        List.of(ingestion("top", 1, new Split.Json("$"), 10), ingestion("nested", 1, new Split.Json("$.data.orders"), 10));
    loop.tick();

    send("top", "[{\"id\": 1}, {\"id\": 2}]");
    send("nested", "{\"meta\": {\"orders\": [0]}, \"data\": {\"count\": 2, \"orders\": [{\"id\": 3}, \"x\"]}, \"after\": []}");

    assertThat(batches.get(0)).extracting(JobRecord::payload).containsExactly("{\"id\":1}", "{\"id\":2}");
    assertThat(batches.get(1)).extracting(JobRecord::payload).containsExactly("{\"id\":3}", "\"x\"");
    assertThat(batches.get(1)).extracting(JobRecord::payloadContentType).containsOnly("application/json");
  }

  @Test
  void aPayloadThatCannotBeReadIsRejectedAndCountedAsFailedNotLeftToBeRetried() throws Exception {
    controlPlane.active = List.of(ingestion("nested", 1, new Split.Json("$.data.orders"), 2));
    loop.tick();

    // No array where the path says: nothing is created, and the exchange ends instead of failing.
    send("nested", "{\"data\": {}}");
    assertThat(batches).isEmpty();

    // Broken after three records: the batch already sent stays sent, the unsent record is dropped.
    send("nested", "{\"data\": {\"orders\": [{\"id\":1},{\"id\":2},{\"id\":3},{\"id\":");
    assertThat(batches).singleElement().satisfies(b -> assertThat(b).hasSize(2));

    loop.tick();
    assertThat(lastReport().metrics()).singleElement().satisfies(w -> {
      assertThat(w.completed()).isZero();
      assertThat(w.failed()).isEqualTo(2);
      assertThat(w.dispatchJobs()).isEqualTo(2);
    });
  }

  private static Ingestion keepingPayloads(Ingestion i) {
    return SourceHarness.ingestion(i.id(), i.version(), i.source(), i.split(), i.target(), true);
  }

  @Test
  void aRejectedPayloadIsReportedWithHowFarItWasReadAndItsStartOnlyWhenPayloadsAreKept() throws Exception {
    controlPlane.active =
        List.of(keepingPayloads(ingestion("keeps", 1, new Split.Json("$"), 10)), ingestion("plain", 1, new Split.Json("$"), 10));
    loop.tick();

    send("keeps", "[{\"id\":1},{\"id\":");
    send("plain", "[{\"id\":1},{\"id\":");
    loop.tick();

    assertThat(lastReport().failures()).hasSize(2);
    RuntimeReport.Failure kept = lastReport().failures().stream().filter(f -> f.ingestionId().equals("keeps")).findFirst().orElseThrow();
    assertThat(kept.kind()).isEqualTo(RuntimeReport.Failure.Kind.REJECTED);
    // The reader looks ahead: the first record was not yet taken when the second turned out broken.
    assertThat(kept.afterRecords()).isZero();
    assertThat(kept.excerpt()).isEqualTo("[{\"id\":1},{\"id\":");
    assertThat(lastReport().failures().stream().filter(f -> f.ingestionId().equals("plain")).findFirst().orElseThrow().excerpt()).isNull();
    // Sent once.
    loop.tick();
    assertThat(lastReport().failures()).isEmpty();
  }

  @Test
  void aSourceThatKeepsFailingIsReportedAsFailingUntilItAnswersOrTheIngestionChanges() throws Exception {
    Ingestion v1 = ingestion("ing_1", 1, new Split.None(), 1);
    controlPlane.active = List.of(v1);
    loop.tick();

    health.failing(v1, "401 from https://api.example/orders?token=abc");
    loop.tick();
    assertThat(lastReport().routes())
        .containsExactly(new RouteState("ing_1", 1, RouteState.Status.FAILING, "401 from https://api.example/orders?token=***"));

    health.recovered(v1);
    loop.tick();
    assertThat(lastReport().routes().getFirst().status()).isEqualTo(RouteState.Status.STARTED);

    // Saved again: a new route, and it starts healthy.
    health.failing(v1, "401");
    controlPlane.active = List.of(ingestion("ing_1", 2, new Split.None(), 1));
    loop.tick();
    assertThat(lastReport().routes()).containsExactly(new RouteState("ing_1", 2, RouteState.Status.STARTED, null));
  }

  @Test
  void credentialsInAMessageAreBlankedBeforeItLeavesTheRuntime() {
    assertThat(ExchangeCounters.clean("could not connect to sftp://ingest:hunter2@sftp.acme.example/out"))
        .isEqualTo("could not connect to sftp://***@sftp.acme.example/out");
    assertThat(ExchangeCounters.clean("GET /orders?api_key=k123&page=2 failed; password=pw"))
        .isEqualTo("GET /orders?api_key=***&page=2 failed; password=***");
  }

  @Test
  void aRefusedBatchFailsTheExchangeSoTheSourceKeepsThePayload() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 2));
    loop.tick();
    platformDown = true;

    assertThatThrownBy(() -> send("ing_1", "A\nB\nC\n")).isInstanceOf(CamelExecutionException.class);
    loop.tick();

    assertThat(lastReport().metrics()).singleElement().satisfies(w -> {
      assertThat(w.completed()).isZero();
      assertThat(w.failed()).isEqualTo(1);
      assertThat(w.dispatchJobs()).isZero();
    });
    assertThat(lastReport().failures()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(RuntimeReport.Failure.Kind.NOT_DISPATCHED);
      assertThat(f.message()).isEqualTo("platform unavailable");
    });
  }

  @Test
  void aChangedVersionRebuildsTheRouteAndAnUnlistedIngestionIsRemoved() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 10), ingestion("ing_2", 1, new Split.None(), 1));
    loop.tick();
    assertThat(camel.getRoutes()).hasSize(2);

    controlPlane.active = List.of(ingestion("ing_1", 2, new Split.None(), 10));
    loop.tick();

    assertThat(camel.getRoute("ing_2")).isNull();
    assertThat(lastReport().routes()).containsExactly(new RouteState("ing_1", 2, RouteState.Status.STARTED, null));
    send("ing_1", "A\nB");
    assertThat(batches).singleElement().satisfies(b -> assertThat(b).hasSize(1));
  }

  @Test
  void anOlderVersionOrAChangedSettingAtTheSameVersionRebuildsTheRoute() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 5, new Split.Csv(false), 10));
    loop.tick();
    Route first = camel.getRoute("ing_1");

    // Going back to version 3.
    controlPlane.active = List.of(ingestion("ing_1", 3, new Split.None(), 10));
    loop.tick();
    assertThat(camel.getRoute("ing_1")).isNotSameAs(first);
    assertThat(lastReport().routes()).containsExactly(new RouteState("ing_1", 3, RouteState.Status.STARTED, null));
    send("ing_1", "A\nB");
    assertThat(batches).singleElement().satisfies(b -> assertThat(b).hasSize(1));

    // Payloads kept from now on, the version unchanged.
    Route second = camel.getRoute("ing_1");
    controlPlane.active = List.of(keepingPayloads(ingestion("ing_1", 3, new Split.None(), 10)));
    loop.tick();
    assertThat(camel.getRoute("ing_1")).isNotSameAs(second);

    // Nothing changed: the route is kept.
    Route third = camel.getRoute("ing_1");
    loop.tick();
    assertThat(camel.getRoute("ing_1")).isSameAs(third);
  }

  @Test
  void aSourceTheRuntimeCannotRunIsReportedAsFailedWithoutStoppingTheOthers() throws Exception {
    SourceEndpoints real = new CamelSourceEndpoints(SourceHarness.secrets(Map.of()), new MemoryState(), null);
    endpoints =
        (context, ingestion) ->
            ingestion.id().equals("bad") ? real.endpoint(context, ingestion) : context.getEndpoint("direct:" + ingestion.id());
    Ingestion good = ingestion("good", 1, new Split.None(), 1);
    Ingestion bad =
        SourceHarness.ingestion(
            "bad", 1, new Source.Sqs("eu-west-1", "bad", new Source.AwsCredential("AKIAEXAMPLE", new SecretRef("aws"))),
            good.split(), good.target(), false);
    controlPlane.active = List.of(bad, good);

    loop.tick();

    assertThat(lastReport().routes())
        .containsExactly(
            new RouteState("bad", 1, RouteState.Status.FAILED, "secret 'aws' is not set for client acme"),
            new RouteState("good", 1, RouteState.Status.STARTED, null));
    assertThat(camel.getRoutes()).hasSize(1);
  }

  @Test
  void whenThePlatformIsDownTheRoutesKeepRunningAndTheCountsAreSentLater() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.None(), 1));
    loop.tick();
    assertThatCode(loop::requireRecentPoll).doesNotThrowAnyException();

    controlPlane.down = true;
    send("ing_1", "A");
    loop.tick();
    assertThat(camel.getRoute("ing_1")).isNotNull();
    assertThat(controlPlane.reports).hasSize(1);

    controlPlane.down = false;
    send("ing_1", "B");
    loop.tick();

    assertThat(lastReport().metrics()).extracting(RuntimeReport.ExchangeWindow::completed).containsExactly(1L, 1L);
  }

  private static Ingestion polled(String id) {
    return SourceHarness.ingestion(
        id, 1,
        new Source.Sftp("sftp.acme.example", 22, "/out", "ingest", new SecretRef("sftp"), null, null, Duration.ofMinutes(1)),
        new Split.None(),
        new Target("logistics:orders:file:map", "https://f.example/map", "pool_1", null, null, Target.Mode.IMMEDIATE, 1),
        false);
  }

  @Test
  void aSingleOwnerIngestionIsStoppedBeforeItsLeaseEndsAndASharedOneKeepsRunning() throws Exception {
    controlPlane.active = List.of(polled("files"), ingestion("queue", 1, new Split.None(), 1));
    loop.tick();
    assertThat(camel.getRoute("files")).isNotNull();

    // One missed poll: the three-minute lease still covers the next interval.
    controlPlane.down = true;
    monotonicNanos += Duration.ofMinutes(1).toNanos();
    loop.tick();
    assertThat(camel.getRoute("files")).isNotNull();

    // A second: the lease would end before a third round could renew it.
    monotonicNanos += Duration.ofMinutes(1).toNanos();
    loop.tick();
    assertThat(camel.getRoute("files")).isNull();
    assertThat(camel.getRoute("queue")).isNotNull();

    controlPlane.down = false;
    loop.tick();
    assertThat(camel.getRoute("files")).isNotNull();
  }

  @Test
  void aLeaseTooShortToCoverTheNextIntervalRunsNoSingleOwnerIngestion() throws Exception {
    controlPlane.lease = Duration.ofSeconds(30);
    controlPlane.active = List.of(polled("files"), ingestion("queue", 1, new Split.None(), 1));
    loop.tick();

    assertThat(camel.getRoute("files")).isNull();
    assertThat(camel.getRoute("queue")).isNotNull();
  }

  @Test
  void aTestTheLoopIsHandedIsRunAndAnsweredInTheReportOnce() throws Exception {
    Ingestion under = ingestion("ing_test", 0, new Split.None(), 1);
    controlPlane.tests =
        List.of(
            new TestRequest("tst_1", Instant.now().plusSeconds(300), under),
            new TestRequest("tst_old", Instant.now().minusSeconds(1), under));

    loop.tick();

    assertThat(tested).extracting(TestRequest::id).containsExactly("tst_1");
    assertThat(lastReport().tests()).singleElement().satisfies(answer -> {
      assertThat(answer.id()).isEqualTo("tst_1");
      assertThat(answer.outcome()).isEqualTo(ConnectionTest.Outcome.RESULTS);
    });
    controlPlane.tests = List.of();
    loop.tick();
    assertThat(lastReport().tests()).isEmpty();
  }

  @Test
  void aTestAnswerIsKeptWhenTheReportCannotBeSentAndSentWithTheNext() throws Exception {
    controlPlane.tests = List.of(new TestRequest("tst_1", Instant.now().plusSeconds(300), ingestion("ing_test", 0, new Split.None(), 1)));
    controlPlane.reportDown = true;
    loop.tick();
    assertThat(tested).hasSize(1);

    controlPlane.reportDown = false;
    controlPlane.tests = List.of();
    loop.tick();

    assertThat(lastReport().tests()).extracting(ConnectionTest::id).containsExactly("tst_1");
  }

  @Test
  void aReportThePlatformRefusesIsDroppedSoTheOnesAfterItAreStillSent() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.None(), 1));
    loop.tick();
    send("ing_1", "A");
    controlPlane.rejects = "REPORT_WINDOW_INVALID";
    loop.tick();
    assertThat(controlPlane.reports).hasSize(1);

    controlPlane.rejects = null;
    send("ing_1", "B");
    loop.tick();

    // Only what was counted after the refused report: it was not kept to be refused again.
    assertThat(lastReport().metrics()).singleElement().satisfies(w -> assertThat(w.completed()).isEqualTo(1));
  }

  @Test
  void aReportCarriesNoMoreThanThePlatformTakesAndTheRestFollows() throws Exception {
    List<Ingestion> many = new ArrayList<>();
    for (int i = 0; i < RuntimeReport.MAX_WINDOWS + 5; i++) {
      many.add(ingestion("ing_" + i, 1, new Split.None(), 1));
    }
    controlPlane.active = many;
    loop.tick();
    for (Ingestion each : many) {
      send(each.id(), "A");
    }

    loop.tick();
    assertThat(lastReport().metrics()).hasSize(RuntimeReport.MAX_WINDOWS);
    assertThat(lastReport().routes()).hasSize(RuntimeReport.MAX_ROUTES);

    loop.tick();
    assertThat(lastReport().metrics()).hasSize(5);
  }
}

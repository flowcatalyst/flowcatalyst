package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatCode;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import io.flowcatalyst.framework.http.Json;
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
    List<RuntimeApi.Unreadable> unreadable = List.of();
    List<Duration> timeouts = new ArrayList<>();
    @Nullable String failReportWith;

    @Override
    public Polled poll(Duration timeout) throws IOException {
      timeouts.add(timeout);
      if (down) {
        throw new IOException("connection refused");
      }
      return new Polled(new PollAnswer(Instant.EPOCH, lease, active, tests), unreadable, List.of());
    }

    @Override
    public void report(RuntimeReport report, Duration timeout) throws IOException {
      if (down || reportDown) {
        throw new IOException("connection refused");
      }
      if (failReportWith != null && report.failures().stream().anyMatch(f -> f.message().contains(failReportWith))) {
        throw new IOException("500 from the platform");
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
  boolean refuseJobs;
  int failNext;
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
          if (refuseJobs) {
            throw new RejectedException("POST /jobs answered 400: VALIDATION, records[0].payload is not valid", 400);
          }
          if (failNext > 0) {
            failNext--;
            throw new IOException("connection reset");
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

  @Test
  void anIngestionWhoseDefinitionCannotBeReadIsReportedFailedAndItsOlderRouteStopped() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.None(), 1), ingestion("ing_2", 1, new Split.None(), 1));
    loop.tick();
    assertThat(camel.getRoute("ing_2")).isNotNull();

    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.None(), 1));
    controlPlane.unreadable = List.of(new RuntimeApi.Unreadable("ing_2", 2, PlatformApi.NOT_SUPPORTED));
    loop.tick();
    loop.tick();

    assertThat(camel.getRoute("ing_2")).isNull();
    assertThat(lastReport().routes())
        .containsExactly(
            new RouteState("ing_1", 1, RouteState.Status.STARTED, null),
            new RouteState("ing_2", 2, RouteState.Status.FAILED, PlatformApi.NOT_SUPPORTED));
  }

  @Test
  void theWatchdogStopsASingleOwnerRouteBeforeTheLeaseEndsWhileTheRoundIsStuck() throws Exception {
    controlPlane.active = List.of(polled("files"), ingestion("queue", 1, new Split.None(), 1));
    loop.tick();
    assertThat(loop.margin()).isEqualTo(Duration.ofMinutes(1));

    // No round renews it: the watchdog alone, a minute before the three-minute lease ends.
    monotonicNanos += Duration.ofMinutes(2).minusSeconds(1).toNanos();
    loop.watch();
    assertThat(camel.getRoute("files")).isNotNull();

    monotonicNanos += Duration.ofSeconds(2).toNanos();
    loop.watch();
    assertThat(camel.getRoute("files")).isNull();
    assertThat(camel.getRoute("queue")).isNotNull();
  }

  @Test
  void thePollAndTheReportWaitAtMostASixthOfTheLease() throws Exception {
    controlPlane.lease = Duration.ofSeconds(30);
    loop.tick();
    loop.tick();

    assertThat(controlPlane.timeouts).containsExactly(Duration.ofSeconds(30), Duration.ofSeconds(5));
    assertThat(loop.margin()).isEqualTo(Duration.ofSeconds(15));
  }

  @Test
  void aRoundThatThrowsAnErrorIsLoggedAndTheNextRoundRuns() throws Exception {
    RuntimeApi broken =
        new RuntimeApi() {
          int polls;

          @Override
          public Polled poll(Duration timeout) {
            if (polls++ == 0) {
              throw new NoClassDefFoundError("io/example/Missing");
            }
            return new Polled(new PollAnswer(Instant.EPOCH, Duration.ofMinutes(3), List.of()));
          }

          @Override
          public void report(RuntimeReport report, Duration timeout) {}
        };
    RuntimeLoop other =
        new RuntimeLoop("runtime-1", Duration.ofMinutes(1), broken, new Reconciler(camel, i -> null), counters, r -> null, clock, () -> monotonicNanos);

    assertThatCode(other::tick).doesNotThrowAnyException();
    assertThatThrownBy(other::requireRecentPoll).isInstanceOf(IllegalStateException.class);
    other.tick();
    assertThatCode(other::requireRecentPoll).doesNotThrowAnyException();
  }

  @Test
  void aReportStaysUnderThePlatformsSizeAndTheRestFollowsOldestFirst() throws Exception {
    String wide = "€".repeat(2048);
    int ingestions = 30;
    for (int i = 0; i < ingestions; i++) {
      Ingestion each = keepingPayloads(ingestion("ing_" + i, 1, new Split.None(), 1));
      for (int f = 0; f < ExchangeCounters.FAILURES_PER_INGESTION; f++) {
        counters.failure(each, RuntimeReport.Failure.Kind.REJECTED, wide, 0L, wide);
      }
    }

    loop.tick();
    RuntimeReport first = lastReport();
    assertThat(Json.mapper().writeValueAsBytes(first).length).isLessThan(RuntimeReport.MAX_BYTES);
    assertThat(first.failures()).hasSizeLessThan(RuntimeReport.MAX_FAILURES);
    assertThat(first.failures().getFirst().ingestionId()).isEqualTo("ing_0");

    int sent = first.failures().size();
    for (int i = 0; i < 3; i++) {
      loop.tick();
      sent += lastReport().failures().size();
    }
    assertThat(sent).isEqualTo(ingestions * ExchangeCounters.FAILURES_PER_INGESTION);
  }

  @Test
  void aReportThatKeepsFailingWhileThePlatformAnswersDropsItsFailuresSoReportingGoesOn() throws Exception {
    Ingestion one = ingestion("ing_1", 1, new Split.None(), 1);
    controlPlane.active = List.of(one);
    loop.tick();
    controlPlane.failReportWith = "poison";
    counters.failure(one, RuntimeReport.Failure.Kind.SOURCE, "poison", null, null);

    int before = controlPlane.reports.size();
    for (int i = 0; i < RuntimeLoop.WEDGED_REPORTS; i++) {
      loop.tick();
    }
    assertThat(controlPlane.reports).hasSize(before);

    loop.tick();
    assertThat(controlPlane.reports).hasSize(before + 1);
    assertThat(lastReport().failures()).isEmpty();
  }

  @Test
  void aSlowTestDoesNotHoldUpTheRoundAndIsAnsweredAsFailedAtItsDeadline() throws Exception {
    java.util.concurrent.CountDownLatch release = new java.util.concurrent.CountDownLatch(1);
    RuntimeLoop slow =
        new RuntimeLoop(
            "runtime-1",
            Duration.ofSeconds(2),
            controlPlane,
            new Reconciler(camel, i -> null),
            counters,
            request -> {
              try {
                release.await();
              } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
              }
              return new ConnectionTest(request.id(), ConnectionTest.Outcome.RESULTS, List.of("late"), null, null);
            },
            clock,
            () -> monotonicNanos);
    controlPlane.tests = List.of(new TestRequest("tst_slow", Instant.now().plusSeconds(300), ingestion("ing_test", 0, new Split.None(), 1)));

    long started = System.nanoTime();
    slow.tick();
    assertThat(Duration.ofNanos(System.nanoTime() - started)).isLessThan(Duration.ofSeconds(5));
    assertThat(lastReport().tests()).isEmpty();

    // Handed again while it runs: not started twice. Past its deadline: answered as failed, once.
    monotonicNanos += RuntimeLoop.MAX_TEST.plusSeconds(1).toNanos();
    slow.tick();
    assertThat(lastReport().tests()).singleElement().satisfies(t -> {
      assertThat(t.id()).isEqualTo("tst_slow");
      assertThat(t.outcome()).isEqualTo(ConnectionTest.Outcome.FAILED);
    });
    release.countDown();
    slow.close();
  }

  @Test
  void jobsThePlatformRefusesForGoodRejectThePayloadSoTheSourceMovesOnAndSayWhy() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 10));
    loop.tick();
    refuseJobs = true;

    // The exchange ends: the source lets go of the payload instead of offering it forever.
    assertThatCode(() -> send("ing_1", "A\nB\n")).doesNotThrowAnyException();
    loop.tick();

    assertThat(lastReport().failures()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(RuntimeReport.Failure.Kind.REJECTED);
      assertThat(f.category()).isEqualTo(RuntimeReport.Failure.Category.REJECTED);
      assertThat(f.message()).contains("the platform refused the dispatch jobs").contains("VALIDATION");
    });
  }

  @Test
  void aBatchThePlatformDidNotTakeIsSentAgainBeforeTheExchangeFails() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 10));
    loop.tick();
    failNext = IngestionRoute.ATTEMPTS - 1;

    send("ing_1", "A\nB\n");
    loop.tick();

    assertThat(batches).singleElement().satisfies(b -> assertThat(b).hasSize(2));
    assertThat(lastReport().failures()).isEmpty();
  }

  @Test
  void aBatchIsCutBySizeAsWellAsByCount() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 10));
    loop.tick();
    String line = "x".repeat(3 * 1024 * 1024);

    send("ing_1", String.join("\n", line, line, line, line, line));

    assertThat(batches).extracting(List::size).containsExactly(2, 2, 1);
    for (List<JobRecord> batch : batches) {
      assertThat(Json.mapper().writeValueAsBytes(Map.of("records", batch)).length).isLessThanOrEqualTo(JobRecord.TARGET_REQUEST_BYTES);
    }
  }

  @Test
  void aPayloadWithAnIdentityHasKeysFromItAndItsGroupIsTheSameEveryTimeItIsRead() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Csv(false), 10));
    loop.tick();

    for (int i = 0; i < 2; i++) {
      try (var producer = camel.createProducerTemplate()) {
        producer.send("direct:ing_1", e -> {
          e.getMessage().setBody("A\nB\n");
          io.flowcatalyst.ingestor.extension.PayloadIdentity.set(e, "sftp.example:/out/a.csv@1|4");
        });
      }
    }

    assertThat(batches).hasSize(2);
    assertThat(batches.get(0)).extracting(JobRecord::externalId)
        .containsExactly(Identities.externalId("sftp.example:/out/a.csv@1|4", 0), Identities.externalId("sftp.example:/out/a.csv@1|4", 1));
    assertThat(batches.get(1)).isEqualTo(batches.get(0));

    // Without one: no key, and the group still follows the payload, not the exchange.
    send("ing_1", "C\n");
    send("ing_1", "C\n");
    assertThat(batches.get(2).getFirst().externalId()).isNull();
    assertThat(batches.get(3).getFirst().messageGroup()).isEqualTo(batches.get(2).getFirst().messageGroup());
  }

  @Test
  void aConnectionThatBreaksWhileThePayloadIsReadKeepsThePayloadInsteadOfRejectingIt() throws Exception {
    controlPlane.active = List.of(ingestion("ing_1", 1, new Split.Json("$"), 10));
    loop.tick();
    java.io.InputStream cut =
        new java.io.SequenceInputStream(
            new java.io.ByteArrayInputStream("[{\"id\":1},".getBytes(java.nio.charset.StandardCharsets.UTF_8)),
            new java.io.InputStream() {
              @Override
              public int read() throws IOException {
                throw new java.net.SocketException("Connection reset");
              }
            });

    try (var producer = camel.createProducerTemplate()) {
      org.apache.camel.Exchange done = producer.send("direct:ing_1", e -> e.getMessage().setBody(cut));
      assertThat(done.isFailed()).isTrue();
      assertThat(done.getProperty(IngestionRoute.REJECTED)).isNull();
    }
    loop.tick();
    assertThat(lastReport().failures()).singleElement().satisfies(f -> assertThat(f.kind()).isEqualTo(RuntimeReport.Failure.Kind.SOURCE));
  }

  @Test
  void anHttpIngestionIsFoundWhileAnotherRouteIsBeingStopped() throws Exception {
    Ingestion http = SourceHarness.ingestion("hook", 1, new Source.Http(new io.flowcatalyst.ingestor.contract.HttpAuth.Bearer(new SecretRef("t")), null), new Split.None(), SourceHarness.target(Target.Mode.IMMEDIATE, 1), false);
    controlPlane.active = List.of(http, ingestion("ing_1", 1, new Split.None(), 1));
    loop.tick();
    Reconciler reconciler = reconcilerOf(loop);

    java.util.concurrent.CountDownLatch inside = new java.util.concurrent.CountDownLatch(1);
    java.util.concurrent.CountDownLatch leave = new java.util.concurrent.CountDownLatch(1);
    // Another thread holds the reconciler's lock, as a reconcile stopping a slow route does.
    Thread holder =
        Thread.ofPlatform().start(() -> {
          synchronized (reconciler) {
            inside.countDown();
            try {
              leave.await();
            } catch (InterruptedException e) {
              Thread.currentThread().interrupt();
            }
          }
        });
    inside.await();
    try {
      java.util.concurrent.CompletableFuture<List<Ingestion>> found =
          java.util.concurrent.CompletableFuture.supplyAsync(() -> reconciler.find(null, "acme", "hook"));
      assertThat(found.get(2, java.util.concurrent.TimeUnit.SECONDS)).extracting(Ingestion::id).containsExactly("hook");
    } finally {
      leave.countDown();
      holder.join();
    }
  }

  private Reconciler reconcilerOf(RuntimeLoop of) throws Exception {
    java.lang.reflect.Field field = RuntimeLoop.class.getDeclaredField("reconciler");
    field.setAccessible(true);
    return (Reconciler) field.get(of);
  }
}

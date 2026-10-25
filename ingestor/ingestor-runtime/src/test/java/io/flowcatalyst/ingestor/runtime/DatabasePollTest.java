package io.flowcatalyst.ingestor.runtime;

import static io.flowcatalyst.ingestor.runtime.SourceHarness.await;
import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.RuntimeReport.RouteState;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.Track;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.framework.database.PostgresFixture;
import java.time.Duration;
import java.util.List;
import java.util.Map;
import org.junit.jupiter.api.AfterAll;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeAll;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.condition.EnabledIf;

/** The database-poll source against a real PostgreSQL. */
@EnabledIf("io.flowcatalyst.ingestor.runtime.Docker#available")
class DatabasePollTest {

  static PostgresFixture pg;

  @BeforeAll
  static void startDatabase() {
    pg = PostgresFixture.start();
  }

  @AfterAll
  static void stopDatabase() {
    pg.close();
  }

  SourceHarness harness;

  @BeforeEach
  void start() {
    pg.reset();
    pg.execute(
        """
        CREATE TABLE orders (
          id BIGINT PRIMARY KEY,
          sku TEXT NOT NULL,
          changed_at TIMESTAMP NOT NULL,
          processed INT NOT NULL DEFAULT 0
        )
        """);
    harness = new SourceHarness(SourceHarness.secrets(Map.of("db", pg.password())), null);
  }

  @AfterEach
  void stop() {
    harness.close();
  }

  private static Ingestion table(Track track) {
    return SourceHarness.ingestion(
        "orders",
        new Source.DatabasePoll(pg.jdbcUrl(), pg.username(), new SecretRef("db"), "orders", track, 2, Duration.ofSeconds(1)),
        new Split.None());
  }

  private static void insert(long id, String changedAt) {
    pg.execute("INSERT INTO orders (id, sku, changed_at) VALUES (" + id + ", 'S" + id + "', '" + changedAt + "')");
  }

  private List<Integer> ids() {
    return harness.payloads().stream().map(p -> Integer.parseInt(p.replaceAll(".*\\\"id\\\":(\\d+).*", "$1"))).toList();
  }

  @Test
  void rowsPastANumberWatermarkBecomeJobsAFewAtATimeAndNewRowsFollow() throws Exception {
    for (int id = 1; id <= 5; id++) {
      insert(id, "2026-10-09 10:00:0" + id);
    }
    harness.reconciler.reconcile(List.of(table(new Track.Watermark("id", Track.Watermark.Type.NUMBER))));
    assertThat(harness.reconciler.states()).extracting(RouteState::status).containsExactly(RouteState.Status.STARTED);

    await(() -> harness.payloads().size() == 5);
    assertThat(ids()).containsExactly(1, 2, 3, 4, 5);
    assertThat(harness.payloads().getFirst())
        .isEqualTo("{\"id\":1,\"sku\":\"S1\",\"changed_at\":\"2026-10-09T10:00:01\",\"processed\":0}");
    // Three polls of at most two rows, each one exchange.
    assertThat(harness.batches).hasSize(3);
    await(() -> "5".equals(harness.state.values.get("orders/watermark")));

    insert(6, "2026-10-09 10:00:06");
    await(() -> harness.payloads().size() == 6);
    assertThat(ids()).containsExactly(1, 2, 3, 4, 5, 6);
    assertThat(pg.query("SELECT sum(processed)::text FROM orders")).isEqualTo("0");
  }

  @Test
  void rowsThatShareATimestampAreNeverSplitBetweenTwoPolls() throws Exception {
    insert(1, "2026-10-09 10:00:01");
    insert(2, "2026-10-09 10:00:02.5");
    insert(3, "2026-10-09 10:00:02.5");
    insert(4, "2026-10-09 10:00:02.5");
    insert(5, "2026-10-09 10:00:03");
    harness.reconciler.reconcile(List.of(table(new Track.Watermark("changed_at", Track.Watermark.Type.TIMESTAMP))));

    await(() -> harness.payloads().size() == 5);

    assertThat(ids()).containsExactlyInAnyOrder(1, 2, 3, 4, 5);
    // The date and time the column holds, in ISO form whatever the JVM's zone.
    await(() -> "2026-10-09T10:00:03".equals(harness.state.values.get("orders/watermark")));
    Thread.sleep(2500);
    assertThat(harness.payloads()).hasSize(5);
    // Each row keyed by its watermark value and its place among the rows of that value.
    List<JobRecord> jobs = harness.batches.stream().flatMap(List::stream).toList();
    assertThat(jobs).extracting(JobRecord::externalId).doesNotContainNull().doesNotHaveDuplicates();
  }

  @Test
  void aWatermarkStoredByAnEarlierRuntimeIsStillReadAndATimestampWithAZoneIsKeptInUtc() throws Exception {
    pg.execute("CREATE TABLE events (id BIGINT PRIMARY KEY, at TIMESTAMPTZ NOT NULL)");
    pg.execute("INSERT INTO events VALUES (1, '2026-10-09 10:00:01+02'), (2, '2026-10-09 10:00:02+02')");
    harness.state.values.put("events/watermark", "2026-10-09 08:00:01.0");
    Ingestion events =
        SourceHarness.ingestion(
            "events",
            new Source.DatabasePoll(pg.jdbcUrl(), pg.username(), new SecretRef("db"), "events", new Track.Watermark("at", Track.Watermark.Type.TIMESTAMP), 10, Duration.ofSeconds(1)),
            new Split.None());

    harness.reconciler.reconcile(List.of(events));

    await(() -> "2026-10-09T08:00:02Z".equals(harness.state.values.get("events/watermark")));
  }

  @Test
  void rowsWithAStatusAreKeyedByTheirKeyColumn() throws Exception {
    insert(1, "2026-10-09 10:00:01");
    insert(2, "2026-10-09 10:00:02");
    harness.reconciler.reconcile(List.of(table(new Track.Status("processed", "id", "0", "1"))));

    await(() -> harness.payloads().size() == 2);

    List<JobRecord> jobs = harness.batches.stream().flatMap(List::stream).toList();
    assertThat(jobs).extracting(JobRecord::externalId)
        .containsExactly(Identities.externalId("orders|id=1", 0), Identities.externalId("orders|id=2", 0));
  }

  @Test
  void rowsThePlatformWouldNotTakeAreReadAgainAndTheWatermarkStays() throws Exception {
    harness.platformDown = true;
    insert(1, "2026-10-09 10:00:01");
    harness.reconciler.reconcile(List.of(table(new Track.Watermark("id", Track.Watermark.Type.NUMBER))));

    await(() -> harness.failedSinceLastAsked() >= 1);
    await(() -> harness.failedSinceLastAsked() >= 1);
    assertThat(harness.state.values).isEmpty();

    harness.platformDown = false;
    await(() -> harness.payloads().size() == 1);
    await(() -> "1".equals(harness.state.values.get("orders/watermark")));
  }

  @Test
  void aWatermarkThatCannotBeStoredMeansTheRowsAreReadAgainNotSkipped() throws Exception {
    harness.state.refuseWrites = true;
    insert(1, "2026-10-09 10:00:01");
    harness.reconciler.reconcile(List.of(table(new Track.Watermark("id", Track.Watermark.Type.NUMBER))));

    await(() -> harness.payloads().size() >= 2);
    assertThat(ids()).containsOnly(1);
  }

  @Test
  void rowsWithAPendingStatusBecomeJobsAndAreMarkedProcessed() throws Exception {
    for (int id = 1; id <= 3; id++) {
      insert(id, "2026-10-09 10:00:0" + id);
    }
    pg.execute("UPDATE orders SET processed = 1 WHERE id = 2");
    harness.reconciler.reconcile(List.of(table(new Track.Status("processed", "id", "0", "1"))));

    await(() -> harness.payloads().size() == 2);
    await(() -> "3".equals(pg.query("SELECT sum(processed)::text FROM orders")));
    assertThat(ids()).containsExactly(1, 3);
    assertThat(harness.state.values).isEmpty();

    insert(4, "2026-10-09 10:00:04");
    await(() -> harness.payloads().size() == 3);
    await(() -> "4".equals(pg.query("SELECT sum(processed)::text FROM orders")));
    Thread.sleep(2500);
    assertThat(ids()).containsExactly(1, 3, 4);
  }

  @Test
  void rowsWithAPendingStatusStayPendingWhileThePlatformIsDown() throws Exception {
    harness.platformDown = true;
    insert(1, "2026-10-09 10:00:01");
    harness.reconciler.reconcile(List.of(table(new Track.Status("processed", "id", "0", "1"))));

    await(() -> harness.failedSinceLastAsked() >= 1);
    assertThat(pg.query("SELECT sum(processed)::text FROM orders")).isEqualTo("0");

    harness.platformDown = false;
    await(() -> "1".equals(pg.query("SELECT sum(processed)::text FROM orders")));
    assertThat(ids()).containsExactly(1);
  }
}

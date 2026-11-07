package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.io.IOException;
import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import org.apache.camel.Exchange;
import org.apache.camel.impl.DefaultCamelContext;
import org.apache.camel.support.DefaultExchange;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;

/** A polled source's failures: reported, logged once, paced, and a failed commit counted as a failure. */
class ReportingPollStrategyTest {

  private record Line(String message, @Nullable Throwable cause) {}

  private final TestClock clock = new TestClock(Instant.parse("2026-11-02T10:00:00Z"));
  private final List<Line> lines = new ArrayList<>();
  private final FailureLog failures = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), clock);
  private final ExchangeCounters counters = new ExchangeCounters(clock);
  private final SourceHealth health = new SourceHealth();
  private final PollPacer pacer = new PollPacer(Backoff.FAILURES, clock);

  private final Ingestion sftp =
      SourceHarness.ingestion(
          "ing_sftp", new Source.Sftp("files.example", 22, "in", "u", new SecretRef("p"), null, null, Duration.ofSeconds(10), true), new Split.None());
  private final ReportingPollStrategy strategy = new ReportingPollStrategy(sftp, counters, health, () -> {}, pacer, failures);

  private List<Failure> reported() {
    return counters.drain(clock.instant()).failures();
  }

  @Test
  void aFailedPollIsReportedAsASourceFailureTheRouteIsFailingAndTheNextPollWaits() {
    assertThat(strategy.begin(null, null)).isTrue();
    strategy.rollback(null, null, 0, new IOException("connection refused"));

    assertThat(reported()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
      assertThat(f.message()).contains("connection refused");
    });
    assertThat(health.problem("ing_sftp", 1)).contains("connection refused");
    // The wait is not over: the consumer's timer fires and the poll is skipped.
    assertThat(strategy.begin(null, null)).isFalse();
    clock.advance(Duration.ofSeconds(11)); // the source's own 10 s is the floor
    assertThat(strategy.begin(null, null)).isTrue();
    strategy.commit(null, null, 0);
    assertThat(health.problem("ing_sftp", 1)).isNull();
  }

  @Test
  void aRefusedSftpLoginIsReportedAsAuth() {
    strategy.begin(null, null);
    strategy.rollback(null, null, 0, new IllegalStateException("cannot connect", new IOException("Auth fail for methods 'password'")));

    assertThat(reported()).singleElement().satisfies(f -> assertThat(f.kind()).isEqualTo(Failure.Kind.AUTH));
  }

  @Test
  void aSourceFailingSixHundredTimesInAMinuteWritesTwoLinesAndOnlyTheFirstHasAStackTrace() {
    RuntimeException boom = new RuntimeException("no route to host");
    for (int i = 0; i < 600; i++) {
      pacer.started();
      strategy.rollback(null, null, 0, boom);
      clock.advance(Duration.ofMillis(100));
    }
    pacer.started();
    strategy.rollback(null, null, 0, boom);

    assertThat(lines).hasSize(2);
    assertThat(lines.get(0).cause()).isSameAs(boom);
    assertThat(lines.get(1).cause()).isNull();
    assertThat(lines.get(1).message()).contains("more like it not logged");
  }

  @Test
  void aSuccessAfterFailuresSaysSoOnceAndEndsTheStreak() {
    strategy.begin(null, null);
    strategy.rollback(null, null, 0, new IOException("down"));
    strategy.rollback(null, null, 0, new IOException("down"));
    clock.advance(Duration.ofMinutes(10));
    strategy.begin(null, null);
    strategy.commit(null, null, 0);

    assertThat(lines.getLast().message()).contains("polls its source again").contains("after 2 failures in a row");
    assertThat(failures.streak("ing_sftp:poll")).isZero();
  }

  // R4: a commit after a good read.

  @Test
  void aFailedMoveAfterAGoodReadIsASourceFailureTheRouteIsFailingAndThePollIsPaced() {
    DefaultExchange good = new DefaultExchange(new DefaultCamelContext());
    strategy.begin(null, null);
    strategy.handleException("Error during commit", good, new IOException("permission denied"));
    strategy.commit(null, null, 1);

    assertThat(reported()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
      assertThat(f.message()).startsWith("read and sent, but could not be moved or deleted: permission denied").endsWith("it will be read again");
    });
    assertThat(health.problem("ing_sftp", 1)).contains("could not be moved or deleted");
    assertThat(pacer.lastFailed()).isTrue();
    assertThat(pacer.due()).isFalse();
    assertThat(lines).singleElement().satisfies(l -> assertThat(l.cause()).hasMessage("permission denied"));
  }

  @Test
  void aCommitThatSucceedsAfterOneThatFailedClearsTheFailure() {
    DefaultExchange good = new DefaultExchange(new DefaultCamelContext());
    strategy.begin(null, null);
    strategy.handleException("Error during commit", good, new IOException("denied"));
    strategy.commit(null, null, 1);
    clock.advance(Duration.ofMinutes(10));
    strategy.begin(null, null);
    strategy.commit(null, null, 1);

    assertThat(health.problem("ing_sftp", 1)).isNull();
    assertThat(failures.streak("ing_sftp:commit")).isZero();
    assertThat(pacer.lastFailed()).isFalse();
  }

  @Test
  void aFailureCamelHandsOverThatWasReportedAlreadyIsNotReportedAgain() {
    IOException poll = new IOException("down");
    strategy.begin(null, null);
    strategy.rollback(null, null, 0, poll);
    // Camel then calls the consumer's exception handler with the same exception.
    strategy.handleException("Failed polling endpoint", poll);
    // An exchange the route failed was counted and logged by the route.
    Exchange failed = new DefaultExchange(new DefaultCamelContext());
    failed.setProperty(IngestionRoute.PENDING, "begun by the route");
    failed.setException(new IOException("cut off"));
    strategy.handleException("Error processing exchange", failed, failed.getException());

    assertThat(reported()).hasSize(1);
    assertThat(lines).hasSize(1);
  }

  @Test
  void aFailedExchangeTheRouteNeverBeganIsReportedHereAsTheComponentsOwnLineIsTurnedDown() {
    Exchange failed = new DefaultExchange(new DefaultCamelContext());
    failed.setException(new IllegalStateException("rejected before the route ran"));
    strategy.begin(null, null);
    strategy.handleException("Error processing exchange", failed, failed.getException());
    // Camel may hand it over a second time (the consumer's callback, then its rollback): reported once.
    strategy.handleException("Exchange failed", failed, failed.getException());

    assertThat(reported()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
      assertThat(f.message()).contains("rejected before the route ran");
    });
    assertThat(lines).singleElement().satisfies(l -> {
      assertThat(l.message()).contains("ingestion ing_sftp: a payload failed and the source keeps it: rejected before the route ran");
      assertThat(l.cause()).hasMessage("rejected before the route ran");
    });
    strategy.commit(null, null, 1);
    assertThat(pacer.lastFailed()).isTrue();
  }

  @Test
  void aRowStatusCommitFailureIsReportedWithTheMessageOfTheDatabaseSource() {
    Ingestion table =
        SourceHarness.ingestion(
            "ing_db",
            new Source.DatabasePoll("jdbc:postgresql://x/y", "u", new SecretRef("p"), "orders", new Source.Track.Status("done", "id", "0", "1"), 10, Duration.ofSeconds(5)),
            new Split.None());
    ReportingPollStrategy db = new ReportingPollStrategy(table, counters, health, () -> {}, pacer, failures);
    db.begin(null, null);
    db.rollback(null, null, 0, new CommitFailedException("marked as read", new IOException("deadlock detected")));

    assertThat(reported()).singleElement().satisfies(f -> {
      assertThat(f.kind()).isEqualTo(Failure.Kind.SOURCE);
      assertThat(f.message()).isEqualTo("read and sent, but could not be marked as read: deadlock detected; it will be read again");
    });
    assertThat(health.problem("ing_db", 1)).isNotNull();
    assertThat(pacer.due()).isFalse();
  }
}

package io.flowcatalyst.ingestor.runtime;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.ingestor.contract.SecretRef;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.contract.Source.Track;
import io.flowcatalyst.ingestor.contract.Split;
import io.flowcatalyst.ingestor.extension.Backoff;
import java.time.Duration;
import java.time.Instant;
import org.junit.jupiter.api.Test;

/** How soon a polled source may poll again after polls that failed (docs/ingestor-failure-pacing.md, R1). */
class PollPacerTest {

  private final TestClock clock = new TestClock(Instant.parse("2026-11-02T10:00:00Z"));
  private final PollPacer pacer = new PollPacer(clock);
  private final Duration interval = Duration.ofSeconds(10);

  private Duration failedPoll() {
    pacer.started();
    pacer.sourceFailed();
    return pacer.ended(interval);
  }

  @Test
  void aSourceThatFailsEveryPollWaitsLongerEachTimeUpToTheCap() {
    Duration previous = Duration.ZERO;
    for (int n = 1; n <= 12; n++) {
      Duration wait = failedPoll();
      Duration ceiling = Backoff.FAILURES.ceiling(n);
      Duration floor = ceiling.dividedBy(2).compareTo(interval) > 0 ? ceiling.dividedBy(2) : interval;
      assertThat(wait).as("failure %d", n).isBetween(floor, ceiling.compareTo(interval) > 0 ? ceiling : interval);
      assertThat(pacer.notBefore()).isEqualTo(clock.instant().plus(wait));
      assertThat(ceiling).isGreaterThanOrEqualTo(previous);
      previous = ceiling;
      clock.advance(wait);
    }
    assertThat(previous).isEqualTo(Duration.ofMinutes(5));
    assertThat(pacer.streak()).isEqualTo(12);
  }

  @Test
  void theWaitIsNeverShorterThanTheSourcesOwnInterval() {
    PollPacer slow = new PollPacer(clock);
    slow.started();
    slow.dispatchFailed();
    assertThat(slow.ended(Duration.ofMinutes(30))).isEqualTo(Duration.ofMinutes(30));
  }

  @Test
  void aPollIsNotDueUntilTheWaitIsOverAndTheFirstSuccessPollsAtTheIntervalAgain() {
    Duration wait = failedPoll();
    assertThat(pacer.due()).isFalse();
    clock.advance(wait.minusMillis(1));
    assertThat(pacer.due()).isFalse();
    clock.advance(Duration.ofMillis(1));
    assertThat(pacer.due()).isTrue();

    pacer.started();
    assertThat(pacer.ended(interval)).isEqualTo(interval);
    assertThat(pacer.lastFailed()).isFalse();
    assertThat(pacer.streak()).isZero();
    assertThat(pacer.due()).isTrue();
    // A failure after the success starts from the shortest wait again.
    assertThat(failedPoll()).isBetween(interval, Backoff.FAILURES.ceiling(1).compareTo(interval) > 0 ? Backoff.FAILURES.ceiling(1) : interval);
  }

  @Test
  void aPlatformThatDidNotTakeAPayloadIsAFailureButNotTheSourcesFailure() {
    pacer.started();
    pacer.dispatchFailed();
    pacer.ended(interval);
    assertThat(pacer.lastFailed()).isTrue();
    assertThat(pacer.lastSourceFailed()).isFalse();
    assertThat(pacer.streak()).isEqualTo(1);
  }

  @Test
  void theIntervalIsTheSourcesOwn() {
    Duration every = Duration.ofSeconds(7);
    Source.DatabasePoll poll =
        new Source.DatabasePoll(
            "jdbc:postgresql://x/y", "u", new SecretRef("p"), "orders", new Track.Watermark("id", Track.Watermark.Type.NUMBER), 10, every);
    assertThat(PollPacer.interval(SourceHarness.ingestion("a", poll, new Split.None()))).isEqualTo(every);
    assertThat(PollPacer.interval(SourceHarness.ingestion("b", new Source.Sqs("eu-west-1", "q", null), new Split.None()))).isEqualTo(CamelSourceEndpoints.SQS_DELAY);
  }
}

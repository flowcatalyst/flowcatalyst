package io.flowcatalyst.ingestor.extension;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import org.junit.jupiter.api.Test;

/** What a {@link PolledSource} waits after each kind of poll (docs/ingestor-failure-pacing.md, R5). */
class PollPacingTest {

  private final PollPacing pacing = new PollPacing();

  @Test
  void aPollThatSucceededWaitsWhatItAskedButNeverLessThan250Milliseconds() {
    assertThat(pacing.afterSuccess(Duration.ZERO)).isEqualTo(Duration.ofMillis(250));
    assertThat(pacing.afterSuccess(Duration.ofMillis(1))).isEqualTo(Duration.ofMillis(250));
    assertThat(pacing.afterSuccess(Duration.ofSeconds(30))).isEqualTo(Duration.ofSeconds(30));
  }

  @Test
  void anExtensionThatReturnsZeroAfterAFailedDispatchIsPacedAndEachFailureWaitsLongerUpToTheCap() {
    Duration retry = Duration.ofMillis(10); // below the one second a failed poll is based on
    Duration previousCeiling = Duration.ZERO;
    for (int n = 1; n <= 12; n++) {
      Duration wait = pacing.afterFailedPoll(Duration.ZERO, retry);
      Duration ceiling = Backoff.FAILURES.ceiling(n);
      assertThat(wait).as("failure %d", n).isBetween(ceiling.dividedBy(2), ceiling);
      assertThat(ceiling).isGreaterThanOrEqualTo(previousCeiling);
      previousCeiling = ceiling;
    }
    assertThat(previousCeiling).isEqualTo(Duration.ofMinutes(5));
    assertThat(pacing.failures()).isEqualTo(12);
  }

  @Test
  void aFailedPollNeverWaitsLessThanItsOwnWaitOrOneSecond() {
    assertThat(pacing.afterFailedPoll(Duration.ofHours(1), Duration.ofMillis(10))).isEqualTo(Duration.ofHours(1));
    PollPacing tiny = new PollPacing(new Backoff(Duration.ofMillis(1), Duration.ofMillis(2)), Backoff.REFUSED_LOGIN);
    assertThat(tiny.afterError(Duration.ZERO)).isEqualTo(Duration.ofSeconds(1));
    assertThat(tiny.afterError(Duration.ofSeconds(20))).isEqualTo(Duration.ofSeconds(20));
  }

  @Test
  void theFirstSuccessEndsTheStreakAndTheNextFailureStartsFromTheShortestWait() {
    for (int i = 0; i < 6; i++) {
      pacing.afterError(Duration.ofSeconds(1));
    }
    assertThat(pacing.afterSuccess(Duration.ofSeconds(2))).isEqualTo(Duration.ofSeconds(2));
    assertThat(pacing.failures()).isZero();
    assertThat(pacing.afterError(Duration.ofSeconds(1))).isBetween(Duration.ofMillis(2500), Duration.ofSeconds(5));
  }

  @Test
  void aRefusedLoginWaitsFiveMinutesThenDoublesToAnHour() {
    Duration[] ceilings = {Duration.ofMinutes(5), Duration.ofMinutes(10), Duration.ofMinutes(20), Duration.ofMinutes(40), Duration.ofHours(1), Duration.ofHours(1)};
    for (int n = 0; n < ceilings.length; n++) {
      assertThat(pacing.afterRefusedLogin()).as("refusal %d", n + 1).isBetween(ceilings[n].dividedBy(2), ceilings[n]);
    }
    assertThat(pacing.refusals()).isEqualTo(6);
    pacing.afterSuccess(Duration.ofSeconds(1));
    assertThat(pacing.refusals()).isZero();
  }
}

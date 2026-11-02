package io.flowcatalyst.ingestor.extension;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import java.time.Duration;
import org.junit.jupiter.api.Test;

class BackoffTest {

  private final Backoff backoff = new Backoff(Duration.ofSeconds(5), Duration.ofMinutes(5));

  @Test
  void doublesFromInitialUpToMax() {
    assertThat(backoff.ceiling(0)).isEqualTo(Duration.ofSeconds(5));
    assertThat(backoff.ceiling(1)).isEqualTo(Duration.ofSeconds(5));
    assertThat(backoff.ceiling(2)).isEqualTo(Duration.ofSeconds(10));
    assertThat(backoff.ceiling(4)).isEqualTo(Duration.ofSeconds(40));
    assertThat(backoff.ceiling(7)).isEqualTo(Duration.ofMinutes(5));
    assertThat(backoff.ceiling(1_000_000)).isEqualTo(Duration.ofMinutes(5));
  }

  @Test
  void jitterStaysBetweenHalfAndTheCeiling() {
    for (int failures = 1; failures < 12; failures++) {
      Duration c = backoff.ceiling(failures);
      for (int i = 0; i < 200; i++) {
        assertThat(backoff.delay(failures)).isBetween(c.dividedBy(2), c);
      }
    }
  }

  @Test
  void neverShorterThanTheFloor() {
    assertThat(backoff.delay(1, Duration.ofMinutes(1))).isEqualTo(Duration.ofMinutes(1));
    assertThat(backoff.delay(20, Duration.ofSeconds(1))).isBetween(Duration.ofSeconds(150), Duration.ofMinutes(5));
  }

  @Test
  void refusesANonsenseSchedule() {
    assertThatThrownBy(() -> new Backoff(Duration.ZERO, Duration.ofSeconds(1))).isInstanceOf(IllegalArgumentException.class);
    assertThatThrownBy(() -> new Backoff(Duration.ofSeconds(2), Duration.ofSeconds(1))).isInstanceOf(IllegalArgumentException.class);
  }
}

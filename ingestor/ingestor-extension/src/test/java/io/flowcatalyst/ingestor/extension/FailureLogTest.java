package io.flowcatalyst.ingestor.extension;

import static org.assertj.core.api.Assertions.assertThat;

import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.ArrayList;
import java.util.List;
import org.jspecify.annotations.Nullable;
import org.junit.jupiter.api.Test;

class FailureLogTest {

  private record Line(String message, @Nullable Throwable cause) {}

  private Instant now = Instant.parse("2026-11-02T10:00:00Z");
  private final List<Line> lines = new ArrayList<>();
  private final FailureLog log = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), () -> now);

  private void advance(Duration d) {
    now = now.plus(d);
  }

  @Test
  void theFirstOfAStreakIsLoggedInFullAndTheRestAtMostOnceAMinute() {
    RuntimeException boom = new RuntimeException("boom");
    for (int i = 0; i < 600; i++) { // ten a second for a minute
      log.failed("ing_1:poll", "ingestion ing_1 could not poll", boom);
      advance(Duration.ofMillis(100));
    }
    log.failed("ing_1:poll", "ingestion ing_1 could not poll", boom);

    assertThat(lines).hasSize(2);
    assertThat(lines.get(0)).isEqualTo(new Line("ingestion ing_1 could not poll", boom));
    assertThat(lines.get(1).cause()).isNull();
    assertThat(lines.get(1).message()).contains("599 more like it not logged").contains("601 in a row");
    assertThat(log.streak("ing_1:poll")).isEqualTo(601);
  }

  @Test
  void keysAreCountedApart() {
    log.failed("a", "a failed", null);
    log.failed("b", "b failed", null);
    log.failed("a", "a failed", null);
    assertThat(lines).extracting(Line::message).containsExactly("a failed", "b failed");
  }

  @Test
  void recoveringEndsTheStreakAndSaysSoWhenThereWasOne() {
    log.failed("a", "a failed", null);
    log.recovered("a", "a works");
    log.failed("a", "a failed", null);
    log.failed("a", "a failed", null);
    log.recovered("a", "a works");
    log.failed("a", "a failed again", new IllegalStateException());

    assertThat(lines).extracting(Line::message)
        .containsExactly("a failed", "a failed", "a works (after 2 failures in a row)", "a failed again");
    assertThat(lines.get(3).cause()).isInstanceOf(IllegalStateException.class);
  }

  @Test
  void aLongQuietStartsANewStreak() {
    RuntimeException boom = new RuntimeException("boom");
    log.failed("a", "a failed", boom);
    advance(Duration.ofMinutes(10));
    log.failed("a", "a failed", boom);
    assertThat(lines).hasSize(2);
    assertThat(lines.get(1).cause()).isSameAs(boom);
  }

  @Test
  void tooManyKeysAreForgotten() {
    FailureLog small = new FailureLog((m, c) -> lines.add(new Line(m, c)), Duration.ofMinutes(1), InstantSource.fixed(now));
    for (int i = 0; i < FailureLog.MOST_KEYS; i++) {
      small.failed("k" + i, "k" + i, null);
    }
    small.failed("one more", "one more", null);
    assertThat(small.streak("k0")).isZero();
    small.failed("k0", "k0 again", null);
    assertThat(lines.getLast().message()).isEqualTo("k0 again");
  }
}

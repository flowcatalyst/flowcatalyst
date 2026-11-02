package io.flowcatalyst.ingestor.extension;

import java.time.Duration;
import java.util.Objects;
import java.util.concurrent.ThreadLocalRandom;

/**
 * How long to wait after failures in a row: {@code initial} doubled for each failure after the first,
 * never more than {@code max}, with "equal jitter" (a delay {@code d} becomes a time picked evenly in
 * {@code [d/2, d]}) so runtimes, ingestions or functions that failed together do not try again in
 * lockstep. The runtime paces every repeated failure with one (docs/ingestor-failure-pacing.md); an
 * extension may use one for calls of its own.
 *
 * @param initial the wait after the first failure (before jitter); more than zero
 * @param max the longest wait; at least {@code initial}
 */
public record Backoff(Duration initial, Duration max) {

  /** The runtime's pacing of a failure that repeats: 5 seconds, doubling, at most 5 minutes. */
  public static final Backoff FAILURES = new Backoff(Duration.ofSeconds(5), Duration.ofMinutes(5));

  /** The runtime's pacing of a refused login: 5 minutes, doubling, at most an hour. */
  public static final Backoff REFUSED_LOGIN = new Backoff(Duration.ofMinutes(5), Duration.ofHours(1));

  /**
   * A backoff.
   *
   * @throws IllegalArgumentException when {@code initial} is not positive or {@code max} is less
   *     than it
   */
  public Backoff {
    Objects.requireNonNull(initial, "initial");
    Objects.requireNonNull(max, "max");
    if (initial.isNegative() || initial.isZero()) {
      throw new IllegalArgumentException("initial must be more than zero");
    }
    if (max.compareTo(initial) < 0) {
      throw new IllegalArgumentException("max must be at least initial");
    }
  }

  /**
   * The wait after {@code failures} failures in a row, before jitter: {@code initial · 2^(failures−1)},
   * at most {@code max}.
   *
   * @param failures failures in a row; less than 1 counts as 1
   * @return the wait
   */
  public Duration ceiling(int failures) {
    long initialMs = initial.toMillis();
    long maxMs = max.toMillis();
    int shift = Math.max(failures, 1) - 1;
    if (shift >= 62 || initialMs > (maxMs >> Math.min(shift, 62))) {
      return Duration.ofMillis(maxMs);
    }
    return Duration.ofMillis(Math.min(initialMs << shift, maxMs));
  }

  /**
   * The wait after {@code failures} failures in a row, with jitter: evenly in {@code [c/2, c]} where
   * {@code c} is {@link #ceiling(int)}.
   *
   * @param failures failures in a row; less than 1 counts as 1
   * @return the wait
   */
  public Duration delay(int failures) {
    long c = ceiling(failures).toMillis();
    long half = c / 2;
    return Duration.ofMillis(half <= 0 ? c : half + ThreadLocalRandom.current().nextLong(c - half + 1));
  }

  /**
   * The wait after {@code failures} failures in a row, never less than {@code floor}: a source's own
   * interval, say, so a backoff only ever lengthens it.
   *
   * @param failures failures in a row; less than 1 counts as 1
   * @param floor the shortest wait
   * @return the longer of the two
   */
  public Duration delay(int failures, Duration floor) {
    Duration d = delay(failures);
    return d.compareTo(floor) < 0 ? floor : d;
  }
}

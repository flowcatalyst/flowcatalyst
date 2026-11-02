package io.flowcatalyst.ingestor.extension;

import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;

/**
 * Logs a failure that repeats without flooding the log. Failures are grouped by a key (an ingestion
 * and what failed, a function version and its exception class). The first of a streak is logged in
 * full, with its stack trace; after that at most one line per {@code every}, without the stack trace,
 * saying how many like it were not logged. {@link #recovered(String)} ends a streak, and a key that
 * has not failed for ten times {@code every} starts a new one. At most {@value #MOST_KEYS} keys are
 * held; past that they are all forgotten, so the next failure of each is logged in full again.
 *
 * <p>Thread-safe. The runtime logs every repeated failure through one
 * (docs/ingestor-failure-pacing.md); an extension may use one for failures of its own.
 */
public final class FailureLog {

  /** The most keys held. */
  public static final int MOST_KEYS = 10_000;

  /** Where the lines go. */
  @FunctionalInterface
  public interface Out {
    /**
     * Writes one line.
     *
     * @param message the line
     * @param cause the failure, with its stack trace; null when it is left out
     */
    void write(String message, @Nullable Throwable cause);
  }

  private static final class Streak {
    Instant lastLogged;
    Instant lastSeen;
    long notLogged;
    long count;

    Streak(Instant now) {
      lastLogged = now;
      lastSeen = now;
      count = 1;
    }
  }

  private final Out out;
  private final Duration every;
  private final Duration stale;
  private final InstantSource clock;
  private final ConcurrentHashMap<String, Streak> streaks = new ConcurrentHashMap<>();

  /**
   * A failure log.
   *
   * @param out where the lines go
   * @param every the most one line per key in this time after the first
   * @param clock the time
   */
  public FailureLog(Out out, Duration every, InstantSource clock) {
    this.out = Objects.requireNonNull(out, "out");
    this.every = Objects.requireNonNull(every, "every");
    this.stale = every.multipliedBy(10);
    this.clock = Objects.requireNonNull(clock, "clock");
  }

  /**
   * One that logs at WARN to {@code log}, at most a line a minute per key after the first.
   *
   * @param log the logger
   * @return the failure log
   */
  public static FailureLog warn(Logger log) {
    return new FailureLog((message, cause) -> {
      if (cause == null) {
        log.warn(message);
      } else {
        log.warn(message, cause);
      }
    }, Duration.ofMinutes(1), InstantSource.system());
  }

  /**
   * A failure under {@code key}: logged in full when it starts a streak, as one line with a count
   * when {@code every} has passed since the last line, otherwise only counted.
   *
   * @param key what failed; never a secret
   * @param message the line, already formatted; never a secret
   * @param cause the failure, or null
   */
  public void failed(String key, String message, @Nullable Throwable cause) {
    Instant now = clock.instant();
    if (streaks.size() >= MOST_KEYS && !streaks.containsKey(key)) {
      streaks.clear();
    }
    // What to write is decided under the key's lock; written outside it.
    String[] line = new String[1];
    boolean[] full = new boolean[1];
    streaks.compute(key, (k, s) -> {
      if (s == null || Duration.between(s.lastSeen, now).compareTo(stale) >= 0) {
        line[0] = message;
        full[0] = true;
        return new Streak(now);
      }
      s.lastSeen = now;
      s.count++;
      if (Duration.between(s.lastLogged, now).compareTo(every) >= 0) {
        line[0] = s.notLogged == 0
            ? message + " (failing since the first of " + s.count + " in a row)"
            : message + " (" + s.notLogged + " more like it not logged; " + s.count + " in a row)";
        s.lastLogged = now;
        s.notLogged = 0;
      } else {
        s.notLogged++;
      }
      return s;
    });
    if (line[0] != null) {
      out.write(line[0], full[0] ? cause : null);
    }
  }

  /**
   * Ends the streak under {@code key}. When there was one of more than one failure, says so in one
   * line.
   *
   * @param key what failed
   * @param message the line when a streak ended, already formatted, for example "ingestion x polls again"
   */
  public void recovered(String key, String message) {
    Streak s = streaks.remove(key);
    if (s != null && s.count > 1) {
      out.write(message + " (after " + s.count + " failures in a row)", null);
    }
  }

  /**
   * Ends the streak under {@code key}, writing nothing.
   *
   * @param key what failed
   */
  public void recovered(String key) {
    streaks.remove(key);
  }

  /**
   * How many failures in a row there have been under {@code key}, 0 when none.
   *
   * @param key what failed
   * @return the count
   */
  public long streak(String key) {
    Streak s = streaks.get(key);
    return s == null ? 0 : s.count;
  }
}

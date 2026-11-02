package io.flowcatalyst.ingestor.extension;

import java.time.Duration;
import java.util.Objects;

/**
 * How long a {@link PolledSource} waits before its next poll, given how the last one went
 * (docs/ingestor-failure-pacing.md, R1 and R5). Not thread-safe: one poll at a time uses it.
 *
 * <ul>
 *   <li>After a poll that succeeded, the wait the poll returned, but at least {@link #MIN_WAIT}.
 *   <li>After a poll that failed (it threw, a payload was not dispatched, a part of it failed),
 *       {@code failures.delay(n, max(retryDelay, 1 s))} for {@code n} failures in a row, and not
 *       less than the wait the poll returned when it returned one.
 *   <li>After a refused login, {@code refusedLogin.delay(n)} for {@code n} refusals in a row.
 * </ul>
 */
final class PollPacing {

  /** The shortest wait after a poll that succeeded: a backlog still drains, a loop that returns zero does not spin. */
  static final Duration MIN_WAIT = Duration.ofMillis(250);

  /** The shortest base wait after a failed poll, whatever the extension's {@code retryDelay()} says. */
  static final Duration MIN_RETRY = Duration.ofSeconds(1);

  private final Backoff failures;
  private final Backoff refusedLogin;
  private int failed;
  private int refused;

  PollPacing(Backoff failures, Backoff refusedLogin) {
    this.failures = Objects.requireNonNull(failures, "failures");
    this.refusedLogin = Objects.requireNonNull(refusedLogin, "refusedLogin");
  }

  PollPacing() {
    this(Backoff.FAILURES, Backoff.REFUSED_LOGIN);
  }

  /** A poll succeeded and returned {@code returned}: the streaks end. */
  Duration afterSuccess(Duration returned) {
    failed = 0;
    refused = 0;
    return returned.compareTo(MIN_WAIT) < 0 ? MIN_WAIT : returned;
  }

  /**
   * A poll returned, but failed in part or did not dispatch a payload.
   *
   * @param returned what the poll returned
   * @param retryDelay the extension's wait after a failure
   */
  Duration afterFailedPoll(Duration returned, Duration retryDelay) {
    Duration wait = afterError(retryDelay);
    return returned.compareTo(wait) > 0 ? returned : wait;
  }

  /** A poll threw. */
  Duration afterError(Duration retryDelay) {
    failed++;
    refused = 0;
    return failures.delay(failed, retryDelay.compareTo(MIN_RETRY) < 0 ? MIN_RETRY : retryDelay);
  }

  /** A poll was refused its login. */
  Duration afterRefusedLogin() {
    refused++;
    failed = 0;
    return refusedLogin.delay(refused);
  }

  /** The failures in a row, counting the last. */
  int failures() {
    return failed;
  }

  /** The refused logins in a row, counting the last. */
  int refusals() {
    return refused;
  }
}

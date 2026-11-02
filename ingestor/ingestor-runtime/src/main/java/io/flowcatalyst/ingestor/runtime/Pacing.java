package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.time.InstantSource;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * What an ingestion's route needs to pace and log failures that repeat
 * (docs/ingestor-failure-pacing.md): the clock, the wait after failures in a row, and where the
 * summarised failures go. Production uses {@link #system()}; tests give their own.
 *
 * @param clock the time
 * @param backoff the wait after failures in a row
 * @param failures where repeated failures are logged
 */
record Pacing(InstantSource clock, Backoff backoff, FailureLog failures) {

  private static final Logger LOG = LoggerFactory.getLogger("io.flowcatalyst.ingestor.runtime.PollFailures");

  /** One for every route of the runtime: the keys name the ingestion, so one log serves them all. */
  private static final FailureLog SHARED = FailureLog.warn(LOG);

  /**
   * The runtime's own: the system clock, {@link Backoff#FAILURES}, and one shared failure log.
   *
   * @return the pacing
   */
  static Pacing system() {
    return new Pacing(InstantSource.system(), Backoff.FAILURES, SHARED);
  }

  /**
   * A new pacer, for one route: a route starts with no failures in a row.
   *
   * @return the pacer
   */
  PollPacer pacer() {
    return new PollPacer(backoff, clock);
  }
}

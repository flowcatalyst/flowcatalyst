package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.extension.Backoff;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.Objects;

/**
 * How soon a polled source may poll again, given how many polls in a row have failed
 * (docs/ingestor-failure-pacing.md, R1). After failure {@code n} in a row the next poll is no sooner
 * than {@code Backoff.FAILURES.delay(n, interval)}; the first poll that succeeds ends the streak and
 * the source polls at its interval again.
 *
 * <p>Mechanism: the pacer is a gate, not a schedule. A source Camel schedules by itself (sqs, s3,
 * sftp, database poll) asks {@link #due()} in the poll strategy's {@code begin} and skips the poll
 * while it is false, so the consumer's own timer keeps running and costs a call; Camel's
 * {@code backoffMultiplier} was not used because it skips a fixed number of polls, which cannot
 * follow a wait that doubles. A source that schedules itself (api-poll, extension polls) takes the
 * wait {@link #ended(Duration)} returns as its next delay.
 *
 * <p>A poll is one thread at a time; the flags a poll's exchanges set ({@link #dispatchFailed()},
 * {@link #sourceFailed()}) may be set from the thread that consumes the payload.
 */
final class PollPacer {

  private final Backoff backoff;
  private final InstantSource clock;
  private volatile boolean dispatchFailed;
  private volatile boolean sourceFailed;
  private volatile Instant notBefore = Instant.MIN;
  private volatile int streak;
  private volatile boolean lastFailed;
  private volatile boolean lastSourceFailed;

  PollPacer(InstantSource clock) {
    this(Backoff.FAILURES, clock);
  }

  PollPacer(Backoff backoff, InstantSource clock) {
    this.backoff = Objects.requireNonNull(backoff, "backoff");
    this.clock = Objects.requireNonNull(clock, "clock");
  }

  /**
   * The normal wait between two polls of an ingestion's source: the source's own interval.
   *
   * @param ingestion the ingestion
   * @return the interval; one second for a source that has none of its own
   */
  static Duration interval(Ingestion ingestion) {
    return switch (ingestion.source()) {
      case Source.Sftp sftp -> sftp.every();
      case Source.S3 s3 -> s3.every();
      case Source.DatabasePoll poll -> poll.every();
      case Source.ApiPoll api -> api.every();
      case Source.Sqs _ -> CamelSourceEndpoints.SQS_DELAY;
      default -> Duration.ofSeconds(1);
    };
  }

  /** Whether the next poll may start now. */
  boolean due() {
    return !clock.instant().isBefore(notBefore);
  }

  /** The earliest the next poll may start. */
  Instant notBefore() {
    return notBefore;
  }

  /** A poll is starting: what the last one reported is forgotten. */
  void started() {
    dispatchFailed = false;
    sourceFailed = false;
  }

  /** A payload of this poll was not taken by the platform: the poll counts as failed for pacing. */
  void dispatchFailed() {
    dispatchFailed = true;
  }

  /** The poll, or what it does after reading (a move, a mark), failed: the source is failing. */
  void sourceFailed() {
    sourceFailed = true;
  }

  /** Whether the poll in progress has failed in any way. */
  boolean failing() {
    return dispatchFailed || sourceFailed;
  }

  /**
   * The poll is over.
   *
   * @param normal the source's usual wait
   * @return {@code normal} after a poll that did not fail; else the wait a failure in a row asks for,
   *     never less than {@code normal}
   */
  Duration ended(Duration normal) {
    boolean failed = failing();
    lastFailed = failed;
    lastSourceFailed = sourceFailed;
    dispatchFailed = false;
    sourceFailed = false;
    if (!failed) {
      streak = 0;
      notBefore = Instant.MIN;
      return normal;
    }
    int n = streak + 1;
    streak = n;
    Duration wait = backoff.delay(n, normal);
    notBefore = clock.instant().plus(wait);
    return wait;
  }

  /** Whether the poll {@link #ended} last had failed. */
  boolean lastFailed() {
    return lastFailed;
  }

  /** Whether the poll {@link #ended} last failed in its source (not only in the platform taking its payloads). */
  boolean lastSourceFailed() {
    return lastSourceFailed;
  }

  /** How many polls in a row have failed. */
  int streak() {
    return streak;
  }
}
